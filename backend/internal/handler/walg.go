package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"pgmanager/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

type WalgHandler struct {
	pool *pgxpool.Pool
}

func NewWalgHandler(pool *pgxpool.Pool) *WalgHandler {
	return &WalgHandler{pool: pool}
}

func (wh *WalgHandler) writeAuditLog(ctx context.Context, entry auditEntry) {
	_, err := wh.pool.Exec(ctx,
		`INSERT INTO audit_log (username, action, database, table_name, detail, ip_address)
		 VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)`,
		entry.Username, entry.Action, entry.Database,
		entry.TableName, entry.Detail, entry.IPAddress,
	)
	if err != nil {
		log.Printf("audit log write failed: %v", err)
	}
}

type walgStatusResponse struct {
	Enabled       bool   `json:"enabled"`
	Archiving     bool   `json:"archiving"`
	Configured    bool   `json:"configured"`
	S3Prefix      string `json:"s3Prefix"`
	LastBackup    string `json:"lastBackup,omitempty"`
	BackupCount   int    `json:"backupCount"`
	TotalSize     int64  `json:"totalSize"`
	IntervalSec   int    `json:"intervalSec"`
	RetentionDays int    `json:"retentionDays"`
}

type walgBackupEntry struct {
	Name       string `json:"name"`
	Time       string `json:"time"`
	WalSegment string `json:"walSegment"`
	Size       int64  `json:"size"`
	Status     string `json:"status"`
}

type walgConfigRequest struct {
	S3Prefix      string `json:"s3Prefix"`
	Endpoint      string `json:"endpoint,omitempty"`
	Region        string `json:"region,omitempty"`
	ForcePathStyle *bool `json:"forcePathStyle,omitempty"`
	Interval      *int   `json:"interval,omitempty"`
	RetentionDays *int   `json:"retentionDays,omitempty"`
}

type walgRestoreRequest struct {
	BackupName string `json:"backupName"`
	Database   string `json:"database"`
}

type walgVerifyResponse struct {
	Status  string `json:"status"`
	Details string `json:"details"`
}

// walgConfig holds all WAL-G settings, read from system_config with env var fallbacks.
type walgConfig struct {
	S3Prefix       string
	Endpoint       string
	Region         string
	ForcePathStyle bool
	Interval       int
	RetentionDays  int
}

// getConfigFromDB reads WAL-G settings from system_config table, falling back to env vars.
func (wh *WalgHandler) getConfigFromDB() walgConfig {
	ctx := context.Background()
	cfg := walgConfig{
		Interval:      3600,
		RetentionDays: 7,
		Region:        "us-east-1",
	}

	rows, err := wh.pool.Query(ctx,
		`SELECT key, value FROM system_config WHERE key LIKE 'walg_%'`)
	if err != nil {
		log.Printf("walg: failed to read config from DB: %v", err)
		// Fall back to env vars
		cfg.S3Prefix = os.Getenv("WALG_S3_PREFIX")
		cfg.Endpoint = os.Getenv("AWS_ENDPOINT")
		cfg.Region = envOr("AWS_REGION", "us-east-1")
		cfg.ForcePathStyle = os.Getenv("AWS_S3_FORCE_PATH_STYLE") == "true"
		if v := os.Getenv("WALG_BACKUP_INTERVAL"); v != "" {
			fmt.Sscanf(v, "%d", &cfg.Interval)
		}
		if v := os.Getenv("WALG_BACKUP_RETENTION_DAYS"); v != "" {
			fmt.Sscanf(v, "%d", &cfg.RetentionDays)
		}
		return cfg
	}
	defer rows.Close()

	dbValues := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		dbValues[key] = value
	}

	// DB values take precedence, env vars are fallbacks
	cfg.S3Prefix = orFallback(dbValues["walg_s3_prefix"], os.Getenv("WALG_S3_PREFIX"))
	cfg.Endpoint = orFallback(dbValues["walg_endpoint"], os.Getenv("AWS_ENDPOINT"))
	cfg.Region = orFallback(dbValues["walg_region"], os.Getenv("AWS_REGION"))
	if v, ok := dbValues["walg_force_path_style"]; ok {
		cfg.ForcePathStyle = v == "true"
	} else {
		cfg.ForcePathStyle = os.Getenv("AWS_S3_FORCE_PATH_STYLE") == "true"
	}
	if v, ok := dbValues["walg_backup_interval"]; ok {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Interval = i
		}
	} else if v := os.Getenv("WALG_BACKUP_INTERVAL"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Interval)
	}
	if v, ok := dbValues["walg_backup_retention_days"]; ok {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.RetentionDays = i
		}
	} else if v := os.Getenv("WALG_BACKUP_RETENTION_DAYS"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.RetentionDays)
	}

	return cfg
}

