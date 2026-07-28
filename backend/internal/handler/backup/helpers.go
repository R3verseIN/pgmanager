package backup

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func sanitizeRedact(s string) string {
	s = regexp.MustCompile(`(postgres://[^:]+:)[^@]+(@)`).ReplaceAllString(s, "${1}***${2}")
	s = regexp.MustCompile(`(?i)(PGPASSWORD=)\S+`).ReplaceAllString(s, "${1}***")
	s = regexp.MustCompile(`(?i)(password=)\S+`).ReplaceAllString(s, "${1}***")
	return s
}

func ListExistingTables(ctx context.Context, baseDSN string, dbName string) ([]string, error) {
	host, port, user, password := getPgCredentials()
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

func getPgCredentials() (host, port, user, password string) {
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
