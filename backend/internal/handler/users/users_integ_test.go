//go:build integration

package users

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
	if err := InitUserSchema(context.Background(), pool); err != nil {
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

	roles := []string{"testuser", "viewer1", "imported", "user_a", "user_b", "testdev"}
	for _, r := range roles {
		testPool.Exec(ctx, "DROP OWNED BY "+core.QuoteIdent(r)+" CASCADE")
		testPool.Exec(ctx, "DROP ROLE IF EXISTS "+core.QuoteIdent(r))
	}
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

func TestListUsers_Empty(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "testdb")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users/", nil).WithContext(adminCtx)
	ListUsers(testPool, w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var users []UserRecord
	json.Unmarshal(w.Body.Bytes(), &users)
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestCreateUser_Success(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	dbName := "userdb1"
	createTestDB(t, dbName)

	body := bytes.NewBufferString(`{"username":"testuser","password":"testpass1234","access":"write","databases":["` + dbName + `"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp CreateUserResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Username != "testuser" {
		t.Errorf("expected username=testuser, got %q", resp.Username)
	}
	if resp.Password == "" {
		t.Error("password should not be empty")
	}
	if resp.ConnectionString == "" {
		t.Error("connection string should not be empty")
	}

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM managed_users WHERE username = 'testuser'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 managed_users entry, got %d", count)
	}
}

func TestCreateUser_GeneratePassword(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "gdb")

	body := bytes.NewBufferString(`{"username":"testuser","access":"read","databases":["gdb"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp CreateUserResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Password) != 16 {
		t.Errorf("expected 16-char generated password, got %d chars", len(resp.Password))
	}
}

func TestCreateUser_DuplicateUsername(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "ddb")

	body := bytes.NewBufferString(`{"username":"dupuser","password":"testpass1234","access":"read","databases":["ddb"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/users/", bytes.NewBufferString(`{"username":"dupuser","password":"testpass1234","access":"read","databases":["ddb"]}`)).WithContext(adminCtx)
	req2.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w2, req2)

	if w2.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestCreateUser_NoDatabases(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	body := bytes.NewBufferString(`{"username":"testuser","password":"testpass1234","access":"read","databases":[]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateUser_NonexistentDatabase(t *testing.T) {
	adminCtx := setupAdminCtx(t)

	body := bytes.NewBufferString(`{"username":"testuser","password":"testpass1234","access":"read","databases":["nodb"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteUser_Success(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "deldb")

	body := bytes.NewBufferString(`{"username":"deluser","password":"testpass1234","access":"write","databases":["deldb"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodDelete, "/api/users/deluser", nil).WithContext(adminCtx)
	DeleteUser(testPool, testBaseDSN, w2, req2)

	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var exists bool
	testPool.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = 'deluser'").Scan(&exists)
	if exists {
		t.Error("expected role to be deleted")
	}

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM managed_users WHERE username = 'deluser'").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 managed_users entries, got %d", count)
	}
}

func TestUpdateUser_Access(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "updb")

	body := bytes.NewBufferString(`{"username":"upuser","password":"testpass1234","access":"read","databases":["updb"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPut, "/api/users/upuser", bytes.NewBufferString(`{"access":"full"}`)).WithContext(adminCtx)
	req2.Header.Set("Content-Type", "application/json")
	UpdateUser(testPool, testBaseDSN, w2, req2)

	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var access string
	testPool.QueryRow(context.Background(), "SELECT access FROM managed_users WHERE username = 'upuser' LIMIT 1").Scan(&access)
	if access != "full" {
		t.Errorf("expected access=full, got %q", access)
	}
}

func TestUpdateUser_GeneratePassword(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "pwdb")

	body := bytes.NewBufferString(`{"username":"pwuser","password":"testpass1234","access":"read","databases":["pwdb"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPut, "/api/users/pwuser", bytes.NewBufferString(`{"generatePassword":true}`)).WithContext(adminCtx)
	req2.Header.Set("Content-Type", "application/json")
	UpdateUser(testPool, testBaseDSN, w2, req2)

	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["password"] == "" {
		t.Error("expected new password in response")
	}
	if resp["password"] == "testpass1234" {
		t.Error("password should have changed")
	}
}

func TestAddUserDatabase(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "db1")
	createTestDB(t, "db2")

	body := bytes.NewBufferString(`{"username":"multidb","password":"testpass1234","access":"read","databases":["db1"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/users/multidb/databases", bytes.NewBufferString(`{"database":"db2"}`)).WithContext(adminCtx)
	req2.Header.Set("Content-Type", "application/json")
	AddUserDatabase(testPool, testBaseDSN, w2, req2)

	if w2.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w2.Code, w2.Body.String())
	}

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM managed_users WHERE username = 'multidb'").Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 managed_users entries, got %d", count)
	}
}

func TestRemoveUserDatabase(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "rdb1")
	createTestDB(t, "rdb2")

	body := bytes.NewBufferString(`{"username":"rmdb","password":"testpass1234","access":"read","databases":["rdb1","rdb2"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodDelete, "/api/users/rmdb/databases/rdb2", nil).WithContext(adminCtx)
	RemoveUserDatabase(testPool, testBaseDSN, w2, req2)

	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM managed_users WHERE username = 'rmdb'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 managed_users entry after remove, got %d", count)
	}
}

func TestAccessLevels(t *testing.T) {
	levels := []string{"read", "write", "ddl", "full"}
	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			adminCtx := setupAdminCtx(t)
			dbName := level + "db"
			createTestDB(t, dbName)

			body := bytes.NewBufferString(`{"username":"` + level + `user","password":"testpass1234","access":"` + level + `","databases":["` + dbName + `"]}`)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
			req.Header.Set("Content-Type", "application/json")
			CreateUser(testPool, testBaseDSN, w, req)

			if w.Code != 201 {
				t.Fatalf("expected 201 for access=%s, got %d: %s", level, w.Code, w.Body.String())
			}

			var access string
			testPool.QueryRow(context.Background(), "SELECT access FROM managed_users WHERE username = $1 LIMIT 1", level+"user").Scan(&access)
			if access != level {
				t.Errorf("expected access=%s, got %q", level, access)
			}
		})
	}
}

func TestCreateUser_InvalidAccess(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "invdb")

	body := bytes.NewBufferString(`{"username":"invuser","password":"testpass1234","access":"superadmin","databases":["invdb"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateUser_InvalidUsername(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "invdb2")

	body := bytes.NewBufferString(`{"username":"123invalid","password":"testpass1234","access":"read","databases":["invdb2"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateUser_ShortPassword(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "spdb")

	body := bytes.NewBufferString(`{"username":"spuser","password":"short","access":"read","databases":["spdb"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuditLog_CreateUser(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "auditdb")

	body := bytes.NewBufferString(`{"username":"audituser","password":"testpass1234","access":"read","databases":["auditdb"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM audit_log WHERE action = 'create_user'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 audit log entry, got %d", count)
	}
}

func TestAuditLog_DeleteUser(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "adel")

	body := bytes.NewBufferString(`{"username":"adeluser","password":"testpass1234","access":"read","databases":["adel"]}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users/", body).WithContext(adminCtx)
	req.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodDelete, "/api/users/adeluser", nil).WithContext(adminCtx)
	DeleteUser(testPool, testBaseDSN, w2, req2)

	var count int
	testPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM audit_log WHERE action = 'delete_user'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 delete_user audit log, got %d", count)
	}
}

func TestListUsers_Multiple(t *testing.T) {
	adminCtx := setupAdminCtx(t)
	createTestDB(t, "listdb")

	body1 := bytes.NewBufferString(`{"username":"listuser1","password":"testpass1234","access":"read","databases":["listdb"]}`)
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "/api/users/", body1).WithContext(adminCtx)
	req1.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w1, req1)

	body2 := bytes.NewBufferString(`{"username":"listuser2","password":"testpass1234","access":"write","databases":["listdb"]}`)
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/users/", body2).WithContext(adminCtx)
	req2.Header.Set("Content-Type", "application/json")
	CreateUser(testPool, testBaseDSN, w2, req2)

	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/api/users/", nil).WithContext(adminCtx)
	ListUsers(testPool, w3, req3)

	if w3.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w3.Code, w3.Body.String())
	}

	var users []UserRecord
	json.Unmarshal(w3.Body.Bytes(), &users)
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
}
