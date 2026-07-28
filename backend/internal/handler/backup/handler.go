package backup

import (
	"bytes"
	"encoding/json"
	"errors"
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

	"pgmanager/internal/auth"
	"pgmanager/internal/handler/core"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ListBackupDatabases(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	rows, err := pool.Query(r.Context(), `
		SELECT datname FROM pg_catalog.pg_database
		WHERE datistemplate = false
		ORDER BY datname
	`)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to list databases")
		return
	}
	defer rows.Close()

	databases := make([]BackupDatabaseEntry, 0)
	for rows.Next() {
		var db BackupDatabaseEntry
		if err := rows.Scan(&db.Name); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to scan row")
			return
		}
		if core.ProtectedDatabases[db.Name] {
			continue
		}
		databases = append(databases, db)
	}

	user := auth.GetUserFromContext(r.Context())
	username := ""
	if user != nil {
		username = user.Username
	}

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  username,
		Action:    "list_backup_databases",
		IPAddress: core.ClientIP(r),
		Detail:    map[string]interface{}{"count": len(databases)},
	})

	core.WriteJSON(w, http.StatusOK, databases)
}

func ListBackupTables(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	dbName := r.URL.Query().Get("db")
	if dbName == "" {
		core.WriteError(w, http.StatusBadRequest, "db parameter is required")
		return
	}
	if !core.ValidName.MatchString(dbName) {
		core.WriteError(w, http.StatusBadRequest, "invalid database name")
		return
	}

	host, port, user, password := getPgCredentials()

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, dbName)
	dbPool, err := pgxpool.New(r.Context(), dsn)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to connect to database")
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
		core.WriteError(w, http.StatusInternalServerError, "failed to list tables: "+sanitizeRedact(err.Error()))
		return
	}
	defer rows.Close()

	tables := make([]BackupTableEntry, 0)
	for rows.Next() {
		var t BackupTableEntry
		if err := rows.Scan(&t.Schema, &t.Name); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to scan row")
			return
		}
		tables = append(tables, t)
	}

	authUser := auth.GetUserFromContext(r.Context())
	username := ""
	if authUser != nil {
		username = authUser.Username
	}

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  username,
		Action:    "list_backup_tables",
		Database:  dbName,
		IPAddress: core.ClientIP(r),
		Detail:    map[string]interface{}{"count": len(tables)},
	})

	core.WriteJSON(w, http.StatusOK, BackupTableListResponse{
		Database: dbName,
		Tables:   tables,
	})
}

func StreamBackup(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	var req BackupCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Database = strings.TrimSpace(req.Database)
	if req.Database == "" {
		core.WriteError(w, http.StatusBadRequest, "database is required")
		return
	}
	if !core.ValidName.MatchString(req.Database) {
		core.WriteError(w, http.StatusBadRequest, "invalid database name")
		return
	}
	if core.ProtectedDatabases[req.Database] {
		core.WriteError(w, http.StatusForbidden, "cannot backup system database")
		return
	}

	for _, table := range req.Tables {
		if !core.ValidName.MatchString(table) {
			core.WriteError(w, http.StatusBadRequest, "invalid table name: "+table)
			return
		}
	}

	host, port, user, password := getPgCredentials()

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

	tmpDir, err := os.MkdirTemp("", "backup-stream-*")
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to create temp directory")
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
		core.WriteError(w, http.StatusInternalServerError, "failed to create temp file")
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

		authUser := auth.GetUserFromContext(r.Context())
		username := ""
		if authUser != nil {
			username = authUser.Username
		}

		core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
			Username:  username,
			Action:    "backup_database",
			Database:  req.Database,
			IPAddress: core.ClientIP(r),
			Detail:    map[string]interface{}{"error": errMsg},
		})

		core.WriteError(w, http.StatusInternalServerError, errMsg)
		return
	}

	fileInfo, statErr := os.Stat(tmpPath)
	if statErr != nil || fileInfo.Size() == 0 {
		core.WriteError(w, http.StatusInternalServerError, "backup produced empty file")
		return
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("%s_%s.dump", req.Database, timestamp)

	authUser := auth.GetUserFromContext(r.Context())
	username := ""
	if authUser != nil {
		username = authUser.Username
	}

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  username,
		Action:    "backup_database",
		Database:  req.Database,
		IPAddress: core.ClientIP(r),
		Detail:    map[string]interface{}{"tables": req.Tables, "size": fileInfo.Size(), "filename": filename},
	})

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))

	tmpFile, err := os.Open(tmpPath)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to read backup file")
		return
	}
	defer tmpFile.Close()

	if _, err := io.Copy(w, tmpFile); err != nil {
		log.Printf("failed to stream backup for %s: %v", req.Database, err)
		return
	}

	log.Printf("backup completed: %s (%d bytes)", filename, fileInfo.Size())
}

