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
)

// withTempHBAFile overrides hbaFilePath to a temp file, returns cleanup func and path.
func withTempHBAFile(t *testing.T) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "pg_hba_*.conf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmp.Close()
	original := hbaFilePath
	hbaFilePath = tmp.Name()
	t.Cleanup(func() {
		hbaFilePath = original
		os.Remove(tmp.Name())
	})
	return tmp.Name()
}

func withTempIniFile(t *testing.T) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "pgbouncer_*.ini")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmp.WriteString("[databases]\n* = host=db port=5432\n\n[pgbouncer]\nlisten_addr = 0.0.0.0\n")
	tmp.Close()
	original := pgbouncerIniPath
	pgbouncerIniPath = tmp.Name()
	t.Cleanup(func() {
		pgbouncerIniPath = original
		os.Remove(tmp.Name())
	})
	return tmp.Name()
}

func setupHBATest(t *testing.T) (*Handler, context.Context) {
	t.Helper()
	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()
	if err := h.InitUserSchema(ctx); err != nil {
		t.Fatalf("InitUserSchema: %v", err)
	}
	pool.Exec(ctx, "DELETE FROM managed_users WHERE username LIKE 'hbatest_%'")
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM managed_users WHERE username LIKE 'hbatest_%'") })
	return h, ctx
}

func setupPgBouncerDBTest(t *testing.T) (*Handler, context.Context) {
	t.Helper()
	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()
	if err := h.InitUserSchema(ctx); err != nil {
		t.Fatalf("InitUserSchema: %v", err)
	}
	pool.Exec(ctx, "DELETE FROM pgbouncer_databases WHERE database_name LIKE 'pbtest_%'")
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM pgbouncer_databases WHERE database_name LIKE 'pbtest_%'")
	})
	return h, ctx
}

func TestRebuildPgBouncerHBA_DefaultAllowAll(t *testing.T) {
	tmpPath := withTempHBAFile(t)
	h, ctx := setupHBATest(t)

	_, err := h.pool.Exec(ctx,
		"INSERT INTO managed_users (username, database_name, access, allowed_ips) VALUES ($1, $2, $3, $4::jsonb)",
		"hbatest_open", "postgres", "read", `["0.0.0.0/0"]`)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	hbaFilePath = tmpPath
	h.RebuildPgBouncerHBA()

	content, _ := os.ReadFile(tmpPath)
	got := string(content)

	if !strings.Contains(got, `"hbatest_open" 0.0.0.0/0 scram-sha-256`) {
		t.Errorf("expected allow-all rule for hbatest_open, got:\n%s", got)
	}
	if !strings.Contains(got, "host all all 0.0.0.0/0 reject") {
		t.Errorf("expected default reject rule, got:\n%s", got)
	}
}

func TestRebuildPgBouncerHBA_SpecificIP(t *testing.T) {
	tmpPath := withTempHBAFile(t)
	h, ctx := setupHBATest(t)

	_, err := h.pool.Exec(ctx,
		"INSERT INTO managed_users (username, database_name, access, allowed_ips) VALUES ($1, $2, $3, $4::jsonb)",
		"hbatest_restricted", "postgres", "read", `["203.0.113.5","10.0.0.0/24"]`)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	hbaFilePath = tmpPath
	h.RebuildPgBouncerHBA()

	content, _ := os.ReadFile(tmpPath)
	got := string(content)

	// bare IP gets /32 suffix
	if !strings.Contains(got, `"hbatest_restricted" 203.0.113.5/32 scram-sha-256`) {
		t.Errorf("expected /32 rule for bare IP, got:\n%s", got)
	}
	// CIDR stays unchanged
	if !strings.Contains(got, `"hbatest_restricted" 10.0.0.0/24 scram-sha-256`) {
		t.Errorf("expected CIDR rule, got:\n%s", got)
	}
}

func TestRebuildPgBouncerHBA_EmptyIPs_DefaultsReject(t *testing.T) {
	tmpPath := withTempHBAFile(t)
	h, ctx := setupHBATest(t)

	// Insert with empty JSON array
	_, err := h.pool.Exec(ctx,
		"INSERT INTO managed_users (username, database_name, access, allowed_ips) VALUES ($1, $2, $3, $4::jsonb)",
		"hbatest_blocked", "postgres", "read", `[]`)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	hbaFilePath = tmpPath
	h.RebuildPgBouncerHBA()

	content, _ := os.ReadFile(tmpPath)
	got := string(content)

	if !strings.Contains(got, `"hbatest_blocked" 0.0.0.0/0 reject`) {
		t.Errorf("expected reject rule for empty IPs, got:\n%s", got)
	}
}

