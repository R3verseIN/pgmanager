package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"pgmanager/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

func tableTestPool(t *testing.T) *pgxpool.Pool {
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

func tableTestBaseDSN(t *testing.T) string {
	return "postgres://pgmanager:pgmanager@localhost:5433/?sslmode=disable"
}

func createTestDB(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	pool.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	_, err := pool.Exec(context.Background(), "CREATE DATABASE "+name)
	if err != nil {
		t.Fatalf("failed to create test db %s: %v", name, err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
}

func setupTableHandler(t *testing.T) (*Handler, *pgxpool.Pool, context.Context) {
	t.Helper()
	pool := tableTestPool(t)
	h := NewWithDSN(pool, tableTestBaseDSN(t))
	ctx := context.Background()
	h.InitUserSchema(ctx)
	return h, pool, ctx
}

func jsonBody(v interface{}) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

func adminCtx(ctx context.Context) context.Context {
	return auth.WithUser(ctx, &auth.SessionUser{ID: 1, Username: "admin", Role: "admin"})
}

func devCtx(ctx context.Context, id int, username string) context.Context {
	return auth.WithUser(ctx, &auth.SessionUser{ID: id, Username: username, Role: "dev"})
}

func viewerCtx(ctx context.Context) context.Context {
	return auth.WithUser(ctx, &auth.SessionUser{ID: 999, Username: "viewer1", Role: "viewer"})
}

func TestListTables_Basic(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testlistdb")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testlistdb")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT)")
	targetPool.Exec(ctx, "CREATE TABLE orders (id SERIAL PRIMARY KEY, amount INT)")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testlistdb/tables", nil).WithContext(adminCtx(ctx))
	h.ListTables(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tables []tableInfo
	json.Unmarshal(w.Body.Bytes(), &tables)
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d: %v", len(tables), tables)
	}
	if tables[0].Name != "orders" || tables[1].Name != "users" {
		t.Fatalf("unexpected table names: %v", tables)
	}
}

func TestListTables_DevAccessAllowed(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testdevlist")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testdevlist")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY)")

	pool.Exec(ctx, "INSERT INTO auth_users (username, password_hash, role) VALUES ('devlist1', 'hash', 'dev')")
	var devID int
	pool.QueryRow(ctx, "SELECT id FROM auth_users WHERE username = 'devlist1'").Scan(&devID)
	pool.Exec(ctx, "INSERT INTO dev_databases (auth_user_id, database_name) VALUES ($1, 'testdevlist')", devID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testdevlist/tables", nil).WithContext(devCtx(ctx, devID, "devlist1"))
	h.ListTables(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListTables_DevBlockedFromUnassignedDB(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testdevblock")

	pool.Exec(ctx, "INSERT INTO auth_users (username, password_hash, role) VALUES ('devblock1', 'hash', 'dev')")
	var devID int
	pool.QueryRow(ctx, "SELECT id FROM auth_users WHERE username = 'devblock1'").Scan(&devID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testdevblock/tables", nil).WithContext(devCtx(ctx, devID, "devblock1"))
	h.ListTables(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListTables_ViewerAllowed(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testviewblock")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testviewblock/tables", nil).WithContext(viewerCtx(ctx))
	h.ListTables(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 (viewer read access), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetColumns_Basic(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testcoldb")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testcoldb")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE testtable (id SERIAL PRIMARY KEY, name TEXT NOT NULL, email TEXT)")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testcoldb/columns/testtable", nil).WithContext(adminCtx(ctx))
	h.GetColumns(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var cols []columnInfo
	json.Unmarshal(w.Body.Bytes(), &cols)
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d: %v", len(cols), cols)
	}
	if !cols[0].IsPrimaryKey {
		t.Fatal("expected id to be primary key")
	}
	if cols[1].Nullable {
		t.Fatal("expected name to be NOT NULL")
	}
	if !cols[2].Nullable {
		t.Fatal("expected email to be nullable")
	}
}

func TestGetColumns_DevBlocked(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testcolblock")

	pool.Exec(ctx, "INSERT INTO auth_users (username, password_hash, role) VALUES ('colblock1', 'hash', 'dev')")
	var devID int
	pool.QueryRow(ctx, "SELECT id FROM auth_users WHERE username = 'colblock1'").Scan(&devID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testcolblock/columns/anything", nil).WithContext(devCtx(ctx, devID, "colblock1"))
	h.GetColumns(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListData_Basic(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testdatadb")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testdatadb")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT)")
	for i := 0; i < 5; i++ {
		targetPool.Exec(ctx, "INSERT INTO items (name) VALUES ($1)", "item")
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testdatadb/data/items?limit=3&offset=0", nil).WithContext(adminCtx(ctx))
	h.ListData(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result dataResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Total != 5 {
		t.Fatalf("expected total 5, got %d", result.Total)
	}
	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result.Rows))
	}
	if len(result.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(result.Columns))
	}
}

func TestListData_WithSort(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testsortdb")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testsortdb")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT)")
	targetPool.Exec(ctx, "INSERT INTO items (name) VALUES ('banana'), ('apple'), ('cherry')")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testsortdb/data/items?sort=name&order=asc", nil).WithContext(adminCtx(ctx))
	h.ListData(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result dataResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result.Rows))
	}
	// Check sorted: apple, banana, cherry
	if result.Rows[0][1] != "apple" {
		t.Fatalf("expected apple first, got %v", result.Rows[0][1])
	}
}

