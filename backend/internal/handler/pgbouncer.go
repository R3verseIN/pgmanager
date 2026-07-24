package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var hbaFilePath = "/etc/pgbouncer/shared/pg_hba.conf"

func (h *Handler) RebuildPgBouncerHBA() {
	ctx := context.Background()

	// 1. Fetch all managed users and their allowed IPs from pgmanager database
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
	
	// Standard rules for internal docker communication
	lines = append(lines, "host all pgbouncer_auth 127.0.0.1/32 trust")
	lines = append(lines, "host all pgbouncer_auth ::1/128 trust")
	lines = append(lines, "host all pgbouncer_auth 172.16.0.0/12 trust") // Docker bridge networks
	lines = append(lines, "host all pgbouncer_auth 192.168.0.0/16 trust") 
	lines = append(lines, "host all pgbouncer_auth 10.0.0.0/8 trust") 
	
	// Ensure the backend go app can connect
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
			// If none, fallback to deny all for this user
			lines = append(lines, fmt.Sprintf("host all \"%s\" 0.0.0.0/0 reject", username))
			continue
		}

		for _, ip := range allowedIps {
			// Add a rule for each allowed IP
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			// ensure CIDR suffix
			if !strings.Contains(ip, "/") {
				ip = ip + "/32"
			}
			lines = append(lines, fmt.Sprintf("host all \"%s\" %s scram-sha-256", username, ip))
		}
	}

	// Default fall-through is reject everything else
	lines = append(lines, "host all all 0.0.0.0/0 reject")
	lines = append(lines, "host all all ::0/0 reject")

	content := strings.Join(lines, "\n") + "\n"

	// 2. Write to the shared volume file
	// Ensure directory exists
	os.MkdirAll("/etc/pgbouncer/shared", 0755)
	
	err = os.WriteFile(hbaFilePath, []byte(content), 0644)
	if err != nil {
		log.Printf("Failed to write %s: %v", hbaFilePath, err)
		return
	}

	log.Println("PgBouncer HBA file regenerated successfully")

	// 3. Issue RELOAD to PgBouncer
	pgbouncerAdminDB := os.Getenv("PGBOUNCER_ADMIN_URL")
	if pgbouncerAdminDB == "" {
		pgbouncerAdminDB = "postgres://pgbouncer_auth:pgbouncer_auth_password@pgbouncer:6432/pgbouncer?sslmode=disable"
	}
	
	// 3. Issue RELOAD to PgBouncer (with short timeout so local/test envs fail fast)
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
