package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://pgmanager:pgmanager@localhost:5433/pgmanager?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestListUsers_Grouping(t *testing.T) {
	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()

	h.InitUserSchema(ctx)

	// cleanup
	pool.Exec(ctx, "DROP OWNED BY testgroupuser CASCADE")
	pool.Exec(ctx, "DROP ROLE IF EXISTS testgroupuser")
	pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testgroupuser'")
	pool.Exec(ctx, "DROP DATABASE IF EXISTS testgroupdb1")
	pool.Exec(ctx, "DROP DATABASE IF EXISTS testgroupdb2")
	pool.Exec(ctx, "CREATE DATABASE testgroupdb1")
	pool.Exec(ctx, "CREATE DATABASE testgroupdb2")
	t.Cleanup(func() {
		pool.Exec(ctx, "DROP OWNED BY testgroupuser CASCADE")
		pool.Exec(ctx, "DROP ROLE IF EXISTS testgroupuser")
		pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testgroupuser'")
		pool.Exec(ctx, "DROP DATABASE testgroupdb1 WITH (FORCE)")
		pool.Exec(ctx, "DROP DATABASE testgroupdb2 WITH (FORCE)")
	})

	// create role
	_, err := pool.Exec(ctx, "CREATE ROLE testgroupuser WITH LOGIN PASSWORD 'testpass123'")
	if err != nil {
		t.Fatalf("failed to create role: %v", err)
	}

	// insert two managed_users rows for same user, same access
	_, err = pool.Exec(ctx, "INSERT INTO managed_users (username, database_name, access) VALUES ('testgroupuser', 'testgroupdb1', 'read')")
	if err != nil {
		t.Fatalf("insert 1: %v", err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO managed_users (username, database_name, access) VALUES ('testgroupuser', 'testgroupdb2', 'read')")
	if err != nil {
		t.Fatalf("insert 2: %v", err)
	}

	// list users via httptest
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil).WithContext(ctx)
	h.ListUsers(w, req)

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var users []userRecord
	if err := json.Unmarshal(w.Body.Bytes(), &users); err != nil {
		t.Fatalf("failed to parse response: %v\nbody: %s", err, w.Body.String())
	}

	var found *userRecord
	for i := range users {
		if users[i].Username == "testgroupuser" {
			found = &users[i]
			break
		}
	}

	if found == nil {
		t.Fatalf("testgroupuser not found in response: %s", w.Body.String())
	}

	if len(found.Databases) != 2 {
		t.Fatalf("expected 2 databases, got %d: %v", len(found.Databases), found.Databases)
	}

	dbMap := map[string]bool{}
	for _, db := range found.Databases {
		dbMap[db] = true
	}
	if !dbMap["testgroupdb1"] || !dbMap["testgroupdb2"] {
		t.Fatalf("expected both testgroupdb1 and testgroupdb2, got: %v", found.Databases)
	}
}
