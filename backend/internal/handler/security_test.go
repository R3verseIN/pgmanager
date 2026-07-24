package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPgBouncer_BlocksPgmanagerUser(t *testing.T) {
	dsn := os.Getenv("PGBOUNCER_URL")
	if dsn == "" {
		dsn = "postgres://pgmanager:pgmanager@localhost:5432/pgmanager?sslmode=disable"
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err == nil {
		conn.Close(ctx)
		t.Fatal("expected connection to fail for pgmanager user through PgBouncer")
	}
}

func TestPgBouncer_AllowsCreatedUser(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// cleanup
	pool.Exec(ctx, "DROP OWNED BY testpguser CASCADE")
	pool.Exec(ctx, "DROP ROLE IF EXISTS testpguser")
	pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testpguser'")
	t.Cleanup(func() {
		pool.Exec(ctx, "DROP OWNED BY testpguser CASCADE")
		pool.Exec(ctx, "DROP ROLE IF EXISTS testpguser")
		pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testpguser'")
	})

	// create test user
	_, err := pool.Exec(ctx, "CREATE ROLE testpguser WITH LOGIN PASSWORD 'testpass123'")
	if err != nil {
		t.Fatalf("failed to create role: %v", err)
	}

	// connect through PgBouncer
	pgbouncerURL := os.Getenv("PGBOUNCER_URL")
	if pgbouncerURL == "" {
		pgbouncerURL = "postgres://testpguser:testpass123@localhost:5432/pgmanager?sslmode=disable"
	}

	conn, err := pgx.Connect(ctx, pgbouncerURL)
	if err != nil {
		t.Fatalf("expected connection to succeed for created user through PgBouncer: %v", err)
	}
	defer conn.Close(ctx)

	var result int
	err = conn.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		t.Fatalf("expected query to succeed: %v", err)
	}
	if result != 1 {
		t.Fatalf("expected 1, got %d", result)
	}
}

func TestBackend_DatabaseConnection(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	var result int
	err := pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		t.Fatalf("expected query to succeed: %v", err)
	}
	if result != 1 {
		t.Fatalf("expected 1, got %d", result)
	}
}

