package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	pkgauth "pgmanager/internal/auth"
	"pgmanager/internal/handler/core"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

func (h *Handler) writeAuditLog(ctx context.Context, entry core.AuditEntry) {
	core.WriteAuditLog(h.pool, ctx, entry)
}

func (h *Handler) SetupCheck(w http.ResponseWriter, r *http.Request) {
	var completed bool
	_ = h.pool.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM system_config WHERE key = 'setup_completed' AND value = 'true')",
	).Scan(&completed)

	var count int
	_ = h.pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM auth_users").Scan(&count)

	needsSetup := !completed && count == 0
	core.WriteJSON(w, http.StatusOK, map[string]bool{"needsSetup": needsSetup})
}

func (h *Handler) Setup(w http.ResponseWriter, r *http.Request) {
	var completed bool
	_ = h.pool.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM system_config WHERE key = 'setup_completed' AND value = 'true')",
	).Scan(&completed)
	if completed {
		core.WriteError(w, http.StatusConflict, "setup already completed — use reset scripts to recover")
		return
	}

	var count int
	_ = h.pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM auth_users").Scan(&count)
	if count > 0 {
		core.WriteError(w, http.StatusConflict, "admin account already exists")
		return
	}

	var req SetupRequest
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
	if len(req.Password) < 8 {
		core.WriteError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if len(req.Password) > 72 {
		core.WriteError(w, http.StatusBadRequest, "password must be at most 72 characters")
		return
	}

	hash, err := pkgauth.HashPassword(req.Password)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		"INSERT INTO auth_users (username, password_hash, role) VALUES ($1, $2, 'admin')",
		req.Username, hash,
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to create user: "+err.Error())
		return
	}

	_, _ = h.pool.Exec(r.Context(),
		"INSERT INTO system_config (key, value) VALUES ('setup_completed', 'true') ON CONFLICT (key) DO UPDATE SET value = 'true'",
	)

	core.WriteJSON(w, http.StatusCreated, map[string]string{"message": "admin account created"})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		core.WriteError(w, http.StatusBadRequest, "username and password are required")
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
		core.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if !pkgauth.CheckPassword(user.PasswordHash, req.Password) {
		core.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := pkgauth.CreateSession(r.Context(), h.pool, user.ID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	pkgauth.SetSessionCookie(w, token)
	core.WriteJSON(w, http.StatusOK, map[string]string{"status": "logged in"})

	h.writeAuditLog(r.Context(), core.AuditEntry{
		Username:  req.Username,
		Action:    "login",
		IPAddress: core.ClientIP(r),
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	token := pkgauth.ExtractTokenFromCookie(r)
	if token != "" {
		pkgauth.DeleteSession(r.Context(), h.pool, token)
	}
	pkgauth.ClearSessionCookie(w)
	core.WriteJSON(w, http.StatusOK, map[string]string{"status": "logged out"})

	user := pkgauth.GetUserFromContext(r.Context())
	username := ""
	if user != nil {
		username = user.Username
	}
	h.writeAuditLog(r.Context(), core.AuditEntry{
		Username:  username,
		Action:    "logout",
		IPAddress: core.ClientIP(r),
	})
}

func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	user := pkgauth.GetUserFromContext(r.Context())
	if user == nil {
		core.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]string{
		"username": user.Username,
		"role":     user.Role,
	})
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := pkgauth.GetUserFromContext(r.Context())
	if user == nil {
		core.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(req.CurrentPassword) == 0 || len(req.NewPassword) == 0 {
		core.WriteError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}
	if len(req.NewPassword) < 8 {
		core.WriteError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	if len(req.NewPassword) > 72 {
		core.WriteError(w, http.StatusBadRequest, "new password must be at most 72 characters")
		return
	}

	var currentHash string
	err := h.pool.QueryRow(r.Context(),
		"SELECT password_hash FROM auth_users WHERE id = $1",
		user.ID,
	).Scan(&currentHash)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	if !pkgauth.CheckPassword(currentHash, req.CurrentPassword) {
		core.WriteError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	newHash, err := pkgauth.HashPassword(req.NewPassword)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		"UPDATE auth_users SET password_hash = $1, updated_at = NOW() WHERE id = $2",
		newHash, user.ID,
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	pkgauth.DeleteUserSessions(r.Context(), h.pool, user.ID)
	pkgauth.ClearSessionCookie(w)

	core.WriteJSON(w, http.StatusOK, map[string]string{"status": "password changed"})

	h.writeAuditLog(r.Context(), core.AuditEntry{
		Username:  user.Username,
		Action:    "change_password",
		IPAddress: core.ClientIP(r),
	})
}
