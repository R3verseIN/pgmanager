package core

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"pgmanager/internal/auth"
)

func TestQuoteIdent(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"users", `"users"`},
		{"my_table", `"my_table"`},
		{"table; DROP", `"table; DROP"`},
		{"", `""`},
	}
	for _, tt := range tests {
		got := QuoteIdent(tt.input)
		if got != tt.expected {
			t.Errorf("QuoteIdent(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestQuoteLiteral(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "'hello'"},
		{"it's", "'it''s'"},
		{"", "''"},
	}
	for _, tt := range tests {
		got := QuoteLiteral(tt.input)
		if got != tt.expected {
			t.Errorf("QuoteLiteral(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestValidName(t *testing.T) {
	good := []string{"users", "my_table", "t1", "schema1_table1"}
	for _, s := range good {
		if !ValidName.MatchString(s) {
			t.Errorf("ValidName(%q) = false, want true", s)
		}
	}
	bad := []string{"", "123table", "table name", "table;DROP", "a.b"}
	for _, s := range bad {
		if ValidName.MatchString(s) {
			t.Errorf("ValidName(%q) = true, want false", s)
		}
	}
}

func TestProtectedDatabases(t *testing.T) {
	protected := []string{"postgres", "template0", "template1", "pgmanager"}
	for _, db := range protected {
		if !ProtectedDatabases[db] {
			t.Errorf("expected %q to be protected", db)
		}
	}
	if ProtectedDatabases["mydb"] {
		t.Error("expected mydb to not be protected")
	}
}

func TestIsBlockedSQL(t *testing.T) {
	blocked := []string{
		"DROP DATABASE mydb",
		"DROP OWNED BY role1",
		"ALTER ROLE admin WITH SUPERUSER",
		"CREATE ROLE admin WITH LOGIN",
		"DROP ROLE admin",
		"GRANT ALL ON users TO public",
		"REVOKE ALL ON users FROM public",
		"TRUNCATE TABLE logs",
		"COMMENT ON DATABASE mydb IS 'test'",
	}
	for _, q := range blocked {
		if !IsBlockedSQL(q) {
			t.Errorf("IsBlockedSQL(%q) = false, want true", q)
		}
	}
	allowed := []string{
		"SELECT * FROM users",
		"INSERT INTO logs (msg) VALUES ('test')",
		"UPDATE users SET name='bob' WHERE id=1",
		"DELETE FROM logs WHERE id=1",
		"SELECT 1",
	}
	for _, q := range allowed {
		if IsBlockedSQL(q) {
			t.Errorf("IsBlockedSQL(%q) = true, want false", q)
		}
	}
}

func TestGeneratePassword(t *testing.T) {
	p1 := GeneratePassword(32)
	p2 := GeneratePassword(32)
	if len(p1) != 32 {
		t.Errorf("GeneratePassword(32) length = %d, want 32", len(p1))
	}
	if p1 == p2 {
		t.Error("GeneratePassword() should produce unique passwords")
	}
	short := GeneratePassword(8)
	if len(short) != 8 {
		t.Errorf("GeneratePassword(8) length = %d, want 8", len(short))
	}
}

func TestValidPassword(t *testing.T) {
	good := []string{"password", "Password1", "abcdefg1", "Str0ngPass", "ALLUPPERCASE", "alllowercase", "12345678"}
	for _, p := range good {
		if !ValidPassword(p) {
			t.Errorf("ValidPassword(%q) = false, want true", p)
		}
	}
	bad := []string{"", "short1", "has space", "has@special", "has.dot"}
	for _, p := range bad {
		if ValidPassword(p) {
			t.Errorf("ValidPassword(%q) = true, want false", p)
		}
	}
}

func TestBuildWhereClauses(t *testing.T) {
	conditions := []WhereCondition{
		{Column: "name", Operator: "=", Value: "test"},
		{Column: "age", Operator: ">", Value: "25"},
	}
	clauses, args, err := BuildWhereClauses(conditions, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clauses) != 2 {
		t.Errorf("expected 2 clauses, got %d", len(clauses))
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}

	_, _, err = BuildWhereClauses(nil, 1)
	if err == nil {
		t.Error("expected error for nil conditions")
	}

	_, _, err = BuildWhereClauses([]WhereCondition{{}}, 1)
	if err == nil {
		t.Error("expected error for empty column")
	}

	_, _, err = BuildWhereClauses([]WhereCondition{{Column: "x", Operator: "BETWEEN", Value: "1"}}, 1)
	if err == nil {
		t.Error("expected error for unsupported operator")
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, 200, map[string]string{"hello": "world"})

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["hello"] != "world" {
		t.Errorf("expected hello=world, got %q", result["hello"])
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, 404, "not found")

	if w.Code != 404 {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var result ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.Error != "not found" {
		t.Errorf("expected error 'not found', got %q", result.Error)
	}
}

func TestWriteJSON_NilValue(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, 204, nil)

	if w.Code != 204 {
		t.Errorf("expected status 204, got %d", w.Code)
	}
}

func TestClientIP_DirectConnection(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	ip := ClientIP(req)
	if ip != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %q", ip)
	}
}

func TestClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	ip := ClientIP(req)
	if ip != "203.0.113.50" {
		t.Errorf("expected 203.0.113.50, got %q", ip)
	}
}

func TestClientIP_CFConnectingIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("CF-Connecting-IP", "198.51.100.1")
	ip := ClientIP(req)
	if ip != "198.51.100.1" {
		t.Errorf("expected 198.51.100.1, got %q", ip)
	}
}

func TestClientIP_TrueClientIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("True-Client-IP", "198.51.100.2")
	ip := ClientIP(req)
	if ip != "198.51.100.2" {
		t.Errorf("expected 198.51.100.2, got %q", ip)
	}
}

func TestClientIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("X-Real-IP", "198.51.100.3")
	ip := ClientIP(req)
	if ip != "198.51.100.3" {
		t.Errorf("expected 198.51.100.3, got %q", ip)
	}
}

func TestClientIP_PublicRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "8.8.8.8:12345"
	ip := ClientIP(req)
	if ip != "8.8.8.8" {
		t.Errorf("expected 8.8.8.8, got %q", ip)
	}
}

func TestClientIP_NoPortInRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100"
	ip := ClientIP(req)
	if ip != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %q", ip)
	}
}

func TestCheckWriteAccess_Admin(t *testing.T) {
	user := &auth.SessionUser{Role: "admin"}
	if err := CheckWriteAccess(context.Background(), user); err != nil {
		t.Errorf("admin should have write access: %v", err)
	}
}

func TestCheckWriteAccess_Dev(t *testing.T) {
	user := &auth.SessionUser{Role: "dev"}
	if err := CheckWriteAccess(context.Background(), user); err != nil {
		t.Errorf("dev should have write access: %v", err)
	}
}

func TestCheckWriteAccess_Viewer(t *testing.T) {
	user := &auth.SessionUser{Role: "viewer"}
	if err := CheckWriteAccess(context.Background(), user); err == nil {
		t.Error("viewer should not have write access")
	}
}

func TestCheckWriteAccess_Nil(t *testing.T) {
	if err := CheckWriteAccess(context.Background(), nil); err == nil {
		t.Error("nil user should not have write access")
	}
}

func TestCheckWriteAccess_UnknownRole(t *testing.T) {
	user := &auth.SessionUser{Role: "unknown"}
	if err := CheckWriteAccess(context.Background(), user); err == nil {
		t.Error("unknown role should not have write access")
	}
}

func TestNotifyDatabaseChange_CallsCallback(t *testing.T) {
	called := make(chan bool, 1)
	h := &Handler{
		OnDatabaseChange: func() {
			called <- true
		},
	}
	h.NotifyDatabaseChange()

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Error("OnDatabaseChange was not called")
	}
}

func TestNotifyDatabaseChange_NilCallback(t *testing.T) {
	h := &Handler{OnDatabaseChange: nil}
	h.NotifyDatabaseChange()
}

func TestNotifyDatabaseChange_Concurrent(t *testing.T) {
	var mu sync.Mutex
	var count int
	h := &Handler{
		OnDatabaseChange: func() {
			mu.Lock()
			count++
			mu.Unlock()
		},
	}
	for i := 0; i < 10; i++ {
		h.NotifyDatabaseChange()
	}
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if count != 10 {
		t.Errorf("expected 10 calls, got %d", count)
	}
}
