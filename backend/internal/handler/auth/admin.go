package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	pkgauth "pgmanager/internal/auth"
	"pgmanager/internal/handler/core"
)

func (h *Handler) CreateAuthUser(w http.ResponseWriter, r *http.Request) {
	var req CreateAuthUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 {
		core.WriteError(w, http.StatusBadRequest, "username must be at least 3 characters")
		return
	}
	if !core.ValidName.MatchString(req.Username) {
		core.WriteError(w, http.StatusBadRequest, "invalid username: must start with letter or underscore, alphanumeric only")
		return
	}
	if req.Password != "" {
		if len(req.Password) < 8 {
			core.WriteError(w, http.StatusBadRequest, "password must be at least 8 characters")
			return
		}
		if len(req.Password) > 72 {
			core.WriteError(w, http.StatusBadRequest, "password must be at most 72 characters")
			return
		}
	} else {
		req.Password = core.GeneratePassword(16)
	}
	if req.Role != "admin" && req.Role != "dev" && req.Role != "viewer" {
		core.WriteError(w, http.StatusBadRequest, "role must be admin, dev, or viewer")
		return
	}

	if req.Role == "dev" {
		if len(req.Databases) == 0 {
			core.WriteError(w, http.StatusBadRequest, "databases are required for dev role")
			return
		}
		for _, db := range req.Databases {
			if core.ProtectedDatabases[db] {
				core.WriteError(w, http.StatusBadRequest, "cannot assign system database: "+db)
				return
			}
			var dbExists bool
			if err := h.pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_database WHERE datname = $1)", db).Scan(&dbExists); err != nil || !dbExists {
				core.WriteError(w, http.StatusBadRequest, "database does not exist: "+db)
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
		core.WriteError(w, http.StatusInternalServerError, "failed to check user")
		return
	}
	if exists {
		core.WriteError(w, http.StatusConflict, "username already exists")
		return
	}

	hash, err := pkgauth.HashPassword(req.Password)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		"INSERT INTO auth_users (username, password_hash, role) VALUES ($1, $2, $3)",
		req.Username, hash, req.Role,
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to create user: "+err.Error())
		return
	}

	if req.Role == "dev" {
		var userID int
		if err := h.pool.QueryRow(r.Context(), "SELECT id FROM auth_users WHERE username = $1", req.Username).Scan(&userID); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to get user id")
			return
		}
		for _, db := range req.Databases {
			if _, err := h.pool.Exec(r.Context(), "INSERT INTO dev_databases (auth_user_id, database_name) VALUES ($1, $2)", userID, db); err != nil {
				core.WriteError(w, http.StatusInternalServerError, "failed to assign database: "+err.Error())
				return
			}
		}
	}

	core.WriteJSON(w, http.StatusCreated, map[string]string{
		"status":   "created",
		"username": req.Username,
		"password": req.Password,
	})

	adminUser := pkgauth.GetUserFromContext(r.Context())
	adminUsername := ""
	if adminUser != nil {
		adminUsername = adminUser.Username
	}
	h.writeAuditLog(r.Context(), core.AuditEntry{
		Username:  adminUsername,
		Action:    "create_auth_user",
		Detail:    map[string]interface{}{"target": req.Username, "role": req.Role},
		IPAddress: core.ClientIP(r),
	})
}

func (h *Handler) UpdateAuthUser(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimPrefix(r.URL.Path, "/api/auth/users/")
	if username == "" {
		core.WriteError(w, http.StatusBadRequest, "username is required")
		return
	}

	currentUser := pkgauth.GetUserFromContext(r.Context())
	if currentUser != nil && currentUser.Username == username && currentUser.Role == "admin" {
		core.WriteError(w, http.StatusBadRequest, "cannot change your own role")
		return
	}

	var req UpdateAuthUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Role != "admin" && req.Role != "dev" && req.Role != "viewer" {
		core.WriteError(w, http.StatusBadRequest, "role must be admin, dev, or viewer")
		return
	}

	var id int
	var currentRole string
	err := h.pool.QueryRow(r.Context(),
		"SELECT id, role FROM auth_users WHERE username = $1",
		username,
	).Scan(&id, &currentRole)
	if err != nil {
		core.WriteError(w, http.StatusNotFound, "user not found")
		return
	}

	if req.Role == "dev" {
		if len(req.Databases) == 0 {
			core.WriteError(w, http.StatusBadRequest, "databases are required for dev role")
			return
		}
		for _, db := range req.Databases {
			if core.ProtectedDatabases[db] {
				core.WriteError(w, http.StatusBadRequest, "cannot assign system database: "+db)
				return
			}
			var dbExists bool
			if err := h.pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_database WHERE datname = $1)", db).Scan(&dbExists); err != nil || !dbExists {
				core.WriteError(w, http.StatusBadRequest, "database does not exist: "+db)
				return
			}
		}
	}

	if currentRole == "admin" && req.Role != "admin" {
		ct, err := h.pool.Exec(r.Context(),
			"UPDATE auth_users SET role = $1, updated_at = NOW() WHERE id = $2 AND (SELECT COUNT(*) FROM auth_users WHERE role = 'admin') > 1",
			req.Role, id,
		)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to update user")
			return
		}
		if ct.RowsAffected() == 0 {
			core.WriteError(w, http.StatusBadRequest, "cannot change role of the last admin")
			return
		}
	} else {
		_, err = h.pool.Exec(r.Context(),
			"UPDATE auth_users SET role = $1, updated_at = NOW() WHERE id = $2",
			req.Role, id,
		)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to update user")
			return
		}
	}

	if req.Role == "dev" {
		h.pool.Exec(r.Context(), "DELETE FROM dev_databases WHERE auth_user_id = $1", id)
		for _, db := range req.Databases {
			h.pool.Exec(r.Context(), "INSERT INTO dev_databases (auth_user_id, database_name) VALUES ($1, $2) ON CONFLICT DO NOTHING", id, db)
		}
	} else {
		h.pool.Exec(r.Context(), "DELETE FROM dev_databases WHERE auth_user_id = $1", id)
	}

	core.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	adminUser := pkgauth.GetUserFromContext(r.Context())
	adminUsername := ""
	if adminUser != nil {
		adminUsername = adminUser.Username
	}
	h.writeAuditLog(r.Context(), core.AuditEntry{
		Username:  adminUsername,
		Action:    "update_auth_user",
		Detail:    map[string]interface{}{"target": username, "role": req.Role},
		IPAddress: core.ClientIP(r),
	})
}

