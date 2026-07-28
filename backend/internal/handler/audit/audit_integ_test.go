//go:build integration

package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

func insertAuditEntry(t *testing.T, username, action, database string) {
	t.Helper()
	testPool.Exec(context.Background(),
		"INSERT INTO audit_log (username, action, database) VALUES ($1, $2, $3)",
		username, action, database)
}

func TestListLogs_Empty(t *testing.T) {
	cleanSlate(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit", nil)
	ListLogs(testPool, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp core.AuditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 0 {
		t.Errorf("expected 0 logs, got %d", resp.Total)
	}
}

func TestListLogs_WithEntries(t *testing.T) {
	cleanSlate(t)
	insertAuditEntry(t, "admin", "create_user", "testdb")
	insertAuditEntry(t, "admin", "delete_user", "testdb")
	insertAuditEntry(t, "viewer", "list_tables", "otherdb")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit", nil)
	ListLogs(testPool, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp core.AuditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 3 {
		t.Errorf("expected 3 logs, got %d", resp.Total)
	}
	if len(resp.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(resp.Entries))
	}
}

func TestListLogs_FilterByUsername(t *testing.T) {
	cleanSlate(t)
	insertAuditEntry(t, "admin", "create_user", "testdb")
	insertAuditEntry(t, "viewer", "list_tables", "testdb")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit?username=admin", nil)
	ListLogs(testPool, w, req)

	var resp core.AuditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Errorf("expected 1 log for admin, got %d", resp.Total)
	}
	if resp.Entries[0].Username != "admin" {
		t.Errorf("expected username=admin, got %q", resp.Entries[0].Username)
	}
}

func TestListLogs_FilterByAction(t *testing.T) {
	cleanSlate(t)
	insertAuditEntry(t, "admin", "create_user", "testdb")
	insertAuditEntry(t, "admin", "delete_user", "testdb")
	insertAuditEntry(t, "admin", "list_tables", "testdb")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit?action=create_user", nil)
	ListLogs(testPool, w, req)

	var resp core.AuditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Errorf("expected 1 log for create_user, got %d", resp.Total)
	}
}

func TestListLogs_FilterByDatabase(t *testing.T) {
	cleanSlate(t)
	insertAuditEntry(t, "admin", "create_user", "db1")
	insertAuditEntry(t, "admin", "create_user", "db2")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit?database=db1", nil)
	ListLogs(testPool, w, req)

	var resp core.AuditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Errorf("expected 1 log for db1, got %d", resp.Total)
	}
}

func TestListLogs_Pagination(t *testing.T) {
	cleanSlate(t)
	for i := 0; i < 10; i++ {
		insertAuditEntry(t, "admin", "action_"+string(rune('a'+i)), "testdb")
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit?limit=3&offset=0", nil)
	ListLogs(testPool, w, req)

	var resp core.AuditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 10 {
		t.Errorf("expected total=10, got %d", resp.Total)
	}
	if len(resp.Entries) != 3 {
		t.Errorf("expected 3 entries with limit=3, got %d", len(resp.Entries))
	}
}

func TestListLogs_LimitCap(t *testing.T) {
	cleanSlate(t)
	for i := 0; i < 5; i++ {
		insertAuditEntry(t, "admin", "action", "testdb")
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit?limit=5000", nil)
	ListLogs(testPool, w, req)

	var resp core.AuditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Entries) != 5 {
		t.Errorf("expected 5 entries (limit capped), got %d", len(resp.Entries))
	}
}

func TestListLogs_OrderDesc(t *testing.T) {
	cleanSlate(t)
	insertAuditEntry(t, "admin", "first", "testdb")
	insertAuditEntry(t, "admin", "second", "testdb")
	insertAuditEntry(t, "admin", "third", "testdb")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit", nil)
	ListLogs(testPool, w, req)

	var resp core.AuditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Entries) < 2 {
		t.Fatal("expected at least 2 entries")
	}
	if resp.Entries[0].Action != "third" {
		t.Errorf("expected first entry to be 'third', got %q", resp.Entries[0].Action)
	}
}

func TestListLogs_CombinedFilters(t *testing.T) {
	cleanSlate(t)
	insertAuditEntry(t, "admin", "create_user", "db1")
	insertAuditEntry(t, "admin", "delete_user", "db1")
	insertAuditEntry(t, "viewer", "create_user", "db1")
	insertAuditEntry(t, "admin", "create_user", "db2")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit?username=admin&action=create_user&database=db1", nil)
	ListLogs(testPool, w, req)

	var resp core.AuditLogResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Errorf("expected 1 log with combined filters, got %d", resp.Total)
	}
}

func TestWriteAuditLog(t *testing.T) {
	cleanSlate(t)

	core.WriteAuditLog(testPool, context.Background(), core.AuditEntry{
		Username: "testuser",
		Action:   "test_action",
		Database: "testdb",
		Detail:   map[string]interface{}{"key": "value"},
	})

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM audit_log WHERE username = 'testuser' AND action = 'test_action'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 audit log entry, got %d", count)
	}
}

func TestWriteAuditLog_WithDetail(t *testing.T) {
	cleanSlate(t)

	core.WriteAuditLog(testPool, context.Background(), core.AuditEntry{
		Username:  "admin",
		Action:    "create_table",
		Database:  "mydb",
		TableName: "users",
		Detail:    map[string]interface{}{"columns": 5},
		IPAddress: "127.0.0.1",
	})

	var detail []byte
	testPool.QueryRow(context.Background(), "SELECT detail FROM audit_log WHERE action = 'create_table'").Scan(&detail)
	if detail == nil {
		t.Error("expected non-nil detail")
	}

	var detailMap map[string]interface{}
	json.Unmarshal(detail, &detailMap)
	if detailMap["columns"] != float64(5) {
		t.Errorf("expected columns=5, got %v", detailMap["columns"])
	}
}

func TestListLogs_InvalidLimit(t *testing.T) {
	cleanSlate(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit?limit=abc", nil)
	ListLogs(testPool, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListLogs_NegativeOffset(t *testing.T) {
	cleanSlate(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit?offset=-5", nil)
	ListLogs(testPool, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
