package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"pgmanager/internal/handler/core"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPool(t *testing.T) *pgxpool.Pool {
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

func TestBaseDSN() string {
	dsn := os.Getenv("TEST_DATABASE_BASE_DSN")
	if dsn == "" {
		dsn = "postgres://pgmanager:pgmanager@localhost:5433/pgmanager?sslmode=disable"
	}
	return dsn
}

func SetupHandler(t *testing.T) *core.Handler {
	t.Helper()
	pool := TestPool(t)
	return core.NewWithDSN(pool, TestBaseDSN())
}

func JSONBody(v any) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

func CreateTestDB(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	ctx := context.Background()
	pool.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	_, err := pool.Exec(ctx, "CREATE DATABASE "+name)
	if err != nil {
		t.Fatalf("failed to create test database %s: %v", name, err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
}

func NewRequest(method, url string, body any) *http.Request {
	var reqBody *bytes.Buffer
	if body != nil {
		reqBody = JSONBody(body)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, url, reqBody)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func NewRequestWithContext(ctx context.Context, method, url string, body any) *http.Request {
	req := NewRequest(method, url, body)
	return req.WithContext(ctx)
}
