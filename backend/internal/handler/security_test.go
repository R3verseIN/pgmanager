package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestPgBouncer_BlocksPgmanagerUser(t *testing.T) {
	dsn := os.Getenv("PGBOUNCER_URL")
	if dsn == "" {
		dsn = "postgres://pgmanager:pgmanager@localhost:5432/postgres?sslmode=disable"
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err == nil {
		conn.Close(ctx)
		t.Fatal("expected connection to fail for pgmanager user through PgBouncer")
	}
}

func TestPgBouncer_AllowsCreatedUser(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// cleanup
	pool.Exec(ctx, "DROP OWNED BY testpguser CASCADE")
	pool.Exec(ctx, "DROP ROLE IF EXISTS testpguser")
	pool.Exec(ctx, "DELETE FROM pgmanager.managed_users WHERE username = 'testpguser'")
	t.Cleanup(func() {
		pool.Exec(ctx, "DROP OWNED BY testpguser CASCADE")
		pool.Exec(ctx, "DROP ROLE IF EXISTS testpguser")
		pool.Exec(ctx, "DELETE FROM pgmanager.managed_users WHERE username = 'testpguser'")
	})

	// create test user
	_, err := pool.Exec(ctx, "CREATE ROLE testpguser WITH LOGIN PASSWORD 'testpass123'")
	if err != nil {
		t.Fatalf("failed to create role: %v", err)
	}

	// connect through PgBouncer
	pgbouncerURL := os.Getenv("PGBOUNCER_URL")
	if pgbouncerURL == "" {
		pgbouncerURL = "postgres://testpguser:testpass123@localhost:5432/postgres?sslmode=disable"
	}

	conn, err := pgx.Connect(ctx, pgbouncerURL)
	if err != nil {
		t.Fatalf("expected connection to succeed for created user through PgBouncer: %v", err)
	}
	defer conn.Close(ctx)

	var result int
	err = conn.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		t.Fatalf("expected query to succeed: %v", err)
	}
	if result != 1 {
		t.Fatalf("expected 1, got %d", result)
	}
}

func TestBackend_DatabaseConnection(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	var result int
	err := pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		t.Fatalf("expected query to succeed: %v", err)
	}
	if result != 1 {
		t.Fatalf("expected 1, got %d", result)
	}
}

func TestBackend_CreateAndDeleteUser(t *testing.T) {
	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()

	h.InitUserSchema(ctx)

	// cleanup
	pool.Exec(ctx, "DROP OWNED BY testsecurityuser CASCADE")
	pool.Exec(ctx, "DROP ROLE IF EXISTS testsecurityuser")
	pool.Exec(ctx, "DELETE FROM pgmanager.managed_users WHERE username = 'testsecurityuser'")
	pool.Exec(ctx, "DROP DATABASE IF EXISTS testsecuritydb")
	pool.Exec(ctx, "CREATE DATABASE testsecuritydb")
	t.Cleanup(func() {
		pool.Exec(ctx, "DROP OWNED BY testsecurityuser CASCADE")
		pool.Exec(ctx, "DROP ROLE IF EXISTS testsecurityuser")
		pool.Exec(ctx, "DELETE FROM pgmanager.managed_users WHERE username = 'testsecurityuser'")
		pool.Exec(ctx, "DROP DATABASE testsecuritydb WITH (FORCE)")
	})

	// create user via API
	body := `{"username":"testsecurityuser","databases":["testsecuritydb"],"access":"read"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	h.CreateUser(w, req)

	if w.Code != 201 {
		t.Fatalf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var result createUserResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if result.Username != "testsecurityuser" {
		t.Fatalf("expected username testsecurityuser, got %s", result.Username)
	}
	if result.Password == "" {
		t.Fatal("password should not be empty")
	}
	if len(result.Databases) != 1 || result.Databases[0] != "testsecuritydb" {
		t.Fatalf("expected databases [testsecuritydb], got %v", result.Databases)
	}

	// delete user via API
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/users/testsecurityuser", nil).WithContext(ctx)
	req.SetPathValue("name", "testsecurityuser")
	h.DeleteUser(w, req)

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}
