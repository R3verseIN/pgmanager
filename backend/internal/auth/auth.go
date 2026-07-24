package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type SessionUser struct {
	ID       int
	Username string
	Role     string
}

type contextKey string

const userContextKey contextKey = "user"

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func CheckPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func GenerateSessionToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func CreateSession(ctx context.Context, pool *pgxpool.Pool, userID int) (string, error) {
	token := GenerateSessionToken()
	_, err := pool.Exec(ctx,
		"INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, NOW() + INTERVAL '24 hours')",
		token, userID,
	)
	return token, err
}

func ValidateSession(ctx context.Context, pool *pgxpool.Pool, token string) (*SessionUser, error) {
	var user SessionUser
	err := pool.QueryRow(ctx,
		`SELECT u.id, u.username, u.role
		 FROM auth_users u
		 JOIN sessions s ON s.user_id = u.id
		 WHERE s.id = $1 AND s.expires_at > NOW()`,
		token,
	).Scan(&user.ID, &user.Username, &user.Role)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func DeleteSession(ctx context.Context, pool *pgxpool.Pool, token string) error {
	_, err := pool.Exec(ctx, "DELETE FROM sessions WHERE id = $1", token)
	return err
}

func DeleteUserSessions(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	_, err := pool.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", userID)
	return err
}

func AuthMiddleware(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session_id")
			if err != nil || cookie.Value == "" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			user, err := ValidateSession(r.Context(), pool, cookie.Value)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserFromContext(r.Context())
		if user == nil || user.Role != "admin" {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func GetUserFromContext(ctx context.Context) *SessionUser {
	user, ok := ctx.Value(userContextKey).(*SessionUser)
	if !ok {
		return nil
	}
	return user
}

func WithUser(ctx context.Context, user *SessionUser) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

func ReadSessionTokenFromResponse(resp *http.Response) string {
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session_id" {
			return cookie.Value
		}
	}
	return ""
}

func SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400,
	})
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func ExtractTokenFromCookie(r *http.Request) string {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}
