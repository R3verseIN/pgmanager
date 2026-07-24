package handler

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"pgmanager/internal/auth"
)

// extractUserFromPath extracts the username from paths like /api/users/{username}[/...]
// Since routes are manually dispatched (not Go 1.22 patterns), r.PathValue() doesn't work.
func extractUserFromPath(path string) string {
	prefix := "/api/users/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if idx := strings.Index(rest, "/"); idx < 0 {
		return rest
	} else {
		return rest[:idx]
	}
}

// extractUserDBFromPath extracts username and database from /api/users/{username}/databases/{database}
func extractUserDBFromPath(path string) (string, string) {
	prefix := "/api/users/"
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}
	rest := path[len(prefix):]
	parts := strings.Split(rest, "/")
	if len(parts) >= 4 && parts[1] == "databases" {
		return parts[0], parts[3]
	}
	return "", ""
}

type userRecord struct {
	Username  string   `json:"username"`
	Databases []string `json:"databases"`
	Access    string   `json:"access"`
	CreatedAt string   `json:"createdAt"`
}

type createUserResponse struct {
	Username         string   `json:"username"`
	Password         string   `json:"password"`
	Databases        []string `json:"databases"`
	ConnectionString string   `json:"connectionString"`
	Access           string   `json:"access"`
	CreatedAt        string   `json:"createdAt"`
}

type createUserRequest struct {
	Databases []string `json:"databases"`
	Username  string   `json:"username"`
	Access    string   `json:"access"`
	Password  string   `json:"password,omitempty"`
}

type updateUserRequest struct {
	Password string `json:"password,omitempty"`
	Access   string `json:"access,omitempty"`
}

type addDatabaseRequest struct {
	Database string `json:"database"`
}

