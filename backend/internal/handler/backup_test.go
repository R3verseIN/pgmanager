package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupBackupTest(t *testing.T) (*Handler, context.Context, string) {
	t.Helper()
	pool := testPool(t)
	h := New(pool)
	ctx := context.Background()
	if err := h.InitUserSchema(ctx); err != nil {
		t.Fatalf("InitUserSchema: %v", err)
	}

	// Set env vars for getPgCredentials() used by ListBackupTables, StreamBackup, etc.
	t.Setenv("PGHOST", "localhost")
	t.Setenv("PGPORT", "5433")
	t.Setenv("PGUSER", "pgmanager")

	// Create temp password file — must match actual DB password
	pw := "test1234"
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		// Extract password from TEST_DATABASE_URL
		if i := strings.Index(v, "://"); i >= 0 {
			rest := v[i+3:]
			if j := strings.Index(rest, "@"); j >= 0 {
				userInfo := rest[:j]
				if k := strings.Index(userInfo, ":"); k >= 0 {
					pw = userInfo[k+1:]
				}
			}
		}
	}
	tmpPW, err := os.CreateTemp("", "pg-pw-*.txt")
	if err != nil {
		t.Fatalf("create temp password file: %v", err)
	}
	tmpPW.WriteString(pw)
	tmpPW.Close()
	t.Setenv("SECRET_PATH", tmpPW.Name())
	t.Cleanup(func() { os.Remove(tmpPW.Name()) })

	testDB := "bktest_db"
	pool.Exec(ctx, "DROP DATABASE IF EXISTS "+testDB+" WITH (FORCE)")
	pool.Exec(ctx, "CREATE DATABASE "+testDB)

	// Connect to the test database to create tables (cross-db syntax not supported)
	testDSN := "postgres://pgmanager:" + pw + "@localhost:5433/" + testDB + "?sslmode=disable"
	testPool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("connect to test db: %v", err)
	}
	defer testPool.Close()

	testPool.Exec(ctx, `CREATE TABLE bktest_users (id SERIAL PRIMARY KEY, name TEXT)`)
	testPool.Exec(ctx, `INSERT INTO bktest_users (name) VALUES ('alice'), ('bob')`)
	testPool.Exec(ctx, `CREATE TABLE bktest_posts (id SERIAL PRIMARY KEY, title TEXT)`)
	testPool.Exec(ctx, `INSERT INTO bktest_posts (title) VALUES ('hello'), ('world')`)

	t.Cleanup(func() {
		pool.Exec(ctx, "DROP DATABASE IF EXISTS "+testDB+" WITH (FORCE)")
		pool.Exec(ctx, "DROP DATABASE IF EXISTS bktest_restore_target WITH (FORCE)")
	})

	return h, ctx, testDB
}

