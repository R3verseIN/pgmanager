package handler

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"time"
)

type userRecord struct {
	Username     string `json:"username"`
	DatabaseName string `json:"database"`
	Access       string `json:"access"`
	CreatedAt    string `json:"createdAt"`
}

type createUserResponse struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	DatabaseName string `json:"database"`
	Access       string `json:"access"`
	CreatedAt    string `json:"createdAt"`
}

type createUserRequest struct {
	DatabaseName string `json:"database"`
	Username     string `json:"username"`
	Access       string `json:"access"`
	Password     string `json:"password,omitempty"`
}

func generatePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	password := make([]byte, length)
	for i := range password {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "fallbackPwd1"
		}
		password[i] = charset[n.Int64()]
	}
	return string(password)
}

func validPassword(s string) bool {
	if len(s) < 8 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if c > 127 {
			return false
		}
	}
	return true
}

func (h *Handler) InitUserSchema(ctx context.Context) error {
	_, err := h.pool.Exec(ctx, `
		CREATE SCHEMA IF NOT EXISTS pgmanager;
		CREATE TABLE IF NOT EXISTS pgmanager.managed_users (
			username      TEXT PRIMARY KEY,
			database_name TEXT NOT NULL,
			access        TEXT NOT NULL CHECK (access IN ('read', 'write', 'ddl', 'full')),
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	return err
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT u.username, u.database_name, u.access, u.created_at
		FROM pgmanager.managed_users u
		INNER JOIN pg_catalog.pg_roles r ON r.rolname = u.username
		ORDER BY u.created_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	defer rows.Close()

	users := make([]userRecord, 0)
	for rows.Next() {
		var u userRecord
		var createdAt time.Time
		if err := rows.Scan(&u.Username, &u.DatabaseName, &u.Access, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan row")
			return
		}
		u.CreatedAt = createdAt.Format(time.RFC3339)
		users = append(users, u)
	}

	writeJSON(w, http.StatusOK, users)
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.DatabaseName = strings.TrimSpace(req.DatabaseName)
	req.Access = strings.TrimSpace(req.Access)

	validAccess := map[string]bool{"read": true, "write": true, "ddl": true, "full": true}
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if len(req.Username) > 63 {
		writeError(w, http.StatusBadRequest, "username too long (max 63 characters)")
		return
	}
	if !validName.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "invalid username: must start with letter or underscore, alphanumeric only")
		return
	}
	if req.DatabaseName == "" {
		writeError(w, http.StatusBadRequest, "database is required")
		return
	}
	if protectedDatabases[req.DatabaseName] {
		writeError(w, http.StatusBadRequest, "cannot grant access to system database")
		return
	}
	if !validAccess[req.Access] {
		writeError(w, http.StatusBadRequest, "invalid access level (read, write, ddl, full)")
		return
	}

	password := req.Password
	if password != "" {
		if !validPassword(password) {
			writeError(w, http.StatusBadRequest, "password must be 8-128 ASCII characters")
			return
		}
	} else {
		password = generatePassword(16)
	}

	// Check database exists
	var dbExists bool
	err := h.pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_database WHERE datname = $1)", req.DatabaseName).Scan(&dbExists)
	if err != nil || !dbExists {
		writeError(w, http.StatusBadRequest, "database does not exist")
		return
	}

	// Check role doesn't already exist
	var roleExists bool
	err = h.pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", req.Username).Scan(&roleExists)
	if err != nil || roleExists {
		writeError(w, http.StatusBadRequest, "username already exists")
		return
	}

	// Create role
	_, err = h.pool.Exec(r.Context(), "CREATE ROLE "+quoteIdent(req.Username)+" WITH LOGIN PASSWORD "+quoteLiteral(password))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create role: "+err.Error())
		return
	}

	// Grant CONNECT on database
	_, err = h.pool.Exec(r.Context(), "GRANT CONNECT ON DATABASE "+quoteIdent(req.DatabaseName)+" TO "+quoteIdent(req.Username))
	if err != nil {
		h.rollbackUser(r.Context(), req.Username)
		writeError(w, http.StatusInternalServerError, "failed to grant connect: "+err.Error())
		return
	}

	// Grant USAGE on schema public
	_, err = h.pool.Exec(r.Context(), "GRANT USAGE ON SCHEMA public TO "+quoteIdent(req.Username))
	if err != nil {
		h.rollbackUser(r.Context(), req.Username)
		writeError(w, http.StatusInternalServerError, "failed to grant usage: "+err.Error())
		return
	}

	switch req.Access {
	case "read":
		_, err = h.pool.Exec(r.Context(), "GRANT SELECT ON ALL TABLES IN SCHEMA public TO "+quoteIdent(req.Username))
		if err != nil {
			h.rollbackUser(r.Context(), req.Username)
			writeError(w, http.StatusInternalServerError, "failed to grant select: "+err.Error())
			return
		}
		_, err = h.pool.Exec(r.Context(), "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO "+quoteIdent(req.Username))
		if err != nil {
			h.rollbackUser(r.Context(), req.Username)
			writeError(w, http.StatusInternalServerError, "failed to set default privileges: "+err.Error())
			return
		}

	case "write":
		_, err = h.pool.Exec(r.Context(), "GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO "+quoteIdent(req.Username))
		if err != nil {
			h.rollbackUser(r.Context(), req.Username)
			writeError(w, http.StatusInternalServerError, "failed to grant write: "+err.Error())
			return
		}
		_, err = h.pool.Exec(r.Context(), "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "+quoteIdent(req.Username))
		if err != nil {
			h.rollbackUser(r.Context(), req.Username)
			writeError(w, http.StatusInternalServerError, "failed to set default privileges: "+err.Error())
			return
		}

	case "ddl":
		_, err = h.pool.Exec(r.Context(), "GRANT USAGE, CREATE ON SCHEMA public TO "+quoteIdent(req.Username))
		if err != nil {
			h.rollbackUser(r.Context(), req.Username)
			writeError(w, http.StatusInternalServerError, "failed to grant create: "+err.Error())
			return
		}
		_, err = h.pool.Exec(r.Context(), "GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO "+quoteIdent(req.Username))
		if err != nil {
			h.rollbackUser(r.Context(), req.Username)
			writeError(w, http.StatusInternalServerError, "failed to grant ddl: "+err.Error())
			return
		}
		_, err = h.pool.Exec(r.Context(), "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "+quoteIdent(req.Username))
		if err != nil {
			h.rollbackUser(r.Context(), req.Username)
			writeError(w, http.StatusInternalServerError, "failed to set default privileges: "+err.Error())
			return
		}

	case "full":
		_, err = h.pool.Exec(r.Context(), "GRANT ALL PRIVILEGES ON DATABASE "+quoteIdent(req.DatabaseName)+" TO "+quoteIdent(req.Username))
		if err != nil {
			h.rollbackUser(r.Context(), req.Username)
			writeError(w, http.StatusInternalServerError, "failed to grant all: "+err.Error())
			return
		}
		_, err = h.pool.Exec(r.Context(), "GRANT ALL ON SCHEMA public TO "+quoteIdent(req.Username))
		if err != nil {
			h.rollbackUser(r.Context(), req.Username)
			writeError(w, http.StatusInternalServerError, "failed to grant all schema: "+err.Error())
			return
		}
		_, err = h.pool.Exec(r.Context(), "GRANT ALL ON ALL TABLES IN SCHEMA public TO "+quoteIdent(req.Username))
		if err != nil {
			h.rollbackUser(r.Context(), req.Username)
			writeError(w, http.StatusInternalServerError, "failed to grant all tables: "+err.Error())
			return
		}
		_, err = h.pool.Exec(r.Context(), "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO "+quoteIdent(req.Username))
		if err != nil {
			h.rollbackUser(r.Context(), req.Username)
			writeError(w, http.StatusInternalServerError, "failed to set default privileges: "+err.Error())
			return
		}
	}

	// Save metadata
	_, err = h.pool.Exec(r.Context(),
		"INSERT INTO pgmanager.managed_users (username, database_name, access) VALUES ($1, $2, $3)",
		req.Username, req.DatabaseName, req.Access,
	)
	if err != nil {
		h.rollbackUser(r.Context(), req.Username)
		writeError(w, http.StatusInternalServerError, "failed to save metadata: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, createUserResponse{
		Username:     req.Username,
		Password:     password,
		DatabaseName: req.DatabaseName,
		Access:       req.Access,
		CreatedAt:    time.Now().Format(time.RFC3339),
	})
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("name")
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if protectedDatabases[username] {
		writeError(w, http.StatusForbidden, "cannot delete system role")
		return
	}

	// Revoke all owned objects first
	_, _ = h.pool.Exec(r.Context(), "DROP OWNED BY "+quoteIdent(username)+" CASCADE")

	_, err := h.pool.Exec(r.Context(), "DROP ROLE IF EXISTS "+quoteIdent(username))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to drop role: "+err.Error())
		return
	}

	_, err = h.pool.Exec(r.Context(), "DELETE FROM pgmanager.managed_users WHERE username = $1", username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove metadata: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) rollbackUser(ctx context.Context, username string) {
	conn, err := h.pool.Acquire(ctx)
	if err != nil {
		return
	}
	defer conn.Release()
	conn.Exec(ctx, "DROP OWNED BY "+quoteIdent(username)+" CASCADE")
	conn.Exec(ctx, "DROP ROLE IF EXISTS "+quoteIdent(username))
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
