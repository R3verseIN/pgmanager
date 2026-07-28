package platform

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func BuildDatabaseURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}

	secretPath := os.Getenv("SECRET_PATH")
	if secretPath == "" {
		secretPath = "/secrets/pgmanager-password"
	}

	password := ReadPassword(secretPath)

	host := os.Getenv("PGHOST")
	if host == "" {
		host = "localhost"
	}

	port := os.Getenv("PGPORT")
	if port == "" {
		port = "5432"
	}

	user := os.Getenv("PGUSER")
	if user == "" {
		user = "pgmanager"
	}

	dbname := os.Getenv("PGDATABASE")
	if dbname == "" {
		dbname = "pgmanager"
	}

	sslmode := "disable"

	url := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, password, host, port, dbname, sslmode)
	log.Printf("connecting to database at %s:%s/%s as %s", host, port, dbname, user)
	return url
}

func BuildBaseDSN() string {
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		if u, err := url.Parse(dbURL); err == nil && u.User != nil {
			user := u.User.Username()
			password, _ := u.User.Password()
			host := u.Hostname()
			port := u.Port()
			if port == "" {
				port = "5432"
			}
			dbname := strings.TrimPrefix(u.Path, "/")
			if dbname == "" {
				dbname = "pgmanager"
			}
			return fmt.Sprintf("postgres://%s:%s@%s:%s/?sslmode=disable", user, password, host, port)
		}
	}

	secretPath := os.Getenv("SECRET_PATH")
	if secretPath == "" {
		secretPath = "/secrets/pgmanager-password"
	}

	password := ReadPassword(secretPath)

	host := os.Getenv("PGHOST")
	if host == "" {
		host = "localhost"
	}

	port := os.Getenv("PGPORT")
	if port == "" {
		port = "5432"
	}

	user := os.Getenv("PGUSER")
	if user == "" {
		user = "pgmanager"
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%s/?sslmode=disable", user, password, host, port)
}

func ReadPassword(path string) string {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("failed to open password file %s: %v", path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		log.Fatalf("failed to read password file %s: %v", path, err)
	}

	password := strings.TrimSpace(string(data))
	if password == "" {
		log.Fatalf("password file %s is empty", path)
	}

	return password
}

func ConnectDB(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	var pool *pgxpool.Pool
	var lastErr error
	for attempt := 1; attempt <= 30; attempt++ {
		pool, lastErr = pgxpool.New(ctx, databaseURL)
		if lastErr != nil {
			log.Printf("attempt %d/30: failed to connect: %v", attempt, lastErr)
			time.Sleep(2 * time.Second)
			continue
		}
		if pingErr := pool.Ping(ctx); pingErr != nil {
			pool.Close()
			lastErr = pingErr
			log.Printf("attempt %d/30: failed to ping: %v", attempt, pingErr)
			time.Sleep(2 * time.Second)
			continue
		}
		lastErr = nil
		break
	}
	return pool, lastErr
}
