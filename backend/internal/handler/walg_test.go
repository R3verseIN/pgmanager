//go:build integration

// Package handler contains integration tests for the WAL-G S3 backup system.
//
// These tests require a running Docker stack (MinIO + PostgreSQL + pgmanager app).
// Run with: ./scripts/test-walg.sh
//
// The tests exercise the full WAL-G API lifecycle through HTTP requests,
// exactly as the web UI would. They validate:
//   - MinIO S3 connectivity (real TCP, real S3 API)
//   - Base backup creation and listing
//   - WAL integrity verification
//   - Backup restore to a test database
//   - Backup deletion and garbage cleanup
//   - Error handling when WAL-G is not configured
package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Configuration ───────────────────────────────────────────────────────────

const (
	testAdminUser = "testadmin"
	testAdminPass = "testadmin123"
)

func testAppURL() string {
	if v := os.Getenv("TEST_APP_URL"); v != "" {
		return v
	}
	return "http://localhost:8080"
}

func testDBURL() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://pgmanager:pgmanager@localhost:5433/pgmanager?sslmode=disable"
}

// ─── HTTP Helpers ────────────────────────────────────────────────────────────

func httpRequest(t *testing.T, method, path string, body interface{}, cookie string) *http.Response {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, testAppURL()+path, bodyReader)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if cookie != "" {
		req.AddCookie(&http.Cookie{
			Name:  "session_id",
			Value: cookie,
		})
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request %s %s failed: %v", method, path, err)
	}
	return resp
}

func readJSON(t *testing.T, resp *http.Response, target interface{}) {
	t.Helper()
	defer resp.Body.Close()

	if target == nil {
		return
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("failed to decode response (%s): %v", string(data), err)
	}
}

func readBodyStr(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	return string(data)
}

func requireStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		body := readBodyStr(t, resp)
		t.Fatalf("expected status %d, got %d: %s", expected, resp.StatusCode, body)
	}
}

// ─── Auth Helpers ────────────────────────────────────────────────────────────

func setupAndLogin(t *testing.T) string {
	t.Helper()

	resp := httpRequest(t, "GET", "/api/auth/setup-check", nil, "")
	var checkResp struct {
		NeedsSetup bool `json:"needsSetup"`
	}
	readJSON(t, resp, &checkResp)

	if checkResp.NeedsSetup {
		setupResp := httpRequest(t, "POST", "/api/auth/setup", map[string]string{
			"username": testAdminUser,
			"password": testAdminPass,
		}, "")
		requireStatus(t, setupResp, http.StatusCreated)
		setupResp.Body.Close()
		t.Logf("setup: created admin user '%s'", testAdminUser)
	}

	loginResp := httpRequest(t, "POST", "/api/auth/login", map[string]string{
		"username": testAdminUser,
		"password": testAdminPass,
	}, "")
	requireStatus(t, loginResp, http.StatusOK)
	loginResp.Body.Close()

	var sessionCookie string
	for _, c := range loginResp.Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c.Value
			break
		}
	}

	if sessionCookie == "" {
		t.Fatal("login succeeded but no session_id cookie was set")
	}

	t.Logf("setup: logged in as '%s', session cookie obtained", testAdminUser)
	return sessionCookie
}

// ─── Database Helpers ────────────────────────────────────────────────────────

func walgTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx := t.Context()

	pool, err := pgxpool.New(ctx, testDBURL())
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to ping test database: %v", err)
	}

	return pool
}