func connectToDB(t *testing.T, dbName string) *pgxpool.Pool {
	t.Helper()
	pw := "test1234"
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		if i := strings.Index(v, "://"); i >= 0 {
			rest := v[i+3:]
			if j := strings.Index(rest, "@"); j >= 0 {
				userInfo := rest[:j]
				if k := strings.Index(userInfo, ":"); k >= 0 {
					pw = userInfo[k+1:]
				}
			}
		}
	}
	dsn := "postgres://pgmanager:" + pw + "@localhost:5433/" + dbName + "?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to %s: %v", dbName, err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestListBackupDatabases(t *testing.T) {
	h, _, testDB := setupBackupTest(t)

	req := httptest.NewRequest("GET", "/api/backup/databases", nil)
	w := httptest.NewRecorder()

	h.ListBackupDatabases(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var databases []backupDatabaseEntry
	if err := json.NewDecoder(w.Body).Decode(&databases); err != nil {
		t.Fatalf("decode: %v", err)
	}

	found := false
	for _, db := range databases {
		if db.Name == testDB {
			found = true
		}
		if protectedDatabases[db.Name] {
			t.Errorf("system database %q should not appear in backup list", db.Name)
		}
	}
	if !found {
		t.Errorf("expected test database %q in list", testDB)
	}
}

func TestListBackupDatabases_ExcludesProtected(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	req := httptest.NewRequest("GET", "/api/backup/databases", nil)
	w := httptest.NewRecorder()

	h.ListBackupDatabases(w, req)

	var databases []backupDatabaseEntry
	json.NewDecoder(w.Body).Decode(&databases)

	protected := []string{"pgmanager", "postgres", "template0", "template1"}
	for _, p := range protected {
		for _, db := range databases {
			if db.Name == p {
				t.Errorf("protected database %q should not be in backup list", p)
			}
		}
	}
}

func TestListBackupTables(t *testing.T) {
	h, _, testDB := setupBackupTest(t)

	req := httptest.NewRequest("GET", "/api/backup/tables?db="+testDB, nil)
	w := httptest.NewRecorder()

	h.ListBackupTables(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result backupTableListResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if result.Database != testDB {
		t.Errorf("expected database %q, got %q", testDB, result.Database)
	}

	if len(result.Tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(result.Tables))
	}

	tableNames := make(map[string]bool)
	for _, table := range result.Tables {
		tableNames[table.Name] = true
		if table.Schema != "public" {
			t.Errorf("expected schema 'public', got %q for table %q", table.Schema, table.Name)
		}
	}

	if !tableNames["bktest_users"] {
		t.Error("expected table bktest_users")
	}
	if !tableNames["bktest_posts"] {
		t.Error("expected table bktest_posts")
	}
}

func TestListBackupTables_MissingDB(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	req := httptest.NewRequest("GET", "/api/backup/tables", nil)
	w := httptest.NewRecorder()

	h.ListBackupTables(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListBackupTables_InvalidName(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	req := httptest.NewRequest("GET", "/api/backup/tables?db=invalid;DROP", nil)
	w := httptest.NewRecorder()

	h.ListBackupTables(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStreamBackup_FullDatabase(t *testing.T) {
	h, _, testDB := setupBackupTest(t)

	body, _ := json.Marshal(backupCreateRequest{Database: testDB})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.StreamBackup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/octet-stream" {
		t.Errorf("expected Content-Type application/octet-stream, got %q", contentType)
	}

	disposition := w.Header().Get("Content-Disposition")
	if disposition == "" {
		t.Error("expected Content-Disposition header")
	}
	if !bytes.Contains([]byte(disposition), []byte(testDB)) {
		t.Errorf("expected filename to contain %q, got %q", testDB, disposition)
	}

	if w.Body.Len() == 0 {
		t.Error("expected non-empty backup body")
	}
}

func TestStreamBackup_SelectedTables(t *testing.T) {
	h, _, testDB := setupBackupTest(t)

	body, _ := json.Marshal(backupCreateRequest{
		Database: testDB,
		Tables:   []string{"bktest_users"},
	})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.StreamBackup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if w.Body.Len() == 0 {
		t.Error("expected non-empty backup body")
	}
}

func TestStreamBackup_EmptyDatabase(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	body, _ := json.Marshal(backupCreateRequest{Database: ""})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.StreamBackup(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStreamBackup_InvalidDatabaseName(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	body, _ := json.Marshal(backupCreateRequest{Database: "invalid;DROP"})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.StreamBackup(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStreamBackup_ProtectedDatabase(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	body, _ := json.Marshal(backupCreateRequest{Database: "pgmanager"})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.StreamBackup(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStreamBackup_NonexistentDatabase(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	body, _ := json.Marshal(backupCreateRequest{Database: "nonexistentdbxyz"})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.StreamBackup(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for nonexistent database, got %d: %s", w.Code, w.Body.String())
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

func TestStreamBackup_InvalidTableName(t *testing.T) {
	h, _, testDB := setupBackupTest(t)

	body, _ := json.Marshal(backupCreateRequest{
		Database: testDB,
		Tables:   []string{"valid_name", "invalid;DROP"},
	})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.StreamBackup(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInspectDump_ValidFile(t *testing.T) {
	h, _, testDB := setupBackupTest(t)

	// Create a backup first
	body, _ := json.Marshal(backupCreateRequest{Database: testDB})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.StreamBackup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("backup failed: %d: %s", w.Code, w.Body.String())
	}

	dumpData := w.Body.Bytes()

	// Now inspect it
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "test.dump")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part.Write(dumpData)
	writer.Close()

	inspectReq := httptest.NewRequest("POST", "/api/backup/inspect", &buf)
	inspectReq.Header.Set("Content-Type", writer.FormDataContentType())
	inspectW := httptest.NewRecorder()

	h.InspectDump(inspectW, inspectReq)

	if inspectW.Code != http.StatusOK {
		t.Fatalf("inspect: expected 200, got %d: %s", inspectW.Code, inspectW.Body.String())
	}

	var result backupInspectResponse
	if err := json.NewDecoder(inspectW.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if result.Format != "custom" {
		t.Errorf("expected format 'custom', got %q", result.Format)
	}

	if len(result.Tables) == 0 {
		t.Error("expected at least one table in inspect result")
	}
}

func TestInspectDump_MissingFile(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/backup/inspect", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	h.InspectDump(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInspectDump_InvalidFile(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "bad.dump")
	part.Write([]byte("this is not a valid pg_dump file"))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/backup/inspect", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	h.InspectDump(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRestoreBackup_FullRestore(t *testing.T) {
	h, ctx, testDB := setupBackupTest(t)

	// Create backup
	body, _ := json.Marshal(backupCreateRequest{Database: testDB})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.StreamBackup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("backup failed: %d: %s", w.Code, w.Body.String())
	}

	dumpData := w.Body.Bytes()

	// Create restore target database
	h.pool.Exec(ctx, "DROP DATABASE IF EXISTS bktest_restore_target WITH (FORCE)")
	h.pool.Exec(ctx, "CREATE DATABASE bktest_restore_target")

	// Restore
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "test.dump")
	part.Write(dumpData)
	writer.WriteField("database", "bktest_restore_target")
	writer.Close()

	restoreReq := httptest.NewRequest("POST", "/api/backup/restore", &buf)
	restoreReq.Header.Set("Content-Type", writer.FormDataContentType())
	restoreW := httptest.NewRecorder()

	h.RestoreBackup(restoreW, restoreReq)

	if restoreW.Code != http.StatusOK {
		t.Fatalf("restore: expected 200, got %d: %s", restoreW.Code, restoreW.Body.String())
	}

	var result map[string]any
	if err := json.NewDecoder(restoreW.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result["success"])
	}

	// Verify tables exist in restored database
	targetPool := connectToDB(t, "bktest_restore_target")
	var tableCount int
	targetPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public'
	`).Scan(&tableCount)

	if tableCount < 2 {
		t.Errorf("expected at least 2 tables in restored database, got %d", tableCount)
	}
}

func TestRestoreBackup_DropFirst(t *testing.T) {
	h, ctx, testDB := setupBackupTest(t)

	// Create backup
	body, _ := json.Marshal(backupCreateRequest{Database: testDB})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.StreamBackup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("backup failed: %d: %s", w.Code, w.Body.String())
	}

	dumpData := w.Body.Bytes()

	// Create restore target with existing table
	h.pool.Exec(ctx, "DROP DATABASE IF EXISTS bktest_restore_target WITH (FORCE)")
	h.pool.Exec(ctx, "CREATE DATABASE bktest_restore_target")
	targetPool := connectToDB(t, "bktest_restore_target")
	targetPool.Exec(ctx, `CREATE TABLE existing_table (id SERIAL PRIMARY KEY)`)

	// Restore with dropFirst
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "test.dump")
	part.Write(dumpData)
	writer.WriteField("database", "bktest_restore_target")
	writer.WriteField("dropFirst", "true")
	writer.Close()

	restoreReq := httptest.NewRequest("POST", "/api/backup/restore", &buf)
	restoreReq.Header.Set("Content-Type", writer.FormDataContentType())
	restoreW := httptest.NewRecorder()

	h.RestoreBackup(restoreW, restoreReq)

	if restoreW.Code != http.StatusOK {
		t.Fatalf("restore: expected 200, got %d: %s", restoreW.Code, restoreW.Body.String())
	}

	// Verify old table is gone
	var exists bool
	targetPool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'existing_table'
		)
	`).Scan(&exists)

	if exists {
		t.Error("existing_table should have been dropped before restore")
	}

	// Verify new tables exist
	var tableCount int
	targetPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public'
	`).Scan(&tableCount)

	if tableCount < 2 {
		t.Errorf("expected at least 2 tables after restore, got %d", tableCount)
	}
}

func TestRestoreBackup_MissingDatabase(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("database", "")
	writer.Close()

	req := httptest.NewRequest("POST", "/api/backup/restore", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	h.RestoreBackup(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRestoreBackup_InvalidDatabaseName(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("database", "invalid;DROP")
	writer.Close()

	req := httptest.NewRequest("POST", "/api/backup/restore", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	h.RestoreBackup(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRestoreBackup_ProtectedDatabase(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("database", "pgmanager")
	writer.Close()

	req := httptest.NewRequest("POST", "/api/backup/restore", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	h.RestoreBackup(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRestoreBackup_MissingFile(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("database", "sometarget")
	writer.Close()

	req := httptest.NewRequest("POST", "/api/backup/restore", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	h.RestoreBackup(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetPgCredentials(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	host, port, user, _ := h.getPgCredentials()

	if host == "" {
		t.Error("expected non-empty host")
	}
	if port == "" {
		t.Error("expected non-empty port")
	}
	if user == "" {
		t.Error("expected non-empty user")
	}
}

func TestBackupRoundTrip(t *testing.T) {
	h, ctx, testDB := setupBackupTest(t)

	// 1. List databases
	dbReq := httptest.NewRequest("GET", "/api/backup/databases", nil)
	dbW := httptest.NewRecorder()
	h.ListBackupDatabases(dbW, dbReq)

	var databases []backupDatabaseEntry
	json.NewDecoder(dbW.Body).Decode(&databases)

	found := false
	for _, db := range databases {
		if db.Name == testDB {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("test database %q not found in database list", testDB)
	}

	// 2. List tables
	tableReq := httptest.NewRequest("GET", "/api/backup/tables?db="+testDB, nil)
	tableW := httptest.NewRecorder()
	h.ListBackupTables(tableW, tableReq)

	var tableResult backupTableListResponse
	json.NewDecoder(tableW.Body).Decode(&tableResult)

	if len(tableResult.Tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tableResult.Tables))
	}

	// 3. Backup specific table
	body, _ := json.Marshal(backupCreateRequest{
		Database: testDB,
		Tables:   []string{"bktest_users"},
	})
	backupReq := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	backupReq.Header.Set("Content-Type", "application/json")
	backupW := httptest.NewRecorder()
	h.StreamBackup(backupW, backupReq)

	if backupW.Code != http.StatusOK {
		t.Fatalf("backup: expected 200, got %d: %s", backupW.Code, backupW.Body.String())
	}

	dumpData := backupW.Body.Bytes()
	if len(dumpData) == 0 {
		t.Fatal("backup produced empty file")
	}

	// 4. Inspect the backup
	var inspectBuf bytes.Buffer
	inspectWriter := multipart.NewWriter(&inspectBuf)
	inspectPart, _ := inspectWriter.CreateFormFile("file", "test.dump")
	inspectPart.Write(dumpData)
	inspectWriter.Close()

	inspectReq := httptest.NewRequest("POST", "/api/backup/inspect", &inspectBuf)
	inspectReq.Header.Set("Content-Type", inspectWriter.FormDataContentType())
	inspectW := httptest.NewRecorder()
	h.InspectDump(inspectW, inspectReq)

	if inspectW.Code != http.StatusOK {
		t.Fatalf("inspect: expected 200, got %d: %s", inspectW.Code, inspectW.Body.String())
	}

	var inspectResult backupInspectResponse
	json.NewDecoder(inspectW.Body).Decode(&inspectResult)

	if inspectResult.Format != "custom" {
		t.Errorf("expected format 'custom', got %q", inspectResult.Format)
	}

	// 5. Restore to new database
	h.pool.Exec(ctx, "DROP DATABASE IF EXISTS bktest_restore_target WITH (FORCE)")
	h.pool.Exec(ctx, "CREATE DATABASE bktest_restore_target")

	var restoreBuf bytes.Buffer
	restoreWriter := multipart.NewWriter(&restoreBuf)
	restorePart, _ := restoreWriter.CreateFormFile("file", "test.dump")
	restorePart.Write(dumpData)
	restoreWriter.WriteField("database", "bktest_restore_target")
	restoreWriter.Close()

	restoreReq := httptest.NewRequest("POST", "/api/backup/restore", &restoreBuf)
	restoreReq.Header.Set("Content-Type", restoreWriter.FormDataContentType())
	restoreW := httptest.NewRecorder()
	h.RestoreBackup(restoreW, restoreReq)

	if restoreW.Code != http.StatusOK {
		t.Fatalf("restore: expected 200, got %d: %s", restoreW.Code, restoreW.Body.String())
	}

	// 6. Verify data in restored database
	targetPool := connectToDB(t, "bktest_restore_target")
	var name string
	err := targetPool.QueryRow(ctx,
		"SELECT name FROM bktest_users WHERE name = 'alice'").
		Scan(&name)
	if err != nil {
		t.Fatalf("failed to query restored data: %v", err)
	}
	if name != "alice" {
		t.Errorf("expected 'alice', got %q", name)
	}

	// 7. Verify posts table does NOT exist (we only backed up users)
	var postExists bool
	targetPool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'bktest_posts'
		)
	`).Scan(&postExists)

	if postExists {
		t.Error("bktest_posts should not exist in restored database (only users was backed up)")
	}
}

func TestBackupCreatesValidDumpFile(t *testing.T) {
	h, _, testDB := setupBackupTest(t)

	body, _ := json.Marshal(backupCreateRequest{Database: testDB})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.StreamBackup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Write dump to temp file and verify magic header
	tmpDir, err := os.MkdirTemp("", "backup-verify-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dumpPath := filepath.Join(tmpDir, "test.dump")
	if err := os.WriteFile(dumpPath, w.Body.Bytes(), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Verify file starts with pg_dump magic header
	f, err := os.Open(dumpPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	header := make([]byte, 5)
	f.Read(header)
	f.Close()

	// PGDMP custom format starts with "PGDMP" (0x50 0x47 0x44 0x4D 0x50)
	if string(header[:5]) != "PGDMP" {
		t.Errorf("expected pg_dump custom format magic 'PGDMP', got %q", string(header[:5]))
	}
}

func TestStreamBackup_NonexistentTable(t *testing.T) {
	h, _, testDB := setupBackupTest(t)

	body, _ := json.Marshal(backupCreateRequest{
		Database: testDB,
		Tables:   []string{"nonexistent_table_xyz"},
	})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.StreamBackup(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for nonexistent table, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRestoreBackup_ExistingTablesWithoutDropFirst(t *testing.T) {
	h, ctx, testDB := setupBackupTest(t)

	// Create backup
	body, _ := json.Marshal(backupCreateRequest{Database: testDB})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.StreamBackup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("backup failed: %d: %s", w.Code, w.Body.String())
	}

	dumpData := w.Body.Bytes()

	// Create restore target with existing tables
	h.pool.Exec(ctx, "DROP DATABASE IF EXISTS bktest_restore_target WITH (FORCE)")
	h.pool.Exec(ctx, "CREATE DATABASE bktest_restore_target")
	targetPool := connectToDB(t, "bktest_restore_target")
	targetPool.Exec(ctx, `CREATE TABLE existing_table (id SERIAL PRIMARY KEY)`)

	// Restore WITHOUT dropFirst — should get 409
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "test.dump")
	part.Write(dumpData)
	writer.WriteField("database", "bktest_restore_target")
	writer.WriteField("dropFirst", "false")
	writer.Close()

	restoreReq := httptest.NewRequest("POST", "/api/backup/restore", &buf)
	restoreReq.Header.Set("Content-Type", writer.FormDataContentType())
	restoreW := httptest.NewRecorder()

	h.RestoreBackup(restoreW, restoreReq)

	if restoreW.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", restoreW.Code, restoreW.Body.String())
	}

	var errResp map[string]any
	if err := json.NewDecoder(restoreW.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp["error"] == "" {
		t.Error("expected error message in response")
	}
	tables, ok := errResp["tables"].([]any)
	if !ok || len(tables) == 0 {
		t.Error("expected tables list in 409 response")
	}
}

func TestRestoreBackup_ExistingTablesWithDropFirst(t *testing.T) {
	h, ctx, testDB := setupBackupTest(t)

	// Create backup
	body, _ := json.Marshal(backupCreateRequest{Database: testDB})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.StreamBackup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("backup failed: %d: %s", w.Code, w.Body.String())
	}

	dumpData := w.Body.Bytes()

	// Create restore target with existing tables
	h.pool.Exec(ctx, "DROP DATABASE IF EXISTS bktest_restore_target WITH (FORCE)")
	h.pool.Exec(ctx, "CREATE DATABASE bktest_restore_target")
	targetPool := connectToDB(t, "bktest_restore_target")
	targetPool.Exec(ctx, `CREATE TABLE existing_table (id SERIAL PRIMARY KEY)`)

	// Restore WITH dropFirst — should succeed
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "test.dump")
	part.Write(dumpData)
	writer.WriteField("database", "bktest_restore_target")
	writer.WriteField("dropFirst", "true")
	writer.Close()

	restoreReq := httptest.NewRequest("POST", "/api/backup/restore", &buf)
	restoreReq.Header.Set("Content-Type", writer.FormDataContentType())
	restoreW := httptest.NewRecorder()

	h.RestoreBackup(restoreW, restoreReq)

	if restoreW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", restoreW.Code, restoreW.Body.String())
	}

	// Verify old table is gone
	var exists bool
	targetPool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'existing_table'
		)
	`).Scan(&exists)

	if exists {
		t.Error("existing_table should have been dropped before restore")
	}
}

func TestInspectDump_PlainSQLDump(t *testing.T) {
	h, _, _ := setupBackupTest(t)

	// Create a plain SQL dump file
	plainSQL := `--
-- PostgreSQL database dump
--

-- Dumped from database version 17.0
-- Dumped by pg_dump version 17.0

SET statement_timeout = 0;
CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);
`

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "backup.sql")
	part.Write([]byte(plainSQL))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/backup/inspect", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	h.InspectDump(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for plain SQL dump, got %d: %s", w.Code, w.Body.String())
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp["error"] == "" {
		t.Error("expected error message")
	}
	if !strings.Contains(errResp["error"], "plain SQL") {
		t.Errorf("expected error to mention 'plain SQL', got: %s", errResp["error"])
	}
}

func TestRestoreBackup_NewDatabaseWithoutDropFirst(t *testing.T) {
	h, ctx, testDB := setupBackupTest(t)

	// Create backup
	body, _ := json.Marshal(backupCreateRequest{Database: testDB})
	req := httptest.NewRequest("POST", "/api/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.StreamBackup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("backup failed: %d: %s", w.Code, w.Body.String())
	}

	dumpData := w.Body.Bytes()

	// Target doesn't exist yet — pg_restore will fail because DB doesn't exist
	h.pool.Exec(ctx, "DROP DATABASE IF EXISTS bktest_restore_target WITH (FORCE)")

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "test.dump")
	part.Write(dumpData)
	writer.WriteField("database", "bktest_restore_target")
	writer.WriteField("dropFirst", "false")
	writer.Close()

	restoreReq := httptest.NewRequest("POST", "/api/backup/restore", &buf)
	restoreReq.Header.Set("Content-Type", writer.FormDataContentType())
	restoreW := httptest.NewRecorder()

	h.RestoreBackup(restoreW, restoreReq)

	// pg_restore fails because target DB doesn't exist
	if restoreW.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for restore to non-existent DB, got %d: %s", restoreW.Code, restoreW.Body.String())
	}
}