func GeneratePassword(length int) string {
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
		CREATE TABLE IF NOT EXISTS managed_users (
			username      TEXT NOT NULL,
			database_name TEXT NOT NULL,
			access        TEXT NOT NULL CHECK (access IN ('read', 'write', 'ddl', 'full')),
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (username, database_name)
		);
	`)
	if err != nil {
		return err
	}

	_, err = h.pool.Exec(ctx, `
		ALTER TABLE managed_users DROP CONSTRAINT IF EXISTS managed_users_pkey;
		ALTER TABLE managed_users ADD PRIMARY KEY (username, database_name);
	`)
	if err != nil {
		return err
	}

	_, err = h.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS auth_users (
			id            SERIAL PRIMARY KEY,
			username      TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role          TEXT NOT NULL CHECK (role IN ('admin', 'dev', 'viewer')),
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return err
	}

	// Migration: update CHECK constraint to include 'dev'
	_, _ = h.pool.Exec(ctx, `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'auth_users_role_check' AND conrelid = 'auth_users'::regclass) THEN
				ALTER TABLE auth_users DROP CONSTRAINT auth_users_role_check;
				ALTER TABLE auth_users ADD CONSTRAINT auth_users_role_check CHECK (role IN ('admin', 'dev', 'viewer'));
			END IF;
		END
		$$;
	`)

	_, err = h.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS dev_databases (
			auth_user_id  INTEGER NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
			database_name TEXT NOT NULL,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (auth_user_id, database_name)
		);
	`)
	if err != nil {
		return err
	}

	_, err = h.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT PRIMARY KEY,
			user_id    INTEGER NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours'
		)
	`)
	if err != nil {
		return err
	}

	_, err = h.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS audit_log (
			id          SERIAL PRIMARY KEY,
			username    TEXT NOT NULL,
			action      TEXT NOT NULL,
			database    TEXT NOT NULL,
			table_name  TEXT,
			detail      JSONB,
			ip_address  TEXT,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return err
	}

	_, _ = h.pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at DESC)`)
	_, _ = h.pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_audit_log_username ON audit_log(username)`)
	_, _ = h.pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(action)`)

	return nil
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT u.username, MIN(u.access) AS access, MAX(u.created_at) AS created_at,
			ARRAY_AGG(u.database_name) AS databases
		FROM managed_users u
		INNER JOIN pg_catalog.pg_roles r ON r.rolname = u.username
		GROUP BY u.username
		ORDER BY MAX(u.created_at) DESC
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
		if err := rows.Scan(&u.Username, &u.Access, &createdAt, &u.Databases); err != nil {
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
	if len(req.Databases) == 0 {
		writeError(w, http.StatusBadRequest, "at least one database is required")
		return
	}
	if !validAccess[req.Access] {
		writeError(w, http.StatusBadRequest, "invalid access level (read, write, ddl, full)")
		return
	}

	for _, db := range req.Databases {
		if protectedDatabases[db] {
			writeError(w, http.StatusBadRequest, "cannot grant access to system database: "+db)
			return
		}
	}

	password := req.Password
	if password != "" {
		if !validPassword(password) {
			writeError(w, http.StatusBadRequest, "password must be 8-128 ASCII characters")
			return
		}
	} else {
		password = GeneratePassword(16)
	}

	var roleExists bool
	err := h.pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", req.Username).Scan(&roleExists)
	if err != nil || roleExists {
		writeError(w, http.StatusBadRequest, "username already exists")
		return
	}

	ctx := r.Context()
	_, err = h.pool.Exec(ctx, "CREATE ROLE "+quoteIdent(req.Username)+" WITH LOGIN PASSWORD "+quoteLiteral(password))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create role: "+err.Error())
		return
	}

	for _, db := range req.Databases {
		var dbExists bool
		err = h.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_database WHERE datname = $1)", db).Scan(&dbExists)
		if err != nil || !dbExists {
			h.rollbackUser(ctx, req.Username)
			writeError(w, http.StatusBadRequest, "database does not exist: "+db)
			return
		}

		if err := h.grantAccess(ctx, req.Username, db, req.Access); err != nil {
			h.rollbackUser(ctx, req.Username)
			writeError(w, http.StatusInternalServerError, "failed to grant access on "+db+": "+err.Error())
			return
		}

		_, err = h.pool.Exec(ctx,
			"INSERT INTO managed_users (username, database_name, access) VALUES ($1, $2, $3)",
			req.Username, db, req.Access,
		)
		if err != nil {
			h.rollbackUser(ctx, req.Username)
			writeError(w, http.StatusInternalServerError, "failed to save metadata: "+err.Error())
			return
		}
	}

	host := os.Getenv("PGMANAGER_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("PGMANAGER_PORT")
	if port == "" {
		port = "5432"
	}
	connStr := "postgres://" + req.Username + ":" + password + "@" + host + ":" + port + "/" + req.Databases[0]

	writeJSON(w, http.StatusCreated, createUserResponse{
		Username:         req.Username,
		Password:         password,
		Databases:        req.Databases,
		ConnectionString: connStr,
		Access:           req.Access,
		CreatedAt:        time.Now().Format(time.RFC3339),
	})

	user := auth.GetUserFromContext(r.Context())
	username := ""
	if user != nil {
		username = user.Username
	}
	h.writeAuditLog(r.Context(), auditEntry{
		Username:  username,
		Action:    "create_user",
		Detail:    map[string]interface{}{"target": req.Username, "databases": req.Databases, "access": req.Access},
		IPAddress: clientIP(r),
	})
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	username := extractUserFromPath(r.URL.Path)
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var roleExists bool
	err := h.pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", username).Scan(&roleExists)
	if err != nil || !roleExists {
		writeError(w, http.StatusBadRequest, "user not found")
		return
	}

	ctx := r.Context()

	if req.Password != "" {
		if !validPassword(req.Password) {
			writeError(w, http.StatusBadRequest, "password must be 8-128 ASCII characters")
			return
		}
		_, err = h.pool.Exec(ctx, "ALTER ROLE "+quoteIdent(username)+" PASSWORD "+quoteLiteral(req.Password))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update password: "+err.Error())
			return
		}
	}

	if req.Access != "" {
		validAccess := map[string]bool{"read": true, "write": true, "ddl": true, "full": true}
		if !validAccess[req.Access] {
			writeError(w, http.StatusBadRequest, "invalid access level (read, write, ddl, full)")
			return
		}

		databases := h.getUserDatabases(ctx, username)
		for _, db := range databases {
			if err := h.revokeAccess(ctx, username, db); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to revoke access: "+err.Error())
				return
			}
			if err := h.grantAccess(ctx, username, db, req.Access); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to grant access: "+err.Error())
				return
			}
		}

		_, err = h.pool.Exec(ctx, "UPDATE managed_users SET access = $1 WHERE username = $2", req.Access, username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update metadata: "+err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	user := auth.GetUserFromContext(r.Context())
	actor := ""
	if user != nil {
		actor = user.Username
	}
	h.writeAuditLog(r.Context(), auditEntry{
		Username:  actor,
		Action:    "update_user",
		Detail:    map[string]interface{}{"target": username, "access": req.Access},
		IPAddress: clientIP(r),
	})
}