func TestListData_InvalidSortColumn(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testsortinv")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testsortinv")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY)")

	// Invalid sort column should just not sort, not error
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testsortinv/data/items?sort=nonexistent&order=asc", nil).WithContext(adminCtx(ctx))
	h.ListData(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListData_EmptyTable(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testemptydb")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testemptydb")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE empty_table (id SERIAL PRIMARY KEY)")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testemptydb/data/empty_table", nil).WithContext(adminCtx(ctx))
	h.ListData(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result dataResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Total != 0 {
		t.Fatalf("expected total 0, got %d", result.Total)
	}
	if len(result.Rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(result.Rows))
	}
}

func TestListData_NonexistentTable(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testnotabledb")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testnotabledb/data/nonexistent", nil).WithContext(adminCtx(ctx))
	h.ListData(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInsertRow_Basic(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testinsertdb")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testinsertdb")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT, value INT)")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"values": map[string]interface{}{"name": "test", "value": 42}})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testinsertdb/data/items", body).WithContext(adminCtx(ctx))
	h.InsertRow(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Verify row exists
	var name string
	var value int
	err = targetPool.QueryRow(ctx, "SELECT name, value FROM items WHERE id = 1").Scan(&name, &value)
	if err != nil {
		t.Fatalf("row not found: %v", err)
	}
	if name != "test" || value != 42 {
		t.Fatalf("unexpected values: %s, %d", name, value)
	}
}