func (wh *WalgHandler) isConfigured() bool {
	cfg := wh.getConfigFromDB()
	return cfg.S3Prefix != ""
}

// walgEnv builds the environment variables for WAL-G CLI commands. It starts
// with the current process environment and overlays AWS/S3 credentials from
// the database config (system_config table). If PGPASSWORD is not already set,
// it reads the PostgreSQL password so WAL-G can authenticate (e.g. for
// backup-push which calls pg_start_backup/pg_stop_backup).
func (wh *WalgHandler) walgEnv() []string {
	cfg := wh.getConfigFromDB()
	env := os.Environ()

	// Override/add WAL-G settings from DB config
	env = appendEnvIfNotSet(env, "WALG_S3_PREFIX", cfg.S3Prefix)
	env = appendEnvIfNotSet(env, "AWS_ENDPOINT", cfg.Endpoint)
	env = appendEnvIfNotSet(env, "AWS_REGION", cfg.Region)
	if cfg.ForcePathStyle {
		env = appendEnvIfNotSet(env, "AWS_S3_FORCE_PATH_STYLE", "true")
	}

	// WAL-G needs PGPASSWORD to authenticate with PostgreSQL (e.g. backup-push).
	// Read from SECRET_PATH or fall back to extracting from DATABASE_URL.
	if _, hasPwd := envLookup(env, "PGPASSWORD"); !hasPwd {
		if pwd := wh.readPgPassword(); pwd != "" {
			env = append(env, "PGPASSWORD="+pwd)
		}
	}

	return env
}

// readPgPassword returns the PostgreSQL password for the pgmanager user.
// It tries two sources in order:
//  1. SECRET_PATH env var (defaults to /secrets/pgmanager-password) —
//     used in production where the init script writes the password to pgdata
//  2. DATABASE_URL env var — extracts the password from the URL userinfo;
//     used in test environments where SECRET_PATH is not set
//
// Returns empty string if neither source has a password.
func (wh *WalgHandler) readPgPassword() string {
	secretPath := os.Getenv("SECRET_PATH")
	if secretPath == "" {
		secretPath = "/secrets/pgmanager-password"
	}
	data, err := os.ReadFile(secretPath)
	if err == nil {
		return strings.TrimSpace(string(data))
	}

	// Fallback: extract password from DATABASE_URL
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		if u, err := url.Parse(dbURL); err == nil && u.User != nil {
			if pwd, ok := u.User.Password(); ok && pwd != "" {
				return pwd
			}
		}
	}
	return ""
}

// envLookup searches an env var slice for a key and returns its value.
func envLookup(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix), true
		}
	}
	return "", false
}

func appendEnvIfNotSet(env []string, key, value string) []string {
	if value == "" {
		return env
	}
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return env // already set
		}
	}
	return append(env, key+"="+value)
}

