package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PgbackrestHandler struct {
	pool *pgxpool.Pool
}

type BackupSettings struct {
	Enabled        bool   `json:"enabled"`
	ArchiveTimeout int    `json:"archive_timeout"`
	RetentionDays  int    `json:"retention_days"`
	FullBackupDay  int    `json:"full_backup_day"` // 0 = Sunday, 1 = Monday, etc.
	BackupHour     int    `json:"backup_hour"`     // 0-23
}

func NewPgbackrestHandler(pool *pgxpool.Pool) *PgbackrestHandler {
	h := &PgbackrestHandler{pool: pool}
	go h.startScheduler()
	return h
}

func (h *PgbackrestHandler) getSettings(ctx context.Context) (BackupSettings, error) {
	settings := BackupSettings{
		Enabled:        false,
		ArchiveTimeout: 60,
		RetentionDays:  7,
		FullBackupDay:  0,
		BackupHour:     2,
	}

	rows, err := h.pool.Query(ctx, "SELECT key, value FROM system_config WHERE key LIKE 'backup_%'")
	if err != nil {
		return settings, err
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "backup_enabled":
			settings.Enabled = value == "true"
		case "backup_archive_timeout":
			fmt.Sscanf(value, "%d", &settings.ArchiveTimeout)
		case "backup_retention_days":
			fmt.Sscanf(value, "%d", &settings.RetentionDays)
		case "backup_full_backup_day":
			fmt.Sscanf(value, "%d", &settings.FullBackupDay)
		case "backup_hour":
			fmt.Sscanf(value, "%d", &settings.BackupHour)
		}
	}
	return settings, nil
}

func (h *PgbackrestHandler) updateSetting(ctx context.Context, key, value string) error {
	_, err := h.pool.Exec(ctx, `
		INSERT INTO system_config (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
	`, key, value)
	return err
}

func (h *PgbackrestHandler) startScheduler() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		settings, err := h.getSettings(ctx)
		if err != nil || !settings.Enabled {
			continue
		}

		now := time.Now()
		if now.Hour() == settings.BackupHour {
			backupType := "incr"
			if int(now.Weekday()) == settings.FullBackupDay {
				backupType = "full"
			}
			
			log.Printf("Starting scheduled pgBackRest backup (type: %s)", backupType)
			cmd := exec.Command("pgbackrest", "--stanza=pgmanager", "--pg1-path=/var/lib/postgresql/data", "--type="+backupType, 
				"--repo1-retention-full-type=time", 
				"--start-fast",
				fmt.Sprintf("--repo1-retention-full=%d", settings.RetentionDays), 
				"backup")
			
			if output, err := cmd.CombinedOutput(); err != nil {
				log.Printf("Scheduled backup failed: %v\nOutput: %s", err, string(output))
			} else {
				log.Printf("Scheduled backup succeeded.")
			}
		}
	}
}

func (h *PgbackrestHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	settings, err := h.getSettings(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Run pgbackrest info to get json
	cmd := exec.Command("pgbackrest", "--stanza=pgmanager", "--output=json", "info")
	output, err := cmd.CombinedOutput()
	
	var info interface{}
	if err == nil && len(output) > 0 {
		json.Unmarshal(output, &info)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"settings": settings,
		"info":     info,
		"configured": os.Getenv("PGBACKREST_REPO1_S3_KEY") != "",
	})
}

func (h *PgbackrestHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req BackupSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	h.updateSetting(ctx, "backup_enabled", fmt.Sprintf("%t", req.Enabled))
	h.updateSetting(ctx, "backup_archive_timeout", fmt.Sprintf("%d", req.ArchiveTimeout))
	h.updateSetting(ctx, "backup_retention_days", fmt.Sprintf("%d", req.RetentionDays))
	h.updateSetting(ctx, "backup_full_backup_day", fmt.Sprintf("%d", req.FullBackupDay))
	h.updateSetting(ctx, "backup_hour", fmt.Sprintf("%d", req.BackupHour))

	// Apply to Postgres
	if req.Enabled {
		// pgbackrest stanza-create is required before archiving can start
		cmd := exec.Command("pgbackrest", "--stanza=pgmanager", "--pg1-path=/var/lib/postgresql/data", "stanza-create")
		
		pgPass := os.Getenv("POSTGRES_PASSWORD")
		if pgPass == "" {
			pgPass = os.Getenv("PGPASSWORD")
			if pgPass == "" {
				pgPass = "pgmanager" // fallback
			}
		}
		cmd.Env = append(os.Environ(), "PGPASSWORD="+pgPass)

		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("stanza-create failed: %v\nOutput: %s", err, string(output))
			http.Error(w, fmt.Sprintf("stanza-create failed: %s", string(output)), http.StatusInternalServerError)
			return
		}

		h.pool.Exec(ctx, "ALTER SYSTEM SET wal_level = 'replica'")
		h.pool.Exec(ctx, "ALTER SYSTEM SET archive_mode = 'on'")
		h.pool.Exec(ctx, "ALTER SYSTEM SET archive_command = 'pgbackrest --stanza=pgmanager --pg1-path=/var/lib/postgresql/data archive-push %p'")
		h.pool.Exec(ctx, fmt.Sprintf("ALTER SYSTEM SET archive_timeout = '%ds'", req.ArchiveTimeout))
		
		// Write the restart signal file to trigger pg-entrypoint.sh to restart postgres
		signalFile := filepath.Join("/var/lib/postgresql/data", "pgmanager-restart-signal")
		if err := os.WriteFile(signalFile, []byte("restart"), 0644); err != nil {
			log.Printf("failed to write restart signal: %v", err)
		}
	} else {
		h.pool.Exec(ctx, "ALTER SYSTEM SET archive_mode = 'off'")
		h.pool.Exec(ctx, "ALTER SYSTEM RESET archive_command")
		h.pool.Exec(ctx, "ALTER SYSTEM RESET archive_timeout")
	}

	// Trigger restart
	os.WriteFile("/var/lib/postgresql/data/pgbouncer-restart-signal", []byte("1"), 0644)
	
	json.NewEncoder(w).Encode(map[string]string{"message": "Settings applied"})
}

