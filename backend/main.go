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

	if err := h.InitUserSchema(ctx); err != nil {
		log.Printf("warning: failed to init user schema: %v", err)
	}

	if err := auth.EnsurePgbouncerAuth(ctx, pool); err != nil {
		log.Printf("warning: failed to ensure pgbouncer auth: %v", err)
	}

	// Periodic healthcheck for pgbouncer auth
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

	mux.HandleFunc("GET /api/databases", h.ListDatabases)
	mux.HandleFunc("POST /api/databases", h.CreateDatabase)
	mux.HandleFunc("DELETE /api/databases/{name}", h.DeleteDatabase)

	mux.HandleFunc("GET /api/users", h.ListUsers)
	mux.HandleFunc("POST /api/users", h.CreateUser)
	mux.HandleFunc("PUT /api/users/{name}", h.UpdateUser)
	mux.HandleFunc("DELETE /api/users/{name}", h.DeleteUser)
	mux.HandleFunc("POST /api/users/{name}/databases", h.AddUserDatabase)
	mux.HandleFunc("DELETE /api/users/{name}/databases/{db}", h.RemoveUserDatabase)

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
		dbname = "postgres"
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