func (wh *WalgHandler) runWalg(ctx context.Context, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "wal-g", args...)
	cmd.Env = wh.walgEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("wal-g %s failed: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// GET /api/walg/status
func (wh *WalgHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	cfg := wh.getConfigFromDB()
	if cfg.S3Prefix == "" {
		writeJSON(w, http.StatusOK, walgStatusResponse{
			Enabled:    false,
			Configured: false,
		})
		return
	}

	ctx := r.Context()
	resp := walgStatusResponse{
		Enabled:       true,
		Configured:    true,
		S3Prefix:      cfg.S3Prefix,
		IntervalSec:   cfg.Interval,
		RetentionDays: cfg.RetentionDays,
	}

	// Check if archiving is active by querying PostgreSQL
	var archiveMode string
	err := wh.pool.QueryRow(ctx, "SHOW archive_mode").Scan(&archiveMode)
	if err == nil && archiveMode == "on" {
		resp.Archiving = true
	}

	// Get backup count, last backup time, and total size
	backups, err := wh.listBackups(ctx)
	if err == nil {
		resp.BackupCount = len(backups)
		var totalSize int64
		for _, b := range backups {
			totalSize += b.Size
		}
		resp.TotalSize = totalSize
		if len(backups) > 0 {
			resp.LastBackup = backups[0].Time
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// GET /api/walg/config
func (wh *WalgHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := wh.getConfigFromDB()
	if cfg.S3Prefix == "" {
		writeJSON(w, http.StatusOK, map[string]string{})
		return
	}

	config := map[string]string{
		"s3Prefix":      cfg.S3Prefix,
		"endpoint":      cfg.Endpoint,
		"region":        cfg.Region,
		"forcePathStyle": strconv.FormatBool(cfg.ForcePathStyle),
		"interval":      strconv.Itoa(cfg.Interval),
		"retentionDays": strconv.Itoa(cfg.RetentionDays),
	}

	writeJSON(w, http.StatusOK, config)
}

// PUT /api/walg/config
func (wh *WalgHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req walgConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.S3Prefix == "" {
		writeError(w, http.StatusBadRequest, "s3Prefix is required")
		return
	}

	ctx := r.Context()
	settings := map[string]string{
		"walg_s3_prefix": req.S3Prefix,
	}
	if req.Endpoint != "" {
		settings["walg_endpoint"] = req.Endpoint
	} else {
		settings["walg_endpoint"] = ""
	}
	if req.Region != "" {
		settings["walg_region"] = req.Region
	} else {
		settings["walg_region"] = "us-east-1"
	}
	if req.ForcePathStyle != nil {
		settings["walg_force_path_style"] = strconv.FormatBool(*req.ForcePathStyle)
	}
	if req.Interval != nil {
		settings["walg_backup_interval"] = strconv.Itoa(*req.Interval)
	}
	if req.RetentionDays != nil {
		settings["walg_backup_retention_days"] = strconv.Itoa(*req.RetentionDays)
	}

	for key, value := range settings {
		_, err := wh.pool.Exec(ctx,
			`INSERT INTO system_config (key, value) VALUES ($1, $2)
			 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
			key, value)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save setting: "+key)
			return
		}
	}

	// Update PostgreSQL archive_command with new S3 prefix
	archiveCmd := fmt.Sprintf("wal-g wal-push %%p")
	_, err := wh.pool.Exec(ctx, fmt.Sprintf("ALTER SYSTEM SET archive_command = '%s';", archiveCmd))
	if err != nil {
		log.Printf("walg: failed to update archive_command: %v", err)
	}
	// Reload PostgreSQL config
	_, err = wh.pool.Exec(ctx, "SELECT pg_reload_conf()")
	if err != nil {
		log.Printf("walg: failed to reload PostgreSQL config: %v", err)
	}

	user := auth.GetUserFromContext(r.Context())
	username := ""
	if user != nil {
		username = user.Username
	}
	wh.writeAuditLog(ctx, auditEntry{
		Username:  username,
		Action:    "walg_config_update",
		IPAddress: clientIP(r),
		Detail:    map[string]interface{}{"s3Prefix": req.S3Prefix},
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "message": "Configuration saved"})
}

// GET /api/walg/backups
func (wh *WalgHandler) ListBackups(w http.ResponseWriter, r *http.Request) {
	if !wh.isConfigured() {
		writeError(w, http.StatusBadRequest, "WAL-G is not configured")
		return
	}

	ctx := r.Context()
	backups, err := wh.listBackups(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list backups: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, backups)
}

func (wh *WalgHandler) listBackups(ctx context.Context) ([]walgBackupEntry, error) {
	output, err := wh.runWalg(ctx, []string{"backup-list", "--detail", "--json"})
	if err != nil {
		return nil, err
	}

	var raw []map[string]interface{}
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse backup list: %w", err)
	}

	backups := make([]walgBackupEntry, 0, len(raw))
	for _, b := range raw {
		entry := walgBackupEntry{
			Status: "ok",
		}
		if name, ok := b["backup_name"].(string); ok {
			entry.Name = name
		}
		if t, ok := b["time"].(string); ok {
			entry.Time = t
		}
		if wal, ok := b["wal_file_name"].(string); ok {
			entry.WalSegment = wal
		}
		if size, ok := b["compressed_size"].(float64); ok {
			entry.Size = int64(size)
		} else if size, ok := b["backup_size"].(float64); ok {
			entry.Size = int64(size)
		} else if size, ok := b["size"].(float64); ok {
			entry.Size = int64(size)
		}
		backups = append(backups, entry)
	}

	return backups, nil
}

// POST /api/walg/backup
func (wh *WalgHandler) TriggerBackup(w http.ResponseWriter, r *http.Request) {
	if !wh.isConfigured() {
		writeError(w, http.StatusBadRequest, "WAL-G is not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	user := auth.GetUserFromContext(r.Context())
	username := ""
	if user != nil {
		username = user.Username
	}

	wh.writeAuditLog(ctx, auditEntry{
		Username:  username,
		Action:    "walg_backup_trigger",
		IPAddress: clientIP(r),
	})

	output, err := wh.runWalg(ctx, []string{"backup-push", wh.getPgDataDir(ctx)})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "backup failed: "+sanitizeRedact(err.Error()))
		return
	}

	wh.writeAuditLog(ctx, auditEntry{
		Username:  username,
		Action:    "walg_backup_complete",
		IPAddress: clientIP(r),
		Detail:    map[string]interface{}{"output": string(output)},
	})

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Base backup completed",
		"output":  string(output),
	})
}

// POST /api/walg/restore
func (wh *WalgHandler) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	if !wh.isConfigured() {
		writeError(w, http.StatusBadRequest, "WAL-G is not configured")
		return
	}

	var req walgRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Database == "" {
		writeError(w, http.StatusBadRequest, "database is required")
		return
	}
	if !validName.MatchString(req.Database) {
		writeError(w, http.StatusBadRequest, "invalid database name")
		return
	}

	backupName := req.BackupName
	if backupName == "" {
		backupName = "LATEST"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	restoreDir, err := os.MkdirTemp("", "walg-restore-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create temp directory")
		return
	}
	defer os.RemoveAll(restoreDir)

	user := auth.GetUserFromContext(r.Context())
	username := ""
	if user != nil {
		username = user.Username
	}

	wh.writeAuditLog(ctx, auditEntry{
		Username:  username,
		Action:    "walg_restore_start",
		Database:  req.Database,
		IPAddress: clientIP(r),
		Detail:    map[string]interface{}{"backupName": backupName},
	})

	// Step 1: Fetch backup (raw PGDATA)
	_, err = wh.runWalg(ctx, []string{"backup-fetch", restoreDir, backupName})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch backup: "+sanitizeRedact(err.Error()))
		return
	}

	pgdataDir := findPGDataDir(restoreDir)
	if pgdataDir == "" {
		writeError(w, http.StatusInternalServerError, "could not find PostgreSQL data directory in backup")
		return
	}

	// Step 2: Start a temporary PostgreSQL instance from the fetched PGDATA
	// to pg_dump the specific database in custom format.
	// The fetched PGDATA is already initialized — no initdb needed.
	tmpPort := "15432"
	tmpSocketDir, err := os.MkdirTemp("", "walg-pg-sock-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create temp socket dir")
		return
	}
	defer os.RemoveAll(tmpSocketDir)

	// Configure trust auth on the temp instance
	hbaConf := filepath.Join(pgdataDir, "pg_hba.conf")
	hbaContent := "local all all trust\nhost all all 127.0.0.1/32 trust\n"
	if err := os.WriteFile(hbaConf, []byte(hbaContent), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write pg_hba.conf")
		return
	}

	// Write a minimal postgresql.conf for the temp instance
	postgresqlConf := filepath.Join(pgdataDir, "postgresql.conf")
	minimalConf := fmt.Sprintf(
		"port = %s\n"+
			"unix_socket_directories = '%s'\n"+
			"listen_addresses = '127.0.0.1'\n"+
			"shared_preload_libraries = ''\n"+
			"archive_mode = off\n"+
			"max_wal_senders = 10\n"+
			"wal_level = minimal\n"+
			"restore_command = 'wal-g wal-fetch %%f %%p'\n"+
			"recovery_target_action = 'promote'\n",
		tmpPort, tmpSocketDir,
	)
	if err := os.WriteFile(postgresqlConf, []byte(minimalConf), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write postgresql.conf")
		return
	}

	// Write pg_hba.conf with trust auth for the temp instance
	pgHbaConf := filepath.Join(pgdataDir, "pg_hba.conf")
	minimalHba := "local all all trust\nhost all all 127.0.0.1/32 trust\n"
	if err := os.WriteFile(pgHbaConf, []byte(minimalHba), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write pg_hba.conf")
		return
	}

	// Create recovery.signal so PostgreSQL enters recovery mode.
	// Keep backup_label — PostgreSQL needs it for the checkpoint record.
	// restore_command = 'wal-g wal-fetch %f %p' fetches WAL from S3.
	os.WriteFile(filepath.Join(pgdataDir, "recovery.signal"), []byte(""), 0644)

	// Chown PGDATA, restoreDir, and socket dir to postgres user (UID 70)
	chownCmd := exec.CommandContext(ctx, "chown", "-R", "70:70", pgdataDir)
	chownCmd.Run()
	chownCmd2 := exec.CommandContext(ctx, "chown", "-R", "70:70", restoreDir)
	chownCmd2.Run()
	chownCmd3 := exec.CommandContext(ctx, "chown", "-R", "70:70", tmpSocketDir)
	chownCmd3.Run()

	var pgStderr bytes.Buffer
	pgCmd := exec.CommandContext(ctx, "su-exec", "postgres",
		"postgres",
		"-D", pgdataDir,
		"-p", tmpPort,
		"-k", tmpSocketDir,
	)
	pgCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	pgCmd.Env = append(os.Environ(), "LC_ALL=C")
	pgCmd.Env = append(pgCmd.Env, wh.walgEnv()...)
	pgCmd.Stdout = &pgStderr
	pgCmd.Stderr = &pgStderr
	if err := pgCmd.Start(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start temp postgres: "+sanitizeRedact(pgStderr.String()))
		return
	}
	log.Printf("walg-restore: temp postgres started (pid=%d)", pgCmd.Process.Pid)
	defer cleanupPostgres(pgCmd)

	// Get admin user for connection checks
	_, _, adminUser, _ := wh.getPgCredentials()

	// Wait for the temp instance to fully promote (not just accepting connections during recovery)
	ready := false
	for i := 0; i < 60; i++ {
		// Check if process is still alive
		if pgCmd.Process != nil {
			if err := pgCmd.Process.Signal(syscall.Signal(0)); err != nil {
				log.Printf("walg-restore: temp postgres died after %ds, stderr: %s", i, pgStderr.String())
				break
			}
		}
		checkCmd := exec.CommandContext(ctx, "pg_isready",
			"-h", "127.0.0.1", "-p", tmpPort,
		)
		if err := checkCmd.Run(); err == nil {
			// Verify recovery is finished by checking pg_is_in_recovery()
			psqlCheck := exec.CommandContext(ctx, "psql",
				"-h", "127.0.0.1", "-p", tmpPort,
				"-U", adminUser,
				"-d", "template1",
				"-tAc", "SELECT pg_is_in_recovery()",
			)
			psqlCheck.Env = append(os.Environ(), "PGPASSWORD=")
			out, err := psqlCheck.Output()
			if err == nil && strings.TrimSpace(string(out)) == "f" {
				ready = true
				break
			}
		}
		time.Sleep(time.Second)
	}
	if !ready {
		log.Printf("walg-restore: temp postgres did not start/promote within 60s. stderr: %s", pgStderr.String())
		writeError(w, http.StatusInternalServerError, "temp postgres did not start/promote within 60s: "+sanitizeRedact(pgStderr.String()))
		return
	}
	log.Printf("walg-restore: temp postgres ready, dumping database pgmanager into %s", req.Database)

	// Step 3: pg_dump the pgmanager database from the backup in custom format.
	// The backup is a full cluster backup — pgmanager is the main database.
	// Use the superuser from the backup data (same as the primary server's admin user).
	_, _, pgUser, _ := wh.getPgCredentials()
	dumpFile := filepath.Join(restoreDir, "dump.dump")
	dumpCmd := exec.CommandContext(ctx, "pg_dump",
		"-h", "127.0.0.1",
		"-p", tmpPort,
		"-U", pgUser,
		"-d", "pgmanager",
		"-Fc",
		"-f", dumpFile,
	)
	dumpCmd.Env = append(os.Environ(), "PGPASSWORD=")
	var dumpStderr bytes.Buffer
	dumpCmd.Stderr = &dumpStderr
	if err := dumpCmd.Run(); err != nil {
		writeError(w, http.StatusInternalServerError, "pg_dump failed: "+sanitizeRedact(dumpStderr.String()))
		return
	}

	// Step 4: pg_restore into the target database
	host, port, pgUser, pgPassword := wh.getPgCredentials()
	restoreCmd := exec.CommandContext(ctx, "pg_restore",
		"-h", host,
		"-p", port,
		"-U", pgUser,
		"-d", req.Database,
		"--no-owner",
		"--no-privileges",
		"--clean",
		"--if-exists",
		dumpFile,
	)
	restoreCmd.Env = append(os.Environ(), "PGPASSWORD="+pgPassword)
	var restoreStderr bytes.Buffer
	restoreCmd.Stderr = &restoreStderr

	err = restoreCmd.Run()
	if err != nil && restoreCmd.ProcessState.ExitCode() > 1 {
		writeError(w, http.StatusInternalServerError,
			"pg_restore failed: "+sanitizeRedact(restoreStderr.String()))
		return
	}

	wh.writeAuditLog(ctx, auditEntry{
		Username:  username,
		Action:    "walg_restore_complete",
		Database:  req.Database,
		IPAddress: clientIP(r),
		Detail:    map[string]interface{}{"backupName": backupName},
	})

	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "success",
		"database": req.Database,
		"backup":   backupName,
	})
}

// cleanupPostgres gracefully shuts down a temp postgres process and all its children.
// Uses process group kill to ensure no orphaned backend workers remain.
func cleanupPostgres(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pgPid := cmd.Process.Pid

	// SIGINT to entire process group (graceful shutdown)
	if err := syscall.Kill(-pgPid, syscall.SIGINT); err != nil {
		// Process group may already be dead
		cmd.Wait()
		reapOrphans()
		return
	}

	// Wait up to 10s for clean shutdown
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		// SIGKILL entire process group (force kill)
		syscall.Kill(-pgPid, syscall.SIGKILL)
		<-done
	}

	reapOrphans()
}

// reapOrphans reaps any remaining child processes that escaped the process group kill.
func reapOrphans() {
	for {
		var wstatus syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &wstatus, syscall.WNOHANG, nil)
		if err != nil || pid <= 0 {
			break
		}
	}
}

func findPGDataDir(root string) string {
	var found string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if info.Name() == "PG_VERSION" {
			found = filepath.Dir(path)
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

// DELETE /api/walg/backup/{name}
func (wh *WalgHandler) DeleteBackup(w http.ResponseWriter, r *http.Request) {
	if !wh.isConfigured() {
		writeError(w, http.StatusBadRequest, "WAL-G is not configured")
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/walg/backup/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "backup name is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	user := auth.GetUserFromContext(r.Context())
	username := ""
	if user != nil {
		username = user.Username
	}

	_, err := wh.runWalg(ctx, []string{"delete", "target", "--confirm", name})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete backup: "+sanitizeRedact(err.Error()))
		return
	}

	wh.writeAuditLog(ctx, auditEntry{
		Username:  username,
		Action:    "walg_backup_delete",
		IPAddress: clientIP(r),
		Detail:    map[string]interface{}{"backupName": name},
	})

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "deleted",
		"backup": name,
	})
}

// POST /api/walg/verify
func (wh *WalgHandler) VerifyIntegrity(w http.ResponseWriter, r *http.Request) {
	if !wh.isConfigured() {
		writeError(w, http.StatusBadRequest, "WAL-G is not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	output, err := wh.runWalg(ctx, []string{"wal-verify", "integrity"})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "verification failed: "+sanitizeRedact(err.Error()))
		return
	}

	status := "OK"
	outputStr := string(output)
	if strings.Contains(outputStr, "FAILURE") {
		status = "FAILURE"
	} else if strings.Contains(outputStr, "WARNING") {
		status = "WARNING"
	}

	writeJSON(w, http.StatusOK, walgVerifyResponse{
		Status:  status,
		Details: outputStr,
	})
}

// DELETE /api/walg/garbage
func (wh *WalgHandler) CleanGarbage(w http.ResponseWriter, r *http.Request) {
	if !wh.isConfigured() {
		writeError(w, http.StatusBadRequest, "WAL-G is not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	user := auth.GetUserFromContext(r.Context())
	username := ""
	if user != nil {
		username = user.Username
	}

	output, err := wh.runWalg(ctx, []string{"delete", "garbage"})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "garbage cleanup failed: "+sanitizeRedact(err.Error()))
		return
	}

	wh.writeAuditLog(ctx, auditEntry{
		Username:  username,
		Action:    "walg_garbage_cleanup",
		IPAddress: clientIP(r),
	})

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "success",
		"output": string(output),
	})
}

// POST /api/walg/test-connection
func (wh *WalgHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	cfg := wh.getConfigFromDB()
	if cfg.S3Prefix == "" {
		writeError(w, http.StatusBadRequest, "WAL-G is not configured. Set S3 bucket path first.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Try to list backups — this validates S3 connectivity and credentials
	_, err := wh.listBackups(ctx)
	if err != nil {
		errMsg := sanitizeRedact(err.Error())
		writeError(w, http.StatusInternalServerError, "connection failed: "+errMsg)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Connected to " + cfg.S3Prefix,
	})
}

func (wh *WalgHandler) getPgDataDir(ctx context.Context) string {
	if v := os.Getenv("PGDATA"); v != "" {
		return v
	}
	var dataDir string
	err := wh.pool.QueryRow(ctx, "SHOW data_directory").Scan(&dataDir)
	if err == nil && dataDir != "" {
		return dataDir
	}
	return "/var/lib/postgresql/data"
}

// getPgCredentials returns PostgreSQL connection parameters for pg_restore
// and other CLI tools. It prefers DATABASE_URL (single source of truth in
// test/dev), then falls back to individual PGHOST/PGPORT/PGUSER env vars
// and readPgPassword() for the password.
func (wh *WalgHandler) getPgCredentials() (host, port, user, password string) {
	// If DATABASE_URL is set, extract all credentials from it.
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		if u, err := url.Parse(dbURL); err == nil && u.User != nil {
			user = u.User.Username()
			password, _ = u.User.Password()
			host = u.Hostname()
			port = u.Port()
			if port == "" {
				port = "5432"
			}
			return
		}
	}

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
	password = wh.readPgPassword()
	return
}

// GetScheduledBackupInterval returns the backup interval from system_config (or env fallback).
func (wh *WalgHandler) GetScheduledBackupInterval() int {
	cfg := wh.getConfigFromDB()
	if cfg.Interval < 60 {
		return 60
	}
	return cfg.Interval
}

// RunScheduledBackup is called by the ticker in main.go
func (wh *WalgHandler) RunScheduledBackup() {
	if !wh.isConfigured() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	log.Println("walg: starting scheduled base backup...")
	output, err := wh.runWalg(ctx, []string{"backup-push", wh.getPgDataDir(ctx)})
	if err != nil {
		log.Printf("walg: scheduled backup failed: %v", err)
		return
	}
	log.Printf("walg: scheduled backup completed: %s", strings.TrimSpace(string(output)))

	// Run garbage cleanup after successful backup to remove expired WAL segments
	// and backups beyond retention period.
	gcCtx, gcCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer gcCancel()

	gcOutput, gcErr := wh.runWalg(gcCtx, []string{"delete", "garbage", "--confirm"})
	if gcErr != nil {
		log.Printf("walg: scheduled garbage cleanup failed: %v", gcErr)
	} else {
		log.Printf("walg: scheduled garbage cleanup completed: %s", strings.TrimSpace(string(gcOutput)))
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func orFallback(dbVal, envVal string) string {
	if dbVal != "" {
		return dbVal
	}
	return envVal
}
