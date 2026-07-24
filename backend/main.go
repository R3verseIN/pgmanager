package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("failed to ping: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	h := handler.New(pool)
	ah := handler.NewAuthHandler(pool)

	if err := h.InitUserSchema(ctx); err != nil {
		log.Printf("warning: failed to init user schema: %v", err)
	}

	if err := auth.EnsurePgbouncerAuth(ctx, pool); err != nil {
		log.Printf("warning: failed to ensure pgbouncer auth: %v", err)
	}

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := auth.EnsurePgbouncerAuth(context.Background(), pool); err != nil {
				log.Printf("warning: pgbouncer auth healthcheck failed: %v", err)
			}
		}
	}()

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
		if method == "DELETE" && strings.HasPrefix(path, "/api/databases/") {
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

	sslmode := os.Getenv("PGSSLMODE")
	if sslmode == "" {
		sslmode = "disable"
	}

	url := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, password, host, port, dbname, sslmode)
	log.Printf("connecting to database at %s:%s/%s as %s", host, port, dbname, user)
	return url
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
