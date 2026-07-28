package databases

import (
	"encoding/json"
	"net/http"
	"strings"

	"pgmanager/internal/auth"
	"pgmanager/internal/handler/core"

	"github.com/jackc/pgx/v5/pgxpool"
)

type database struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}

type createRequest struct {
	Name string `json:"name"`
}

func ListDatabases(pool *pgxpool.Pool, w http.ResponseWriter, r *http.Request) {
	showSystem := r.URL.Query().Get("showSystem") == "true"

	user := auth.GetUserFromContext(r.Context())

	var allowedDatabases map[string]bool
	if user != nil && user.Role == "dev" {
		allowedDatabases = make(map[string]bool)
		rows, err := pool.Query(r.Context(), "SELECT database_name FROM dev_databases WHERE auth_user_id = $1", user.ID)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to list dev databases")
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

	rows, err := pool.Query(r.Context(), `
		SELECT datname
		FROM pg_catalog.pg_database
		ORDER BY datname
	`)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to list databases")
		return
	}
	defer rows.Close()

	databases := make([]database, 0)
	for rows.Next() {
		var db database
		if err := rows.Scan(&db.Name); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to scan row")
			return
		}
		db.Protected = core.ProtectedDatabases[db.Name]
		if !showSystem && db.Protected {
			continue
		}
		if allowedDatabases != nil && !allowedDatabases[db.Name] {
			continue
		}
		databases = append(databases, db)
	}

	core.WriteJSON(w, http.StatusOK, databases)
}

func CreateDatabase(pool *pgxpool.Pool, notifyChange func(), w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		core.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(name) > 63 {
		core.WriteError(w, http.StatusBadRequest, "name too long (max 63 characters)")
		return
	}
	if !core.ValidName.MatchString(name) {
		core.WriteError(w, http.StatusBadRequest, "invalid name: must start with letter or underscore, alphanumeric only")
		return
	}
	if core.ProtectedDatabases[name] {
		core.WriteError(w, http.StatusForbidden, "cannot create system database")
		return
	}

	sql := "CREATE DATABASE " + core.QuoteIdent(name)
	if _, err := pool.Exec(r.Context(), sql); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			core.WriteError(w, http.StatusConflict, "database already exists: "+name)
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "failed to create database: "+err.Error())
		return
	}

	core.WriteJSON(w, http.StatusCreated, database{
		Name: name,
	})

	pool.Exec(r.Context(),
		`INSERT INTO pgbouncer_databases (database_name, allowed) VALUES ($1, true) ON CONFLICT (database_name) DO NOTHING`,
		name)

	if notifyChange != nil {
		go notifyChange()
	}

	user := auth.GetUserFromContext(r.Context())
	username := ""
	if user != nil {
		username = user.Username
	}
	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  username,
		Action:    "create_database",
		Database:  name,
		IPAddress: core.ClientIP(r),
	})
}

func DeleteDatabase(pool *pgxpool.Pool, notifyChange func(), w http.ResponseWriter, r *http.Request) {
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
		core.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	if core.ProtectedDatabases[name] {
		core.WriteError(w, http.StatusForbidden, "cannot delete system database")
		return
	}

	sql := "DROP DATABASE " + core.QuoteIdent(name) + " WITH (FORCE)"
	if _, err := pool.Exec(r.Context(), sql); err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to delete database: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)

	pool.Exec(r.Context(),
		`DELETE FROM pgbouncer_databases WHERE database_name = $1`, name)

	if notifyChange != nil {
		go notifyChange()
	}

	user := auth.GetUserFromContext(r.Context())
	username := ""
	if user != nil {
		username = user.Username
	}
	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  username,
		Action:    "delete_database",
		Database:  name,
		IPAddress: core.ClientIP(r),
	})
}
