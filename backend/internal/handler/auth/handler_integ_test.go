//go:build integration

package auth

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
	_, _ = testPool.Exec(ctx, "DELETE FROM sessions")
	_, _ = testPool.Exec(ctx, "DELETE FROM dev_databases")
	_, _ = testPool.Exec(ctx, "DELETE FROM audit_log")
	_, _ = testPool.Exec(ctx, "DELETE FROM auth_users")
	_, _ = testPool.Exec(ctx, "DELETE FROM system_config")
	_, _ = testPool.Exec(ctx, "DELETE FROM pgbouncer_databases WHERE database_name NOT IN ('pgmanager','postgres','template0','template1')")
}

func newHandler(t *testing.T) *Handler {
	t.Helper()
	cleanSlate(t)
	return New(testPool)
}

func setupAdmin(t *testing.T, h *Handler) context.Context {
	t.Helper()
	cleanSlate(t)
	body := bytes.NewBufferString(`{"username":"admin","password":"admin1234"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", body)
	req.Header.Set("Content-Type", "application/json")
	h.Setup(w, req)
	if w.Code != 201 {
		t.Fatalf("setup admin failed: %d: %s", w.Code, w.Body.String())
	}

	var userID int
	err := testPool.QueryRow(context.Background(), "SELECT id FROM auth_users WHERE username = 'admin'").Scan(&userID)
	if err != nil {
		t.Fatalf("get admin id: %v", err)
	}
	token, err := auth.CreateSession(context.Background(), testPool, userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	_ = token
	return auth.WithUser(context.Background(), &auth.SessionUser{ID: userID, Username: "admin", Role: "admin"})
}

func TestSetupCheck_Empty(t *testing.T) {
	h := newHandler(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/setup-check", nil)
	h.SetupCheck(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]bool
	json.Unmarshal(w.Body.Bytes(), &result)
	if !result["needsSetup"] {
		t.Error("expected needsSetup=true")
	}
}

func TestSetupCheck_AfterSetup(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/setup-check", nil).WithContext(adminCtx)
	h.SetupCheck(w, req)

	var result map[string]bool
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["needsSetup"] {
		t.Error("expected needsSetup=false after setup")
	}
}

func TestSetup_Success(t *testing.T) {
	h := newHandler(t)
	cleanSlate(t)

	body := bytes.NewBufferString(`{"username":"admin","password":"admin1234"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", body)
	req.Header.Set("Content-Type", "application/json")
	h.Setup(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM auth_users WHERE username = 'admin'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 admin user, got %d", count)
	}
}

func TestSetup_DuplicateAdmin(t *testing.T) {
	h := newHandler(t)
	setupAdmin(t, h)

	body := bytes.NewBufferString(`{"username":"admin2","password":"admin1234"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", body).WithContext(context.Background())
	req.Header.Set("Content-Type", "application/json")
	h.Setup(w, req)

	if w.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetup_ShortUsername(t *testing.T) {
	h := newHandler(t)
	cleanSlate(t)

	body := bytes.NewBufferString(`{"username":"ab","password":"admin1234"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", body)
	req.Header.Set("Content-Type", "application/json")
	h.Setup(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetup_ShortPassword(t *testing.T) {
	h := newHandler(t)
	cleanSlate(t)

	body := bytes.NewBufferString(`{"username":"admin","password":"short"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", body)
	req.Header.Set("Content-Type", "application/json")
	h.Setup(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestLogin_Success(t *testing.T) {
	h := newHandler(t)
	setupAdmin(t, h)

	body := bytes.NewBufferString(`{"username":"admin","password":"admin1234"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	h.Login(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "session_id" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected session_id cookie")
	}
}

func TestLogin_BadPassword(t *testing.T) {
	h := newHandler(t)
	setupAdmin(t, h)

	body := bytes.NewBufferString(`{"username":"admin","password":"wrongpassword"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	h.Login(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLogin_NonexistentUser(t *testing.T) {
	h := newHandler(t)
	setupAdmin(t, h)

	body := bytes.NewBufferString(`{"username":"nobody","password":"password1234"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	h.Login(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetMe(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil).WithContext(adminCtx)
	h.GetMe(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["username"] != "admin" {
		t.Errorf("expected username=admin, got %q", result["username"])
	}
	if result["role"] != "admin" {
		t.Errorf("expected role=admin, got %q", result["role"])
	}
}

func TestGetMe_Unauthorized(t *testing.T) {
	h := newHandler(t)
	setupAdmin(t, h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	h.GetMe(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestChangePassword_Success(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	body := bytes.NewBufferString(`{"current_password":"admin1234","new_password":"newpass1234"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/auth/password", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	h.ChangePassword(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	loginBody := bytes.NewBufferString(`{"username":"admin","password":"newpass1234"}`)
	loginW := httptest.NewRecorder()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
	loginReq.Header.Set("Content-Type", "application/json")
	h.Login(loginW, loginReq)
	if loginW.Code != 200 {
		t.Error("new password should work")
	}
}

func TestChangePassword_WrongCurrent(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	body := bytes.NewBufferString(`{"current_password":"wrongpassword","new_password":"newpass1234"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/auth/password", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	h.ChangePassword(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChangePassword_Unauthorized(t *testing.T) {
	h := newHandler(t)
	setupAdmin(t, h)

	body := bytes.NewBufferString(`{"current_password":"admin1234","new_password":"newpass1234"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/auth/password", body)
	req.Header.Set("Content-Type", "application/json")
	h.ChangePassword(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestCreateAuthUser_Success(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	body := bytes.NewBufferString(`{"username":"viewer1","password":"viewer1234","role":"viewer"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	h.CreateAuthUser(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM auth_users WHERE username = 'viewer1'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 viewer1 user, got %d", count)
	}
}

func TestCreateAuthUser_DuplicateUsername(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	body := bytes.NewBufferString(`{"username":"viewer1","password":"viewer1234","role":"viewer"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	h.CreateAuthUser(w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/users", bytes.NewBufferString(`{"username":"viewer1","password":"viewer1234","role":"viewer"}`)).WithContext(adminCtx)
	req2.Header.Set("Content-Type", "application/json")
	h.CreateAuthUser(w2, req2)

	if w2.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestCreateAuthUser_InvalidRole(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	body := bytes.NewBufferString(`{"username":"user1","password":"user12345","role":"superadmin"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	h.CreateAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListAuthUsers(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	body := bytes.NewBufferString(`{"username":"viewer1","password":"viewer1234","role":"viewer"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	h.CreateAuthUser(w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/auth/users", nil).WithContext(adminCtx)
	h.ListAuthUsers(w2, req2)

	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var userList []map[string]any
	json.Unmarshal(w2.Body.Bytes(), &userList)
	if len(userList) != 2 {
		t.Fatalf("expected 2 users, got %d", len(userList))
	}
}

func TestDeleteAuthUser_Success(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	body := bytes.NewBufferString(`{"username":"viewer1","password":"viewer1234","role":"viewer"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	h.CreateAuthUser(w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodDelete, "/api/auth/users/viewer1", nil).WithContext(adminCtx)
	h.DeleteAuthUser(w2, req2)

	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM auth_users WHERE username = 'viewer1'").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 viewer1 users after delete, got %d", count)
	}
}

func TestDeleteLastAdmin(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/users/admin", nil).WithContext(adminCtx)
	h.DeleteAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateAuthUser_Role(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	body := bytes.NewBufferString(`{"username":"viewer1","password":"viewer1234","role":"viewer"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	h.CreateAuthUser(w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPut, "/api/auth/users/viewer1", bytes.NewBufferString(`{"role":"admin"}`)).WithContext(adminCtx)
	req2.Header.Set("Content-Type", "application/json")
	h.UpdateAuthUser(w2, req2)

	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var role string
	testPool.QueryRow(context.Background(), "SELECT role FROM auth_users WHERE username = 'viewer1'").Scan(&role)
	if role != "admin" {
		t.Errorf("expected role=admin after update, got %q", role)
	}
}

func TestResetAuthUserPassword(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	body := bytes.NewBufferString(`{"username":"viewer1","password":"viewer1234","role":"viewer"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	h.CreateAuthUser(w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/users/viewer1/reset-password", nil).WithContext(adminCtx)
	h.ResetAuthUserPassword(w2, req2)

	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["password"] == "" {
		t.Error("password should not be empty")
	}
}

func TestLogout(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil).WithContext(adminCtx)
	h.Logout(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuditLog_CreateUser(t *testing.T) {
	h := newHandler(t)
	adminCtx := setupAdmin(t, h)

	body := bytes.NewBufferString(`{"username":"auditviewer","password":"auditpass123","role":"viewer"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	h.CreateAuthUser(w, req)

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM audit_log WHERE action = 'create_auth_user'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 audit log entry, got %d", count)
	}
}
