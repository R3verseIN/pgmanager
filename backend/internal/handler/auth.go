package handler

import (
	"encoding/json"
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
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type updateAuthUserRequest struct {
	Role string `json:"role,omitempty"`
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
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token := auth.ExtractTokenFromCookie(r)
	if token != "" {
		auth.DeleteSession(r.Context(), h.pool, token)
	}
	auth.ClearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
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
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if req.Role != "admin" && req.Role != "viewer" {
		writeError(w, http.StatusBadRequest, "role must be admin or viewer")
		return
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

	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (h *AuthHandler) UpdateAuthUser(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimPrefix(r.URL.Path, "/api/auth/users/")
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	var req updateAuthUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Role != "admin" && req.Role != "viewer" {
		writeError(w, http.StatusBadRequest, "role must be admin or viewer")
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

	if currentRole == "admin" && req.Role == "viewer" {
		var adminCount int
		err = h.pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM auth_users WHERE role = 'admin'").Scan(&adminCount)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check admin count")
			return
		}
		if adminCount <= 1 {
			writeError(w, http.StatusBadRequest, "cannot change role of the last admin")
			return
		}
	}

	_, err = h.pool.Exec(r.Context(),
		"UPDATE auth_users SET role = $1, updated_at = NOW() WHERE id = $2",
		req.Role, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *AuthHandler) DeleteAuthUser(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimPrefix(r.URL.Path, "/api/auth/users/")
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
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
		var adminCount int
		err = h.pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM auth_users WHERE role = 'admin'").Scan(&adminCount)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check admin count")
			return
		}
		if adminCount <= 1 {
			writeError(w, http.StatusBadRequest, "cannot delete the last admin")
			return
		}
	}

	auth.DeleteUserSessions(r.Context(), h.pool, id)
	_, err = h.pool.Exec(r.Context(), "DELETE FROM auth_users WHERE id = $1", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type authUserListItem struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
}

func (h *AuthHandler) ListAuthUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), "SELECT id, username, role, created_at::text FROM auth_users ORDER BY id")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	defer rows.Close()

	var users []authUserListItem
	for rows.Next() {
		var u authUserListItem
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt); err != nil {
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

	newPassword := GeneratePassword(16)
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
}
