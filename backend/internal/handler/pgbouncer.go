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

func readPasswordFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("failed to read password file %s: %v", path, err)
		return ""
	}
	return strings.TrimSpace(string(data))
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
