package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"pgmanager/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthHandler struct {
	pool *pgxpool.Pool
}

func NewAuthHandler(pool *pgxpool.Pool) *AuthHandler {
	return &AuthHandler{pool: pool}
}

func (h *AuthHandler) writeAuditLog(ctx context.Context, entry auditEntry) {
	_, err := h.pool.Exec(ctx,
		`INSERT INTO audit_log (username, action, database, table_name, detail, ip_address)
		 VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)`,
		entry.Username, entry.Action, entry.Database,
		entry.TableName, entry.Detail, entry.IPAddress,
	)
	if err != nil {
		log.Printf("audit log write failed: %v", err)
	}
}

type setupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type createAuthUserRequest struct {
	Username  string   `json:"username"`
	Password  string   `json:"password"`
	Role      string   `json:"role"`
	Databases []string `json:"databases"`
}

type updateAuthUserRequest struct {
	Role      string   `json:"role,omitempty"`
	Databases []string `json:"databases"`
}

func (h *AuthHandler) SetupCheck(w http.ResponseWriter, r *http.Request) {
	var count int
	err := h.pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM auth_users").Scan(&count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needsSetup": count == 0})
}

func (h *AuthHandler) Setup(w http.ResponseWriter, r *http.Request) {
	var count int
	err := h.pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM auth_users").Scan(&count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check users")
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "admin account already exists")
		return
	}

	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 {
		writeError(w, http.StatusBadRequest, "username must be at least 3 characters")
		return
	}
	if !validName.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "invalid username: must start with letter or underscore, alphanumeric only")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if len(req.Password) > 72 {
		writeError(w, http.StatusBadRequest, "password must be at most 72 characters")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		"INSERT INTO auth_users (username, password_hash, role) VALUES ($1, $2, 'admin')",
		req.Username, hash,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "admin account created"})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	var user struct {
		ID           int
		PasswordHash string
		Role         string
	}
	err := h.pool.QueryRow(r.Context(),
		"SELECT id, password_hash, role FROM auth_users WHERE username = $1",
		req.Username,
	).Scan(&user.ID, &user.PasswordHash, &user.Role)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := auth.CreateSession(r.Context(), h.pool, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	auth.SetSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged in"})

	h.writeAuditLog(r.Context(), auditEntry{
		Username:  req.Username,
		Action:    "login",
		IPAddress: clientIP(r),
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token := auth.ExtractTokenFromCookie(r)
	if token != "" {
		auth.DeleteSession(r.Context(), h.pool, token)
	}
	auth.ClearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})

	user := auth.GetUserFromContext(r.Context())
	username := ""
	if user != nil {
		username = user.Username
	}
	h.writeAuditLog(r.Context(), auditEntry{
		Username:  username,
		Action:    "logout",
		IPAddress: clientIP(r),
	})
}

