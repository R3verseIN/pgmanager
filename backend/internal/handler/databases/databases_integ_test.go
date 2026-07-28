//go:build integration

package databases

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

func setupAdminCtx(t *testing.T) context.Context {
	t.Helper()
	cleanSlate(t)
	ctx := context.Background()
	hash, _ := auth.HashPassword("admin1234")
	var userID int
	err := testPool.QueryRow(ctx,
		"INSERT INTO auth_users (username, password_hash, role) VALUES ('admin', $1, 'admin') RETURNING id",
		hash,
	).Scan(&userID)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	return auth.WithUser(ctx, &auth.SessionUser{ID: userID, Username: "admin", Role: "admin"})
}

func TestListDatabases(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/?showSystem=true", nil).WithContext(adminCtx)
	ListDatabases(testPool, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var dbs []database
	json.Unmarshal(w.Body.Bytes(), &dbs)

	found := false
	for _, db := range dbs {
		if db.Name == "pgmanager" {
			found = true
			if !db.Protected {
				t.Error("pgmanager should be protected")
			}
		}
	}
	if !found {
		t.Error("expected to find pgmanager database")
	}
}

func TestListDatabases_HideSystem(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/", nil).WithContext(adminCtx)
	ListDatabases(testPool, w, req)

	var dbs []database
	json.Unmarshal(w.Body.Bytes(), &dbs)

	for _, db := range dbs {
		if core.ProtectedDatabases[db.Name] {
			t.Errorf("system database %s should be hidden by default", db.Name)
		}
	}
}

func TestListDatabases_ShowSystem(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/?showSystem=true", nil).WithContext(adminCtx)
	ListDatabases(testPool, w, req)

	var dbs []database
	json.Unmarshal(w.Body.Bytes(), &dbs)

	found := false
	for _, db := range dbs {
		if db.Name == "pgmanager" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find pgmanager with showSystem=true")
	}
}

func TestCreateDatabase_Success(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	body := bytes.NewBufferString(`{"name":"newdb"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateDatabase(testPool, nil, w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var exists bool
	testPool.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = 'newdb')").Scan(&exists)
	if !exists {
		t.Error("expected database newdb to exist")
	}

	testPool.Exec(context.Background(), "DROP DATABASE IF EXISTS newdb WITH (FORCE)")
}

func TestCreateDatabase_Protected(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	body := bytes.NewBufferString(`{"name":"postgres"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateDatabase(testPool, nil, w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateDatabase_Duplicate(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	body := bytes.NewBufferString(`{"name":"dupdb"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateDatabase(testPool, nil, w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/databases/", bytes.NewBufferString(`{"name":"dupdb"}`)).WithContext(adminCtx)
	req2.Header.Set("Content-Type", "application/json")
	CreateDatabase(testPool, nil, w2, req2)

	if w2.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w2.Code, w2.Body.String())
	}

	testPool.Exec(context.Background(), "DROP DATABASE IF EXISTS dupdb WITH (FORCE)")
}

func TestDeleteDatabase_Success(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	testPool.Exec(context.Background(), "CREATE DATABASE delme")
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DROP DATABASE IF EXISTS delme WITH (FORCE)")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/databases/delme", nil).WithContext(adminCtx)
	DeleteDatabase(testPool, nil, w, req)

	if w.Code != 204 {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	var exists bool
	testPool.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = 'delme')").Scan(&exists)
	if exists {
		t.Error("expected database delme to be deleted")
	}
}

func TestDeleteDatabase_Protected(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/databases/postgres", nil).WithContext(adminCtx)
	DeleteDatabase(testPool, nil, w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteDatabase_Nonexistent(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/databases/nodb", nil).WithContext(adminCtx)
	DeleteDatabase(testPool, nil, w, req)

	if w.Code != 500 {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateDatabase_AuditLog(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	body := bytes.NewBufferString(`{"name":"auditdb"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateDatabase(testPool, nil, w, req)

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM audit_log WHERE action = 'create_database'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 audit log entry, got %d", count)
	}

	testPool.Exec(context.Background(), "DROP DATABASE IF EXISTS auditdb WITH (FORCE)")
}

func TestDeleteDatabase_AuditLog(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	testPool.Exec(context.Background(), "CREATE DATABASE adeldb")
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DROP DATABASE IF EXISTS adeldb WITH (FORCE)")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/databases/adeldb", nil).WithContext(adminCtx)
	DeleteDatabase(testPool, nil, w, req)

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM audit_log WHERE action = 'delete_database'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 audit log entry, got %d", count)
	}
}
