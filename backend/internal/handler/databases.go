package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"

	"pgmanager/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

var protectedDatabases = map[string]bool{
	"template0": true,
	"template1": true,
	"postgres":  true,
	"pgmanager": true,
}

var validName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

type Handler struct {
	pool    *pgxpool.Pool
	baseDSN string
}

func New(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

func NewWithDSN(pool *pgxpool.Pool, baseDSN string) *Handler {
	return &Handler{pool: pool, baseDSN: baseDSN}
}

type database struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type createRequest struct {
	Name string `json:"name"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func (h *Handler) ListDatabases(w http.ResponseWriter, r *http.Request) {
	showSystem := r.URL.Query().Get("showSystem") == "true"

	user := auth.GetUserFromContext(r.Context())

	var allowedDatabases map[string]bool
	if user != nil && user.Role == "dev" {
		allowedDatabases = make(map[string]bool)
		rows, err := h.pool.Query(r.Context(), "SELECT database_name FROM dev_databases WHERE auth_user_id = $1", user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list dev databases")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var db string
			if err := rows.Scan(&db); err == nil {
				allowedDatabases[db] = true
			}
		}
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT datname
		FROM pg_catalog.pg_database
		ORDER BY datname
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list databases")
		return
	}
	defer rows.Close()

	databases := make([]database, 0)
	for rows.Next() {
		var db database
		if err := rows.Scan(&db.Name); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan row")
			return
		}
		db.Protected = protectedDatabases[db.Name]
		if !showSystem && db.Protected {
			continue
		}
		if allowedDatabases != nil && !allowedDatabases[db.Name] {
			continue
		}
		databases = append(databases, db)
	}

	writeJSON(w, http.StatusOK, databases)
}

func (h *Handler) CreateDatabase(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(name) > 63 {
		writeError(w, http.StatusBadRequest, "name too long (max 63 characters)")
		return
	}
	if !validName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid name: must start with letter or underscore, alphanumeric only")
		return
	}
	if protectedDatabases[name] {
		writeError(w, http.StatusForbidden, "cannot create system database")
		return
	}

	sql := "CREATE DATABASE " + quoteIdent(name)
	if _, err := h.pool.Exec(r.Context(), sql); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, "database already exists: "+name)
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create database: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, database{
		Name: name,
	})
}

func (h *Handler) DeleteDatabase(w http.ResponseWriter, r *http.Request) {
	// Extract name from path: /api/databases/{name}
	name := ""
	path := r.URL.Path
	prefix := "/api/databases/"
	if strings.HasPrefix(path, prefix) {
		rest := path[len(prefix):]
		if idx := strings.Index(rest, "/"); idx < 0 {
			name = rest
		} else {
			name = rest[:idx]
		}
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if protectedDatabases[name] {
		writeError(w, http.StatusForbidden, "cannot delete system database")
		return
	}

	sql := "DROP DATABASE " + quoteIdent(name) + " WITH (FORCE)"
	if _, err := h.pool.Exec(r.Context(), sql); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete database: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
