package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := TestContainer(t)
	return pool
}

func TestBaseDSN(t *testing.T) string {
	t.Helper()
	_, baseDSN := TestContainer(t)
	return baseDSN
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
