package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupAuthTest(t *testing.T) (*AuthHandler, context.Context) {
	t.Helper()
	pool := testPool(t)
	ah := NewAuthHandler(pool)
	ctx := context.Background()

	// clean slate
	pool.Exec(ctx, "DELETE FROM sessions")
	pool.Exec(ctx, "DELETE FROM auth_users")

	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM sessions")
		pool.Exec(ctx, "DELETE FROM auth_users")
	})

	return ah, ctx
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

func TestSetup_CreatesAdmin(t *testing.T) {
	ah, ctx := setupAuthTest(t)
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
	ah, ctx := setupAuthTest(t)
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

func TestListAuthUsers(t *testing.T) {
	ah, ctx := setupAuthTest(t)
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

func TestDeleteLastAdmin(t *testing.T) {
	ah, ctx := setupAuthTest(t)
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
	ah, ctx := setupAuthTest(t)
	createTestAdmin(t, ah, ctx, "admin", "admin1234")
	createTestAuthUser(t, ah, ctx, "admin2", "admin1234", "admin")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/users/admin", nil).WithContext(ctx)
	ah.DeleteAuthUser(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChangeLastAdminRole(t *testing.T) {
	ah, ctx := setupAuthTest(t)
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
	ah, ctx := setupAuthTest(t)
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

func TestResetAuthUserPassword(t *testing.T) {
	ah, ctx := setupAuthTest(t)
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
	ah, ctx := setupAuthTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/users/nobody/reset-password", nil).WithContext(ctx)
	ah.ResetAuthUserPassword(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteNonexistentUser(t *testing.T) {
	ah, ctx := setupAuthTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/users/nobody", nil).WithContext(ctx)
	ah.DeleteAuthUser(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateNonexistentUser(t *testing.T) {
	ah, ctx := setupAuthTest(t)

	body := `{"role":"viewer"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/auth/users/nobody", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	ah.UpdateAuthUser(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAuthUser_DuplicateUsername(t *testing.T) {
	ah, ctx := setupAuthTest(t)
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
	ah, ctx := setupAuthTest(t)
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
