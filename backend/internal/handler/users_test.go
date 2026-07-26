package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
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

func TestListUsers_Grouping(t *testing.T) {
	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()

	h.InitUserSchema(ctx)

	// cleanup
	pool.Exec(ctx, "DROP OWNED BY testgroupuser CASCADE")
	pool.Exec(ctx, "DROP ROLE IF EXISTS testgroupuser")
	pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testgroupuser'")
	pool.Exec(ctx, "DROP DATABASE IF EXISTS testgroupdb1")
	pool.Exec(ctx, "DROP DATABASE IF EXISTS testgroupdb2")
	pool.Exec(ctx, "CREATE DATABASE testgroupdb1")
	pool.Exec(ctx, "CREATE DATABASE testgroupdb2")
	t.Cleanup(func() {
		pool.Exec(ctx, "DROP OWNED BY testgroupuser CASCADE")
		pool.Exec(ctx, "DROP ROLE IF EXISTS testgroupuser")
		pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testgroupuser'")
		pool.Exec(ctx, "DROP DATABASE testgroupdb1 WITH (FORCE)")
		pool.Exec(ctx, "DROP DATABASE testgroupdb2 WITH (FORCE)")
	})

	// create role
	_, err := pool.Exec(ctx, "CREATE ROLE testgroupuser WITH LOGIN PASSWORD 'testpass123'")
	if err != nil {
		t.Fatalf("failed to create role: %v", err)
	}

	// insert two managed_users rows for same user, same access
	_, err = pool.Exec(ctx, "INSERT INTO managed_users (username, database_name, access) VALUES ('testgroupuser', 'testgroupdb1', 'read')")
	if err != nil {
		t.Fatalf("insert 1: %v", err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO managed_users (username, database_name, access) VALUES ('testgroupuser', 'testgroupdb2', 'read')")
	if err != nil {
		t.Fatalf("insert 2: %v", err)
	}

	// list users via httptest
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil).WithContext(ctx)
	h.ListUsers(w, req)

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var users []userRecord
	if err := json.Unmarshal(w.Body.Bytes(), &users); err != nil {
		t.Fatalf("failed to parse response: %v\nbody: %s", err, w.Body.String())
	}

	var found *userRecord
	for i := range users {
		if users[i].Username == "testgroupuser" {
			found = &users[i]
			break
		}
	}

	if found == nil {
		t.Fatalf("testgroupuser not found in response: %s", w.Body.String())
	}

	if len(found.Databases) != 2 {
		t.Fatalf("expected 2 databases, got %d: %v", len(found.Databases), found.Databases)
	}

	dbMap := map[string]bool{}
	for _, db := range found.Databases {
		dbMap[db] = true
	}
	if !dbMap["testgroupdb1"] || !dbMap["testgroupdb2"] {
		t.Fatalf("expected both testgroupdb1 and testgroupdb2, got: %v", found.Databases)
	}
}

func TestResolveConnectionStringHost(t *testing.T) {
	// Save and restore env
	origHost := os.Getenv("PGMANAGER_HOST")
	origPort := os.Getenv("PGMANAGER_PORT")
	t.Cleanup(func() {
		os.Setenv("PGMANAGER_HOST", origHost)
		os.Setenv("PGMANAGER_PORT", origPort)
	})

	tests := []struct {
		name       string
		pgHost     string
		pgPort     string
		requestHost string
		want       string
	}{
		{
			name:        "auto-detect from request host with port",
			pgHost:      "",
			pgPort:      "",
			requestHost: "192.168.0.13:8080",
			want:        "192.168.0.13:5432",
		},
		{
			name:        "auto-detect from request host without port",
			pgHost:      "",
			pgPort:      "",
			requestHost: "pg.example.com",
			want:        "pg.example.com:5432",
		},
		{
			name:        "auto-detect from localhost with port",
			pgHost:      "",
			pgPort:      "",
			requestHost: "localhost:8080",
			want:        "localhost:5432",
		},
		{
			name:        "env host with port",
			pgHost:      "pg.example.com:5050",
			pgPort:      "",
			requestHost: "192.168.0.13:8080",
			want:        "pg.example.com:5050",
		},
		{
			name:        "env host without port defaults to 5432",
			pgHost:      "pg.example.com",
			pgPort:      "",
			requestHost: "192.168.0.13:8080",
			want:        "pg.example.com:5432",
		},
		{
			name:        "env host without port uses PGMANAGER_PORT",
			pgHost:      "pg.example.com",
			pgPort:      "5050",
			requestHost: "192.168.0.13:8080",
			want:        "pg.example.com:5050",
		},
		{
			name:        "env localhost",
			pgHost:      "localhost",
			pgPort:      "",
			requestHost: "some-host:8080",
			want:        "localhost:5432",
		},
		{
			name:        "env IP with port",
			pgHost:      "10.0.0.1:5432",
			pgPort:      "",
			requestHost: "some-host:8080",
			want:        "10.0.0.1:5432",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("PGMANAGER_HOST", tt.pgHost)
			os.Setenv("PGMANAGER_PORT", tt.pgPort)

			req := httptest.NewRequest(http.MethodPost, "/api/users", nil)
			req.Host = tt.requestHost

			got := resolveConnectionStringHost(req)
			if got != tt.want {
				t.Errorf("resolveConnectionStringHost() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateUser_ConnectionString_Host(t *testing.T) {
	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()

	h.InitUserSchema(ctx)

	// Save and restore env
	origHost := os.Getenv("PGMANAGER_HOST")
	origPort := os.Getenv("PGMANAGER_PORT")
	t.Cleanup(func() {
		os.Setenv("PGMANAGER_HOST", origHost)
		os.Setenv("PGMANAGER_PORT", origPort)
	})

	// cleanup
	pool.Exec(ctx, "DROP OWNED BY testconnstruser CASCADE")
	pool.Exec(ctx, "DROP ROLE IF EXISTS testconnstruser")
	pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testconnstruser'")
	pool.Exec(ctx, "DROP DATABASE IF EXISTS testconnstrdb")
	pool.Exec(ctx, "CREATE DATABASE testconnstrdb")
	t.Cleanup(func() {
		pool.Exec(ctx, "DROP OWNED BY testconnstruser CASCADE")
		pool.Exec(ctx, "DROP ROLE IF EXISTS testconnstruser")
		pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testconnstruser'")
		pool.Exec(ctx, "DROP DATABASE testconnstrdb WITH (FORCE)")
	})

	t.Run("auto-detect from request Host", func(t *testing.T) {
		os.Unsetenv("PGMANAGER_HOST")
		os.Unsetenv("PGMANAGER_PORT")

		body := `{"username":"testconnstruser","databases":["testconnstrdb"],"access":"read","password":"testpass123"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Host = "192.168.0.13:8080"
		h.CreateUser(w, req)

		if w.Code != 201 {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}

		var result createUserResponse
		json.Unmarshal(w.Body.Bytes(), &result)

		want := "postgres://testconnstruser:testpass123@192.168.0.13:5432/testconnstrdb"
		if result.ConnectionString != want {
			t.Errorf("connectionString = %q, want %q", result.ConnectionString, want)
		}
	})

	// cleanup between sub-tests
	pool.Exec(ctx, "DROP OWNED BY testconnstruser CASCADE")
	pool.Exec(ctx, "DROP ROLE IF EXISTS testconnstruser")
	pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testconnstruser'")

	t.Run("env PGMANAGER_HOST overrides request Host", func(t *testing.T) {
		os.Setenv("PGMANAGER_HOST", "pg.example.com:5050")
		os.Unsetenv("PGMANAGER_PORT")

		body := `{"username":"testconnstruser","databases":["testconnstrdb"],"access":"read","password":"testpass123"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Host = "192.168.0.13:8080"
		h.CreateUser(w, req)

		if w.Code != 201 {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}

		var result createUserResponse
		json.Unmarshal(w.Body.Bytes(), &result)

		want := "postgres://testconnstruser:testpass123@pg.example.com:5050/testconnstrdb"
		if result.ConnectionString != want {
			t.Errorf("connectionString = %q, want %q", result.ConnectionString, want)
		}
	})

	// cleanup between sub-tests
	pool.Exec(ctx, "DROP OWNED BY testconnstruser CASCADE")
	pool.Exec(ctx, "DROP ROLE IF EXISTS testconnstruser")
	pool.Exec(ctx, "DELETE FROM managed_users WHERE username = 'testconnstruser'")

	t.Run("env host without port appends PGMANAGER_PORT", func(t *testing.T) {
		os.Setenv("PGMANAGER_HOST", "pg.example.com")
		os.Setenv("PGMANAGER_PORT", "5050")

		body := `{"username":"testconnstruser","databases":["testconnstrdb"],"access":"read","password":"testpass123"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Host = "192.168.0.13:8080"
		h.CreateUser(w, req)

		if w.Code != 201 {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}

		var result createUserResponse
		json.Unmarshal(w.Body.Bytes(), &result)

		want := "postgres://testconnstruser:testpass123@pg.example.com:5050/testconnstrdb"
		if result.ConnectionString != want {
			t.Errorf("connectionString = %q, want %q", result.ConnectionString, want)
		}
	})
}