func TestBackend_CreateAndDeleteUser(t *testing.T) {
	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()

	h.InitUserSchema(ctx)

	// cleanup
	pool.Exec(ctx, "DROP OWNED BY testsecurityuser CASCADE")
	pool.Exec(ctx, "DROP ROLE IF EXISTS testsecurityuser")
	pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testsecurityuser'")
	pool.Exec(ctx, "DROP DATABASE IF EXISTS testsecuritydb")
	pool.Exec(ctx, "CREATE DATABASE testsecuritydb")
	t.Cleanup(func() {
		pool.Exec(ctx, "DROP OWNED BY testsecurityuser CASCADE")
		pool.Exec(ctx, "DROP ROLE IF EXISTS testsecurityuser")
		pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testsecurityuser'")
		pool.Exec(ctx, "DROP DATABASE testsecuritydb WITH (FORCE)")
	})

	// create user via API
	body := `{"username":"testsecurityuser","databases":["testsecuritydb"],"access":"read"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	h.CreateUser(w, req)

	if w.Code != 201 {
		t.Fatalf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var result createUserResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if result.Username != "testsecurityuser" {
		t.Fatalf("expected username testsecurityuser, got %s", result.Username)
	}
	if result.Password == "" {
		t.Fatal("password should not be empty")
	}
	if len(result.Databases) != 1 || result.Databases[0] != "testsecuritydb" {
		t.Fatalf("expected databases [testsecuritydb], got %v", result.Databases)
	}
	// default allowedIps should be 0.0.0.0/0
	if len(result.AllowedIps) == 0 || result.AllowedIps[0] != "0.0.0.0/0" {
		t.Fatalf("expected default allowedIps [0.0.0.0/0], got %v", result.AllowedIps)
	}

	// delete user via API
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/users/testsecurityuser", nil).WithContext(ctx)
	req.SetPathValue("name", "testsecurityuser")
	h.DeleteUser(w, req)

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateUser_WithAllowedIPs_HBAFileContainsRules(t *testing.T) {
	tmpPath := withTempHBAFile(t)
	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()

	h.InitUserSchema(ctx)

	pool.Exec(ctx, "DROP OWNED BY testipuser CASCADE")
	pool.Exec(ctx, "DROP ROLE IF EXISTS testipuser")
	pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testipuser'")
	pool.Exec(ctx, "DROP DATABASE IF EXISTS testipdb")
	pool.Exec(ctx, "CREATE DATABASE testipdb")
	t.Cleanup(func() {
		pool.Exec(ctx, "DROP OWNED BY testipuser CASCADE")
		pool.Exec(ctx, "DROP ROLE IF EXISTS testipuser")
		pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testipuser'")
		pool.Exec(ctx, "DROP DATABASE testipdb WITH (FORCE)")
	})

	// Create user via API with specific allowedIps
	body := `{"username":"testipuser","databases":["testipdb"],"access":"read","allowedIps":["203.0.113.42","10.0.0.0/24"]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	hbaFilePath = tmpPath
	h.CreateUser(w, req)

	// Wait for async rebuild to finish
	time.Sleep(150 * time.Millisecond)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result createUserResponse
	json.Unmarshal(w.Body.Bytes(), &result)

	// Verify returned allowedIps
	if len(result.AllowedIps) != 2 {
		t.Fatalf("expected 2 allowedIps, got %v", result.AllowedIps)
	}

	// Verify HBA file content
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(content)

	if !strings.Contains(got, `"testipuser" 203.0.113.42/32 scram-sha-256`) {
		t.Errorf("expected /32 rule for bare IP in HBA file, got:\n%s", got)
	}
	if !strings.Contains(got, `"testipuser" 10.0.0.0/24 scram-sha-256`) {
		t.Errorf("expected CIDR rule in HBA file, got:\n%s", got)
	}
}

func TestUpdateUser_AllowedIPs_HBAFileUpdated(t *testing.T) {
	tmpPath := withTempHBAFile(t)
	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()

	h.InitUserSchema(ctx)

	pool.Exec(ctx, "DROP OWNED BY testupdateipuser CASCADE")
	pool.Exec(ctx, "DROP ROLE IF EXISTS testupdateipuser")
	pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testupdateipuser'")
	pool.Exec(ctx, "DROP DATABASE IF EXISTS testupdateipdb")
	pool.Exec(ctx, "CREATE DATABASE testupdateipdb")
	t.Cleanup(func() {
		pool.Exec(ctx, "DROP OWNED BY testupdateipuser CASCADE")
		pool.Exec(ctx, "DROP ROLE IF EXISTS testupdateipuser")
		pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testupdateipuser'")
		pool.Exec(ctx, "DROP DATABASE testupdateipdb WITH (FORCE)")
	})

	// Create user with open access
	hbaFilePath = tmpPath
	body := `{"username":"testupdateipuser","databases":["testupdateipdb"],"access":"read"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	h.CreateUser(w, req)
	if w.Code != 201 {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Wait for async rebuild
	time.Sleep(150 * time.Millisecond)

	// Initial HBA should have 0.0.0.0/0
	content, _ := os.ReadFile(tmpPath)
	if !strings.Contains(string(content), `"testupdateipuser" 0.0.0.0/0 scram-sha-256`) {
		t.Errorf("expected open rule initially, got:\n%s", string(content))
	}

	// Now lock down to specific IP via update
	updateBody := `{"allowedIps":["172.18.0.5"]}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/users/testupdateipuser", bytes.NewBufferString(updateBody)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	h.UpdateUser(w, req)
	if w.Code != 200 {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Wait for async rebuild
	time.Sleep(150 * time.Millisecond)

	// HBA file should now have restricted rule
	content, _ = os.ReadFile(tmpPath)
	got := string(content)
	if strings.Contains(got, `"testupdateipuser" 0.0.0.0/0 scram-sha-256`) {
		t.Errorf("old open rule should be gone after IP update")
	}
	if !strings.Contains(got, `"testupdateipuser" 172.18.0.5/32 scram-sha-256`) {
		t.Errorf("expected locked-down rule after update, got:\n%s", got)
	}
}