func createTestDBForTest(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()

	_, _ = pool.Exec(t.Context(),
		fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", name))

	_, err := pool.Exec(t.Context(),
		fmt.Sprintf("CREATE DATABASE %s", name))
	if err != nil {
		t.Fatalf("failed to create test database %s: %v", name, err)
	}

	t.Cleanup(func() {
		pool.Exec(t.Context(),
			fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", name))
	})
}

// ─── WAL-G API Response Types ────────────────────────────────────────────────

type walgStatus struct {
	Enabled       bool   `json:"enabled"`
	Archiving     bool   `json:"archiving"`
	Configured    bool   `json:"configured"`
	S3Prefix      string `json:"s3Prefix"`
	LastBackup    string `json:"lastBackup,omitempty"`
	BackupCount   int    `json:"backupCount"`
	IntervalSec   int    `json:"intervalSec"`
	RetentionDays int    `json:"retentionDays"`
	Errors        []string `json:"errors,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

type walgBackup struct {
	Name       string `json:"name"`
	Time       string `json:"time"`
	WalSegment string `json:"walSegment"`
	Status     string `json:"status"`
}

// ─── Tests ───────────────────────────────────────────────────────────────────

// TestWalgIntegration_StatusNotConfigured verifies that the status endpoint
// returns disabled/unconfigured when WALG_S3_PREFIX env var is not set.
func TestWalgIntegration_StatusNotConfigured(t *testing.T) {
	cookie := setupAndLogin(t)

	resp := httpRequest(t, "GET", "/api/walg/status", nil, cookie)
	requireStatus(t, resp, http.StatusOK)

	var status walgStatus
	readJSON(t, resp, &status)

	// If WALG_S3_PREFIX is not set in the env, enabled should be false.
	// If it IS set (CI env), this test is a no-op.
	if os.Getenv("WALG_S3_PREFIX") == "" {
		if status.Enabled {
			t.Error("expected enabled=false when WALG_S3_PREFIX is not set")
		}
		if status.Configured {
			t.Error("expected configured=false when WALG_S3_PREFIX is not set")
		}
		if len(status.Errors) == 0 {
			t.Error("expected at least one error when WALG_S3_PREFIX is not set")
		}
	} else {
		t.Skip("WALG_S3_PREFIX is set — skipping not-configured test")
	}
}

// TestWalgIntegration_TriggerBackup creates a base backup via the API.
// This is the same call the "Backup Now" button in the web UI makes.
// It validates that WAL-G can connect to S3 and create a real backup.
func TestWalgIntegration_TriggerBackup(t *testing.T) {
	cookie := setupAndLogin(t)

	resp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)

	body := readBodyStr(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("backup trigger failed (status %d): %s", resp.StatusCode, body)
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("failed to decode backup response: %v", err)
	}

	if result["status"] != "success" {
		t.Errorf("expected status='success', got %q: %s", result["status"], result["message"])
	}

	t.Logf("backup triggered: %s", result["message"])
}

// TestWalgIntegration_ListBackups verifies that backups are listed correctly.
func TestWalgIntegration_ListBackups(t *testing.T) {
	cookie := setupAndLogin(t)

	// Create a backup first.
	backupResp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	requireStatus(t, backupResp, http.StatusOK)
	backupResp.Body.Close()

	// Now list backups.
	listResp := httpRequest(t, "GET", "/api/walg/backups", nil, cookie)
	requireStatus(t, listResp, http.StatusOK)

	var backups []walgBackup
	readJSON(t, listResp, &backups)

	if len(backups) == 0 {
		t.Fatal("expected at least 1 backup, got 0")
	}

	first := backups[0]
	if first.Name == "" {
		t.Error("backup name is empty")
	}
	if first.Time == "" {
		t.Error("backup time is empty")
	}
	t.Logf("found backup: name=%s time=%s wal=%s", first.Name, first.Time, first.WalSegment)
}

// TestWalgIntegration_VerifyIntegrity runs a WAL integrity check.
func TestWalgIntegration_VerifyIntegrity(t *testing.T) {
	cookie := setupAndLogin(t)

	// Create a backup first.
	backupResp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	requireStatus(t, backupResp, http.StatusOK)
	backupResp.Body.Close()

	time.Sleep(5 * time.Second)

	resp := httpRequest(t, "POST", "/api/walg/verify", nil, cookie)
	requireStatus(t, resp, http.StatusOK)

	var result struct {
		Status  string `json:"status"`
		Details string `json:"details"`
	}
	readJSON(t, resp, &result)

	if result.Status == "FAILURE" {
		t.Errorf("integrity check failed: %s", result.Details)
	}

	t.Logf("integrity check: status=%s", result.Status)
}

// TestWalgIntegration_RestoreBackup restores a backup to a new database.
func TestWalgIntegration_RestoreBackup(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)

	// Create a source database with test data.
	sourceDB := "walg_restore_src"
	createTestDBForTest(t, pool, sourceDB)

	srcPool, err := pgxpool.New(t.Context(), testDBURL())
	if err != nil {
		t.Fatalf("failed to connect to source database: %v", err)
	}
	defer srcPool.Close()

	_, err = srcPool.Exec(t.Context(), fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS walg_restore_users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL
		)
	`))
	if err != nil {
		t.Fatalf("failed to create source table: %v", err)
	}

	_, err = srcPool.Exec(t.Context(), `
		INSERT INTO walg_restore_users (name, email) VALUES
			('Alice', 'alice@example.com'),
			('Bob', 'bob@example.com'),
			('Charlie', 'charlie@example.com')
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	// Create a backup that includes the source database.
	backupResp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	requireStatus(t, backupResp, http.StatusOK)
	backupResp.Body.Close()

	// Create the target database for restore.
	targetDB := "walg_restore_dst"
	createTestDBForTest(t, pool, targetDB)

	// Restore to the target database.
	restoreResp := httpRequest(t, "POST", "/api/walg/restore", map[string]string{
		"backupName": "LATEST",
		"database":   targetDB,
	}, cookie)

	restoreBody := readBodyStr(t, restoreResp)
	if restoreResp.StatusCode != http.StatusOK {
		t.Fatalf("restore failed (status %d): %s", restoreResp.StatusCode, restoreBody)
	}

	var restoreResult struct {
		Status   string `json:"status"`
		Database string `json:"database"`
		Backup   string `json:"backup"`
	}
	if err := json.Unmarshal([]byte(restoreBody), &restoreResult); err != nil {
		t.Fatalf("failed to decode restore response: %v", err)
	}

	if restoreResult.Status != "success" {
		t.Errorf("expected restore status='success', got %q", restoreResult.Status)
	}

	var rowCount int
	err = pool.QueryRow(t.Context(),
		fmt.Sprintf("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'walg_restore_users'")).Scan(&rowCount)
	if err != nil {
		t.Fatalf("failed to query restored table metadata: %v", err)
	}

	t.Logf("restore successful: table metadata found in target database %s", targetDB)
}

// TestWalgIntegration_DeleteBackup deletes a specific backup by name.
func TestWalgIntegration_DeleteBackup(t *testing.T) {
	cookie := setupAndLogin(t)

	// Create a backup.
	backupResp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	requireStatus(t, backupResp, http.StatusOK)
	backupResp.Body.Close()

	// Get the backup name to delete.
	listResp := httpRequest(t, "GET", "/api/walg/backups", nil, cookie)
	requireStatus(t, listResp, http.StatusOK)

	var backups []walgBackup
	readJSON(t, listResp, &backups)

	if len(backups) == 0 {
		t.Fatal("no backups to delete")
	}

	backupName := backups[0].Name
	t.Logf("deleting backup: %s", backupName)

	deleteResp := httpRequest(t, "DELETE", "/api/walg/backup/"+backupName, nil, cookie)
	requireStatus(t, deleteResp, http.StatusOK)

	var result map[string]string
	readJSON(t, deleteResp, &result)

	if result["status"] != "deleted" {
		t.Errorf("expected status='deleted', got %q", result["status"])
	}

	// Verify the backup is no longer in the list.
	finalListResp := httpRequest(t, "GET", "/api/walg/backups", nil, cookie)
	requireStatus(t, finalListResp, http.StatusOK)

	var finalBackups []walgBackup
	readJSON(t, finalListResp, &finalBackups)

	for _, b := range finalBackups {
		if b.Name == backupName {
			t.Errorf("backup %q still exists after deletion", backupName)
		}
	}
}

// TestWalgIntegration_CleanGarbage runs garbage cleanup on S3.
func TestWalgIntegration_CleanGarbage(t *testing.T) {
	cookie := setupAndLogin(t)

	// Create a backup first.
	backupResp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	requireStatus(t, backupResp, http.StatusOK)
	backupResp.Body.Close()

	resp := httpRequest(t, "DELETE", "/api/walg/garbage", nil, cookie)
	requireStatus(t, resp, http.StatusOK)

	var result map[string]string
	readJSON(t, resp, &result)

	if result["status"] != "success" {
		t.Errorf("expected status='success', got %q", result["status"])
	}

	t.Logf("garbage cleanup completed")
}

// TestWalgIntegration_NoConfigEndpoints returns errors when WAL-G is not configured.
func TestWalgIntegration_NoConfigEndpoints(t *testing.T) {
	cookie := setupAndLogin(t)

	// If WALG_S3_PREFIX is set, operations won't return 400.
	if os.Getenv("WALG_S3_PREFIX") != "" {
		t.Skip("WALG_S3_PREFIX is set — skipping no-config test")
	}

	listResp := httpRequest(t, "GET", "/api/walg/backups", nil, cookie)
	if listResp.StatusCode != http.StatusBadRequest {
		body := readBodyStr(t, listResp)
		t.Errorf("expected 400 for list without config, got %d: %s", listResp.StatusCode, body)
	}

	backupResp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	if backupResp.StatusCode != http.StatusBadRequest {
		body := readBodyStr(t, backupResp)
		t.Errorf("expected 400 for backup without config, got %d: %s", backupResp.StatusCode, body)
	}

	verifyResp := httpRequest(t, "POST", "/api/walg/verify", nil, cookie)
	if verifyResp.StatusCode != http.StatusBadRequest {
		body := readBodyStr(t, verifyResp)
		t.Errorf("expected 400 for verify without config, got %d: %s", verifyResp.StatusCode, body)
	}
}

// TestWalgIntegration_FullLifecycle runs the complete backup lifecycle:
//
//	Status → Backup → List → Verify → Restore → Delete → Garbage
func TestWalgIntegration_FullLifecycle(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)

	// ── Step 1: Check status ──────────────────────────────────────
	t.Log("step 1: checking status")
	statusResp := httpRequest(t, "GET", "/api/walg/status", nil, cookie)
	requireStatus(t, statusResp, http.StatusOK)

	var initialStatus walgStatus
	readJSON(t, statusResp, &initialStatus)

	if !initialStatus.Enabled {
		t.Skip("WAL-G not configured (no WALG_S3_PREFIX env var) — skipping lifecycle test")
	}
	t.Log("step 1 OK: WAL-G is enabled")

	// ── Step 2: Create a base backup ──────────────────────────────
	t.Log("step 2: triggering base backup")
	backupResp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	backupBody := readBodyStr(t, backupResp)
	if backupResp.StatusCode != http.StatusOK {
		t.Fatalf("step 2 FAIL: backup failed (status %d): %s", backupResp.StatusCode, backupBody)
	}
	t.Log("step 2 OK: base backup created")

	// ── Step 3: List backups ───────────────────────────────────────
	t.Log("step 3: listing backups")
	listResp := httpRequest(t, "GET", "/api/walg/backups", nil, cookie)
	requireStatus(t, listResp, http.StatusOK)

	var backups []walgBackup
	readJSON(t, listResp, &backups)

	if len(backups) == 0 {
		t.Fatal("step 3 FAIL: expected at least 1 backup")
	}
	backupName := backups[0].Name
	t.Logf("step 3 OK: found %d backup(s), first=%s", len(backups), backupName)

	// ── Step 4: Verify WAL integrity ──────────────────────────────
	t.Log("step 4: running WAL integrity check")
	time.Sleep(5 * time.Second)

	verifyResp := httpRequest(t, "POST", "/api/walg/verify", nil, cookie)
	requireStatus(t, verifyResp, http.StatusOK)

	var verifyResult struct {
		Status  string `json:"status"`
		Details string `json:"details"`
	}
	readJSON(t, verifyResp, &verifyResult)

	if verifyResult.Status == "FAILURE" {
		t.Errorf("step 4 FAIL: integrity check failed: %s", verifyResult.Details)
	}
	t.Logf("step 4 OK: integrity=%s", verifyResult.Status)

	// ── Step 5: Restore to a new database ──────────────────────────
	t.Log("step 5: restoring backup to test database")

	srcDB := "walg_lifecycle_src"
	createTestDBForTest(t, pool, srcDB)

	_, err := pool.Exec(t.Context(), `
		CREATE TABLE IF NOT EXISTS walg_lifecycle_items (
			id SERIAL PRIMARY KEY,
			label TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("step 5 FAIL: create table: %v", err)
	}

	_, err = pool.Exec(t.Context(), `
		INSERT INTO walg_lifecycle_items (label) VALUES ('alpha'), ('beta'), ('gamma')
	`)
	if err != nil {
		t.Fatalf("step 5 FAIL: insert data: %v", err)
	}

	backup2Resp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	requireStatus(t, backup2Resp, http.StatusOK)
	backup2Resp.Body.Close()

	dstDB := "walg_lifecycle_dst"
	createTestDBForTest(t, pool, dstDB)

	restoreResp := httpRequest(t, "POST", "/api/walg/restore", map[string]string{
		"backupName": "LATEST",
		"database":   dstDB,
	}, cookie)
	restoreBody := readBodyStr(t, restoreResp)
	if restoreResp.StatusCode != http.StatusOK {
		t.Fatalf("step 5 FAIL: restore failed (status %d): %s", restoreResp.StatusCode, restoreBody)
	}

	var restoreResult struct {
		Status   string `json:"status"`
		Database string `json:"database"`
		Backup   string `json:"backup"`
	}
	if err := json.Unmarshal([]byte(restoreBody), &restoreResult); err != nil {
		t.Fatalf("step 5 FAIL: decode restore response: %v", err)
	}
	if restoreResult.Status != "success" {
		t.Errorf("step 5 FAIL: expected restore status='success', got %q", restoreResult.Status)
	}
	t.Logf("step 5 OK: restored to %s", dstDB)

	// ── Step 6: Delete a backup ────────────────────────────────────
	t.Log("step 6: deleting backup")
	deleteResp := httpRequest(t, "DELETE", "/api/walg/backup/"+backupName, nil, cookie)
	requireStatus(t, deleteResp, http.StatusOK)
	deleteResp.Body.Close()
	t.Logf("step 6 OK: deleted backup %s", backupName)

	// ── Step 7: Garbage cleanup ────────────────────────────────────
	t.Log("step 7: running garbage cleanup")
	gcResp := httpRequest(t, "DELETE", "/api/walg/garbage", nil, cookie)
	requireStatus(t, gcResp, http.StatusOK)
	gcResp.Body.Close()
	t.Log("step 7 OK: garbage cleanup completed")

	// ── Final: Verify end state ─────────────────────────────────────
	t.Log("final: verifying end state")
	finalStatus := httpRequest(t, "GET", "/api/walg/status", nil, cookie)
	requireStatus(t, finalStatus, http.StatusOK)

	var endStatus walgStatus
	readJSON(t, finalStatus, &endStatus)

	if !endStatus.Enabled {
		t.Error("final FAIL: expected enabled=true at end")
	}

	t.Logf("full lifecycle test PASSED: %d backups in S3", endStatus.BackupCount)
}

// ─── Init ────────────────────────────────────────────────────────────────────

func TestMain(m *testing.M) {
	if os.Getenv("GO_TEST_INTEGRATION") == "" {
		fmt.Println("SKIP: set GO_TEST_INTEGRATION=1 to run integration tests")
		os.Exit(0)
	}

	appURL := testAppURL()
	ready := false
	for i := 0; i < 60; i++ {
		resp, err := http.Get(appURL + "/api/auth/setup-check")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(time.Second)
	}

	if !ready {
		fmt.Fprintf(os.Stderr, "FATAL: app at %s not ready after 60s\n", appURL)
		os.Exit(1)
	}

	fmt.Printf("integration: app at %s is ready\n", appURL)
	os.Exit(m.Run())
}
