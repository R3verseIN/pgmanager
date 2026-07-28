package backup

import "testing"

func TestSanitizeRedact(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "postgres URL",
			input:    "postgres://pgmanager:secretpass@localhost:5433/mydb",
			expected: "postgres://pgmanager:***@localhost:5433/mydb",
		},
		{
			name:     "PGPASSWORD env var",
			input:    "PGPASSWORD=mysecretpassword pg_dump",
			expected: "PGPASSWORD=*** pg_dump",
		},
		{
			name:     "password= param",
			input:    "password=mysecret123 other=stuff",
			expected: "password=*** other=stuff",
		},
		{
			name:     "multiple redactions",
			input:    "PGPASSWORD=pass1 postgres://user:pass2@host/db password=pass3",
			expected: "PGPASSWORD=*** postgres://user:***@host/db password=***",
		},
		{
			name:     "no matches",
			input:    "nothing to redact here",
			expected: "nothing to redact here",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeRedact(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeRedact(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
