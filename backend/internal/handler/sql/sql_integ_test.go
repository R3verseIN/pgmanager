//go:build integration

package sql

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"pgmanager/internal/auth"
	"pgmanager/internal/handler/core"
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

func TestExecuteQuery_Select(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "sqldb"
	createTestDB(t, dbName)

	conn, _ := pgxpool.New(context.Background(), testBaseDSN+"&dbname="+dbName)
	conn.Exec(context.Background(), "CREATE TABLE test_t (id SERIAL PRIMARY KEY, name TEXT)")
	conn.Exec(context.Background(), "INSERT INTO test_t (name) VALUES ('hello'), ('world')")
	conn.Close()

	body := bytes.NewBufferString(`{"sql":"SELECT * FROM test_t ORDER BY id"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/"+dbName+"/query", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	ExecuteQuery(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result core.QueryResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.RowCount != 2 {
		t.Errorf("expected 2 rows, got %d", result.RowCount)
	}
	if len(result.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(result.Columns))
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestExecuteQuery_Insert(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "insqdb"
	createTestDB(t, dbName)

	conn, _ := pgxpool.New(context.Background(), testBaseDSN+"&dbname="+dbName)
	conn.Exec(context.Background(), "CREATE TABLE test_t (id SERIAL PRIMARY KEY, name TEXT)")
	conn.Close()

	body := bytes.NewBufferString(`{"sql":"INSERT INTO test_t (name) VALUES ('new')"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/"+dbName+"/query", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	ExecuteQuery(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	conn2, _ := pgxpool.New(context.Background(), testBaseDSN+"&dbname="+dbName)
	defer conn2.Close()
	var count int
	conn2.QueryRow(context.Background(), "SELECT COUNT(*) FROM test_t").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}
}

func TestExecuteQuery_Blocked(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "blksql"
	createTestDB(t, dbName)

	body := bytes.NewBufferString(`{"sql":"DROP DATABASE postgres"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/"+dbName+"/query", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	ExecuteQuery(testPool, testBaseDSN, w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteQuery_InvalidSQL(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "invsql"
	createTestDB(t, dbName)

	body := bytes.NewBufferString(`{"sql":"SELCT * FROM nonexistent"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/"+dbName+"/query", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	ExecuteQuery(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 (error in result), got %d: %s", w.Code, w.Body.String())
	}

	var result core.QueryResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Error == "" {
		t.Error("expected error in query result")
	}
}

func TestExecuteQuery_EmptySQL(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "empsql"
	createTestDB(t, dbName)

	body := bytes.NewBufferString(`{"sql":""}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/"+dbName+"/query", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	ExecuteQuery(testPool, testBaseDSN, w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteQuery_CreateTable(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "crtsql"
	createTestDB(t, dbName)

	body := bytes.NewBufferString(`{"sql":"CREATE TABLE newtable (id SERIAL PRIMARY KEY, data TEXT)"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/"+dbName+"/query", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	ExecuteQuery(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	conn, _ := pgxpool.New(context.Background(), testBaseDSN+"&dbname="+dbName)
	defer conn.Close()
	var exists bool
	conn.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='newtable')").Scan(&exists)
	if !exists {
		t.Error("expected newtable to exist")
	}
}

func TestAuditLog_ExecuteQuery(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "sqlaudit"
	createTestDB(t, dbName)

	body := bytes.NewBufferString(`{"sql":"SELECT 1"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/"+dbName+"/query", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	ExecuteQuery(testPool, testBaseDSN, w, req)

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM audit_log WHERE action = 'raw_query'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 audit log entry, got %d", count)
	}
}
