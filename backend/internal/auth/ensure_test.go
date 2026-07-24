package auth

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func getTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	host := os.Getenv("PGHOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("PGPORT")
	if port == "" {
		port = "5433"
	}
	user := os.Getenv("PGUSER")
	if user == "" {
		user = "pgmanager"
	}
	dbname := os.Getenv("PGDATABASE")
	if dbname == "" {
		dbname = "pgmanager"
	}

	url := "postgres://" + user + ":" + user + "@" + host + ":" + port + "/" + dbname + "?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	return pool
}

func TestEnsurePgbouncerAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := getTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	// First run: should create user and function
	if err := EnsurePgbouncerAuth(ctx, pool); err != nil {
		t.Fatalf("first run failed: %v", err)
	}

	// Second run: should detect healthy state
	if err := EnsurePgbouncerAuth(ctx, pool); err != nil {
		t.Fatalf("second run failed: %v", err)
	}
}

func TestEnsurePgbouncerAuth_RestoresAfterDrop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := getTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	// Ensure it exists first
	if err := EnsurePgbouncerAuth(ctx, pool); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}

	// Drop the function
	_, err := pool.Exec(ctx, "DROP FUNCTION IF EXISTS pgbouncer_get_user(TEXT)")
	if err != nil {
		t.Fatalf("failed to drop function: %v", err)
	}

	// Should restore
	if err := EnsurePgbouncerAuth(ctx, pool); err != nil {
		t.Fatalf("restore after drop failed: %v", err)
	}

	// Verify function works
	var exists bool
	err = pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'pgbouncer_get_user')").Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check function: %v", err)
	}
	if !exists {
		t.Fatal("function not restored")
	}
}
