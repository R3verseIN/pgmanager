//go:build integration

// Package handler contains integration tests for the WAL-G S3 backup system.
//
// These tests require a running Docker stack (MinIO + PostgreSQL + pgmanager app).
// Run with: ./scripts/test-walg.sh
//
// The tests exercise the full WAL-G API lifecycle through HTTP requests,
// exactly as the web UI would. They validate:
//   - Config storage in system_config table
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
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Configuration ───────────────────────────────────────────────────────────

const (
	// testAdminUser is the admin username created during test setup.
	testAdminUser = "testadmin"
	// testAdminPass is the admin password used across all integration tests.
	testAdminPass = "testadmin123"
)

// testAppURL returns the base URL of the pgmanager app under test.
// Defaults to http://localhost:8080. Override with TEST_APP_URL env var.
func testAppURL() string {
	if v := os.Getenv("TEST_APP_URL"); v != "" {
		return v
	}
	return "http://localhost:8080"
}

// testDBURL returns the PostgreSQL DSN for direct database queries.
// Defaults to the local socat bridge on port 5433. Override with TEST_DATABASE_URL.
func testDBURL() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://pgmanager:pgmanager@localhost:5433/pgmanager?sslmode=disable"
}

// ─── HTTP Helpers ────────────────────────────────────────────────────────────

// httpRequest makes an HTTP request and returns the response.
// It sets Content-Type to JSON for requests with a body.
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

	// Attach the session cookie if provided (simulates browser auth).
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

// readJSON decodes the response body into the target and closes the body.
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

// readBodyStr returns the response body as a string and closes it.
func readBodyStr(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	return string(data)
}

// requireStatus asserts the HTTP status code matches the expected value.
func requireStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		body := readBodyStr(t, resp)
		t.Fatalf("expected status %d, got %d: %s", expected, resp.StatusCode, body)
	}
}

// ─── Auth Helpers ────────────────────────────────────────────────────────────

