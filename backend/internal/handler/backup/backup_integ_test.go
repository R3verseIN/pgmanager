//go:build integration

package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

	// Set env vars so getPgCredentials() works for backup handlers
	// Parse host:port from testBaseDSN (format: postgres://user:pass@host:port/dbname?sslmode=disable)
	parsed := strings.TrimPrefix(testBaseDSN, "postgres://")
	parsed = strings.Split(parsed, "?")[0]
	atIdx := strings.LastIndex(parsed, "@")
	hostPortDB := parsed[atIdx+1:]
	parts := strings.SplitN(hostPortDB, "/", 2)
	hostPort := parts[0]
	host := strings.Split(hostPort, ":")[0]
	port := strings.Split(hostPort, ":")[1]
	userPass := parsed[:atIdx]
	username := strings.Split(userPass, ":")[0]
	password := strings.Split(userPass, ":")[1]

	os.Setenv("PGHOST", host)
	os.Setenv("PGPORT", port)
	os.Setenv("PGUSER", username)

	// Write password to temp file for secret path
	tmpFile, _ := os.CreateTemp("", "pgmanager-password-*")
	tmpFile.WriteString(password)
	tmpFile.Close()
	os.Setenv("SECRET_PATH", tmpFile.Name())
	defer os.Remove(tmpFile.Name())

	code := m.Run()
	os.Remove(tmpFile.Name())
	os.Exit(code)
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
	t.Cleanup(func() {
		testPool.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
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

func TestListBackupDatabases(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "backupdb1")
	createTestDB(t, "backupdb2")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backup/databases", nil).WithContext(adminCtx)
	ListBackupDatabases(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var dbs []BackupDatabaseEntry
	json.Unmarshal(w.Body.Bytes(), &dbs)

	found1, found2 := false, false
	for _, db := range dbs {
		if db.Name == "backupdb1" {
			found1 = true
		}
		if db.Name == "backupdb2" {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Error("expected backupdb1 and backupdb2 in list")
	}

	for _, db := range dbs {
		if core.ProtectedDatabases[db.Name] {
			t.Errorf("system database %s should not appear in backup list", db.Name)
		}
	}
}

func TestListBackupTables(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "bkptables"
	createTestDB(t, dbName)

	conn, _ := pgxpool.New(context.Background(), testBaseDSN+"&dbname="+dbName)
	conn.Exec(context.Background(), "CREATE TABLE users (id SERIAL PRIMARY KEY)")
	conn.Exec(context.Background(), "CREATE TABLE orders (id SERIAL PRIMARY KEY)")
	conn.Close()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backup/tables?db="+dbName, nil).WithContext(adminCtx)
	ListBackupTables(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp BackupTableListResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Database != dbName {
		t.Errorf("expected database=%s, got %q", dbName, resp.Database)
	}
	if len(resp.Tables) != 2 {
		t.Errorf("expected 2 tables, got %d", len(resp.Tables))
	}
}

func TestListBackupTables_NoDB(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backup/tables", nil).WithContext(adminCtx)
	ListBackupTables(testPool, testBaseDSN, w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStreamBackup_ProtectedDB(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	body := bytes.NewBufferString(`{"database":"postgres"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/backup/stream", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	StreamBackup(testPool, testBaseDSN, w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStreamBackup_EmptyDB(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	body := bytes.NewBufferString(`{"database":""}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/backup/stream", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	StreamBackup(testPool, testBaseDSN, w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStreamBackup_InvalidDBName(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	body := bytes.NewBufferString(`{"database":"123invalid"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/backup/stream", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	StreamBackup(testPool, testBaseDSN, w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuditLog_ListBackupDatabases(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backup/databases", nil).WithContext(adminCtx)
	ListBackupDatabases(testPool, testBaseDSN, w, req)

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM audit_log WHERE action = 'list_backup_databases'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 audit log entry, got %d", count)
	}
}
