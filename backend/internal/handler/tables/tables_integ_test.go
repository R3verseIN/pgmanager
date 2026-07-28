//go:build integration

package tables

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

func createTestTable(t *testing.T, dbName, tableName string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgxpool.New(ctx, testBaseDSN+"&dbname="+dbName)
	if err != nil {
		t.Fatalf("connect to %s: %v", dbName, err)
	}
	defer conn.Close()
	_, err = conn.Exec(ctx, "CREATE TABLE "+tableName+" (id SERIAL PRIMARY KEY, name TEXT NOT NULL, value INTEGER DEFAULT 0)")
	if err != nil {
		t.Fatalf("create table %s: %v", tableName, err)
	}
}

func insertTestData(t *testing.T, dbName, tableName string, count int) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgxpool.New(ctx, testBaseDSN+"&dbname="+dbName)
	if err != nil {
		t.Fatalf("connect to %s: %v", dbName, err)
	}
	defer conn.Close()
	for i := 0; i < count; i++ {
		conn.Exec(ctx, "INSERT INTO "+tableName+" (name, value) VALUES ($1, $2)", "item"+string(rune('A'+i)), i*10)
	}
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

func setupDevCtx(t *testing.T, dbName string) context.Context {
	t.Helper()
	ctx := context.Background()
	hash, _ := auth.HashPassword("devpass1234")
	var userID int
	testPool.QueryRow(ctx,
		"INSERT INTO auth_users (username, password_hash, role) VALUES ('devuser', $1, 'dev') RETURNING id",
		hash,
	).Scan(&userID)
	testPool.Exec(ctx, "INSERT INTO dev_databases (auth_user_id, database_name) VALUES ($1, $2)", userID, dbName)
	return auth.WithUser(ctx, &auth.SessionUser{ID: userID, Username: "devuser", Role: "dev"})
}

func TestListTables(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "tablesdb"
	createTestDB(t, dbName)
	createTestTable(t, dbName, "users")
	createTestTable(t, dbName, "orders")
	insertTestData(t, dbName, "users", 5)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/"+dbName+"/tables", nil).WithContext(adminCtx)
	ListTables(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tables []core.TableInfo
	json.Unmarshal(w.Body.Bytes(), &tables)
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}

	names := map[string]bool{}
	for _, tb := range tables {
		names[tb.Name] = true
	}
	if !names["users"] || !names["orders"] {
		t.Error("expected users and orders tables")
	}
}

func TestListTables_Empty(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "emptydb"
	createTestDB(t, dbName)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/"+dbName+"/tables", nil).WithContext(adminCtx)
	ListTables(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tables []core.TableInfo
	json.Unmarshal(w.Body.Bytes(), &tables)
	if len(tables) != 0 {
		t.Errorf("expected 0 tables, got %d", len(tables))
	}
}

func TestGetColumns(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "coldb"
	createTestDB(t, dbName)
	createTestTable(t, dbName, "items")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/"+dbName+"/columns/items", nil).WithContext(adminCtx)
	GetColumns(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var cols []core.ColumnInfo
	json.Unmarshal(w.Body.Bytes(), &cols)
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}

	colNames := map[string]bool{}
	for _, c := range cols {
		colNames[c.Name] = true
	}
	if !colNames["id"] || !colNames["name"] || !colNames["value"] {
		t.Error("expected id, name, value columns")
	}
}

func TestCreateTable(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "creatdb"
	createTestDB(t, dbName)

	body := bytes.NewBufferString(`{"name":"newtable","columns":[{"name":"id","type":"SERIAL","isPrimaryKey":true},{"name":"title","type":"TEXT","nullable":false}]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/"+dbName+"/tables", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateTable(testPool, testBaseDSN, w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var exists bool
	conn, _ := pgxpool.New(context.Background(), testBaseDSN+"&dbname="+dbName)
	defer conn.Close()
	conn.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='newtable')").Scan(&exists)
	if !exists {
		t.Error("expected table newtable to exist")
	}
}

func TestCreateTable_Duplicate(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "dupcoldb"
	createTestDB(t, dbName)
	createTestTable(t, dbName, "existing")

	body := bytes.NewBufferString(`{"name":"existing","columns":[{"name":"id","type":"SERIAL"}]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/"+dbName+"/tables", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateTable(testPool, testBaseDSN, w, req)

	if w.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAddColumn(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "addcoldb"
	createTestDB(t, dbName)
	createTestTable(t, dbName, "items")

	body := bytes.NewBufferString(`{"name":"description","type":"TEXT","nullable":true}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/"+dbName+"/tables/items/columns", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	AddColumn(testPool, testBaseDSN, w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	conn, _ := pgxpool.New(context.Background(), testBaseDSN+"&dbname="+dbName)
	defer conn.Close()
	var exists bool
	conn.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='items' AND column_name='description')").Scan(&exists)
	if !exists {
		t.Error("expected description column to exist")
	}
}

func TestDropColumn(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "dropcoldb"
	createTestDB(t, dbName)
	createTestTable(t, dbName, "items")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/databases/"+dbName+"/tables/items/columns/value", nil).WithContext(adminCtx)
	DropColumn(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	conn, _ := pgxpool.New(context.Background(), testBaseDSN+"&dbname="+dbName)
	defer conn.Close()
	var exists bool
	conn.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='items' AND column_name='value')").Scan(&exists)
	if exists {
		t.Error("expected value column to be dropped")
	}
}

func TestDropColumn_PrimaryKey(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "pkcoldb"
	createTestDB(t, dbName)
	createTestTable(t, dbName, "items")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/databases/"+dbName+"/tables/items/columns/id", nil).WithContext(adminCtx)
	DropColumn(testPool, testBaseDSN, w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDevAccess(t *testing.T) {
	dbName := "devdb"
	createTestDB(t, dbName)
	createTestTable(t, dbName, "items")
	devCtx := setupDevCtx(t, dbName)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/"+dbName+"/tables", nil).WithContext(devCtx)
	ListTables(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 for dev, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDevAccess_Denied(t *testing.T) {
	dbName := "nodevdb"
	createTestDB(t, dbName)
	createTestTable(t, dbName, "items")

	ctx := context.Background()
	hash, _ := auth.HashPassword("devpass1234")
	var userID int
	testPool.QueryRow(ctx,
		"INSERT INTO auth_users (username, password_hash, role) VALUES ('nodev', $1, 'dev') RETURNING id",
		hash,
	).Scan(&userID)
	devCtx := auth.WithUser(ctx, &auth.SessionUser{ID: userID, Username: "nodev", Role: "dev"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/"+dbName+"/tables", nil).WithContext(devCtx)
	ListTables(testPool, testBaseDSN, w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuditLog_ListTables(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "taudit"
	createTestDB(t, dbName)
	createTestTable(t, dbName, "items")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/"+dbName+"/tables", nil).WithContext(adminCtx)
	ListTables(testPool, testBaseDSN, w, req)

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM audit_log WHERE action = 'list_tables'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 audit log entry, got %d", count)
	}
}

func TestAuditLog_CreateTable(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "caudit"
	createTestDB(t, dbName)

	body := bytes.NewBufferString(`{"name":"newtable","columns":[{"name":"id","type":"SERIAL","isPrimaryKey":true}]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/"+dbName+"/tables", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateTable(testPool, testBaseDSN, w, req)

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM audit_log WHERE action = 'create_table'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 audit log entry, got %d", count)
	}
}