func TestRebuildPgBouncerHBA_InternalRulesAlwaysPresent(t *testing.T) {
	tmpPath := withTempHBAFile(t)
	h, _ := setupHBATest(t)

	hbaFilePath = tmpPath
	h.RebuildPgBouncerHBA()

	content, _ := os.ReadFile(tmpPath)
	got := string(content)

	required := []string{
		"host all pgbouncer_auth 172.16.0.0/12 trust",
		"host all pgmanager 172.16.0.0/12 trust",
		"host all all 0.0.0.0/0 reject",
		"host all all ::0/0 reject",
	}
	for _, rule := range required {
		if !strings.Contains(got, rule) {
			t.Errorf("expected internal rule %q to be present, got:\n%s", rule, got)
		}
	}
}

func TestListPgBouncerDatabases(t *testing.T) {
	h, ctx := setupPgBouncerDBTest(t)

	// Insert test data
	h.pool.Exec(ctx, `INSERT INTO pgbouncer_databases (database_name, allowed) VALUES ('pbtest_db1', true), ('pbtest_db2', false) ON CONFLICT DO NOTHING`)

	req := httptest.NewRequest("GET", "/api/pgbouncer/databases", nil)
	w := httptest.NewRecorder()

	h.ListPgBouncerDatabases(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var databases []pgbouncerDatabase
	if err := json.NewDecoder(w.Body).Decode(&databases); err != nil {
		t.Fatalf("decode: %v", err)
	}

	found1, found2 := false, false
	for _, db := range databases {
		if db.DatabaseName == "pbtest_db1" && db.Allowed {
			found1 = true
		}
		if db.DatabaseName == "pbtest_db2" && !db.Allowed {
			found2 = true
		}
	}
	if !found1 {
		t.Error("expected pbtest_db1 with allowed=true")
	}
	if !found2 {
		t.Error("expected pbtest_db2 with allowed=false")
	}
}

func TestTogglePgBouncerDatabase(t *testing.T) {
	h, ctx := setupPgBouncerDBTest(t)

	h.pool.Exec(ctx, `INSERT INTO pgbouncer_databases (database_name, allowed) VALUES ('pbtest_toggle', false) ON CONFLICT DO NOTHING`)

	body, _ := json.Marshal(map[string]bool{"allowed": true})
	req := httptest.NewRequest("PUT", "/api/pgbouncer/databases/pbtest_toggle", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.TogglePgBouncerDatabase(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify in database
	var allowed bool
	h.pool.QueryRow(ctx, `SELECT allowed FROM pgbouncer_databases WHERE database_name = 'pbtest_toggle'`).Scan(&allowed)
	if !allowed {
		t.Error("expected allowed=true after toggle")
	}
}

func TestTogglePgBouncerDatabase_NotFound(t *testing.T) {
	h, _ := setupPgBouncerDBTest(t)

	body, _ := json.Marshal(map[string]bool{"allowed": true})
	req := httptest.NewRequest("PUT", "/api/pgbouncer/databases/pbtest_nonexistent", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.TogglePgBouncerDatabase(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRebuildPgBouncerDatabases(t *testing.T) {
	h, ctx := setupPgBouncerDBTest(t)
	iniPath := withTempIniFile(t)

	// Insert test data
	h.pool.Exec(ctx, `DELETE FROM pgbouncer_databases WHERE database_name LIKE 'pbtest_%'`)
	h.pool.Exec(ctx, `INSERT INTO pgbouncer_databases (database_name, allowed) VALUES ('pbtest_allowed', true), ('pbtest_blocked', false)`)

	h.RebuildPgBouncerHBA()

	content, err := os.ReadFile(iniPath)
	if err != nil {
		t.Fatalf("read ini: %v", err)
	}
	got := string(content)

	if !strings.Contains(got, "pbtest_allowed = host=db port=5432 dbname=pbtest_allowed") {
		t.Errorf("expected allowed database in [databases] section, got:\n%s", got)
	}
	if strings.Contains(got, "pbtest_blocked") {
		t.Errorf("blocked database should not appear in [databases] section, got:\n%s", got)
	}
	if !strings.Contains(got, "[pgbouncer]") {
		t.Errorf("expected [pgbouncer] section preserved, got:\n%s", got)
	}
}

func TestRebuildPgBouncerDatabases_Empty(t *testing.T) {
	h, ctx := setupPgBouncerDBTest(t)
	iniPath := withTempIniFile(t)

	// Remove all allowed databases
	h.pool.Exec(ctx, `UPDATE pgbouncer_databases SET allowed = false WHERE database_name LIKE 'pbtest_%'`)

	h.RebuildPgBouncerHBA()

	content, err := os.ReadFile(iniPath)
	if err != nil {
		t.Fatalf("read ini: %v", err)
	}
	got := string(content)

	if !strings.Contains(got, "; no databases allowed through PgBouncer") {
		t.Errorf("expected 'no databases allowed' comment, got:\n%s", got)
	}
	if strings.Contains(got, "host=db") {
		t.Errorf("no databases should appear when none allowed, got:\n%s", got)
	}
}
