package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// sanitizeRedact removes sensitive values from error output.
// pg_dump/pg_restore never output passwords to stderr (they use PGPASSWORD env var),
// but we defensively redact any that might appear from Go error messages (e.g., DSN strings).
func sanitizeRedact(s string) string {
	// Redact DSN strings: postgres://user:PASSWORD@host/db → postgres://user:***@host/db
	s = regexp.MustCompile(`(postgres://[^:]+:)[^@]+(@)`).ReplaceAllString(s, "${1}***${2}")
	// Redact PGPASSWORD=VALUE patterns
	s = regexp.MustCompile(`(?i)(PGPASSWORD=)\S+`).ReplaceAllString(s, "${1}***")
	// Redact password=VALUE patterns in connection strings
	s = regexp.MustCompile(`(?i)(password=)\S+`).ReplaceAllString(s, "${1}***")
	return s
}

func (h *Handler) listExistingTables(ctx context.Context, dbName string) ([]string, error) {
	host, port, user, password := h.getPgCredentials()
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, dbName)
	dbPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer dbPool.Close()

	rows, err := dbPool.Query(ctx, `
		SELECT tablename FROM pg_catalog.pg_tables
		WHERE schemaname = 'public'
		ORDER BY tablename
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, nil
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
		writeError(w, http.StatusInternalServerError, "failed to connect to database")
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
		writeError(w, http.StatusInternalServerError, "failed to list tables: "+sanitizeRedact(err.Error()))
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

	// Buffer pg_dump output to temp file before streaming to client.
	// This prevents corrupt/partial downloads if pg_dump fails mid-stream.
	tmpDir, err := os.MkdirTemp("", "backup-stream-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create temp directory")
		return
	}
	defer os.RemoveAll(tmpDir)

	tmpPath := filepath.Join(tmpDir, "dump.dump")

	cmd := exec.Command("pg_dump", args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+password)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	outFile, err := os.Create(tmpPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create temp file")
		return
	}
	cmd.Stdout = outFile

	err = cmd.Run()
	outFile.Close()

	if err != nil {
		log.Printf("pg_dump failed for %s: %v: %s", req.Database, err, stderr.String())

		stderrStr := stderr.String()
		var errMsg string
		switch {
		case strings.Contains(stderrStr, "does not exist"):
			errMsg = "database does not exist"
		case strings.Contains(stderrStr, "connection refused"):
			errMsg = "database connection refused"
		case strings.Contains(stderrStr, "permission denied"):
			errMsg = "permission denied"
		case strings.Contains(stderrStr, "No such file"):
			errMsg = "database not found"
		default:
			errMsg = "backup failed: " + sanitizeRedact(stderrStr)
		}

		writeError(w, http.StatusInternalServerError, errMsg)
		return
	}

	// Verify file was created and has content
	fileInfo, statErr := os.Stat(tmpPath)
	if statErr != nil || fileInfo.Size() == 0 {
		writeError(w, http.StatusInternalServerError, "backup produced empty file")
		return
	}

	// Stream the completed temp file to the client
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("%s_%s.dump", req.Database, timestamp)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))

	tmpFile, err := os.Open(tmpPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read backup file")
		return
	}
	defer tmpFile.Close()

	if _, err := io.Copy(w, tmpFile); err != nil {
		log.Printf("failed to stream backup for %s: %v", req.Database, err)
		return
	}

	log.Printf("backup completed: %s (%d bytes)", filename, fileInfo.Size())
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

	// Verify file starts with a valid pg_dump format header
	// Custom format: "PGDMP" (0x50 0x47 0x44 0x4D 0x50)
	// Tar format: 0x1F 0x8B (gzip) or "ustar" at offset 257
	// Plain SQL: "--" or "/*"
	headerBuf := make([]byte, 300)
	if f, err := os.Open(tmpPath); err == nil {
		n, _ := f.Read(headerBuf)
		f.Close()
		if n < 5 {
			writeError(w, http.StatusBadRequest, "file too small to be a valid backup")
			return
		}
		headerStr := string(headerBuf[:n])
		// Check for plain SQL dumps
		if strings.Contains(headerStr, "-- PostgreSQL database dump") ||
			strings.Contains(headerStr, "-- Dumped by pg_dump") ||
			strings.Contains(headerStr, "CREATE DATABASE") {
			writeError(w, http.StatusBadRequest,
				"this appears to be a plain SQL dump. Only PostgreSQL custom format (.dump created with pg_dump -Fc) is supported for restore. Re-create the backup using the pgmanager web UI or run: pg_dump -Fc -h HOST -U USER DATABASE > backup.dump")
			return
		}
		// Check for valid pg_dump custom format magic: "PGDMP"
		if n >= 5 && string(headerBuf[:5]) != "PGDMP" {
			// Check for tar format (bytes at offset 257: "ustar")
			isTar := n >= 263 && string(headerBuf[257:263]) == "ustar\x00"
			if !isTar {
				writeError(w, http.StatusBadRequest, "not a valid PostgreSQL backup file (expected PGDMP custom format)")
				return
			}
		}
	}

	_, port, user, password := h.getPgCredentials()

	cmd := exec.Command("pg_restore", "-l", "-h", "localhost", "-p", port, "-U", user, tmpPath)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// pg_restore -l returns exit code 1 for warnings — output is still valid
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			// Continue parsing — output is valid despite warnings
		} else {
			writeError(w, http.StatusBadRequest, "invalid or corrupt backup file: "+sanitizeRedact(string(output)))
			return
		}
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

	// Check for existing tables when dropFirst is false — prevent silent data duplication
	if !dropFirst {
		existingTables, err := h.listExistingTables(r.Context(), targetDB)
		if err == nil && len(existingTables) > 0 {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":   "target database has existing tables",
				"tables":  existingTables,
				"message": fmt.Sprintf("Database '%s' contains %d table(s). Enable 'Drop target first' to replace them, or restore to a new database.", targetDB, len(existingTables)),
			})
			return
		}
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
			writeError(w, http.StatusInternalServerError, "failed to drop database: "+sanitizeRedact(err.Error()))
			return
		}
		createSQL := "CREATE DATABASE " + quoteIdent(targetDB)
		if _, err := h.pool.Exec(r.Context(), createSQL); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create database: "+sanitizeRedact(err.Error()))
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
		// pg_restore returns exit code 1 for warnings (missing owner, privilege issues)
		// These are non-fatal — data WAS restored successfully.
		// However, some fatal errors also return exit code 1, so check stderr.
		stderrStr := stderr.String()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 &&
			!strings.Contains(stderrStr, "FATAL:") &&
			!strings.Contains(stderrStr, "ERROR:") {
			log.Printf("restore completed with warnings for %s: %s", targetDB, stderrStr)
			writeJSON(w, http.StatusOK, map[string]any{
				"success":  true,
				"database": targetDB,
				"message":  "restore completed with warnings",
				"warnings": sanitizeRedact(stderrStr),
			})
			return
		}
		log.Printf("pg_restore failed for %s: %v: %s", targetDB, err, stderrStr)
		writeError(w, http.StatusInternalServerError, "restore failed: "+sanitizeRedact(stderrStr))
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