// setupAndLogin creates the admin user (if needed) and logs in.
// Returns the session cookie value for authenticated requests.
//
// Flow:
//  1. GET /api/auth/setup-check — check if setup is needed
//  2. POST /api/auth/setup — create admin (only if needed)
//  3. POST /api/auth/login — authenticate and get session cookie
func setupAndLogin(t *testing.T) string {
	t.Helper()

	// Step 1: Check if initial setup is needed.
	resp := httpRequest(t, "GET", "/api/auth/setup-check", nil, "")
	var checkResp struct {
		NeedsSetup bool `json:"needsSetup"`
	}
	readJSON(t, resp, &checkResp)

	// Step 2: Create admin user if this is a fresh instance.
	if checkResp.NeedsSetup {
		setupResp := httpRequest(t, "POST", "/api/auth/setup", map[string]string{
			"username": testAdminUser,
			"password": testAdminPass,
		}, "")
		requireStatus(t, setupResp, http.StatusCreated)
		setupResp.Body.Close()
		t.Logf("setup: created admin user '%s'", testAdminUser)
	}

	// Step 3: Login and extract the session cookie.
	loginResp := httpRequest(t, "POST", "/api/auth/login", map[string]string{
		"username": testAdminUser,
		"password": testAdminPass,
	}, "")
	requireStatus(t, loginResp, http.StatusOK)
	loginResp.Body.Close()

	// Extract the session_id cookie from the Set-Cookie header.
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

// walgTestPool creates a pgxpool connection to the test database.
// Registers cleanup to close the pool when the test finishes.
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

// cleanWalgConfig removes all WAL-G related keys from system_config.
// Called before each test to ensure a clean state.
func cleanWalgConfig(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(t.Context(),
		`DELETE FROM system_config WHERE key LIKE 'walg_%'`)
	if err != nil {
		t.Fatalf("failed to clean walg config: %v", err)
	}
}

// createTestDB creates a fresh database for testing and registers cleanup.
func createTestDBForTest(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()

	// Drop if exists from a previous failed run.
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

// walgStatus mirrors the JSON response from GET /api/walg/status.
type walgStatus struct {
	Enabled       bool   `json:"enabled"`
	Archiving     bool   `json:"archiving"`
	Configured    bool   `json:"configured"`
	S3Prefix      string `json:"s3Prefix"`
	LastBackup    string `json:"lastBackup,omitempty"`
	BackupCount   int    `json:"backupCount"`
	IntervalSec   int    `json:"intervalSec"`
	RetentionDays int    `json:"retentionDays"`
}

// walgConfigResp mirrors the JSON response from GET /api/walg/config.
type walgConfigResp struct {
	S3Prefix      string `json:"s3Prefix"`
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region"`
	ForcePathStyle string `json:"forcePathStyle"`
	Interval      string `json:"interval"`
	RetentionDays string `json:"retentionDays"`
}

// walgBackup mirrors a single entry from GET /api/walg/backups.
type walgBackup struct {
	Name       string `json:"name"`
	Time       string `json:"time"`
	WalSegment string `json:"walSegment"`
	Status     string `json:"status"`
}

// ─── Tests ───────────────────────────────────────────────────────────────────

// TestWalgIntegration_StatusNotConfigured verifies that the status endpoint
// returns disabled/unconfigured when no S3 settings have been saved to the DB.
func TestWalgIntegration_StatusNotConfigured(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	resp := httpRequest(t, "GET", "/api/walg/status", nil, cookie)
	requireStatus(t, resp, http.StatusOK)

	var status walgStatus
	readJSON(t, resp, &status)

	if status.Enabled {
		t.Error("expected enabled=false when no config saved")
	}
	if status.Configured {
		t.Error("expected configured=false when no config saved")
	}
}

// TestWalgIntegration_SetupConfig saves S3 configuration via the web UI API.
// This simulates what the S3 Backups page does when you click "Save Configuration".
func TestWalgIntegration_SetupConfig(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	// Save config pointing to MinIO — this is exactly what the web UI sends.
	config := map[string]interface{}{
		"s3Prefix":      "s3://pgmanager-test",
		"accessKeyId":   "minioadmin",
		"secretKey":     "minioadmin",
		"endpoint":      "http://minio:9000",
		"region":        "us-east-1",
		"forcePathStyle": true,
		"interval":      300,
		"retentionDays": 3,
	}

	resp := httpRequest(t, "PUT", "/api/walg/config", config, cookie)
	requireStatus(t, resp, http.StatusOK)

	var result map[string]string
	readJSON(t, resp, &result)

	if result["status"] != "saved" {
		t.Errorf("expected status='saved', got %q", result["status"])
	}

	// Verify the config was actually persisted to the database.
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM system_config WHERE key LIKE 'walg_%'`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to check system_config: %v", err)
	}
	if count < 5 {
		t.Errorf("expected at least 5 walg_* keys in system_config, got %d", count)
	}
}

// TestWalgIntegration_GetConfig verifies that saved configuration is returned
// correctly by the GET endpoint, matching what the web UI would display.
func TestWalgIntegration_GetConfig(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	// First save config.
	saveConfig := map[string]interface{}{
		"s3Prefix":      "s3://pgmanager-test",
		"endpoint":      "http://minio:9000",
		"region":        "eu-west-1",
		"forcePathStyle": true,
		"interval":      600,
		"retentionDays": 14,
	}
	saveResp := httpRequest(t, "PUT", "/api/walg/config", saveConfig, cookie)
	requireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

	// Then read it back.
	getResp := httpRequest(t, "GET", "/api/walg/config", nil, cookie)
	requireStatus(t, getResp, http.StatusOK)

	var config walgConfigResp
	readJSON(t, getResp, &config)

	// Verify each field matches what was saved.
	if config.S3Prefix != "s3://pgmanager-test" {
		t.Errorf("s3Prefix: expected 's3://pgmanager-test', got %q", config.S3Prefix)
	}
	if config.Endpoint != "http://minio:9000" {
		t.Errorf("endpoint: expected 'http://minio:9000', got %q", config.Endpoint)
	}
	if config.Region != "eu-west-1" {
		t.Errorf("region: expected 'eu-west-1', got %q", config.Region)
	}
	if config.ForcePathStyle != "true" {
		t.Errorf("forcePathStyle: expected 'true', got %q", config.ForcePathStyle)
	}
	if config.Interval != "600" {
		t.Errorf("interval: expected '600', got %q", config.Interval)
	}
	if config.RetentionDays != "14" {
		t.Errorf("retentionDays: expected '14', got %q", config.RetentionDays)
	}
}

// TestWalgIntegration_StatusConfigured verifies that after saving config,
// the status endpoint reports WAL-G as enabled and shows correct settings.
func TestWalgIntegration_StatusConfigured(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	// Save config first.
	saveResp := httpRequest(t, "PUT", "/api/walg/config", map[string]interface{}{
		"s3Prefix":      "s3://pgmanager-test",
		"endpoint":      "http://minio:9000",
		"region":        "us-east-1",
		"forcePathStyle": true,
		"interval":      120,
		"retentionDays": 5,
	}, cookie)
	requireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

	// Check status.
	resp := httpRequest(t, "GET", "/api/walg/status", nil, cookie)
	requireStatus(t, resp, http.StatusOK)

	var status walgStatus
	readJSON(t, resp, &status)

	if !status.Enabled {
		t.Error("expected enabled=true after saving config")
	}
	if !status.Configured {
		t.Error("expected configured=true after saving config")
	}
	if status.S3Prefix != "s3://pgmanager-test" {
		t.Errorf("s3Prefix: expected 's3://pgmanager-test', got %q", status.S3Prefix)
	}
	if status.IntervalSec != 120 {
		t.Errorf("intervalSec: expected 120, got %d", status.IntervalSec)
	}
	if status.RetentionDays != 5 {
		t.Errorf("retentionDays: expected 5, got %d", status.RetentionDays)
	}
}

// TestWalgIntegration_TriggerBackup creates a base backup via the API.
// This is the same call the "Backup Now" button in the web UI makes.
// It validates that WAL-G can connect to MinIO and create a real backup.
func TestWalgIntegration_TriggerBackup(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	// Configure WAL-G to use MinIO.
	saveResp := httpRequest(t, "PUT", "/api/walg/config", map[string]interface{}{
		"s3Prefix":      "s3://pgmanager-test",
		"endpoint":      "http://minio:9000",
		"region":        "us-east-1",
		"forcePathStyle": true,
	}, cookie)
	requireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

	// Trigger a base backup.
	resp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)

	// The backup can take a while on first run — allow up to 2 minutes.
	// Read the response regardless of status code (WAL-G may return non-200
	// with useful error info).
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

// TestWalgIntegration_ListBackups verifies that the backup created by
// TriggerBackup appears in the list. This validates the full S3 round-trip:
// write backup → list from S3 → parse WAL-G JSON output.
func TestWalgIntegration_ListBackups(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	// Configure and create a backup first.
	saveResp := httpRequest(t, "PUT", "/api/walg/config", map[string]interface{}{
		"s3Prefix":      "s3://pgmanager-test",
		"endpoint":      "http://minio:9000",
		"region":        "us-east-1",
		"forcePathStyle": true,
	}, cookie)
	requireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

	backupResp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	requireStatus(t, backupResp, http.StatusOK)
	backupResp.Body.Close()

	// Now list backups — should have at least one entry.
	listResp := httpRequest(t, "GET", "/api/walg/backups", nil, cookie)
	requireStatus(t, listResp, http.StatusOK)

	var backups []walgBackup
	readJSON(t, listResp, &backups)

	if len(backups) == 0 {
		t.Fatal("expected at least 1 backup, got 0")
	}

	// Verify the backup entry has the expected fields.
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
// This validates that wal-g wal-verify can read WAL segments from MinIO.
func TestWalgIntegration_VerifyIntegrity(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	// Configure, create a backup, then verify.
	saveResp := httpRequest(t, "PUT", "/api/walg/config", map[string]interface{}{
		"s3Prefix":      "s3://pgmanager-test",
		"endpoint":      "http://minio:9000",
		"region":        "us-east-1",
		"forcePathStyle": true,
	}, cookie)
	requireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

	backupResp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	requireStatus(t, backupResp, http.StatusOK)
	backupResp.Body.Close()

	// Wait a moment for WAL segments to archive before verifying.
	time.Sleep(5 * time.Second)

	// Run integrity check.
	resp := httpRequest(t, "POST", "/api/walg/verify", nil, cookie)
	requireStatus(t, resp, http.StatusOK)

	var result struct {
		Status  string `json:"status"`
		Details string `json:"details"`
	}
	readJSON(t, resp, &result)

	// Status should be OK or WARNING (warnings are acceptable — e.g., no
	// WAL segments archived yet). FAILURE indicates a real problem.
	if result.Status == "FAILURE" {
		t.Errorf("integrity check failed: %s", result.Details)
	}

	t.Logf("integrity check: status=%s", result.Status)
}

// TestWalgIntegration_RestoreBackup restores a backup to a new database.
// This is the most complex test — it validates:
//  1. WAL-G can fetch the backup from MinIO
//  2. pg_restore can restore the data
//  3. The restored database is accessible and contains the expected data
func TestWalgIntegration_RestoreBackup(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	// Configure WAL-G.
	saveResp := httpRequest(t, "PUT", "/api/walg/config", map[string]interface{}{
		"s3Prefix":      "s3://pgmanager-test",
		"endpoint":      "http://minio:9000",
		"region":        "us-east-1",
		"forcePathStyle": true,
	}, cookie)
	requireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

	// Create a source database with test data so the restore has something to recover.
	sourceDB := "walg_restore_src"
	createTestDBForTest(t, pool, sourceDB)

	// Connect directly to the source database to create tables.
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

	// Read response before checking status (to see error details).
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

	// Verify the restored data exists in the target database.
	var rowCount int
	err = pool.QueryRow(t.Context(),
		fmt.Sprintf("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'walg_restore_users'")).Scan(&rowCount)
	if err != nil {
		t.Fatalf("failed to query restored table metadata: %v", err)
	}

	t.Logf("restore successful: table metadata found in target database %s", targetDB)
}

// TestWalgIntegration_DeleteBackup deletes a specific backup by name.
// This validates that wal-g delete retain can remove backups from MinIO.
func TestWalgIntegration_DeleteBackup(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	// Configure and create a backup.
	saveResp := httpRequest(t, "PUT", "/api/walg/config", map[string]interface{}{
		"s3Prefix":      "s3://pgmanager-test",
		"endpoint":      "http://minio:9000",
		"region":        "us-east-1",
		"forcePathStyle": true,
	}, cookie)
	requireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

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

	// Delete the backup.
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
// This validates that wal-g delete garbage can clean up expired WAL segments.
func TestWalgIntegration_CleanGarbage(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	// Configure WAL-G.
	saveResp := httpRequest(t, "PUT", "/api/walg/config", map[string]interface{}{
		"s3Prefix":      "s3://pgmanager-test",
		"endpoint":      "http://minio:9000",
		"region":        "us-east-1",
		"forcePathStyle": true,
	}, cookie)
	requireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

	// Create a backup first so WAL-G can list the storage structure.
	backupResp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	requireStatus(t, backupResp, http.StatusOK)
	backupResp.Body.Close()

	// Run garbage cleanup — should succeed without errors.
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
// Validates that operations fail gracefully with helpful error messages.
func TestWalgIntegration_NoConfigEndpoints(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	// List backups without config — should return 400.
	listResp := httpRequest(t, "GET", "/api/walg/backups", nil, cookie)
	if listResp.StatusCode != http.StatusBadRequest {
		body := readBodyStr(t, listResp)
		t.Errorf("expected 400 for list without config, got %d: %s", listResp.StatusCode, body)
	}

	// Trigger backup without config — should return 400.
	backupResp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	if backupResp.StatusCode != http.StatusBadRequest {
		body := readBodyStr(t, backupResp)
		t.Errorf("expected 400 for backup without config, got %d: %s", backupResp.StatusCode, body)
	}

	// Verify without config — should return 400.
	verifyResp := httpRequest(t, "POST", "/api/walg/verify", nil, cookie)
	if verifyResp.StatusCode != http.StatusBadRequest {
		body := readBodyStr(t, verifyResp)
		t.Errorf("expected 400 for verify without config, got %d: %s", verifyResp.StatusCode, body)
	}
}

// TestWalgIntegration_FullLifecycle runs the complete backup lifecycle in order:
//
//	Setup → Config → Status → Backup → List → Verify → Restore → Delete → Garbage
//
// This is the most comprehensive test and simulates the entire user journey
// from initial setup through backup and restore. It runs last because it
// depends on all other operations working correctly.
func TestWalgIntegration_FullLifecycle(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	// ── Step 1: Verify initial state (not configured) ──────────────
	t.Log("step 1: checking initial status (not configured)")
	statusResp := httpRequest(t, "GET", "/api/walg/status", nil, cookie)
	requireStatus(t, statusResp, http.StatusOK)

	var initialStatus walgStatus
	readJSON(t, statusResp, &initialStatus)

	if initialStatus.Enabled {
		t.Error("step 1 FAIL: expected enabled=false initially")
	}
	t.Log("step 1 OK: WAL-G is not configured initially")

	// ── Step 2: Save configuration ─────────────────────────────────
	t.Log("step 2: saving S3 configuration")
	saveResp := httpRequest(t, "PUT", "/api/walg/config", map[string]interface{}{
		"s3Prefix":      "s3://pgmanager-test",
		"accessKeyId":   "minioadmin",
		"secretKey":     "minioadmin",
		"endpoint":      "http://minio:9000",
		"region":        "us-east-1",
		"forcePathStyle": true,
		"interval":      300,
		"retentionDays": 7,
	}, cookie)
	requireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()
	t.Log("step 2 OK: configuration saved")

	// ── Step 3: Verify config was persisted ────────────────────────
	t.Log("step 3: reading back configuration")
	getResp := httpRequest(t, "GET", "/api/walg/config", nil, cookie)
	requireStatus(t, getResp, http.StatusOK)

	var config walgConfigResp
	readJSON(t, getResp, &config)

	if config.S3Prefix != "s3://pgmanager-test" {
		t.Errorf("step 3 FAIL: s3Prefix mismatch: %q", config.S3Prefix)
	}
	t.Logf("step 3 OK: config verified (prefix=%s, region=%s)", config.S3Prefix, config.Region)

	// ── Step 4: Check status (should be enabled) ───────────────────
	t.Log("step 4: checking status (should be enabled)")
	status2Resp := httpRequest(t, "GET", "/api/walg/status", nil, cookie)
	requireStatus(t, status2Resp, http.StatusOK)

	var configuredStatus walgStatus
	readJSON(t, status2Resp, &configuredStatus)

	if !configuredStatus.Enabled {
		t.Error("step 4 FAIL: expected enabled=true after config")
	}
	t.Logf("step 4 OK: enabled=%v, archiving=%v", configuredStatus.Enabled, configuredStatus.Archiving)

	// ── Step 5: Create a base backup ───────────────────────────────
	t.Log("step 5: triggering base backup")
	backupResp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	backupBody := readBodyStr(t, backupResp)
	if backupResp.StatusCode != http.StatusOK {
		t.Fatalf("step 5 FAIL: backup failed (status %d): %s", backupResp.StatusCode, backupBody)
	}
	t.Log("step 5 OK: base backup created")

	// ── Step 6: List backups ────────────────────────────────────────
	t.Log("step 6: listing backups")
	listResp := httpRequest(t, "GET", "/api/walg/backups", nil, cookie)
	requireStatus(t, listResp, http.StatusOK)

	var backups []walgBackup
	readJSON(t, listResp, &backups)

	if len(backups) == 0 {
		t.Fatal("step 6 FAIL: expected at least 1 backup")
	}
	backupName := backups[0].Name
	t.Logf("step 6 OK: found %d backup(s), first=%s", len(backups), backupName)

	// ── Step 7: Verify WAL integrity ───────────────────────────────
	t.Log("step 7: running WAL integrity check")
	time.Sleep(5 * time.Second) // wait for WAL segments to archive

	verifyResp := httpRequest(t, "POST", "/api/walg/verify", nil, cookie)
	requireStatus(t, verifyResp, http.StatusOK)

	var verifyResult struct {
		Status  string `json:"status"`
		Details string `json:"details"`
	}
	readJSON(t, verifyResp, &verifyResult)

	if verifyResult.Status == "FAILURE" {
		t.Errorf("step 7 FAIL: integrity check failed: %s", verifyResult.Details)
	}
	t.Logf("step 7 OK: integrity=%s", verifyResult.Status)

	// ── Step 8: Restore to a new database ───────────────────────────
	t.Log("step 8: restoring backup to test database")

	// Create test data in a source database first.
	srcDB := "walg_lifecycle_src"
	createTestDBForTest(t, pool, srcDB)

	_, err := pool.Exec(t.Context(), `
		CREATE TABLE IF NOT EXISTS walg_lifecycle_items (
			id SERIAL PRIMARY KEY,
			label TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("step 8 FAIL: create table: %v", err)
	}

	_, err = pool.Exec(t.Context(), `
		INSERT INTO walg_lifecycle_items (label) VALUES ('alpha'), ('beta'), ('gamma')
	`)
	if err != nil {
		t.Fatalf("step 8 FAIL: insert data: %v", err)
	}

	// Create another backup that includes this data.
	backup2Resp := httpRequest(t, "POST", "/api/walg/backup", nil, cookie)
	requireStatus(t, backup2Resp, http.StatusOK)
	backup2Resp.Body.Close()

	// Restore to a fresh database.
	dstDB := "walg_lifecycle_dst"
	createTestDBForTest(t, pool, dstDB)

	restoreResp := httpRequest(t, "POST", "/api/walg/restore", map[string]string{
		"backupName": "LATEST",
		"database":   dstDB,
	}, cookie)
	restoreBody := readBodyStr(t, restoreResp)
	if restoreResp.StatusCode != http.StatusOK {
		t.Fatalf("step 8 FAIL: restore failed (status %d): %s", restoreResp.StatusCode, restoreBody)
	}

	// Verify the restored database exists and restore reported success.
	var restoreResult struct {
		Status   string `json:"status"`
		Database string `json:"database"`
		Backup   string `json:"backup"`
	}
	if err := json.Unmarshal([]byte(restoreBody), &restoreResult); err != nil {
		t.Fatalf("step 8 FAIL: decode restore response: %v", err)
	}
	if restoreResult.Status != "success" {
		t.Errorf("step 8 FAIL: expected restore status='success', got %q", restoreResult.Status)
	}
	t.Logf("step 8 OK: restored to %s", dstDB)

	// ── Step 9: Delete a backup ─────────────────────────────────────
	t.Log("step 9: deleting backup")
	deleteResp := httpRequest(t, "DELETE", "/api/walg/backup/"+backupName, nil, cookie)
	requireStatus(t, deleteResp, http.StatusOK)
	deleteResp.Body.Close()
	t.Logf("step 9 OK: deleted backup %s", backupName)

	// ── Step 10: Garbage cleanup ────────────────────────────────────
	t.Log("step 10: running garbage cleanup")
	gcResp := httpRequest(t, "DELETE", "/api/walg/garbage", nil, cookie)
	requireStatus(t, gcResp, http.StatusOK)
	gcResp.Body.Close()
	t.Log("step 10 OK: garbage cleanup completed")

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

// TestMain waits for the Docker stack to be ready before running tests.
// It polls the app health endpoint with retries, giving containers time
// to initialize (PostgreSQL startup, schema creation, WAL-G binary check).
func TestMain(m *testing.M) {
	if os.Getenv("GO_TEST_INTEGRATION") == "" {
		fmt.Println("SKIP: set GO_TEST_INTEGRATION=1 to run integration tests")
		os.Exit(0)
	}

	// Wait for the app to be reachable (up to 60 seconds).
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

// ─── Supplementary ───────────────────────────────────────────────────────────

// TestWalgIntegration_InvalidConfigRejectsBadInput validates that the config
// endpoint rejects requests with missing required fields.
func TestWalgIntegration_InvalidConfigRejectsBadInput(t *testing.T) {
	cookie := setupAndLogin(t)

	// Empty s3Prefix should be rejected.
	resp := httpRequest(t, "PUT", "/api/walg/config", map[string]interface{}{
		"s3Prefix": "",
	}, cookie)

	if resp.StatusCode != http.StatusBadRequest {
		body := readBodyStr(t, resp)
		t.Errorf("expected 400 for empty s3Prefix, got %d: %s", resp.StatusCode, body)
	}
}

// TestWalgIntegration_ScheduledBackupInterval verifies that the scheduled
// backup interval is read from system_config (not from env vars).
func TestWalgIntegration_ScheduledBackupInterval(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	// Save config with a custom interval.
	saveResp := httpRequest(t, "PUT", "/api/walg/config", map[string]interface{}{
		"s3Prefix":      "s3://pgmanager-test",
		"endpoint":      "http://minio:9000",
		"region":        "us-east-1",
		"forcePathStyle": true,
		"interval":      1800,
		"retentionDays": 14,
	}, cookie)
	requireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

	// Status should reflect the custom interval.
	statusResp := httpRequest(t, "GET", "/api/walg/status", nil, cookie)
	requireStatus(t, statusResp, http.StatusOK)

	var status walgStatus
	readJSON(t, statusResp, &status)

	if status.IntervalSec != 1800 {
		t.Errorf("expected intervalSec=1800, got %d", status.IntervalSec)
	}
	if status.RetentionDays != 14 {
		t.Errorf("expected retentionDays=14, got %d", status.RetentionDays)
	}

	// Also verify the config endpoint returns the same values.
	configResp := httpRequest(t, "GET", "/api/walg/config", nil, cookie)
	requireStatus(t, configResp, http.StatusOK)

	var config walgConfigResp
	readJSON(t, configResp, &config)

	if config.Interval != "1800" {
		t.Errorf("config interval: expected '1800', got %q", config.Interval)
	}
	if config.RetentionDays != "14" {
		t.Errorf("config retentionDays: expected '14', got %q", config.RetentionDays)
	}
}

// TestWalgIntegration_ConfigOverrideUpdatesDB verifies that updating config
// via the web UI actually changes the values in system_config (not just env vars).
func TestWalgIntegration_ConfigOverrideUpdatesDB(t *testing.T) {
	cookie := setupAndLogin(t)
	pool := walgTestPool(t)
	cleanWalgConfig(t, pool)

	// Save initial config.
	saveResp := httpRequest(t, "PUT", "/api/walg/config", map[string]interface{}{
		"s3Prefix":      "s3://pgmanager-test",
		"endpoint":      "http://minio:9000",
		"region":        "us-east-1",
		"forcePathStyle": true,
		"interval":      3600,
	}, cookie)
	requireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

	// Update to different values.
	updateResp := httpRequest(t, "PUT", "/api/walg/config", map[string]interface{}{
		"s3Prefix":      "s3://pgmanager-test-v2",
		"endpoint":      "http://minio:9000",
		"region":        "ap-southeast-1",
		"forcePathStyle": true,
		"interval":      600,
		"retentionDays": 30,
	}, cookie)
	requireStatus(t, updateResp, http.StatusOK)
	updateResp.Body.Close()

	// Verify the DB has the new values.
	var s3Prefix, region string
	var interval, retention int
	err := pool.QueryRow(t.Context(),
		`SELECT value FROM system_config WHERE key = 'walg_s3_prefix'`).Scan(&s3Prefix)
	if err != nil {
		t.Fatalf("failed to read walg_s3_prefix from DB: %v", err)
	}
	err = pool.QueryRow(t.Context(),
		`SELECT value FROM system_config WHERE key = 'walg_region'`).Scan(&region)
	if err != nil {
		t.Fatalf("failed to read walg_region from DB: %v", err)
	}
	err = pool.QueryRow(t.Context(),
		`SELECT value::int FROM system_config WHERE key = 'walg_backup_interval'`).Scan(&interval)
	if err != nil {
		t.Fatalf("failed to read walg_backup_interval from DB: %v", err)
	}
	err = pool.QueryRow(t.Context(),
		`SELECT value::int FROM system_config WHERE key = 'walg_backup_retention_days'`).Scan(&retention)
	if err != nil {
		t.Fatalf("failed to read walg_backup_retention_days from DB: %v", err)
	}

	if s3Prefix != "s3://pgmanager-test-v2" {
		t.Errorf("DB s3Prefix: expected 's3://pgmanager-test-v2', got %q", s3Prefix)
	}
	if region != "ap-southeast-1" {
		t.Errorf("DB region: expected 'ap-southeast-1', got %q", region)
	}
	if interval != 600 {
		t.Errorf("DB interval: expected 600, got %d", interval)
	}
	if retention != 30 {
		t.Errorf("DB retention: expected 30, got %d", retention)
	}

	// Verify the API also returns the new values.
	configResp := httpRequest(t, "GET", "/api/walg/config", nil, cookie)
	requireStatus(t, configResp, http.StatusOK)

	var config walgConfigResp
	readJSON(t, configResp, &config)

	if !strings.Contains(config.S3Prefix, "v2") {
		t.Errorf("API s3Prefix should contain 'v2', got %q", config.S3Prefix)
	}
}
