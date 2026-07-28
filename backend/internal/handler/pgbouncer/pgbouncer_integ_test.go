//go:build integration

package pgbouncer

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"pgmanager/internal/auth"
	"pgmanager/internal/handler/testutil"
	"pgmanager/internal/handler/users"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	testPool    *pgxpool.Pool
	testBaseDSN string
)

func TestMain(m *testing.M) {
	pool, dsn := testutil.TestContainer(&testing.T{})
	testPool = pool
	testBaseDSN = dsn
	if err := users.InitUserSchema(context.Background(), pool); err != nil {
		panic("InitUserSchema: " + err.Error())
	}
	os.Exit(m.Run())
}

func cleanSlate(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	testPool.Exec(ctx, "DELETE FROM sessions")
	testPool.Exec(ctx, "DELETE FROM dev_databases")
	testPool.Exec(ctx, "DELETE FROM managed_users")
	testPool.Exec(ctx, "DELETE FROM audit_log")
	testPool.Exec(ctx, "DELETE FROM auth_users")
}

func createTestDB(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	testPool.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	_, err := testPool.Exec(ctx, "CREATE DATABASE "+name)
	if err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}
	testPool.Exec(ctx, "INSERT INTO pgbouncer_databases (database_name, allowed) VALUES ($1, true) ON CONFLICT (database_name) DO NOTHING", name)
	t.Cleanup(func() {
		testPool.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		testPool.Exec(ctx, "DELETE FROM pgbouncer_databases WHERE database_name = $1", name)
	})
}

func setupAdminCtx(t *testing.T) context.Context {
	t.Helper()
	cleanSlate(t)
	ctx := context.Background()
	hash, _ := auth.HashPassword("admin1234")
	var userID int
	testPool.QueryRow(ctx,
		"INSERT INTO auth_users (username, password_hash, role) VALUES ('admin', $1, 'admin') RETURNING id",
		hash,
	).Scan(&userID)
	return auth.WithUser(ctx, &auth.SessionUser{ID: userID, Username: "admin", Role: "admin"})
}

func TestListPgBouncerDatabases(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "pbgdb")

	h := New(testPool, testBaseDSN)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/pgbouncer/databases", nil).WithContext(adminCtx)
	h.ListPgBouncerDatabases(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var dbs []PgbouncerDatabase
	json.Unmarshal(w.Body.Bytes(), &dbs)

	found := false
	for _, db := range dbs {
		if db.DatabaseName == "pbgdb" {
			found = true
			if !db.Allowed {
				t.Error("expected pbgdb to be allowed")
			}
		}
	}
	if !found {
		t.Error("expected to find pbgdb in list")
	}
}

func TestTogglePgBouncerDatabase(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "pbgdb2")

	h := New(testPool, testBaseDSN)

	body := bytes.NewBufferString(`{"allowed":false}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/pgbouncer/databases/pbgdb2", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	h.TogglePgBouncerDatabase(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var allowed bool
	testPool.QueryRow(context.Background(), "SELECT allowed FROM pgbouncer_databases WHERE database_name = 'pbgdb2'").Scan(&allowed)
	if allowed {
		t.Error("expected allowed=false after toggle")
	}
}

func TestGetPgBouncerConfig(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	h := New(testPool, testBaseDSN)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/pgbouncer/config", nil).WithContext(adminCtx)
	h.GetPgBouncerConfig(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var config PgbouncerConfig
	json.Unmarshal(w.Body.Bytes(), &config)
	if config.PoolMode == "" {
		t.Error("expected non-empty pool mode")
	}
}

func TestUpdatePgBouncerConfig(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	h := New(testPool, testBaseDSN)

	body := bytes.NewBufferString(`{"poolMode":"session","defaultPoolSize":30,"maxClientConn":200}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/pgbouncer/config", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	h.UpdatePgBouncerConfig(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var poolMode string
	testPool.QueryRow(context.Background(), "SELECT value FROM system_config WHERE key = 'pgbouncer_pool_mode'").Scan(&poolMode)
	if poolMode != "session" {
		t.Errorf("expected pool_mode=session, got %q", poolMode)
	}

	var poolSize string
	testPool.QueryRow(context.Background(), "SELECT value FROM system_config WHERE key = 'pgbouncer_default_pool_size'").Scan(&poolSize)
	if poolSize != "30" {
		t.Errorf("expected pool_size=30, got %q", poolSize)
	}
}

func TestRebuildPgBouncerHBA(t *testing.T) {
	h := New(testPool, testBaseDSN)

	// RebuildPgBouncerHBA will try to write to /etc/pgbouncer/shared/
	// which may not exist in test env — that's OK, just verify it doesn't panic
	h.RebuildPgBouncerHBA()
}
