package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"pgmanager/internal/auth"
	authhandler "pgmanager/internal/handler/auth"
	"pgmanager/internal/handler/audit"
	"pgmanager/internal/handler/backup"
	"pgmanager/internal/handler/core"
	"pgmanager/internal/handler/databases"
	"pgmanager/internal/handler/data"
	pgbouncerhandler "pgmanager/internal/handler/pgbouncer"
	"pgmanager/internal/handler/settings"
	"pgmanager/internal/handler/sql"
	"pgmanager/internal/handler/ssl"
	"pgmanager/internal/handler/tables"
	"pgmanager/internal/handler/users"
	"pgmanager/internal/platform"
)

//go:embed ui/dist/*
var uiFS embed.FS

func main() {
	ctx := context.Background()

	databaseURL := platform.BuildDatabaseURL()
	pool, err := platform.ConnectDB(ctx, databaseURL)
	if err != nil {
		log.Fatalf("failed to connect after 30 attempts: %v", err)
	}
	defer pool.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	baseDSN := platform.BuildBaseDSN()
	h := core.NewWithDSN(pool, baseDSN)
	ah := authhandler.New(pool)
	sh := settings.New(pool)
	ssh := ssl.New("/var/lib/postgresql/data")
	pbh := pgbouncerhandler.New(pool, baseDSN)

	h.OnDatabaseChange = pbh.RebuildPgBouncerHBA

	if err := users.InitUserSchema(ctx, pool); err != nil {
		log.Printf("warning: failed to init user schema: %v", err)
	}

	pbh.RebuildPgBouncerHBA()

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			pbh.RebuildPgBouncerHBA()
		}
	}()

	go platform.StartAuditLogRetention(ctx, pool)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/auth/setup-check", ah.SetupCheck)
	mux.HandleFunc("POST /api/auth/setup", ah.Setup)
	mux.HandleFunc("POST /api/auth/login", ah.Login)

	mux.Handle("/api/", auth.AuthMiddleware(pool)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		method := r.Method

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

		if method == "GET" && path == "/api/databases" {
			databases.ListDatabases(pool, w, r)
			return
		}
		if method == "GET" && path == "/api/users" {
			users.ListUsers(pool, w, r)
			return
		}

		if method == "GET" && strings.HasSuffix(path, "/tables") && strings.Count(path, "/") == 4 {
			tables.ListTables(pool, baseDSN, w, r)
			return
		}
		if method == "GET" && strings.Contains(path, "/columns/") && !strings.HasSuffix(path, "/columns") {
			tables.GetColumns(pool, baseDSN, w, r)
			return
		}
		if strings.Contains(path, "/data/") && strings.Count(path, "/") == 5 {
			switch method {
			case "GET":
				data.ListData(pool, baseDSN, w, r)
				return
			case "POST":
				data.InsertRow(pool, baseDSN, w, r)
				return
			case "PUT":
				data.UpdateRow(pool, baseDSN, w, r)
				return
			case "DELETE":
				data.DeleteRow(pool, baseDSN, w, r)
				return
			}
		}
		if method == "POST" && strings.HasSuffix(path, "/tables") && strings.Count(path, "/") == 4 {
			tables.CreateTable(pool, baseDSN, w, r)
			return
		}
		if method == "POST" && strings.HasSuffix(path, "/columns") && strings.Count(path, "/") == 6 {
			tables.AddColumn(pool, baseDSN, w, r)
			return
		}
		if method == "DELETE" && strings.Contains(path, "/columns/") && strings.Count(path, "/") == 7 {
			tables.DropColumn(pool, baseDSN, w, r)
			return
		}
		if method == "POST" && strings.HasSuffix(path, "/query") && strings.Count(path, "/") == 4 {
			sql.ExecuteQuery(pool, baseDSN, w, r)
			return
		}

		if method == "GET" && path == "/api/backup/databases" {
			user := auth.GetUserFromContext(r.Context())
			if user == nil || (user.Role != "admin" && user.Role != "dev") {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			backup.ListBackupDatabases(pool, baseDSN, w, r)
			return
		}
		if method == "GET" && path == "/api/backup/tables" {
			user := auth.GetUserFromContext(r.Context())
			if user == nil || (user.Role != "admin" && user.Role != "dev") {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			backup.ListBackupTables(pool, baseDSN, w, r)
			return
		}
		if method == "POST" && path == "/api/backup/create" {
			user := auth.GetUserFromContext(r.Context())
			if user == nil || (user.Role != "admin" && user.Role != "dev") {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			backup.StreamBackup(pool, baseDSN, w, r)
			return
		}
		if method == "POST" && path == "/api/backup/inspect" {
			user := auth.GetUserFromContext(r.Context())
			if user == nil || (user.Role != "admin" && user.Role != "dev") {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			backup.InspectDump(pool, baseDSN, w, r)
			return
		}

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
			databases.CreateDatabase(pool, pbh.RebuildPgBouncerHBA, w, r)
			return
		}
		if method == "DELETE" && strings.HasPrefix(path, "/api/databases/") && strings.Count(path, "/") == 3 {
			databases.DeleteDatabase(pool, pbh.RebuildPgBouncerHBA, w, r)
			return
		}
		if method == "POST" && path == "/api/users" {
			users.CreateUser(pool, baseDSN, w, r)
			return
		}
		if method == "POST" && strings.HasSuffix(path, "/databases") {
			users.AddUserDatabase(pool, baseDSN, w, r)
			return
		}
		if method == "DELETE" && strings.Contains(path, "/databases/") {
			users.RemoveUserDatabase(pool, baseDSN, w, r)
			return
		}
		if method == "PUT" && strings.HasPrefix(path, "/api/users/") {
			users.UpdateUser(pool, baseDSN, w, r)
			return
		}
		if method == "DELETE" && strings.HasPrefix(path, "/api/users/") {
			users.DeleteUser(pool, baseDSN, w, r)
			return
		}
		if method == "GET" && path == "/api/logs" {
			audit.ListLogs(pool, w, r)
			return
		}
		if method == "GET" && path == "/api/pgbouncer/databases" {
			pbh.ListPgBouncerDatabases(w, r)
			return
		}
		if method == "PUT" && strings.HasPrefix(path, "/api/pgbouncer/databases/") {
			pbh.TogglePgBouncerDatabase(w, r)
			return
		}
		if method == "GET" && path == "/api/pgbouncer/config" {
			pbh.GetPgBouncerConfig(w, r)
			return
		}
		if method == "PUT" && path == "/api/pgbouncer/config" {
			pbh.UpdatePgBouncerConfig(w, r)
			return
		}

		if method == "GET" && path == "/api/settings" {
			sh.GetSettings(w, r)
			return
		}
		if method == "PUT" && path == "/api/settings" {
			sh.UpdateSettings(w, r)
			return
		}

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

		if method == "POST" && path == "/api/backup/restore" {
			backup.RestoreBackup(pool, baseDSN, w, r)
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