func TestInsertRow_EmptyValues(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testinsertempty")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testinsertempty")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY)")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"values": map[string]interface{}{}})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testinsertempty/data/items", body).WithContext(adminCtx(ctx))
	h.InsertRow(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInsertRow_DevAccess(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testinsertdev")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testinsertdev")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT)")

	pool.Exec(ctx, "INSERT INTO auth_users (username, password_hash, role) VALUES ('devinsert', 'hash', 'dev')")
	var devID int
	pool.QueryRow(ctx, "SELECT id FROM auth_users WHERE username = 'devinsert'").Scan(&devID)
	pool.Exec(ctx, "INSERT INTO dev_databases (auth_user_id, database_name) VALUES ($1, 'testinsertdev')", devID)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"values": map[string]interface{}{"name": "fromdev"}})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testinsertdev/data/items", body).WithContext(devCtx(ctx, devID, "devinsert"))
	h.InsertRow(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInsertRow_DevBlockedFromUnassignedDB(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testinsertblock")

	pool.Exec(ctx, "INSERT INTO auth_users (username, password_hash, role) VALUES ('devinsertblock', 'hash', 'dev')")
	var devID int
	pool.QueryRow(ctx, "SELECT id FROM auth_users WHERE username = 'devinsertblock'").Scan(&devID)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"values": map[string]interface{}{"name": "bad"}})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testinsertblock/data/items", body).WithContext(devCtx(ctx, devID, "devinsertblock"))
	h.InsertRow(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInsertRow_ViewerBlocked(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testinsertview")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"values": map[string]interface{}{"name": "bad"}})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testinsertview/data/items", body).WithContext(viewerCtx(ctx))
	h.InsertRow(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateRow_Basic(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testupdatedb")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testupdatedb")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT)")
	targetPool.Exec(ctx, "INSERT INTO items (name) VALUES ('old_name')")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"values": map[string]interface{}{"name": "new_name"},
		"where":  []map[string]interface{}{{"column": "id", "operator": "=", "value": 1}},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/databases/testupdatedb/data/items", body).WithContext(adminCtx(ctx))
	h.UpdateRow(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var name string
	targetPool.QueryRow(ctx, "SELECT name FROM items WHERE id = 1").Scan(&name)
	if name != "new_name" {
		t.Fatalf("expected new_name, got %s", name)
	}
}

func TestUpdateRow_NoWhereClause(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testupdate_nowhere")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testupdate_nowhere")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT)")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"values": map[string]interface{}{"name": "new_name"},
		"where":  []map[string]interface{}{},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/databases/testupdate_nowhere/data/items", body).WithContext(adminCtx(ctx))
	h.UpdateRow(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateRow_NoValues(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testupdate_noval")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testupdate_noval")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT)")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"values": map[string]interface{}{},
		"where":  []map[string]interface{}{{"column": "id", "operator": "=", "value": 1}},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/databases/testupdate_noval/data/items", body).WithContext(adminCtx(ctx))
	h.UpdateRow(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateRow_WhereIsNull(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testupdate_isnull")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testupdate_isnull")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT)")
	targetPool.Exec(ctx, "INSERT INTO items (name) VALUES (NULL), ('has_name')")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"values": map[string]interface{}{"name": "was_null"},
		"where":  []map[string]interface{}{{"column": "name", "operator": "IS NULL"}},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/databases/testupdate_isnull/data/items", body).WithContext(adminCtx(ctx))
	h.UpdateRow(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	targetPool.QueryRow(ctx, "SELECT COUNT(*) FROM items WHERE name = 'was_null'").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 row updated, got %d", count)
	}
}

func TestDeleteRow_Basic(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testdeletedb")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testdeletedb")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT)")
	targetPool.Exec(ctx, "INSERT INTO items (name) VALUES ('to_delete'), ('to_keep')")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"where": []map[string]interface{}{{"column": "id", "operator": "=", "value": 1}},
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/databases/testdeletedb/data/items", body).WithContext(adminCtx(ctx))
	h.DeleteRow(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	targetPool.QueryRow(ctx, "SELECT COUNT(*) FROM items").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 row remaining, got %d", count)
	}
}

