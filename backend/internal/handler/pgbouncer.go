package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var hbaFilePath = "/etc/pgbouncer/shared/pg_hba.conf"
var pgbouncerIniPath = "/etc/pgbouncer/shared/pgbouncer.ini"

func readPasswordFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("failed to read password file %s: %v", path, err)
		return ""
	}
	return strings.TrimSpace(string(data))
}

type pgbouncerDatabase struct {
	DatabaseName string `json:"databaseName"`
	Allowed      bool   `json:"allowed"`
	CreatedAt    string `json:"createdAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
}

func (h *Handler) ListPgBouncerDatabases(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT database_name, allowed, created_at::text, updated_at::text FROM pgbouncer_databases ORDER BY database_name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list pgbouncer databases")
		return
	}
	defer rows.Close()

	databases := make([]pgbouncerDatabase, 0)
	for rows.Next() {
		var db pgbouncerDatabase
		if err := rows.Scan(&db.DatabaseName, &db.Allowed, &db.CreatedAt, &db.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan row")
			return
		}
		databases = append(databases, db)
	}
	writeJSON(w, http.StatusOK, databases)
}

func (h *Handler) TogglePgBouncerDatabase(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/pgbouncer/databases/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "database name is required")
		return
	}
	if !validName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid database name")
		return
	}

	var req struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`UPDATE pgbouncer_databases SET allowed = $1, updated_at = NOW() WHERE database_name = $2`,
		req.Allowed, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update pgbouncer database")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "database not found in pgbouncer_databases")
		return
	}

	h.RebuildPgBouncerHBA()
	writeJSON(w, http.StatusOK, pgbouncerDatabase{DatabaseName: name, Allowed: req.Allowed})
}

func (h *Handler) RebuildPgBouncerHBA() {
	ctx := context.Background()

	rows, err := h.pool.Query(ctx, `
		SELECT DISTINCT ON (username) username, allowed_ips
		FROM managed_users
		ORDER BY username, created_at
	`)
	if err != nil {
		log.Printf("Failed to query allowed_ips for PgBouncer HBA: %v", err)
		return
	}
	defer rows.Close()

	var lines []string

	lines = append(lines, "host all pgbouncer_auth 127.0.0.1/32 trust")
	lines = append(lines, "host all pgbouncer_auth ::1/128 trust")
	lines = append(lines, "host all pgbouncer_auth 172.16.0.0/12 trust")
	lines = append(lines, "host all pgbouncer_auth 192.168.0.0/16 trust")
	lines = append(lines, "host all pgbouncer_auth 10.0.0.0/8 trust")

	lines = append(lines, "host all pgmanager 172.16.0.0/12 trust")
	lines = append(lines, "host all pgmanager 192.168.0.0/16 trust")
	lines = append(lines, "host all pgmanager 10.0.0.0/8 trust")

	for rows.Next() {
		var username string
		var allowedIpsRaw []byte
		if err := rows.Scan(&username, &allowedIpsRaw); err != nil {
			log.Printf("Failed to scan allowed_ips: %v", err)
			continue
		}
		var allowedIps []string
		if err := json.Unmarshal(allowedIpsRaw, &allowedIps); err != nil {
			log.Printf("Failed to unmarshal allowed_ips for %s: %v", username, err)
			continue
		}

		if len(allowedIps) == 0 {
			lines = append(lines, fmt.Sprintf("host all \"%s\" 0.0.0.0/0 reject", username))
			continue
		}

		for _, ip := range allowedIps {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			if !strings.Contains(ip, "/") {
				ip = ip + "/32"
			}
			lines = append(lines, fmt.Sprintf("host all \"%s\" %s scram-sha-256", username, ip))
		}
	}

	lines = append(lines, "host all all 0.0.0.0/0 reject")
	lines = append(lines, "host all all ::0/0 reject")

	content := strings.Join(lines, "\n") + "\n"

	os.MkdirAll("/etc/pgbouncer/shared", 0755)

	err = os.WriteFile(hbaFilePath, []byte(content), 0644)
	if err != nil {
		log.Printf("Failed to write %s: %v", hbaFilePath, err)
		return
	}

	log.Println("PgBouncer HBA file regenerated successfully")

	// Rebuild [databases] section from pgbouncer_databases table
	h.rebuildPgBouncerDatabases(ctx)

	authPasswordPath := os.Getenv("PGBOUNCER_AUTH_PASSWORD")
	if authPasswordPath == "" {
		log.Printf("PGBOUNCER_AUTH_PASSWORD not set, cannot RELOAD PgBouncer")
		return
	}
	authPassword := readPasswordFile(authPasswordPath)
	if authPassword == "" {
		log.Printf("pgbouncer_auth password file empty, cannot RELOAD PgBouncer")
		return
	}

	pgbouncerAdminDB := fmt.Sprintf(
		"postgres://pgbouncer_auth:%s@pgbouncer:6432/pgbouncer?sslmode=disable",
		authPassword,
	)

	reloadCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	conn, err := pgx.Connect(reloadCtx, pgbouncerAdminDB)
	if err != nil {
		log.Printf("Failed to connect to PgBouncer admin DB for reload: %v", err)
		return
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "RELOAD;")
	if err != nil {
		log.Printf("Failed to issue RELOAD to PgBouncer: %v", err)
		return
	}

	log.Println("PgBouncer successfully reloaded HBA rules")
}

func (h *Handler) rebuildPgBouncerDatabases(ctx context.Context) {
	rows, err := h.pool.Query(ctx,
		`SELECT database_name FROM pgbouncer_databases WHERE allowed = true ORDER BY database_name`)
	if err != nil {
		log.Printf("Failed to query pgbouncer_databases: %v", err)
		return
	}
	defer rows.Close()

	var dbLines []string
	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err != nil {
			log.Printf("Failed to scan pgbouncer_databases row: %v", err)
			continue
		}
		dbLines = append(dbLines, fmt.Sprintf("%s = host=db port=5432 dbname=%s", dbName, dbName))
	}

	// Read existing pgbouncer.ini
	data, err := os.ReadFile(pgbouncerIniPath)
	if err != nil {
		log.Printf("Failed to read %s: %v", pgbouncerIniPath, err)
		return
	}

	content := string(data)

	// Replace [databases] section
	databasesSection := "[databases]\n"
	if len(dbLines) > 0 {
		databasesSection += strings.Join(dbLines, "\n") + "\n"
	} else {
		databasesSection += "; no databases allowed through PgBouncer\n"
	}

	// Find and replace [databases] section up to next [ section or EOF
	lines := strings.Split(content, "\n")
	var result []string
	inDatabases := false
	replaced := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[databases]" {
			inDatabases = true
			replaced = true
			result = append(result, databasesSection)
			continue
		}
		if inDatabases && strings.HasPrefix(trimmed, "[") {
			inDatabases = false
		}
		if !inDatabases {
			result = append(result, line)
		}
	}

	if !replaced {
		result = append(result, databasesSection)
	}

	os.MkdirAll("/etc/pgbouncer/shared", 0755)

	err = os.WriteFile(pgbouncerIniPath, []byte(strings.Join(result, "\n")), 0644)
	if err != nil {
		log.Printf("Failed to write %s: %v", pgbouncerIniPath, err)
		return
	}

	log.Printf("PgBouncer databases config regenerated (%d allowed)", len(dbLines))
}