func InspectDump(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		core.WriteError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	tmpDir, err := os.MkdirTemp("", "backup-inspect-*")
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to create temp dir")
		return
	}
	defer os.RemoveAll(tmpDir)

	tmpPath := filepath.Join(tmpDir, "dump.dump")
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to create temp file")
		return
	}
	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		core.WriteError(w, http.StatusInternalServerError, "failed to write temp file")
		return
	}
	tmpFile.Close()

	headerBuf := make([]byte, 300)
	if f, err := os.Open(tmpPath); err == nil {
		n, _ := f.Read(headerBuf)
		f.Close()
		if n < 5 {
			core.WriteError(w, http.StatusBadRequest, "file too small to be a valid backup")
			return
		}
		headerStr := string(headerBuf[:n])
		if strings.Contains(headerStr, "-- PostgreSQL database dump") ||
			strings.Contains(headerStr, "-- Dumped by pg_dump") ||
			strings.Contains(headerStr, "CREATE DATABASE") {
			core.WriteError(w, http.StatusBadRequest,
				"this appears to be a plain SQL dump. Only PostgreSQL custom format (.dump created with pg_dump -Fc) is supported for restore. Re-create the backup using the pgmanager web UI or run: pg_dump -Fc -h HOST -U USER DATABASE > backup.dump")
			return
		}
		if n >= 5 && string(headerBuf[:5]) != "PGDMP" {
			isTar := n >= 263 && string(headerBuf[257:263]) == "ustar\x00"
			if !isTar {
				core.WriteError(w, http.StatusBadRequest, "not a valid PostgreSQL backup file (expected PGDMP custom format)")
				return
			}
		}
	}

	_, port, user, password := getPgCredentials()

	cmd := exec.Command("pg_restore", "-l", "-h", "localhost", "-p", port, "-U", user, tmpPath)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		} else {
			core.WriteError(w, http.StatusBadRequest, "invalid or corrupt backup file: "+sanitizeRedact(string(output)))
			return
		}
	}

	tables := make([]BackupTableEntry, 0)
	dbName := ""

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

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

		fields := strings.Fields(detail)
		if len(fields) >= 5 {
			objType := fields[2]
			if objType == "TABLE" && fields[3] != "DATA" && len(fields) >= 5 {
				schema := fields[3]
				name := fields[4]
				if schema != "pg_catalog" && schema != "information_schema" {
					tables = append(tables, BackupTableEntry{
						Schema: schema,
						Name:   name,
					})
				}
			}
		}
	}

	authUser := auth.GetUserFromContext(r.Context())
	username := ""
	if authUser != nil {
		username = authUser.Username
	}

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  username,
		Action:    "inspect_backup",
		Database:  dbName,
		IPAddress: core.ClientIP(r),
		Detail:    map[string]interface{}{"tables": tables, "size": header.Size},
	})

	core.WriteJSON(w, http.StatusOK, BackupInspectResponse{
		Database: dbName,
		Format:   "custom",
		Tables:   tables,
		Size:     header.Size,
	})
}

func RestoreBackup(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		core.WriteError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	targetDB := r.FormValue("database")
	targetDB = strings.TrimSpace(targetDB)
	if targetDB == "" {
		core.WriteError(w, http.StatusBadRequest, "database is required")
		return
	}
	if !core.ValidName.MatchString(targetDB) {
		core.WriteError(w, http.StatusBadRequest, "invalid database name")
		return
	}
	if core.ProtectedDatabases[targetDB] {
		core.WriteError(w, http.StatusForbidden, "cannot restore to system database")
		return
	}

	dropFirst := false
	if df := r.FormValue("dropFirst"); df != "" {
		dropFirst, _ = strconv.ParseBool(df)
	}

	if !dropFirst {
		existingTables, err := ListExistingTables(r.Context(), baseDSN, targetDB)
		if err == nil && len(existingTables) > 0 {
			core.WriteJSON(w, http.StatusConflict, map[string]any{
				"error":   "target database has existing tables",
				"tables":  existingTables,
				"message": fmt.Sprintf("Database '%s' contains %d table(s). Enable 'Drop target first' to replace them, or restore to a new database.", targetDB, len(existingTables)),
			})
			return
		}
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	tmpDir, err := os.MkdirTemp("", "backup-restore-*")
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to create temp dir")
		return
	}
	defer os.RemoveAll(tmpDir)

	tmpPath := filepath.Join(tmpDir, "dump.dump")
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to create temp file")
		return
	}
	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		core.WriteError(w, http.StatusInternalServerError, "failed to write temp file")
		return
	}
	tmpFile.Close()

	if dropFirst {
		dropSQL := "DROP DATABASE IF EXISTS " + core.QuoteIdent(targetDB) + " WITH (FORCE)"
		if _, err := pool.Exec(r.Context(), dropSQL); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to drop database: "+sanitizeRedact(err.Error()))
			return
		}
		createSQL := "CREATE DATABASE " + core.QuoteIdent(targetDB)
		if _, err := pool.Exec(r.Context(), createSQL); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to create database: "+sanitizeRedact(err.Error()))
			return
		}
	}

	host, port, user, password := getPgCredentials()

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
		stderrStr := stderr.String()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 &&
			!strings.Contains(stderrStr, "FATAL:") &&
			!strings.Contains(stderrStr, "ERROR:") {
			log.Printf("restore completed with warnings for %s: %s", targetDB, stderrStr)
			core.WriteJSON(w, http.StatusOK, map[string]any{
				"success":  true,
				"database": targetDB,
				"message":  "restore completed with warnings",
				"warnings": sanitizeRedact(stderrStr),
			})
			return
		}
		log.Printf("pg_restore failed for %s: %v: %s", targetDB, err, stderrStr)
		authUser := auth.GetUserFromContext(r.Context())
		username := ""
		if authUser != nil {
			username = authUser.Username
		}
		core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
			Username:  username,
			Action:    "restore_backup",
			Database:  targetDB,
			IPAddress: core.ClientIP(r),
			Detail:    map[string]interface{}{"error": sanitizeRedact(stderrStr), "dropFirst": dropFirst},
		})
		core.WriteError(w, http.StatusInternalServerError, "restore failed: "+sanitizeRedact(stderrStr))
		return
	}

	log.Printf("restore completed: %s", targetDB)

	authUser := auth.GetUserFromContext(r.Context())
	username := ""
	if authUser != nil {
		username = authUser.Username
	}

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  username,
		Action:    "restore_backup",
		Database:  targetDB,
		IPAddress: core.ClientIP(r),
		Detail:    map[string]interface{}{"dropFirst": dropFirst},
	})

	core.WriteJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"database": targetDB,
		"message":  "restore completed successfully",
	})
}