func (h *Handler) AddUserDatabase(w http.ResponseWriter, r *http.Request) {
	username := extractUserFromPath(r.URL.Path)
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	var req addDatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Database = strings.TrimSpace(req.Database)

	if req.Database == "" {
		writeError(w, http.StatusBadRequest, "database is required")
		return
	}
	if protectedDatabases[req.Database] {
		writeError(w, http.StatusBadRequest, "cannot grant access to system database")
		return
	}

	ctx := r.Context()

	var roleExists bool
	err := h.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", username).Scan(&roleExists)
	if err != nil || !roleExists {
		writeError(w, http.StatusBadRequest, "user not found")
		return
	}

	var dbExists bool
	err = h.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_database WHERE datname = $1)", req.Database).Scan(&dbExists)
	if err != nil || !dbExists {
		writeError(w, http.StatusBadRequest, "database does not exist")
		return
	}

	var alreadyHas bool
	err = h.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM managed_users WHERE username = $1 AND database_name = $2)", username, req.Database).Scan(&alreadyHas)
	if err != nil || alreadyHas {
		writeError(w, http.StatusBadRequest, "user already has access to this database")
		return
	}

	var access string
	err = h.pool.QueryRow(ctx, "SELECT access FROM managed_users WHERE username = $1 LIMIT 1", username).Scan(&access)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get user access level")
		return
	}

	if err := h.grantAccess(ctx, username, req.Database, access); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to grant access: "+err.Error())
		return
	}

	_, err = h.pool.Exec(ctx,
		"INSERT INTO managed_users (username, database_name, access) VALUES ($1, $2, $3)",
		username, req.Database, access,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save metadata: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "granted"})

	user := auth.GetUserFromContext(r.Context())
	actor := ""
	if user != nil {
		actor = user.Username
	}
	h.writeAuditLog(r.Context(), auditEntry{
		Username:  actor,
		Action:    "add_user_database",
		Database:  req.Database,
		Detail:    map[string]interface{}{"target": username},
		IPAddress: clientIP(r),
	})
}