func TestDeleteRow_NoWhereClause(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testdelete_nowhere")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testdelete_nowhere")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY)")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"where": []map[string]interface{}{},
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/databases/testdelete_nowhere/data/items", body).WithContext(adminCtx(ctx))
	h.DeleteRow(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTable_Basic(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testcreatetable")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"name": "new_table",
		"columns": []map[string]interface{}{
			{"name": "id", "type": "SERIAL", "nullable": false, "isPrimaryKey": true},
			{"name": "name", "type": "TEXT", "nullable": false},
			{"name": "email", "type": "TEXT", "nullable": true},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testcreatetable/tables", body).WithContext(adminCtx(ctx))
	h.CreateTable(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Verify table exists
	targetPool, cleanup, err := h.connectToDatabase(ctx, "testcreatetable")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()

	var exists bool
	targetPool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'new_table')").Scan(&exists)
	if !exists {
		t.Fatal("table was not created")
	}
}

func TestCreateTable_DuplicateTable(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testdupetable")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testdupetable")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE existing (id SERIAL PRIMARY KEY)")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"name": "existing",
		"columns": []map[string]interface{}{
			{"name": "id", "type": "SERIAL", "nullable": false, "isPrimaryKey": true},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testdupetable/tables", body).WithContext(adminCtx(ctx))
	h.CreateTable(w, req)

	if w.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTable_NoColumns(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testnocols")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"name":    "empty_table",
		"columns": []map[string]interface{}{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testnocols/tables", body).WithContext(adminCtx(ctx))
	h.CreateTable(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTable_EmptyName(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testnoname")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"name": "",
		"columns": []map[string]interface{}{
			{"name": "id", "type": "SERIAL", "nullable": false, "isPrimaryKey": true},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testnoname/tables", body).WithContext(adminCtx(ctx))
	h.CreateTable(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTable_DevAccess(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testcreatedev")

	pool.Exec(ctx, "INSERT INTO auth_users (username, password_hash, role) VALUES ('devcreate', 'hash', 'dev')")
	var devID int
	pool.QueryRow(ctx, "SELECT id FROM auth_users WHERE username = 'devcreate'").Scan(&devID)
	pool.Exec(ctx, "INSERT INTO dev_databases (auth_user_id, database_name) VALUES ($1, 'testcreatedev')", devID)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"name": "dev_table",
		"columns": []map[string]interface{}{
			{"name": "id", "type": "SERIAL", "nullable": false, "isPrimaryKey": true},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testcreatedev/tables", body).WithContext(devCtx(ctx, devID, "devcreate"))
	h.CreateTable(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTable_ViewerBlocked(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testcreateview")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"name": "view_table",
		"columns": []map[string]interface{}{
			{"name": "id", "type": "SERIAL", "nullable": false, "isPrimaryKey": true},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testcreateview/tables", body).WithContext(viewerCtx(ctx))
	h.CreateTable(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAddColumn_Basic(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testaddcoldb")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testaddcoldb")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY)")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"name":     "age",
		"type":     "INT",
		"nullable": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testaddcoldb/tables/items/columns", body).WithContext(adminCtx(ctx))
	h.AddColumn(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Verify column exists
	var exists bool
	targetPool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'items' AND column_name = 'age')").Scan(&exists)
	if !exists {
		t.Fatal("column was not added")
	}
}

func TestAddColumn_DevBlocked(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testaddcolblock")

	pool.Exec(ctx, "INSERT INTO auth_users (username, password_hash, role) VALUES ('addcolblock', 'hash', 'dev')")
	var devID int
	pool.QueryRow(ctx, "SELECT id FROM auth_users WHERE username = 'addcolblock'").Scan(&devID)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"name":     "col",
		"type":     "TEXT",
		"nullable": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testaddcolblock/tables/items/columns", body).WithContext(devCtx(ctx, devID, "addcolblock"))
	h.AddColumn(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDropColumn_Basic(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testdropcoldb")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testdropcoldb")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT, temp TEXT)")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/databases/testdropcoldb/tables/items/columns/temp", nil).WithContext(adminCtx(ctx))
	h.DropColumn(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var exists bool
	targetPool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'items' AND column_name = 'temp')").Scan(&exists)
	if exists {
		t.Fatal("column was not dropped")
	}
}

func TestDropColumn_DevBlocked(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testdropcolblock")

	pool.Exec(ctx, "INSERT INTO auth_users (username, password_hash, role) VALUES ('dropcolblock', 'hash', 'dev')")
	var devID int
	pool.QueryRow(ctx, "SELECT id FROM auth_users WHERE username = 'dropcolblock'").Scan(&devID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/databases/testdropcolblock/tables/items/columns/col", nil).WithContext(devCtx(ctx, devID, "dropcolblock"))
	h.DropColumn(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteQuery_Select(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testquerydb")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testquerydb")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT)")
	targetPool.Exec(ctx, "INSERT INTO items (name) VALUES ('alpha'), ('beta')")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": "SELECT * FROM items ORDER BY id"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testquerydb/query", body).WithContext(adminCtx(ctx))
	h.ExecuteQuery(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result queryResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.RowCount != 2 {
		t.Fatalf("expected 2 rows, got %d", result.RowCount)
	}
	if len(result.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(result.Columns))
	}
	if result.Duration <= 0 {
		t.Fatal("expected positive duration")
	}
}

func TestExecuteQuery_Insert(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testqueryins")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testqueryins")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT)")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": "INSERT INTO items (name) VALUES ('hello')"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testqueryins/query", body).WithContext(adminCtx(ctx))
	h.ExecuteQuery(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result queryResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.RowCount != 1 {
		t.Fatalf("expected 1 row affected, got %d", result.RowCount)
	}
}

func TestExecuteQuery_BlockedDropDatabase(t *testing.T) {
	h, _, ctx := setupTableHandler(t)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": "DROP DATABASE some_db"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/anydb/query", body).WithContext(adminCtx(ctx))
	h.ExecuteQuery(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteQuery_BlockedTruncate(t *testing.T) {
	h, _, ctx := setupTableHandler(t)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": "TRUNCATE TABLE users"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/anydb/query", body).WithContext(adminCtx(ctx))
	h.ExecuteQuery(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteQuery_BlockedGrant(t *testing.T) {
	h, _, ctx := setupTableHandler(t)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": "GRANT ALL ON DATABASE test TO public"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/anydb/query", body).WithContext(adminCtx(ctx))
	h.ExecuteQuery(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteQuery_BlockedRevoke(t *testing.T) {
	h, _, ctx := setupTableHandler(t)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": "REVOKE ALL ON DATABASE test FROM public"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/anydb/query", body).WithContext(adminCtx(ctx))
	h.ExecuteQuery(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteQuery_BlockedCreateRole(t *testing.T) {
	h, _, ctx := setupTableHandler(t)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": "CREATE ROLE evil_user"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/anydb/query", body).WithContext(adminCtx(ctx))
	h.ExecuteQuery(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteQuery_BlockedAlterRole(t *testing.T) {
	h, _, ctx := setupTableHandler(t)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": "ALTER ROLE admin WITH SUPERUSER"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/anydb/query", body).WithContext(adminCtx(ctx))
	h.ExecuteQuery(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteQuery_BlockedDropOwnedBy(t *testing.T) {
	h, _, ctx := setupTableHandler(t)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": "DROP OWNED BY evil_user CASCADE"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/anydb/query", body).WithContext(adminCtx(ctx))
	h.ExecuteQuery(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteQuery_EmptySQL(t *testing.T) {
	h, _, ctx := setupTableHandler(t)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/anydb/query", body).WithContext(adminCtx(ctx))
	h.ExecuteQuery(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteQuery_ViewerBlocked(t *testing.T) {
	h, _, ctx := setupTableHandler(t)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": "SELECT 1"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/anydb/query", body).WithContext(viewerCtx(ctx))
	h.ExecuteQuery(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteQuery_DevAccess(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testquerydev")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testquerydev")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT)")

	pool.Exec(ctx, "INSERT INTO auth_users (username, password_hash, role) VALUES ('devquery', 'hash', 'dev')")
	var devID int
	pool.QueryRow(ctx, "SELECT id FROM auth_users WHERE username = 'devquery'").Scan(&devID)
	pool.Exec(ctx, "INSERT INTO dev_databases (auth_user_id, database_name) VALUES ($1, 'testquerydev')", devID)

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": "SELECT * FROM items"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testquerydev/query", body).WithContext(devCtx(ctx, devID, "devquery"))
	h.ExecuteQuery(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteQuery_SQLSyntaxError(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testqueryerr")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": "SELCT * FORM nonexistent"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testqueryerr/query", body).WithContext(adminCtx(ctx))
	h.ExecuteQuery(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result queryResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Error == "" {
		t.Fatal("expected error in result")
	}
}

func TestBuildWhereClauses_Equal(t *testing.T) {
	clauses, args, err := buildWhereClauses([]whereCondition{
		{Column: "id", Operator: "=", Value: float64(42)},
	}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clauses) != 1 || clauses[0] != `"id" = $1` {
		t.Fatalf("unexpected clause: %v", clauses)
	}
	if len(args) != 1 || args[0] != float64(42) {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestBuildWhereClauses_Like(t *testing.T) {
	clauses, _, err := buildWhereClauses([]whereCondition{
		{Column: "name", Operator: "LIKE", Value: "%test%"},
	}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clauses) != 1 || clauses[0] != `"name" LIKE $1` {
		t.Fatalf("unexpected clause: %v", clauses)
	}
}

func TestBuildWhereClauses_IsNull(t *testing.T) {
	clauses, _, err := buildWhereClauses([]whereCondition{
		{Column: "email", Operator: "IS NULL"},
	}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clauses) != 1 || clauses[0] != `"email" IS NULL` {
		t.Fatalf("unexpected clause: %v", clauses)
	}
}

func TestBuildWhereClauses_MultipleConditions(t *testing.T) {
	clauses, args, err := buildWhereClauses([]whereCondition{
		{Column: "id", Operator: "=", Value: float64(1)},
		{Column: "name", Operator: "!=", Value: "deleted"},
		{Column: "age", Operator: ">=", Value: float64(18)},
	}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clauses) != 3 {
		t.Fatalf("expected 3 clauses, got %d", len(clauses))
	}
	if clauses[0] != `"id" = $1` || clauses[1] != `"name" != $2` || clauses[2] != `"age" >= $3` {
		t.Fatalf("unexpected clauses: %v", clauses)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(args))
	}
}

func TestBuildWhereClauses_EmptyColumn(t *testing.T) {
	_, _, err := buildWhereClauses([]whereCondition{
		{Column: "", Operator: "=", Value: 1},
	}, 1)
	if err == nil {
		t.Fatal("expected error for empty column")
	}
}

func TestBuildWhereClauses_InvalidColumn(t *testing.T) {
	_, _, err := buildWhereClauses([]whereCondition{
		{Column: "id; DROP TABLE", Operator: "=", Value: 1},
	}, 1)
	if err == nil {
		t.Fatal("expected error for invalid column")
	}
}

func TestBuildWhereClauses_UnsupportedOperator(t *testing.T) {
	_, _, err := buildWhereClauses([]whereCondition{
		{Column: "id", Operator: "BETWEEN", Value: 1},
	}, 1)
	if err == nil {
		t.Fatal("expected error for unsupported operator")
	}
}

func TestAuditLog_Created(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testauditdb")

	// Clean slate
	pool.Exec(ctx, "DELETE FROM audit_log WHERE database = 'testauditdb'")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testauditdb")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY)")

	// Perform an action that triggers audit log
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testauditdb/tables", nil).WithContext(adminCtx(ctx))
	h.ListTables(w, req)

	// Check audit log was written
	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_log WHERE action = 'list_tables' AND database = 'testauditdb'").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 audit log entry, got %d", count)
	}

	// Verify fields
	var username, action, database string
	pool.QueryRow(ctx, "SELECT username, action, database FROM audit_log WHERE action = 'list_tables' AND database = 'testauditdb'").Scan(&username, &action, &database)
	if username != "admin" || action != "list_tables" || database != "testauditdb" {
		t.Fatalf("unexpected audit fields: %s, %s, %s", username, action, database)
	}
}

func TestAuditLog_RawQuery(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testauditraw")

	// Clean slate
	pool.Exec(ctx, "DELETE FROM audit_log WHERE database = 'testauditraw'")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testauditraw")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE items (id SERIAL PRIMARY KEY)")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"sql": "SELECT * FROM items"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases/testauditraw/query", body).WithContext(adminCtx(ctx))
	h.ExecuteQuery(w, req)

	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_log WHERE action = 'raw_query' AND database = 'testauditraw'").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 audit log entry for raw_query, got %d", count)
	}
}

func TestListLogs_Basic(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)

	// Create some audit entries
	createTestDB(t, pool, "testlogsdb")
	targetPool, cleanup, err := h.connectToDatabase(ctx, "testlogsdb")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE t (id SERIAL PRIMARY KEY)")

	// Trigger some audit events
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testlogsdb/tables", nil).WithContext(adminCtx(ctx))
	h.ListTables(w, req)

	// List logs
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/logs", nil).WithContext(adminCtx(ctx))
	h.ListLogs(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp auditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total == 0 {
		t.Fatal("expected at least 1 log entry")
	}
	if len(resp.Entries) == 0 {
		t.Fatal("expected at least 1 entry in response")
	}
}

func TestListLogs_FilterByUsername(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testlogfilter")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testlogfilter")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE t (id SERIAL PRIMARY KEY)")

	// Trigger audit as admin
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testlogfilter/tables", nil).WithContext(adminCtx(ctx))
	h.ListTables(w, req)

	// Filter by admin
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/logs?username=admin", nil).WithContext(adminCtx(ctx))
	h.ListLogs(w, req)

	var resp auditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total == 0 {
		t.Fatal("expected at least 1 log entry for admin")
	}

	// Filter by nonexistent user
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/logs?username=nobody", nil).WithContext(adminCtx(ctx))
	h.ListLogs(w, req)

	var resp2 auditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp2)
	if resp2.Total != 0 {
		t.Fatalf("expected 0 entries for nobody, got %d", resp2.Total)
	}
}

func TestListLogs_FilterByAction(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testlogaction")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testlogaction")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE t (id SERIAL PRIMARY KEY)")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/databases/testlogaction/tables", nil).WithContext(adminCtx(ctx))
	h.ListTables(w, req)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/logs?action=list_tables", nil).WithContext(adminCtx(ctx))
	h.ListLogs(w, req)

	var resp auditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total == 0 {
		t.Fatal("expected at least 1 log entry for list_tables action")
	}
}

func TestListLogs_Pagination(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testlogpage")

	targetPool, cleanup, err := h.connectToDatabase(ctx, "testlogpage")
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer cleanup()
	targetPool.Exec(ctx, "CREATE TABLE t (id SERIAL PRIMARY KEY)")

	// Trigger multiple audit events
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/databases/testlogpage/tables", nil).WithContext(adminCtx(ctx))
		h.ListTables(w, req)
	}

	// Get first page
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/logs?limit=2&offset=0", nil).WithContext(adminCtx(ctx))
	h.ListLogs(w, req)

	var resp auditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Entries) != 2 {
		t.Fatalf("expected 2 entries on first page, got %d", len(resp.Entries))
	}
	if resp.Total < 5 {
		t.Fatalf("expected total >= 5, got %d", resp.Total)
	}
}

func TestAuditLog_CreateDatabase(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)

	pool.Exec(ctx, "DELETE FROM audit_log WHERE action = 'create_database' AND database = 'testauditcreatdb'")

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{"name": "testauditcreatdb"})
	req := httptest.NewRequest(http.MethodPost, "/api/databases", body).WithContext(adminCtx(ctx))
	h.CreateDatabase(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_log WHERE action = 'create_database' AND database = 'testauditcreatdb'").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 audit log entry for create_database, got %d", count)
	}

	var username string
	pool.QueryRow(ctx, "SELECT username FROM audit_log WHERE action = 'create_database' AND database = 'testauditcreatdb'").Scan(&username)
	if username != "admin" {
		t.Fatalf("expected username 'admin', got %q", username)
	}

	t.Cleanup(func() {
		pool.Exec(ctx, "DROP DATABASE IF EXISTS testauditcreatdb WITH (FORCE)")
	})
}

func TestAuditLog_DeleteDatabase(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testauditdeldb")

	pool.Exec(ctx, "DELETE FROM audit_log WHERE action = 'delete_database'")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/databases/testauditdeldb", nil).WithContext(adminCtx(ctx))
	h.DeleteDatabase(w, req)

	if w.Code != 204 {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_log WHERE action = 'delete_database' AND database = 'testauditdeldb'").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 audit log entry for delete_database, got %d", count)
	}
}

func TestAuditLog_CreateUser(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testauditcreateuserdb")

	pool.Exec(ctx, "DELETE FROM audit_log WHERE action = 'create_user'")
	pool.Exec(ctx, "DROP OWNED BY testauditcu CASCADE")
	pool.Exec(ctx, "DROP ROLE IF EXISTS testauditcu")
	pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testauditcu'")
	t.Cleanup(func() {
		pool.Exec(ctx, "DROP OWNED BY testauditcu CASCADE")
		pool.Exec(ctx, "DROP ROLE IF EXISTS testauditcu")
		pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testauditcu'")
	})

	w := httptest.NewRecorder()
	body := jsonBody(map[string]interface{}{
		"username":  "testauditcu",
		"databases": []string{"testauditcreateuserdb"},
		"access":    "read",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", body).WithContext(adminCtx(ctx))
	h.CreateUser(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_log WHERE action = 'create_user'").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 audit log entry for create_user, got %d", count)
	}
}

func TestAuditLog_DeleteUser(t *testing.T) {
	h, pool, ctx := setupTableHandler(t)
	createTestDB(t, pool, "testauditdeluserdb")

	pool.Exec(ctx, "DELETE FROM audit_log WHERE action = 'delete_user'")
	pool.Exec(ctx, "DROP OWNED BY testauditdu CASCADE")
	pool.Exec(ctx, "DROP ROLE IF EXISTS testauditdu")
	pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testauditdu'")

	_, err := pool.Exec(ctx, "CREATE ROLE testauditdu WITH LOGIN PASSWORD 'testpass123'")
	if err != nil {
		t.Fatalf("failed to create role: %v", err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO managed_users (username, database_name, access) VALUES ('testauditdu', 'testauditdeluserdb', 'read')")
	if err != nil {
		t.Fatalf("failed to insert managed_user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DROP OWNED BY testauditdu CASCADE")
		pool.Exec(ctx, "DROP ROLE IF EXISTS testauditdu")
		pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testauditdu'")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/users/testauditdu", nil).WithContext(adminCtx(ctx))
	h.DeleteUser(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_log WHERE action = 'delete_user'").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 audit log entry for delete_user, got %d", count)
	}
}
