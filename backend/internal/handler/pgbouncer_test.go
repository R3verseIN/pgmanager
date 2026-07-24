package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// generateHBAContent is a helper that invokes the HBA generation logic
// by pointing hbaFilePath at a temp file, running RebuildPgBouncerHBA,
// and returning the file contents.
func generateHBAContent(t *testing.T) string {
	t.Helper()

	tmp, err := os.CreateTemp("", "pg_hba_*.conf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	// Override path so RebuildPgBouncerHBA writes to temp file
	original := hbaFilePath
	hbaFilePath = tmp.Name()
	t.Cleanup(func() { hbaFilePath = original })

	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()

	if err := h.InitUserSchema(ctx); err != nil {
		t.Fatalf("InitUserSchema: %v", err)
	}
	// Remove test rows before and after
	pool.Exec(ctx, "DELETE FROM managed_users WHERE username LIKE 'hbatest_%'")
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM managed_users WHERE username LIKE 'hbatest_%'") })

	return tmp.Name()
}

func TestRebuildPgBouncerHBA_DefaultAllowAll(t *testing.T) {
	tmpPath := generateHBAContent(t)
	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()

	// Insert a user with the default 0.0.0.0/0 allow-all
	_, err := pool.Exec(ctx, "INSERT INTO managed_users (username, database_name, access, allowed_ips) VALUES ($1, $2, $3, $4)",
		"hbatest_open", "postgres", "read", []string{"0.0.0.0/0"})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	hbaFilePath = tmpPath
	h.RebuildPgBouncerHBA() // RELOAD will fail in test env, that's OK

	content, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(content)

	if !strings.Contains(got, `"hbatest_open" 0.0.0.0/0 scram-sha-256`) {
		t.Errorf("expected allow-all rule for hbatest_open, got:\n%s", got)
	}
	if !strings.Contains(got, "host all all 0.0.0.0/0 reject") {
		t.Errorf("expected default reject rule, got:\n%s", got)
	}
}

func TestRebuildPgBouncerHBA_SpecificIP(t *testing.T) {
	tmpPath := generateHBAContent(t)
	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()

	_, err := pool.Exec(ctx, "INSERT INTO managed_users (username, database_name, access, allowed_ips) VALUES ($1, $2, $3, $4)",
		"hbatest_restricted", "postgres", "read", []string{"203.0.113.5", "10.0.0.0/24"})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	hbaFilePath = tmpPath
	h.RebuildPgBouncerHBA()

	content, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(content)

	// Single IP should get /32 suffix appended
	if !strings.Contains(got, `"hbatest_restricted" 203.0.113.5/32 scram-sha-256`) {
		t.Errorf("expected /32 rule for bare IP, got:\n%s", got)
	}
	// CIDR stays unchanged
	if !strings.Contains(got, `"hbatest_restricted" 10.0.0.0/24 scram-sha-256`) {
		t.Errorf("expected CIDR rule, got:\n%s", got)
	}
}

func TestRebuildPgBouncerHBA_EmptyIPs_DefaultsReject(t *testing.T) {
	tmpPath := generateHBAContent(t)
	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()

	// Manually insert with empty allowed_ips slice so we can test reject fallback
	_, err := pool.Exec(ctx, "INSERT INTO managed_users (username, database_name, access, allowed_ips) VALUES ($1, $2, $3, $4)",
		"hbatest_blocked", "postgres", "read", []string{})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	hbaFilePath = tmpPath
	h.RebuildPgBouncerHBA()

	content, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(content)

	if !strings.Contains(got, `"hbatest_blocked" 0.0.0.0/0 reject`) {
		t.Errorf("expected reject rule for empty IPs, got:\n%s", got)
	}
}

func TestRebuildPgBouncerHBA_InternalRulesAlwaysPresent(t *testing.T) {
	tmpPath := generateHBAContent(t)
	pool := testPool(t)
	h := New(pool)

	hbaFilePath = tmpPath
	h.RebuildPgBouncerHBA()

	content, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
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
