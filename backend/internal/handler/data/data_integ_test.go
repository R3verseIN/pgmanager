//go:build integration

package data

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

func createTestTableWithData(t *testing.T, dbName string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgxpool.New(ctx, testBaseDSN+"&dbname="+dbName)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()
	conn.Exec(ctx, `CREATE TABLE items (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		value INTEGER DEFAULT 0,
		active BOOLEAN DEFAULT true
	)`)
	for i := 0; i < 5; i++ {
		conn.Exec(ctx, "INSERT INTO items (name, value, active) VALUES ($1, $2, $3)",
			"item"+string(rune('A'+i)), i*10, i%2 == 0)
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

func TestListData(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "listdb"
	createTestDB(t, dbName)
	createTestTableWithData(t, dbName)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/"+dbName+"/data/items", nil).WithContext(adminCtx)
	ListData(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result core.DataResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Total != 5 {
		t.Errorf("expected 5 rows, got %d", result.Total)
	}
	if len(result.Columns) != 4 {
		t.Errorf("expected 4 columns, got %d", len(result.Columns))
	}
}

func TestListData_Pagination(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "pagdb"
	createTestDB(t, dbName)
	createTestTableWithData(t, dbName)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/"+dbName+"/data/items?limit=2&offset=0", nil).WithContext(adminCtx)
	ListData(testPool, testBaseDSN, w, req)

	var result core.DataResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows with limit=2, got %d", len(result.Rows))
	}
	if result.Total != 5 {
		t.Errorf("expected total=5, got %d", result.Total)
	}
}

func TestListData_Sort(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "sortdb"
	createTestDB(t, dbName)
	createTestTableWithData(t, dbName)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/"+dbName+"/data/items?sort=value&order=DESC", nil).WithContext(adminCtx)
	ListData(testPool, testBaseDSN, w, req)

	var result core.DataResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Rows) < 2 {
		t.Fatal("expected at least 2 rows")
	}
	firstVal := result.Rows[0][2].(float64)
	secondVal := result.Rows[1][2].(float64)
	if firstVal < secondVal {
		t.Errorf("expected descending order: %f < %f", firstVal, secondVal)
	}
}



func TestInsertRow(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "insdb"
	createTestDB(t, dbName)
	createTestTableWithData(t, dbName)

	body := bytes.NewBufferString(`{"values":{"name":"newitem","value":99,"active":false}}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/"+dbName+"/data/items", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	InsertRow(testPool, testBaseDSN, w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	conn, _ := pgxpool.New(context.Background(), testBaseDSN+"&dbname="+dbName)
	defer conn.Close()
	var count int
	conn.QueryRow(context.Background(), "SELECT COUNT(*) FROM items WHERE name = 'newitem'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 newitem row, got %d", count)
	}
}

func TestUpdateRow(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "upddb"
	createTestDB(t, dbName)
	createTestTableWithData(t, dbName)

	body := bytes.NewBufferString(`{"values":{"value":999},"where":[{"column":"name","operator":"=","value":"itemA"}]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/databases/"+dbName+"/data/items", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	UpdateRow(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	conn, _ := pgxpool.New(context.Background(), testBaseDSN+"&dbname="+dbName)
	defer conn.Close()
	var value int
	conn.QueryRow(context.Background(), "SELECT value FROM items WHERE name = 'itemA'").Scan(&value)
	if value != 999 {
		t.Errorf("expected value=999, got %d", value)
	}
}

func TestDeleteRow(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "deldb2"
	createTestDB(t, dbName)
	createTestTableWithData(t, dbName)

	body := bytes.NewBufferString(`{"where":[{"column":"name","operator":"=","value":"itemA"}]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/databases/"+dbName+"/data/items", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	DeleteRow(testPool, testBaseDSN, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	conn, _ := pgxpool.New(context.Background(), testBaseDSN+"&dbname="+dbName)
	defer conn.Close()
	var count int
	conn.QueryRow(context.Background(), "SELECT COUNT(*) FROM items WHERE name = 'itemA'").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 itemA rows after delete, got %d", count)
	}
}

func TestAuditLog_ListData(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "daudit"
	createTestDB(t, dbName)
	createTestTableWithData(t, dbName)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/"+dbName+"/data/items", nil).WithContext(adminCtx)
	ListData(testPool, testBaseDSN, w, req)

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM audit_log WHERE action = 'view_data'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 audit log entry, got %d", count)
	}
}
