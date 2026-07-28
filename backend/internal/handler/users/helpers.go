package users

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ExtractUserFromPath(path string) string {
	prefix := "/api/users/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if idx := strings.Index(rest, "/"); idx < 0 {
		return rest
	} else {
		return rest[:idx]
	}
}

func ExtractUserDBFromPath(path string) (string, string) {
	prefix := "/api/users/"
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}
	rest := path[len(prefix):]
	parts := strings.Split(rest, "/")
	if len(parts) >= 3 && parts[1] == "databases" {
		return parts[0], parts[2]
	}
	return "", ""
}

func ResolveConnectionStringHost(r *http.Request) string {
	host := os.Getenv("PGMANAGER_HOST")
	if host == "" {
		h, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		} else {
			host = h
		}
		host = net.JoinHostPort(host, "5432")
	} else if _, _, err := net.SplitHostPort(host); err != nil {
		port := os.Getenv("PGMANAGER_PORT")
		if port == "" {
			port = "5432"
		}
		host = net.JoinHostPort(host, port)
	}
	return host
}

func IsSSLEnabled(pool *pgxpool.Pool) bool {
	var setting string
	err := pool.QueryRow(context.Background(), "SHOW ssl").Scan(&setting)
	if err != nil {
		return false
	}
	return setting == "on"
}

func GetUserDatabases(ctx context.Context, pool *pgxpool.Pool, username string) []string {
	rows, err := pool.Query(ctx, "SELECT database_name FROM managed_users WHERE username = $1", username)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var dbs []string
	for rows.Next() {
		var db string
		if err := rows.Scan(&db); err == nil {
			dbs = append(dbs, db)
		}
	}
	return dbs
}

func WithDatabase(ctx context.Context, baseDSN string, dbName string, fn func(conn *pgx.Conn) error) error {
	dsn := baseDSN + "&dbname=" + dbName
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	return fn(conn)
}
