package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type backupCreateRequest struct {
	Database string   `json:"database"`
	Tables   []string `json:"tables,omitempty"`
}

type backupDatabaseEntry struct {
	Name string `json:"name"`
}

type backupTableEntry struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

type backupTableListResponse struct {
	Database string             `json:"database"`
	Tables   []backupTableEntry `json:"tables"`
}

type backupInspectResponse struct {
	Database string             `json:"database"`
	Format   string             `json:"format"`
	Tables   []backupTableEntry `json:"tables"`
	Size     int64              `json:"size"`
}

type backupRestoreRequest struct {
	Database  string `json:"database"`
	DropFirst bool   `json:"dropFirst"`
}

func (h *Handler) ListBackupDatabases(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT datname FROM pg_catalog.pg_database
		WHERE datistemplate = false
		ORDER BY datname
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list databases")
		return
	}
	defer rows.Close()

	databases := make([]backupDatabaseEntry, 0)
	for rows.Next() {
		var db backupDatabaseEntry
		if err := rows.Scan(&db.Name); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan row")
			return
		}
		if protectedDatabases[db.Name] {
			continue
		}
		databases = append(databases, db)
	}

	writeJSON(w, http.StatusOK, databases)
}

func (h *Handler) ListBackupTables(w http.ResponseWriter, r *http.Request) {
	dbName := r.URL.Query().Get("db")
	if dbName == "" {
		writeError(w, http.StatusBadRequest, "db parameter is required")
		return
	}
	if !validName.MatchString(dbName) {
		writeError(w, http.StatusBadRequest, "invalid database name")
		return
	}

	host, port, user, password := h.getPgCredentials()

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, dbName)
	dbPool, err := pgxpool.New(r.Context(), dsn)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to connect to database: "+err.Error())
		return
	}
	defer dbPool.Close()

	rows, err := dbPool.Query(r.Context(), `
		SELECT schemaname, tablename
		FROM pg_catalog.pg_tables
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		ORDER BY schemaname, tablename
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tables: "+err.Error())
		return
	}
	defer rows.Close()

	tables := make([]backupTableEntry, 0)
	for rows.Next() {
		var t backupTableEntry
		if err := rows.Scan(&t.Schema, &t.Name); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan row")
			return
		}
		tables = append(tables, t)
	}

	writeJSON(w, http.StatusOK, backupTableListResponse{
		Database: dbName,
		Tables:   tables,
	})
}

func (h *Handler) StreamBackup(w http.ResponseWriter, r *http.Request) {
	var req backupCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Database = strings.TrimSpace(req.Database)
	if req.Database == "" {
		writeError(w, http.StatusBadRequest, "database is required")
		return
	}
	if !validName.MatchString(req.Database) {
		writeError(w, http.StatusBadRequest, "invalid database name")
		return
	}
	if protectedDatabases[req.Database] {
		writeError(w, http.StatusForbidden, "cannot backup system database")
		return
	}

	for _, table := range req.Tables {
		if !validName.MatchString(table) {
			writeError(w, http.StatusBadRequest, "invalid table name: "+table)
			return
		}
	}

	host, port, user, password := h.getPgCredentials()

	args := []string{
		"-Fc",
		"--no-owner",
		"--no-privileges",
		"-h", host,
		"-p", port,
		"-U", user,
	}

	if len(req.Tables) > 0 {
		for _, table := range req.Tables {
			args = append(args, "-t", table)
		}
	}

	args = append(args, req.Database)

	cmd := exec.Command("pg_dump", args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+password)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create pipe: "+err.Error())
		return
	}

	if err := cmd.Start(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start pg_dump: "+err.Error())
		return
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("%s_%s.dump", req.Database, timestamp)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	flusher, canFlush := w.(http.Flusher)

	buf := make([]byte, 32*1024)
	for {
		n, readErr := stdout.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				break
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if readErr != nil {
			break
		}
	}

	if err := cmd.Wait(); err != nil {
		log.Printf("pg_dump failed for %s: %v: %s", req.Database, err, stderr.String())
		return
	}

	log.Printf("backup completed: %s", filename)
}

func (h *Handler) InspectDump(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	tmpDir, err := os.MkdirTemp("", "backup-inspect-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create temp dir")
		return
	}
	defer os.RemoveAll(tmpDir)

	tmpPath := filepath.Join(tmpDir, "dump.dump")
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create temp file")
		return
	}
	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		writeError(w, http.StatusInternalServerError, "failed to write temp file")
		return
	}
	tmpFile.Close()

	_, port, user, password := h.getPgCredentials()

	cmd := exec.Command("pg_restore", "-l", "-h", "localhost", "-p", port, "-U", user, tmpPath)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or corrupt backup file: "+string(output))
		return
	}

	tables := make([]backupTableEntry, 0)
	dbName := ""

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse dbname from header comment: ";     dbname: shopdb"
		if strings.HasPrefix(line, ";") {
			trimmed := strings.TrimSpace(line[1:])
			if strings.HasPrefix(trimmed, "dbname:") {
				dbName = strings.TrimSpace(strings.TrimPrefix(trimmed, "dbname:"))
			}
			continue
		}

		if strings.HasPrefix(line, "--") {
			continue
		}

		parts := strings.SplitN(line, ";", 3)
		if len(parts) < 2 {
			continue
		}
		detail := strings.TrimSpace(parts[1])

		if strings.HasPrefix(detail, "DATABASE ") {
			dbName = strings.TrimPrefix(detail, "DATABASE ")
			dbName = strings.TrimSpace(dbName)
			continue
		}

		// pg_restore -l format: TOC_ID; OID1 OID2 TYPE SCHEMA NAME DB
		// e.g. "220; 1259 16504 TABLE public orders pgmanager"
		// or  "3470; 0 16504 TABLE DATA public orders pgmanager"
		fields := strings.Fields(detail)
		if len(fields) >= 5 {
			objType := fields[2]
			// For "TABLE" entries: fields = [OID1, OID2, TABLE, SCHEMA, NAME, DB]
			// For "TABLE DATA" entries: fields = [OID1, OID2, TABLE, DATA, SCHEMA, NAME, DB]
			if objType == "TABLE" && fields[3] != "DATA" && len(fields) >= 5 {
				schema := fields[3]
				name := fields[4]
				if schema != "pg_catalog" && schema != "information_schema" {
					tables = append(tables, backupTableEntry{
						Schema: schema,
						Name:   name,
					})
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, backupInspectResponse{
		Database: dbName,
		Format:   "custom",
		Tables:   tables,
		Size:     header.Size,
	})
}

func (h *Handler) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	targetDB := r.FormValue("database")
	targetDB = strings.TrimSpace(targetDB)
	if targetDB == "" {
		writeError(w, http.StatusBadRequest, "database is required")
		return
	}
	if !validName.MatchString(targetDB) {
		writeError(w, http.StatusBadRequest, "invalid database name")
		return
	}
	if protectedDatabases[targetDB] {
		writeError(w, http.StatusForbidden, "cannot restore to system database")
		return
	}

	dropFirst := false
	if df := r.FormValue("dropFirst"); df != "" {
		dropFirst, _ = strconv.ParseBool(df)
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	tmpDir, err := os.MkdirTemp("", "backup-restore-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create temp dir")
		return
	}
	defer os.RemoveAll(tmpDir)

	tmpPath := filepath.Join(tmpDir, "dump.dump")
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create temp file")
		return
	}
	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		writeError(w, http.StatusInternalServerError, "failed to write temp file")
		return
	}
	tmpFile.Close()

	if dropFirst {
		dropSQL := "DROP DATABASE IF EXISTS " + quoteIdent(targetDB) + " WITH (FORCE)"
		if _, err := h.pool.Exec(r.Context(), dropSQL); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to drop database: "+err.Error())
			return
		}
		createSQL := "CREATE DATABASE " + quoteIdent(targetDB)
		if _, err := h.pool.Exec(r.Context(), createSQL); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create database: "+err.Error())
			return
		}
	}

	host, port, user, password := h.getPgCredentials()

	cmd := exec.Command("pg_restore",
		"-d", targetDB,
		"--no-owner",
		"--no-privileges",
		"-h", host,
		"-p", port,
		"-U", user,
		tmpPath,
	)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+password)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("pg_restore failed for %s: %v: %s", targetDB, err, stderr.String())
		writeError(w, http.StatusInternalServerError, "restore failed: "+stderr.String())
		return
	}

	log.Printf("restore completed: %s", targetDB)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"database": targetDB,
		"message":  "restore completed successfully",
	})
}

func (h *Handler) getPgCredentials() (host, port, user, password string) {
	host = os.Getenv("PGHOST")
	if host == "" {
		host = "localhost"
	}
	port = os.Getenv("PGPORT")
	if port == "" {
		port = "5432"
	}
	user = os.Getenv("PGUSER")
	if user == "" {
		user = "pgmanager"
	}

	secretPath := os.Getenv("SECRET_PATH")
	if secretPath == "" {
		secretPath = "/secrets/pgmanager-password"
	}
	data, err := os.ReadFile(secretPath)
	if err != nil {
		log.Printf("failed to read password file %s: %v", secretPath, err)
		password = ""
		return
	}
	password = strings.TrimSpace(string(data))
	return
}
