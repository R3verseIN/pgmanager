package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"pgmanager/internal/auth"
	"pgmanager/internal/handler"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed ui/dist/*
var uiFS embed.FS

func main() {
	ctx := context.Background()

	databaseURL := buildDatabaseURL()

	var pool *pgxpool.Pool
	var lastErr error
	for attempt := 1; attempt <= 30; attempt++ {
		pool, lastErr = pgxpool.New(ctx, databaseURL)
		if lastErr != nil {
			log.Printf("attempt %d/30: failed to connect: %v", attempt, lastErr)
			time.Sleep(2 * time.Second)
			continue
		}
		if pingErr := pool.Ping(ctx); pingErr != nil {
			pool.Close()
			lastErr = pingErr
			log.Printf("attempt %d/30: failed to ping: %v", attempt, pingErr)
			time.Sleep(2 * time.Second)
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		log.Fatalf("failed to connect after 30 attempts: %v", lastErr)
	}
	defer pool.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	h := handler.NewWithDSN(pool, buildBaseDSN())
	ah := handler.NewAuthHandler(pool)
	sh := handler.NewSettingsHandler(pool)
	ssh := handler.NewSSLHandler("/var/lib/postgresql/data")

	if err := h.InitUserSchema(ctx); err != nil {
		log.Printf("warning: failed to init user schema: %v", err)
	}

	// Generate PgBouncer HBA file on startup
	h.RebuildPgBouncerHBA()

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			h.RebuildPgBouncerHBA()
		}
	}()

	go startAuditLogRetention(ctx, pool)

	mux := http.NewServeMux()

	// Auth routes (no auth required)
	mux.HandleFunc("GET /api/auth/setup-check", ah.SetupCheck)
	mux.HandleFunc("POST /api/auth/setup", ah.Setup)
	mux.HandleFunc("POST /api/auth/login", ah.Login)

	// API routes with auth
	mux.Handle("/api/", auth.AuthMiddleware(pool)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		method := r.Method

		// Auth routes (any role)
		if method == "POST" && path == "/api/auth/logout" {
			ah.Logout(w, r)
			return
		}
		if method == "GET" && path == "/api/auth/me" {
			ah.GetMe(w, r)
			return
		}
		if method == "PUT" && path == "/api/auth/password" {
			ah.ChangePassword(w, r)
			return
		}

		// Read-only routes (any role)
		if method == "GET" && path == "/api/databases" {
			h.ListDatabases(w, r)
			return
		}
		if method == "GET" && path == "/api/users" {
			h.ListUsers(w, r)
			return
		}

		// Database content routes (any role for reads, admin/dev for writes)
		// GET /api/databases/{name}/tables → 4 slashes
		if method == "GET" && strings.HasSuffix(path, "/tables") && strings.Count(path, "/") == 4 {
			h.ListTables(w, r)
			return
		}
		// GET /api/databases/{name}/columns/{table} → 5 slashes
		if method == "GET" && strings.Contains(path, "/columns/") && !strings.HasSuffix(path, "/columns") {
			h.GetColumns(w, r)
			return
		}
		// Data routes: /api/databases/{name}/data/{table} → 5 slashes
		if strings.Contains(path, "/data/") && strings.Count(path, "/") == 5 {
			switch method {
			case "GET":
				h.ListData(w, r)
				return
			case "POST":
				h.InsertRow(w, r)
				return
			case "PUT":
				h.UpdateRow(w, r)
				return
			case "DELETE":
				h.DeleteRow(w, r)
				return
			}
		}
		// POST /api/databases/{name}/tables → 4 slashes
		if method == "POST" && strings.HasSuffix(path, "/tables") && strings.Count(path, "/") == 4 {
			h.CreateTable(w, r)
			return
		}
		// POST /api/databases/{name}/tables/{table}/columns → 6 slashes
		if method == "POST" && strings.HasSuffix(path, "/columns") && strings.Count(path, "/") == 6 {
			h.AddColumn(w, r)
			return
		}
		// DELETE /api/databases/{name}/tables/{table}/columns/{column} → 7 slashes
		if method == "DELETE" && strings.Contains(path, "/columns/") && strings.Count(path, "/") == 7 {
			h.DropColumn(w, r)
			return
		}
		// POST /api/databases/{name}/query → 4 slashes
		if method == "POST" && strings.HasSuffix(path, "/query") && strings.Count(path, "/") == 4 {
			h.ExecuteQuery(w, r)
			return
		}

		// Backup routes (admin or dev can list/download)
		if method == "GET" && path == "/api/backup/databases" {
			user := auth.GetUserFromContext(r.Context())
			if user == nil || (user.Role != "admin" && user.Role != "dev") {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			h.ListBackupDatabases(w, r)
			return
		}
		if method == "GET" && path == "/api/backup/tables" {
			user := auth.GetUserFromContext(r.Context())
			if user == nil || (user.Role != "admin" && user.Role != "dev") {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			h.ListBackupTables(w, r)
			return
		}
		if method == "POST" && path == "/api/backup/create" {
			user := auth.GetUserFromContext(r.Context())
			if user == nil || (user.Role != "admin" && user.Role != "dev") {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			h.StreamBackup(w, r)
			return
		}
		if method == "POST" && path == "/api/backup/inspect" {
			user := auth.GetUserFromContext(r.Context())
			if user == nil || (user.Role != "admin" && user.Role != "dev") {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			h.InspectDump(w, r)
			return
		}

		// Admin-only routes
		user := auth.GetUserFromContext(r.Context())
		if user == nil || user.Role != "admin" {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}

		if method == "POST" && path == "/api/auth/users" {
			ah.CreateAuthUser(w, r)
			return
		}
		if method == "PUT" && strings.HasPrefix(path, "/api/auth/users/") {
			ah.UpdateAuthUser(w, r)
			return
		}
		if method == "DELETE" && strings.HasPrefix(path, "/api/auth/users/") {
			ah.DeleteAuthUser(w, r)
			return
		}
		if method == "GET" && path == "/api/auth/users" {
			ah.ListAuthUsers(w, r)
			return
		}
		if method == "POST" && strings.HasSuffix(path, "/reset-password") {
			ah.ResetAuthUserPassword(w, r)
			return
		}
		if method == "POST" && path == "/api/databases" {
			h.CreateDatabase(w, r)
			return
		}
		if method == "DELETE" && strings.HasPrefix(path, "/api/databases/") && strings.Count(path, "/") == 3 {
			h.DeleteDatabase(w, r)
			return
		}
		if method == "POST" && path == "/api/users" {
			h.CreateUser(w, r)
			return
		}
		if method == "POST" && strings.HasSuffix(path, "/databases") {
			h.AddUserDatabase(w, r)
			return
		}
		if method == "DELETE" && strings.Contains(path, "/databases/") {
			h.RemoveUserDatabase(w, r)
			return
		}
		if method == "PUT" && strings.HasPrefix(path, "/api/users/") {
			h.UpdateUser(w, r)
			return
		}
		if method == "DELETE" && strings.HasPrefix(path, "/api/users/") {
			h.DeleteUser(w, r)
			return
		}
		if method == "GET" && path == "/api/logs" {
			h.ListLogs(w, r)
			return
		}
		if method == "GET" && path == "/api/pgbouncer/databases" {
			h.ListPgBouncerDatabases(w, r)
			return
		}
		if method == "PUT" && strings.HasPrefix(path, "/api/pgbouncer/databases/") {
			h.TogglePgBouncerDatabase(w, r)
			return
		}
		if method == "GET" && path == "/api/pgbouncer/config" {
			h.GetPgBouncerConfig(w, r)
			return
		}
		if method == "PUT" && path == "/api/pgbouncer/config" {
			h.UpdatePgBouncerConfig(w, r)
			return
		}

		// Settings routes (admin only)
		if method == "GET" && path == "/api/settings" {
			sh.GetSettings(w, r)
			return
		}
		if method == "PUT" && path == "/api/settings" {
			sh.UpdateSettings(w, r)
			return
		}

		// SSL routes (admin only)
		if method == "GET" && path == "/api/ssl/status" {
			ssh.GetStatus(w, r)
			return
		}
		if method == "POST" && path == "/api/ssl/generate" {
			ssh.GenerateCerts(w, r)
			return
		}
		if method == "POST" && path == "/api/ssl/upload" {
			ssh.UploadCerts(w, r)
			return
		}
		if method == "GET" && path == "/api/ssl/download" {
			ssh.DownloadCA(w, r)
			return
		}
		if method == "POST" && path == "/api/ssl/enable" {
			ssh.EnableCerts(w, r)
			return
		}
		if method == "POST" && path == "/api/ssl/disable" {
			ssh.DisableCerts(w, r)
			return
		}
		if method == "DELETE" && path == "/api/ssl" {
			ssh.DeleteCerts(w, r)
			return
		}

		// Restore is admin-only (must be after admin check below)
		if method == "POST" && path == "/api/backup/restore" {
			h.RestoreBackup(w, r)
			return
		}

		http.NotFound(w, r)
	})))

	subFS, err := fs.Sub(uiFS, "ui/dist")
	if err != nil {
		log.Fatalf("failed to get ui sub fs: %v", err)
	}

	mux.Handle("/", spaHandler(subFS))

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		log.Printf("listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
}

func buildDatabaseURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}

	secretPath := os.Getenv("SECRET_PATH")
	if secretPath == "" {
		secretPath = "/secrets/pgmanager-password"
	}

	password := readPassword(secretPath)

	host := os.Getenv("PGHOST")
	if host == "" {
		host = "localhost"
	}

	port := os.Getenv("PGPORT")
	if port == "" {
		port = "5432"
	}

	user := os.Getenv("PGUSER")
	if user == "" {
		user = "pgmanager"
	}

	dbname := os.Getenv("PGDATABASE")
	if dbname == "" {
		dbname = "pgmanager"
	}

	sslmode := "disable"

	url := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, password, host, port, dbname, sslmode)
	log.Printf("connecting to database at %s:%s/%s as %s", host, port, dbname, user)
	return url
}

func buildBaseDSN() string {
	// If DATABASE_URL is set, extract credentials from it directly.
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		if u, err := url.Parse(dbURL); err == nil && u.User != nil {
			user := u.User.Username()
			password, _ := u.User.Password()
			host := u.Hostname()
			port := u.Port()
			if port == "" {
				port = "5432"
			}
			dbname := strings.TrimPrefix(u.Path, "/")
			if dbname == "" {
				dbname = "pgmanager"
			}
		return fmt.Sprintf("postgres://%s:%s@%s:%s/?sslmode=disable", user, password, host, port)
		}
	}

	secretPath := os.Getenv("SECRET_PATH")
	if secretPath == "" {
		secretPath = "/secrets/pgmanager-password"
	}

	password := readPassword(secretPath)

	host := os.Getenv("PGHOST")
	if host == "" {
		host = "localhost"
	}

	port := os.Getenv("PGPORT")
	if port == "" {
		port = "5432"
	}

	user := os.Getenv("PGUSER")
	if user == "" {
		user = "pgmanager"
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%s/?sslmode=disable", user, password, host, port)
}

func readPassword(path string) string {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("failed to open password file %s: %v", path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		log.Fatalf("failed to read password file %s: %v", path, err)
	}

	password := strings.TrimSpace(string(data))
	if password == "" {
		log.Fatalf("password file %s is empty", path)
	}

	return password
}

func spaHandler(distFS fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(distFS))
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := fs.Stat(distFS, strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	}
}

func startAuditLogRetention(ctx context.Context, pool *pgxpool.Pool) {
	// Run once after 1 hour, then every 24 hours
	select {
	case <-time.After(1 * time.Hour):
	case <-ctx.Done():
		return
	}

	cleanupAuditLog(ctx, pool)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cleanupAuditLog(ctx, pool)
		case <-ctx.Done():
			return
		}
	}
}

func cleanupAuditLog(ctx context.Context, pool *pgxpool.Pool) {
	var val string
	err := pool.QueryRow(ctx,
		`SELECT value FROM system_config WHERE key = 'audit_log_retention_days'`).
		Scan(&val)
	if err != nil {
		return // key doesn't exist, skip
	}

	days, err := strconv.Atoi(val)
	if err != nil || days <= 0 {
		return // 0 = keep forever, or invalid value
	}

	tag, err := pool.Exec(ctx,
		`DELETE FROM audit_log WHERE created_at < NOW() - ($1 || ' days')::INTERVAL`, val)
	if err != nil {
		log.Printf("audit log cleanup failed: %v", err)
		return
	}
	if tag.RowsAffected() > 0 {
		log.Printf("audit log cleanup: deleted %d rows older than %d days", tag.RowsAffected(), days)
	}
}