func (h *Handler) DeleteAuthUser(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimPrefix(r.URL.Path, "/api/auth/users/")
	if username == "" {
		core.WriteError(w, http.StatusBadRequest, "username is required")
		return
	}

	currentUser := pkgauth.GetUserFromContext(r.Context())
	if currentUser != nil && currentUser.Username == username {
		core.WriteError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	var id int
	var role string
	err := h.pool.QueryRow(r.Context(),
		"SELECT id, role FROM auth_users WHERE username = $1",
		username,
	).Scan(&id, &role)
	if err != nil {
		core.WriteError(w, http.StatusNotFound, "user not found")
		return
	}

	if role == "admin" {
		ct, err := h.pool.Exec(r.Context(),
			"DELETE FROM auth_users WHERE id = $1 AND role = 'admin' AND (SELECT COUNT(*) FROM auth_users WHERE role = 'admin') > 1",
			id,
		)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to delete user")
			return
		}
		if ct.RowsAffected() == 0 {
			core.WriteError(w, http.StatusBadRequest, "cannot delete the last admin")
			return
		}
		pkgauth.DeleteUserSessions(r.Context(), h.pool, id)
		core.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

		adminUser := pkgauth.GetUserFromContext(r.Context())
		adminUsername := ""
		if adminUser != nil {
			adminUsername = adminUser.Username
		}
		h.writeAuditLog(r.Context(), core.AuditEntry{
			Username:  adminUsername,
			Action:    "delete_auth_user",
			Detail:    map[string]interface{}{"target": username},
			IPAddress: core.ClientIP(r),
		})
		return
	}

	pkgauth.DeleteUserSessions(r.Context(), h.pool, id)
	_, err = h.pool.Exec(r.Context(), "DELETE FROM auth_users WHERE id = $1", id)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	core.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	adminUser := pkgauth.GetUserFromContext(r.Context())
	adminUsername := ""
	if adminUser != nil {
		adminUsername = adminUser.Username
	}
	h.writeAuditLog(r.Context(), core.AuditEntry{
		Username:  adminUsername,
		Action:    "delete_auth_user",
		Detail:    map[string]interface{}{"target": username},
		IPAddress: core.ClientIP(r),
	})
}

func (h *Handler) ListAuthUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT au.id, au.username, au.role, au.created_at::text,
			COALESCE(array_remove(array_agg(dd.database_name) FILTER (WHERE dd.database_name IS NOT NULL), NULL), '{}') AS databases
		FROM auth_users au
		LEFT JOIN dev_databases dd ON dd.auth_user_id = au.id
		GROUP BY au.id, au.username, au.role, au.created_at
		ORDER BY au.id
	`)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	defer rows.Close()

	var users []AuthUserListItem
	for rows.Next() {
		var u AuthUserListItem
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt, &u.Databases); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to scan user")
			return
		}
		users = append(users, u)
	}
	if users == nil {
		users = []AuthUserListItem{}
	}
	core.WriteJSON(w, http.StatusOK, users)
}

func (h *Handler) ResetAuthUserPassword(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	prefix := "/api/auth/users/"
	suffix := "/reset-password"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		core.WriteError(w, http.StatusBadRequest, "invalid path")
		return
	}
	username := strings.TrimPrefix(path, prefix)
	username = strings.TrimSuffix(username, suffix)
	if username == "" {
		core.WriteError(w, http.StatusBadRequest, "username is required")
		return
	}

	var id int
	err := h.pool.QueryRow(r.Context(),
		"SELECT id FROM auth_users WHERE username = $1",
		username,
	).Scan(&id)
	if err != nil {
		core.WriteError(w, http.StatusNotFound, "user not found")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var newPassword string
	if req.Password != "" {
		if len(req.Password) < 8 {
			core.WriteError(w, http.StatusBadRequest, "password must be at least 8 characters")
			return
		}
		if len(req.Password) > 72 {
			core.WriteError(w, http.StatusBadRequest, "password must be at most 72 characters")
			return
		}
		newPassword = req.Password
	} else {
		newPassword = core.GeneratePassword(16)
	}

	hash, err := pkgauth.HashPassword(newPassword)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		"UPDATE auth_users SET password_hash = $1, updated_at = NOW() WHERE id = $2",
		hash, id,
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	pkgauth.DeleteUserSessions(r.Context(), h.pool, id)

	core.WriteJSON(w, http.StatusOK, map[string]string{"password": newPassword})

	adminUser := pkgauth.GetUserFromContext(r.Context())
	adminUsername := ""
	if adminUser != nil {
		adminUsername = adminUser.Username
	}
	h.writeAuditLog(r.Context(), core.AuditEntry{
		Username:  adminUsername,
		Action:    "reset_password",
		Detail:    map[string]interface{}{"target": username},
		IPAddress: core.ClientIP(r),
	})
}