func (h *PgbackrestHandler) ListBackups(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("pgbackrest", "--stanza=pgmanager", "--output=json", "info")
	output, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, string(output), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(output)
}

func (h *PgbackrestHandler) TriggerBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type string `json:"type"` // full or incr
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Type == "" {
		req.Type = "incr"
	}

	settings, _ := h.getSettings(r.Context())

	cmd := exec.Command("pgbackrest", "--stanza=pgmanager", "--pg1-path=/var/lib/postgresql/data", "--type="+req.Type, 
		"--repo1-retention-full-type=time", 
		fmt.Sprintf("--repo1-retention-full=%d", settings.RetentionDays), 
		"--start-fast",
		"backup")
	
	pgPass := os.Getenv("POSTGRES_PASSWORD")
	if pgPass == "" {
		pgPass = os.Getenv("PGPASSWORD")
		if pgPass == "" {
			pgPass = "pgmanager" // fallback
		}
	}
	cmd.Env = append(os.Environ(), "PGPASSWORD="+pgPass)

	output, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, string(output), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"message": "Backup triggered successfully"})
}

func (h *PgbackrestHandler) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Database   string `json:"database"`
		BackupName string `json:"backup_name"` // Base backup label
		TargetTime string `json:"target_time"` // Point-in-time target
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.Database == "" {
		http.Error(w, "database required", http.StatusBadRequest)
		return
	}

	// 1. Create temp directory
	tempDir, err := os.MkdirTemp("/var/lib/postgresql/data", "restore-*")
	if err != nil {
		http.Error(w, "failed to create temp dir", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tempDir)

	// 2. Restore pgbackrest to temp directory
	args := []string{"--stanza=pgmanager", "--pg1-path=" + tempDir, "restore"}
	if req.BackupName != "" {
		args = append(args, "--set="+req.BackupName)
	} else if req.TargetTime != "" {
		args = append(args, "--type=time", "--target="+req.TargetTime)
	}
	cmd := exec.Command("pgbackrest", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		http.Error(w, fmt.Sprintf("pgbackrest restore failed: %s", out), http.StatusInternalServerError)
		return
	}

	// Change ownership of the temp directory to postgres user
	exec.Command("chown", "-R", "postgres:postgres", tempDir).Run()
	// Change permissions as required by postgres (must be 0700)
	exec.Command("chmod", "-R", "0700", tempDir).Run()

	// 3. Start temp postgres
	port := "15432"
	pgCmd := exec.Command("su-exec", "postgres", "postgres", "-D", tempDir, "-p", port, "-k", "/tmp")
	var pgErr bytes.Buffer
	pgCmd.Stderr = &pgErr
	if err := pgCmd.Start(); err != nil {
		http.Error(w, fmt.Sprintf("failed to start temp postgres: %v", err), http.StatusInternalServerError)
		return
	}
	defer func() {
		pgCmd.Process.Kill()
		pgCmd.Wait()
	}()

	// Wait for temp postgres to accept connections
	ready := false
	for i := 0; i < 30; i++ {
		if err := exec.Command("pg_isready", "-h", "127.0.0.1", "-p", port, "-U", "pgmanager").Run(); err == nil {
			ready = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !ready {
		http.Error(w, fmt.Sprintf("temp postgres failed to start. Logs: %s", pgErr.String()), http.StatusInternalServerError)
		return
	}

	// 4. pg_dump from temp to live
	dumpCmd := exec.Command("pg_dump", "-h", "127.0.0.1", "-p", port, "-U", "pgmanager", "-d", req.Database, "-c")
	dumpCmd.Env = append(os.Environ(), "PGPASSWORD=pgmanager")
	var dumpOut, dumpErr bytes.Buffer
	dumpCmd.Stdout = &dumpOut
	dumpCmd.Stderr = &dumpErr

	if err := dumpCmd.Run(); err != nil {
		http.Error(w, fmt.Sprintf("pg_dump failed: %v, stderr: %s", err, dumpErr.String()), http.StatusInternalServerError)
		return
	}

	// 5. pg_restore (psql) to live
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "db"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5432"
	}
	liveCmd := exec.Command("psql", "-h", dbHost, "-p", dbPort, "-U", "pgmanager", "-d", req.Database)
	liveCmd.Stdin = &dumpOut
	
	pgPass := os.Getenv("POSTGRES_PASSWORD")
	if pgPass == "" {
		pgPass = os.Getenv("PGPASSWORD")
	}
	if pgPass != "" {
		liveCmd.Env = append(os.Environ(), "PGPASSWORD="+pgPass)
	} else {
		liveCmd.Env = os.Environ()
	}

	if out, err := liveCmd.CombinedOutput(); err != nil {
		http.Error(w, fmt.Sprintf("restore to live failed: %v\noutput: %s", err, out), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"message": "Restore completed"})
}

func (h *PgbackrestHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("pgbackrest", "info")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if !strings.Contains(string(out), "No stanzas exist") {
			http.Error(w, string(out), http.StatusInternalServerError)
			return
		}
	}
	json.NewEncoder(w).Encode(map[string]string{"message": "Connection OK"})
}
