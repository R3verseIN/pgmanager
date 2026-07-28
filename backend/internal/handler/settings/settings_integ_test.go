//go:build integration

package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

func TestGetSettings(t *testing.T) {
	h := New(testPool)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	h.GetSettings(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var settings map[string]string
	json.Unmarshal(w.Body.Bytes(), &settings)

	if _, ok := settings["pgbouncer_pool_mode"]; !ok {
		t.Error("expected pgbouncer_pool_mode in settings")
	}
	if _, ok := settings["pgbouncer_default_pool_size"]; !ok {
		t.Error("expected pgbouncer_default_pool_size in settings")
	}
	if _, ok := settings["pgbouncer_max_client_conn"]; !ok {
		t.Error("expected pgbouncer_max_client_conn in settings")
	}
}

func TestGetSettings_Values(t *testing.T) {
	h := New(testPool)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	h.GetSettings(w, req)

	var settings map[string]string
	json.Unmarshal(w.Body.Bytes(), &settings)

	if settings["pgbouncer_pool_mode"] != "transaction" {
		t.Errorf("expected pool_mode=transaction, got %q", settings["pgbouncer_pool_mode"])
	}
	if settings["pgbouncer_default_pool_size"] != "20" {
		t.Errorf("expected default_pool_size=20, got %q", settings["pgbouncer_default_pool_size"])
	}
	if settings["pgbouncer_max_client_conn"] != "100" {
		t.Errorf("expected max_client_conn=100, got %q", settings["pgbouncer_max_client_conn"])
	}
}

func TestUpdateSettings(t *testing.T) {
	h := New(testPool)

	body := bytes.NewBufferString(`{"pgbouncer_pool_mode":"session","pgbouncer_default_pool_size":"50"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/settings", body)
	req.Header.Set("Content-Type", "application/json")
	h.UpdateSettings(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	h.GetSettings(w2, req2)

	var settings map[string]string
	json.Unmarshal(w2.Body.Bytes(), &settings)

	if settings["pgbouncer_pool_mode"] != "session" {
		t.Errorf("expected pool_mode=session after update, got %q", settings["pgbouncer_pool_mode"])
	}
	if settings["pgbouncer_default_pool_size"] != "50" {
		t.Errorf("expected default_pool_size=50 after update, got %q", settings["pgbouncer_default_pool_size"])
	}
}

func TestUpdateSettings_NewKey(t *testing.T) {
	h := New(testPool)

	body := bytes.NewBufferString(`{"custom_setting":"custom_value"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/settings", body)
	req.Header.Set("Content-Type", "application/json")
	h.UpdateSettings(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	h.GetSettings(w2, req2)

	var settings map[string]string
	json.Unmarshal(w2.Body.Bytes(), &settings)

	if settings["custom_setting"] != "custom_value" {
		t.Errorf("expected custom_setting=custom_value, got %q", settings["custom_setting"])
	}
}

func TestUpdateSettings_EmptyBody(t *testing.T) {
	h := New(testPool)

	body := bytes.NewBufferString(`{}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/settings", body)
	req.Header.Set("Content-Type", "application/json")
	h.UpdateSettings(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateSettings_InvalidJSON(t *testing.T) {
	h := New(testPool)

	body := bytes.NewBufferString(`not json`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/settings", body)
	req.Header.Set("Content-Type", "application/json")
	h.UpdateSettings(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateSettings_Persistence(t *testing.T) {
	h := New(testPool)

	body := bytes.NewBufferString(`{"persist_key":"persist_value"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/settings", body)
	req.Header.Set("Content-Type", "application/json")
	h.UpdateSettings(w, req)

	var value string
	err := testPool.QueryRow(context.Background(), "SELECT value FROM system_config WHERE key = 'persist_key'").Scan(&value)
	if err != nil {
		t.Fatalf("failed to read persisted setting: %v", err)
	}
	if value != "persist_value" {
		t.Errorf("expected persist_value, got %q", value)
	}
}