func (h *Handler) RemoveUserDatabase(w http.ResponseWriter, r *http.Request) {
	username, db := extractUserDBFromPath(r.URL.Path)
	if username == "" || db == "" {
		writeError(w, http.StatusBadRequest, "username and database are required")
		return
	}

	ctx := r.Context()

	_, err := h.pool.Exec(ctx, "DELETE FROM managed_users WHERE username = $1 AND database_name = $2", username, db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove metadata")
		return
	}

	_ = h.revokeAccess(ctx, username, db)

	remaining := h.getUserDatabases(ctx, username)
	if len(remaining) == 0 {
		_, _ = h.pool.Exec(ctx, "DROP OWNED BY "+quoteIdent(username)+" CASCADE")
		_, _ = h.pool.Exec(ctx, "DROP ROLE IF EXISTS "+quoteIdent(username))
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})

	user := auth.GetUserFromContext(r.Context())
	actor := ""
	if user != nil {
		actor = user.Username
	}
	h.writeAuditLog(r.Context(), auditEntry{
		Username:  actor,
		Action:    "remove_user_database",
		Database:  db,
		Detail:    map[string]interface{}{"target": username},
		IPAddress: clientIP(r),
	})
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	username := extractUserFromPath(r.URL.Path)
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	ctx := r.Context()
	_, _ = h.pool.Exec(ctx, "DROP OWNED BY "+quoteIdent(username)+" CASCADE")
	_, _ = h.pool.Exec(ctx, "DROP ROLE IF EXISTS "+quoteIdent(username))
	_, _ = h.pool.Exec(ctx, "DELETE FROM managed_users WHERE username = $1", username)

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	user := auth.GetUserFromContext(r.Context())
	actor := ""
	if user != nil {
		actor = user.Username
	}
	h.writeAuditLog(r.Context(), auditEntry{
		Username:  actor,
		Action:    "delete_user",
		Detail:    map[string]interface{}{"target": username},
		IPAddress: clientIP(r),
	})
}

func (h *Handler) getUserDatabases(ctx context.Context, username string) []string {
	rows, err := h.pool.Query(ctx, "SELECT database_name FROM managed_users WHERE username = $1", username)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var dbs []string
	for rows.Next() {
		var db string
		if err := rows.Scan(&db); err == nil {
			dbs = append(dbs, db)
		}
	}
	return dbs
}

func (h *Handler) grantAccess(ctx context.Context, username, db, access string) error {
	if _, err := h.pool.Exec(ctx, "GRANT CONNECT ON DATABASE "+quoteIdent(db)+" TO "+quoteIdent(username)); err != nil {
		return err
	}
	if _, err := h.pool.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+quoteIdent(username)); err != nil {
		return err
	}

	switch access {
	case "read":
		if _, err := h.pool.Exec(ctx, "GRANT SELECT ON ALL TABLES IN SCHEMA public TO "+quoteIdent(username)); err != nil {
			return err
		}
		if _, err := h.pool.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO "+quoteIdent(username)); err != nil {
			return err
		}
	case "write":
		if _, err := h.pool.Exec(ctx, "GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO "+quoteIdent(username)); err != nil {
			return err
		}
		if _, err := h.pool.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "+quoteIdent(username)); err != nil {
			return err
		}
	case "ddl":
		if _, err := h.pool.Exec(ctx, "GRANT USAGE, CREATE ON SCHEMA public TO "+quoteIdent(username)); err != nil {
			return err
		}
		if _, err := h.pool.Exec(ctx, "GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO "+quoteIdent(username)); err != nil {
			return err
		}
		if _, err := h.pool.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "+quoteIdent(username)); err != nil {
			return err
		}
	case "full":
		if _, err := h.pool.Exec(ctx, "GRANT ALL PRIVILEGES ON DATABASE "+quoteIdent(db)+" TO "+quoteIdent(username)); err != nil {
			return err
		}
		if _, err := h.pool.Exec(ctx, "GRANT ALL ON SCHEMA public TO "+quoteIdent(username)); err != nil {
			return err
		}
		if _, err := h.pool.Exec(ctx, "GRANT ALL ON ALL TABLES IN SCHEMA public TO "+quoteIdent(username)); err != nil {
			return err
		}
		if _, err := h.pool.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO "+quoteIdent(username)); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) revokeAccess(ctx context.Context, username, db string) error {
	h.pool.Exec(ctx, "REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM "+quoteIdent(username))
	h.pool.Exec(ctx, "REVOKE USAGE ON SCHEMA public FROM "+quoteIdent(username))
	h.pool.Exec(ctx, "REVOKE CREATE ON SCHEMA public FROM "+quoteIdent(username))
	h.pool.Exec(ctx, "REVOKE ALL PRIVILEGES ON DATABASE "+quoteIdent(db)+" FROM "+quoteIdent(username))
	h.pool.Exec(ctx, "REVOKE CONNECT ON DATABASE "+quoteIdent(db)+" FROM "+quoteIdent(username))
	return nil
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
