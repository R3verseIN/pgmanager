package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pgmanager/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupAuthTest(t *testing.T) (*AuthHandler, *pgxpool.Pool, context.Context) {
	t.Helper()
	pool := testPool(t)
	ah := NewAuthHandler(pool)
	ctx := context.Background()

	// clean slate
	pool.Exec(ctx, "DELETE FROM sessions")
	pool.Exec(ctx, "DELETE FROM dev_databases")
	pool.Exec(ctx, "DELETE FROM auth_users")
	pool.Exec(ctx, "DROP DATABASE IF EXISTS mydb")
	pool.Exec(ctx, "CREATE DATABASE mydb")

	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM sessions")
		pool.Exec(ctx, "DELETE FROM dev_databases")
		pool.Exec(ctx, "DELETE FROM auth_users")
		pool.Exec(ctx, "DROP DATABASE IF EXISTS mydb")
	})

	return ah, pool, ctx
}

func createTestAdmin(t *testing.T, ah *AuthHandler, ctx context.Context, username, password string) {
	t.Helper()
	body := `{"username":"` + username + `","password":"` + password + `"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.Setup(w, req)
	if w.Code != 201 {
		t.Fatalf("createTestAdmin: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func createTestAuthUser(t *testing.T, ah *AuthHandler, ctx context.Context, username, password, role string) {
	t.Helper()
	body := `{"username":"` + username + `","password":"` + password + `","role":"` + role + `"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.CreateAuthUser(w, req)
	if w.Code != 201 {
		t.Fatalf("createTestAuthUser: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func loginTestUser(t *testing.T, ah *AuthHandler, pool *pgxpool.Pool, ctx context.Context, username, password string) context.Context {
	t.Helper()
	var userID int
	err := pool.QueryRow(ctx, "SELECT id FROM auth_users WHERE username = $1", username).Scan(&userID)
	if err != nil {
		t.Fatalf("loginTestUser: user %s not found: %v", username, err)
	}
	token, err := auth.CreateSession(ctx, pool, userID)
	if err != nil {
		t.Fatalf("loginTestUser: failed to create session: %v", err)
	}
	_ = token
	return auth.WithUser(ctx, &auth.SessionUser{ID: userID, Username: username, Role: "admin"})
}

func setupAuthTestWithUser(t *testing.T) (*AuthHandler, *pgxpool.Pool, context.Context, context.Context) {
	t.Helper()
	ah, pool, cleanCtx := setupAuthTest(t)
	createTestAdmin(t, ah, cleanCtx, "admin", "admin1234")

	var userID int
	err := pool.QueryRow(cleanCtx, "SELECT id FROM auth_users WHERE username = 'admin'").Scan(&userID)
	if err != nil {
		t.Fatalf("setupAuthTestWithUser: failed to get admin id: %v", err)
	}
	token, err := auth.CreateSession(cleanCtx, pool, userID)
	if err != nil {
		t.Fatalf("setupAuthTestWithUser: failed to create session: %v", err)
	}
	_ = token
	adminCtx := auth.WithUser(cleanCtx, &auth.SessionUser{ID: userID, Username: "admin", Role: "admin"})
	return ah, pool, cleanCtx, adminCtx
}

// --- Setup tests ---

func TestSetup_CreatesAdmin(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	body := `{"username":"admin","password":"admin1234"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.Setup(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetup_RejectsSecondAdmin(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")

	body := `{"username":"admin2","password":"admin1234"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.Setup(w, req)

	if w.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

// --- List tests ---

func TestListAuthUsers(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestAuthUser(t, ah, ctx, "viewer1", "viewer1234", "viewer")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/users", nil).WithContext(ctx)
	ah.ListAuthUsers(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var users []authUserListItem
	if err := json.Unmarshal(w.Body.Bytes(), &users); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
}

func TestListAuthUsers_Empty(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/users", nil).WithContext(ctx)
	ah.ListAuthUsers(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var users []authUserListItem
	if err := json.Unmarshal(w.Body.Bytes(), &users); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(users) != 0 {
		t.Fatalf("expected 0 users, got %d", len(users))
	}
}

// --- Delete tests ---

func TestDeleteLastAdmin(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/users/admin", nil).WithContext(ctx)
	ah.DeleteAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "cannot delete the last admin" {
		t.Fatalf("expected 'cannot delete the last admin', got %q", resp["error"])
	}
}

func TestDeleteAdmin_SecondExists(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestAuthUser(t, ah, ctx, "admin2", "admin1234", "admin")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/users/admin", nil).WithContext(ctx)
	ah.DeleteAuthUser(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteNonexistentUser(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/users/nobody", nil).WithContext(ctx)
	ah.DeleteAuthUser(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteSelf(t *testing.T) {
	ah, pool, cleanCtx, adminCtx := setupAuthTestWithUser(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/users/admin", nil).WithContext(adminCtx)
	ah.DeleteAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "cannot delete your own account" {
		t.Fatalf("expected 'cannot delete your own account', got %q", resp["error"])
	}

	// verify admin still exists
	var count int
	pool.QueryRow(cleanCtx, "SELECT COUNT(*) FROM auth_users WHERE username = 'admin'").Scan(&count)
	if count != 1 {
		t.Fatalf("admin should still exist, count: %d", count)
	}
}

// --- Update role tests ---

func TestChangeLastAdminRole(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")

	body := `{"role":"viewer"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/auth/users/admin", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.UpdateAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "cannot change role of the last admin" {
		t.Fatalf("expected 'cannot change role of the last admin', got %q", resp["error"])
	}
}

func TestChangeAdminRole_SecondExists(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestAuthUser(t, ah, ctx, "admin2", "admin1234", "admin")

	body := `{"role":"viewer"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/auth/users/admin", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.UpdateAuthUser(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateNonexistentUser(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)

	body := `{"role":"viewer"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/auth/users/nobody", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.UpdateAuthUser(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChangeSelfRole(t *testing.T) {
	ah, _, cleanCtx, adminCtx := setupAuthTestWithUser(t)
	createTestAuthUser(t, ah, cleanCtx, "admin2", "admin1234", "admin")

	body := `{"role":"viewer"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/auth/users/admin", bytes.NewBufferString(body)).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	ah.UpdateAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "cannot change your own role" {
		t.Fatalf("expected 'cannot change your own role', got %q", resp["error"])
	}
}

// --- Reset password tests ---

func TestResetAuthUserPassword(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestAuthUser(t, ah, ctx, "viewer1", "viewer1234", "viewer")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users/viewer1/reset-password", nil).WithContext(ctx)
	ah.ResetAuthUserPassword(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["password"] == "" {
		t.Fatal("password should not be empty")
	}
	if resp["password"] == "viewer1234" {
		t.Fatal("password should be different from original")
	}
}

func TestResetPassword_NonexistentUser(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users/nobody/reset-password", nil).WithContext(ctx)
	ah.ResetAuthUserPassword(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Create auth user tests ---

func TestCreateAuthUser_DuplicateUsername(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestAuthUser(t, ah, ctx, "viewer1", "viewer1234", "viewer")

	body := `{"username":"viewer1","password":"viewer1234","role":"admin"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.CreateAuthUser(w, req)

	if w.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAuthUser_InvalidRole(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")

	body := `{"username":"user1","password":"user1234","role":"superadmin"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.CreateAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Password max-length tests ---

func TestSetup_PasswordTooLong(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	body := `{"username":"admin","password":"` + strings.Repeat("a", 73) + `"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.Setup(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp["error"], "72") {
		t.Fatalf("expected error about 72 chars, got %q", resp["error"])
	}
}

func TestSetup_PasswordAtMaxLength(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	body := `{"username":"admin","password":"` + strings.Repeat("a", 72) + `"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.Setup(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201 for 72-char password, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAuthUser_PasswordTooLong(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")

	body := `{"username":"user1","password":"` + strings.Repeat("a", 73) + `","role":"viewer"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.CreateAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChangePassword_NewPasswordTooLong(t *testing.T) {
	ah, pool, cleanCtx := setupAuthTest(t)
	createTestAdmin(t, ah, cleanCtx, "admin", "admin1234")

	var userID int
	pool.QueryRow(cleanCtx, "SELECT id FROM auth_users WHERE username = 'admin'").Scan(&userID)
	token, err := auth.CreateSession(cleanCtx, pool, userID)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	adminCtx := auth.WithUser(cleanCtx, &auth.SessionUser{ID: userID, Username: "admin", Role: "admin"})
	_ = token

	body := `{"current_password":"admin1234","new_password":"` + strings.Repeat("a", 73) + `"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/auth/password", bytes.NewBufferString(body)).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	ah.ChangePassword(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Reset password with custom password ---

func TestResetPassword_WithCustomPassword(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestAuthUser(t, ah, ctx, "viewer1", "viewer1234", "viewer")

	body := `{"password":"MyNewPass123"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users/viewer1/reset-password", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.ResetAuthUserPassword(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["password"] != "MyNewPass123" {
		t.Fatalf("expected custom password 'MyNewPass123', got %q", resp["password"])
	}
}

func TestResetPassword_WithTooLongCustomPassword(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestAuthUser(t, ah, ctx, "viewer1", "viewer1234", "viewer")

	body := `{"password":"` + strings.Repeat("a", 73) + `"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users/viewer1/reset-password", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.ResetAuthUserPassword(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Reset password invalidates sessions ---

func TestResetPassword_InvalidatesSessions(t *testing.T) {
	ah, pool, cleanCtx := setupAuthTest(t)
	createTestAdmin(t, ah, cleanCtx, "admin", "admin1234")
	createTestAuthUser(t, ah, cleanCtx, "viewer1", "viewer1234", "viewer")

	var viewerID int
	pool.QueryRow(cleanCtx, "SELECT id FROM auth_users WHERE username = 'viewer1'").Scan(&viewerID)

	_, err := auth.CreateSession(cleanCtx, pool, viewerID)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// verify session exists
	var sessionCount int
	pool.QueryRow(cleanCtx, "SELECT COUNT(*) FROM sessions WHERE user_id = $1", viewerID).Scan(&sessionCount)
	if sessionCount != 1 {
		t.Fatalf("expected 1 session, got %d", sessionCount)
	}

	// reset password
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users/viewer1/reset-password", nil).WithContext(cleanCtx)
	ah.ResetAuthUserPassword(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// verify sessions deleted
	pool.QueryRow(cleanCtx, "SELECT COUNT(*) FROM sessions WHERE user_id = $1", viewerID).Scan(&sessionCount)
	if sessionCount != 0 {
		t.Fatalf("expected 0 sessions after reset, got %d", sessionCount)
	}
}

// --- Dev role tests ---

func createTestDev(t *testing.T, ah *AuthHandler, ctx context.Context, username, password string, databases []string) {
	t.Helper()
	dbsJSON, _ := json.Marshal(databases)
	body := `{"username":"` + username + `","password":"` + password + `","role":"dev","databases":` + string(dbsJSON) + `}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.CreateAuthUser(w, req)
	if w.Code != 201 {
		t.Fatalf("createTestDev: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAuthUser_DevWithDatabases(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")

	body := `{"username":"dev1","password":"devpass123","role":"dev","databases":["mydb"]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.CreateAuthUser(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAuthUser_DevWithoutDatabases(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")

	body := `{"username":"dev1","password":"devpass123","role":"dev","databases":[]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.CreateAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "databases are required for dev role" {
		t.Fatalf("expected 'databases are required for dev role', got %q", resp["error"])
	}
}

func TestCreateAuthUser_DevWithSystemDatabase(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")

	body := `{"username":"dev1","password":"devpass123","role":"dev","databases":["pgmanager"]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.CreateAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "cannot assign system database: pgmanager" {
		t.Fatalf("expected 'cannot assign system database: pgmanager', got %q", resp["error"])
	}
}

func TestCreateAuthUser_DevWithNonexistentDatabase(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")

	body := `{"username":"dev1","password":"devpass123","role":"dev","databases":["nonexistent"]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.CreateAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "database does not exist: nonexistent" {
		t.Fatalf("expected 'database does not exist: nonexistent', got %q", resp["error"])
	}
}

func TestListAuthUsers_WithDevDatabases(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestDev(t, ah, ctx, "dev1", "devpass123", []string{"mydb"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/users", nil).WithContext(ctx)
	ah.ListAuthUsers(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var users []authUserListItem
	if err := json.Unmarshal(w.Body.Bytes(), &users); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}

	// Find the dev user
	var devUser *authUserListItem
	for i, u := range users {
		if u.Username == "dev1" {
			devUser = &users[i]
			break
		}
	}
	if devUser == nil {
		t.Fatal("dev user not found")
	}
	if devUser.Role != "dev" {
		t.Fatalf("expected role 'dev', got %q", devUser.Role)
	}
	if len(devUser.Databases) != 1 || devUser.Databases[0] != "mydb" {
		t.Fatalf("expected databases ['mydb'], got %v", devUser.Databases)
	}
}

func TestUpdateAuthUser_ChangeToDev(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestAuthUser(t, ah, ctx, "viewer1", "viewer1234", "viewer")

	body := `{"role":"dev","databases":["mydb"]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/auth/users/viewer1", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.UpdateAuthUser(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify databases assigned
	var dbCount int
	ah.pool.QueryRow(ctx, "SELECT COUNT(*) FROM dev_databases WHERE auth_user_id = (SELECT id FROM auth_users WHERE username = 'viewer1')").Scan(&dbCount)
	if dbCount != 1 {
		t.Fatalf("expected 1 dev_database, got %d", dbCount)
	}
}

func TestUpdateAuthUser_ChangeFromDev(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestDev(t, ah, ctx, "dev1", "devpass123", []string{"mydb"})

	body := `{"role":"viewer"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/auth/users/dev1", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.UpdateAuthUser(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify databases cleared
	var dbCount int
	ah.pool.QueryRow(ctx, "SELECT COUNT(*) FROM dev_databases WHERE auth_user_id = (SELECT id FROM auth_users WHERE username = 'dev1')").Scan(&dbCount)
	if dbCount != 0 {
		t.Fatalf("expected 0 dev_databases after role change, got %d", dbCount)
	}
}

func TestUpdateAuthUser_DevWithoutDatabases(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestAuthUser(t, ah, ctx, "viewer1", "viewer1234", "viewer")

	body := `{"role":"dev","databases":[]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/auth/users/viewer1", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.UpdateAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteLastAdmin_WithDevs(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestDev(t, ah, ctx, "dev1", "devpass123", []string{"mydb"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/users/admin", nil).WithContext(ctx)
	ah.DeleteAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "cannot delete the last admin" {
		t.Fatalf("expected 'cannot delete the last admin', got %q", resp["error"])
	}
}

func TestChangeLastAdminRole_WithDevs(t *testing.T) {
	ah, _, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestDev(t, ah, ctx, "dev1", "devpass123", []string{"mydb"})

	body := `{"role":"dev","databases":["mydb"]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/auth/users/admin", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.UpdateAuthUser(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "cannot change role of the last admin" {
		t.Fatalf("expected 'cannot change role of the last admin', got %q", resp["error"])
	}
}
