package sql

import "testing"

func TestExtractDBNameFromQuery(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/api/databases/mydb/query", "mydb"},
		{"/api/databases/test_db/query", "test_db"},
		{"/api/databases/mydb/query/extra", "mydb"},
		{"/api/databases/", ""},
		{"/api/other/mydb/query", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractDBNameFromQuery(tt.path)
		if got != tt.expected {
			t.Errorf("extractDBNameFromQuery(%q) = %q, want %q", tt.path, got, tt.expected)
		}
	}
}
