package handler

import (
	"context"
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
