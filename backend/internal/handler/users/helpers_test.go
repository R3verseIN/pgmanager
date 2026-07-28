package users

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestExtractUserFromPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/api/users/alice", "alice"},
		{"/api/users/bob/databases/mydb", "bob"},
		{"/api/users/", ""},
		{"/api/users", ""},
		{"/api/other/alice", ""},
		{"/api/users/alice/extra/stuff", "alice"},
		{"", ""},
	}
	for _, tt := range tests {
		got := ExtractUserFromPath(tt.path)
		if got != tt.expected {
			t.Errorf("ExtractUserFromPath(%q) = %q, want %q", tt.path, got, tt.expected)
		}
	}
}

func TestExtractUserDBFromPath(t *testing.T) {
	tests := []struct {
		path         string
		expectedUser string
		expectedDB   string
	}{
		{"/api/users/alice/databases/mydb", "alice", "mydb"},
		{"/api/users/bob/databases/testdb", "bob", "testdb"},
		{"/api/users/alice/databases", "", ""},
		{"/api/users/alice/databases/mydb/extra", "alice", "mydb"},
		{"/api/users/alice", "", ""},
		{"/api/other/alice/databases/mydb", "", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		user, db := ExtractUserDBFromPath(tt.path)
		if user != tt.expectedUser || db != tt.expectedDB {
			t.Errorf("ExtractUserDBFromPath(%q) = (%q, %q), want (%q, %q)",
				tt.path, user, db, tt.expectedUser, tt.expectedDB)
		}
	}
}

func TestResolveConnectionStringHost(t *testing.T) {
	tests := []struct {
		name        string
		pgHost      string
		pgPort      string
		requestHost string
		want        string
	}{
		{
			name:        "auto-detect from request host with port",
			pgHost:      "",
			pgPort:      "",
			requestHost: "192.168.0.13:8080",
			want:        "192.168.0.13:5432",
		},
		{
			name:        "auto-detect from request host without port",
			pgHost:      "",
			pgPort:      "",
			requestHost: "pg.example.com",
			want:        "pg.example.com:5432",
		},
		{
			name:        "env host with port",
			pgHost:      "pg.example.com:5050",
			pgPort:      "",
			requestHost: "192.168.0.13:8080",
			want:        "pg.example.com:5050",
		},
		{
			name:        "env host without port uses PGMANAGER_PORT",
			pgHost:      "pg.example.com",
			pgPort:      "5050",
			requestHost: "192.168.0.13:8080",
			want:        "pg.example.com:5050",
		},
		{
			name:        "env host without port defaults to 5432",
			pgHost:      "pg.example.com",
			pgPort:      "",
			requestHost: "192.168.0.13:8080",
			want:        "pg.example.com:5432",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("PGMANAGER_HOST", tt.pgHost)
			os.Setenv("PGMANAGER_PORT", tt.pgPort)
			t.Cleanup(func() {
				os.Unsetenv("PGMANAGER_HOST")
				os.Unsetenv("PGMANAGER_PORT")
			})

			req := httptest.NewRequest(http.MethodPost, "/api/users", nil)
			req.Host = tt.requestHost

			got := ResolveConnectionStringHost(req)
			if got != tt.want {
				t.Errorf("ResolveConnectionStringHost() = %q, want %q", got, tt.want)
			}
		})
	}
}