func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"username": user.Username,
		"role":     user.Role,
	})
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(req.CurrentPassword) == 0 || len(req.NewPassword) == 0 {
		writeError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	if len(req.NewPassword) > 72 {
		writeError(w, http.StatusBadRequest, "new password must be at most 72 characters")
		return
	}

	var currentHash string
	err := h.pool.QueryRow(r.Context(),
		"SELECT password_hash FROM auth_users WHERE id = $1",
		user.ID,
	).Scan(&currentHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	if !auth.CheckPassword(currentHash, req.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		"UPDATE auth_users SET password_hash = $1, updated_at = NOW() WHERE id = $2",
		newHash, user.ID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	auth.DeleteUserSessions(r.Context(), h.pool, user.ID)
	auth.ClearSessionCookie(w)

	writeJSON(w, http.StatusOK, map[string]string{"status": "password changed"})

	h.writeAuditLog(r.Context(), auditEntry{
		Username:  user.Username,
		Action:    "change_password",
		IPAddress: clientIP(r),
	})
}

func (h *AuthHandler) CreateAuthUser(w http.ResponseWriter, r *http.Request) {
	var req createAuthUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 {
		writeError(w, http.StatusBadRequest, "username must be at least 3 characters")
		return
	}
	if !validName.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "invalid username: must start with letter or underscore, alphanumeric only")
		return
	}
	if req.Password != "" {
		if len(req.Password) < 8 {
			writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
			return
		}
		if len(req.Password) > 72 {
			writeError(w, http.StatusBadRequest, "password must be at most 72 characters")
			return
		}
	} else {
		req.Password = GeneratePassword(16)
	}
	if req.Role != "admin" && req.Role != "dev" && req.Role != "viewer" {
		writeError(w, http.StatusBadRequest, "role must be admin, dev, or viewer")
		return
	}

	if req.Role == "dev" {
		if len(req.Databases) == 0 {
			writeError(w, http.StatusBadRequest, "databases are required for dev role")
			return
		}
		for _, db := range req.Databases {
			if protectedDatabases[db] {
				writeError(w, http.StatusBadRequest, "cannot assign system database: "+db)
				return
			}
			var dbExists bool
			if err := h.pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_database WHERE datname = $1)", db).Scan(&dbExists); err != nil || !dbExists {
				writeError(w, http.StatusBadRequest, "database does not exist: "+db)
				return
			}
		}
	}

	var exists bool
	err := h.pool.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM auth_users WHERE username = $1)",
		req.Username,
	).Scan(&exists)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check user")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "username already exists")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		"INSERT INTO auth_users (username, password_hash, role) VALUES ($1, $2, $3)",
		req.Username, hash, req.Role,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user: "+err.Error())
		return
	}

	if req.Role == "dev" {
		var userID int
		if err := h.pool.QueryRow(r.Context(), "SELECT id FROM auth_users WHERE username = $1", req.Username).Scan(&userID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get user id")
			return
		}
		for _, db := range req.Databases {
			if _, err := h.pool.Exec(r.Context(), "INSERT INTO dev_databases (auth_user_id, database_name) VALUES ($1, $2)", userID, db); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to assign database: "+err.Error())
				return
			}
		}
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"status":   "created",
		"username": req.Username,
		"password": req.Password,
	})

	adminUser := auth.GetUserFromContext(r.Context())
	adminUsername := ""
	if adminUser != nil {
		adminUsername = adminUser.Username
	}
	h.writeAuditLog(r.Context(), auditEntry{
		Username:  adminUsername,
		Action:    "create_auth_user",
		Detail:    map[string]interface{}{"target": req.Username, "role": req.Role},
		IPAddress: clientIP(r),
	})
}

func (h *AuthHandler) UpdateAuthUser(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimPrefix(r.URL.Path, "/api/auth/users/")
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	currentUser := auth.GetUserFromContext(r.Context())
	if currentUser != nil && currentUser.Username == username && currentUser.Role == "admin" {
		writeError(w, http.StatusBadRequest, "cannot change your own role")
		return
	}

	var req updateAuthUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Role != "admin" && req.Role != "dev" && req.Role != "viewer" {
		writeError(w, http.StatusBadRequest, "role must be admin, dev, or viewer")
		return
	}

	var id int
	var currentRole string
	err := h.pool.QueryRow(r.Context(),
		"SELECT id, role FROM auth_users WHERE username = $1",
		username,
	).Scan(&id, &currentRole)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	// Validate databases if switching to dev
	if req.Role == "dev" {
		if len(req.Databases) == 0 {
			writeError(w, http.StatusBadRequest, "databases are required for dev role")
			return
		}
		for _, db := range req.Databases {
			if protectedDatabases[db] {
				writeError(w, http.StatusBadRequest, "cannot assign system database: "+db)
				return
			}
			var dbExists bool
			if err := h.pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_database WHERE datname = $1)", db).Scan(&dbExists); err != nil || !dbExists {
				writeError(w, http.StatusBadRequest, "database does not exist: "+db)
				return
			}
		}
	}

	// Handle demoting the last admin
	if currentRole == "admin" && req.Role != "admin" {
		ct, err := h.pool.Exec(r.Context(),
			"UPDATE auth_users SET role = $1, updated_at = NOW() WHERE id = $2 AND (SELECT COUNT(*) FROM auth_users WHERE role = 'admin') > 1",
			req.Role, id,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update user")
			return
		}
		if ct.RowsAffected() == 0 {
			writeError(w, http.StatusBadRequest, "cannot change role of the last admin")
			return
		}
	} else {
		_, err = h.pool.Exec(r.Context(),
			"UPDATE auth_users SET role = $1, updated_at = NOW() WHERE id = $2",
			req.Role, id,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update user")
			return
		}
	}

	// Manage dev_databases
	if req.Role == "dev" {
		h.pool.Exec(r.Context(), "DELETE FROM dev_databases WHERE auth_user_id = $1", id)
		for _, db := range req.Databases {
			h.pool.Exec(r.Context(), "INSERT INTO dev_databases (auth_user_id, database_name) VALUES ($1, $2) ON CONFLICT DO NOTHING", id, db)
		}
	} else {
		// Clear dev_databases when switching away from dev
		h.pool.Exec(r.Context(), "DELETE FROM dev_databases WHERE auth_user_id = $1", id)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	adminUser := auth.GetUserFromContext(r.Context())
	adminUsername := ""
	if adminUser != nil {
		adminUsername = adminUser.Username
	}
	h.writeAuditLog(r.Context(), auditEntry{
		Username:  adminUsername,
		Action:    "update_auth_user",
		Detail:    map[string]interface{}{"target": username, "role": req.Role},
		IPAddress: clientIP(r),
	})
}

func (h *AuthHandler) DeleteAuthUser(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimPrefix(r.URL.Path, "/api/auth/users/")
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	currentUser := auth.GetUserFromContext(r.Context())
	if currentUser != nil && currentUser.Username == username {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	var id int
	var role string
	err := h.pool.QueryRow(r.Context(),
		"SELECT id, role FROM auth_users WHERE username = $1",
		username,
	).Scan(&id, &role)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if role == "admin" {
		ct, err := h.pool.Exec(r.Context(),
			"DELETE FROM auth_users WHERE id = $1 AND role = 'admin' AND (SELECT COUNT(*) FROM auth_users WHERE role = 'admin') > 1",
			id,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete user")
			return
		}
		if ct.RowsAffected() == 0 {
			writeError(w, http.StatusBadRequest, "cannot delete the last admin")
			return
		}
		auth.DeleteUserSessions(r.Context(), h.pool, id)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

		adminUser := auth.GetUserFromContext(r.Context())
		adminUsername := ""
		if adminUser != nil {
			adminUsername = adminUser.Username
		}
		h.writeAuditLog(r.Context(), auditEntry{
			Username:  adminUsername,
			Action:    "delete_auth_user",
			Detail:    map[string]interface{}{"target": username},
			IPAddress: clientIP(r),
		})
		return
	}

	auth.DeleteUserSessions(r.Context(), h.pool, id)
	_, err = h.pool.Exec(r.Context(), "DELETE FROM auth_users WHERE id = $1", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	adminUser := auth.GetUserFromContext(r.Context())
	adminUsername := ""
	if adminUser != nil {
		adminUsername = adminUser.Username
	}
	h.writeAuditLog(r.Context(), auditEntry{
		Username:  adminUsername,
		Action:    "delete_auth_user",
		Detail:    map[string]interface{}{"target": username},
		IPAddress: clientIP(r),
	})
}

type authUserListItem struct {
	ID        int      `json:"id"`
	Username  string   `json:"username"`
	Role      string   `json:"role"`
	Databases []string `json:"databases,omitempty"`
	CreatedAt string   `json:"createdAt"`
}

func (h *AuthHandler) ListAuthUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT au.id, au.username, au.role, au.created_at::text,
			COALESCE(array_remove(array_agg(dd.database_name) FILTER (WHERE dd.database_name IS NOT NULL), NULL), '{}') AS databases
		FROM auth_users au
		LEFT JOIN dev_databases dd ON dd.auth_user_id = au.id
		GROUP BY au.id, au.username, au.role, au.created_at
		ORDER BY au.id
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	defer rows.Close()

	var users []authUserListItem
	for rows.Next() {
		var u authUserListItem
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt, &u.Databases); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan user")
			return
		}
		users = append(users, u)
	}
	if users == nil {
		users = []authUserListItem{}
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *AuthHandler) ResetAuthUserPassword(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	prefix := "/api/auth/users/"
	suffix := "/reset-password"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	username := strings.TrimPrefix(path, prefix)
	username = strings.TrimSuffix(username, suffix)
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	var id int
	err := h.pool.QueryRow(r.Context(),
		"SELECT id FROM auth_users WHERE username = $1",
		username,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	// It's optional, so we ignore errors if body is empty or invalid
	_ = json.NewDecoder(r.Body).Decode(&req)

	var newPassword string
	if req.Password != "" {
		if len(req.Password) < 8 {
			writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
			return
		}
		if len(req.Password) > 72 {
			writeError(w, http.StatusBadRequest, "password must be at most 72 characters")
			return
		}
		newPassword = req.Password
	} else {
		newPassword = GeneratePassword(16)
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		"UPDATE auth_users SET password_hash = $1, updated_at = NOW() WHERE id = $2",
		hash, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	auth.DeleteUserSessions(r.Context(), h.pool, id)

	writeJSON(w, http.StatusOK, map[string]string{"password": newPassword})

	adminUser := auth.GetUserFromContext(r.Context())
	adminUsername := ""
	if adminUser != nil {
		adminUsername = adminUser.Username
	}
	h.writeAuditLog(r.Context(), auditEntry{
		Username:  adminUsername,
		Action:    "reset_password",
		Detail:    map[string]interface{}{"target": username},
		IPAddress: clientIP(r),
	})
}
