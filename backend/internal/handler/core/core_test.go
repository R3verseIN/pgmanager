package core

import (
	"testing"
)

func TestQuoteIdent(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"users", `"users"`},
		{"my_table", `"my_table"`},
		{"table; DROP", `"table; DROP"`},
		{"", `""`},
	}
	for _, tt := range tests {
		got := QuoteIdent(tt.input)
		if got != tt.expected {
			t.Errorf("QuoteIdent(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestQuoteLiteral(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "'hello'"},
		{"it's", "'it''s'"},
		{"", "''"},
	}
	for _, tt := range tests {
		got := QuoteLiteral(tt.input)
		if got != tt.expected {
			t.Errorf("QuoteLiteral(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestValidName(t *testing.T) {
	good := []string{"users", "my_table", "t1", "schema1_table1"}
	for _, s := range good {
		if !ValidName.MatchString(s) {
			t.Errorf("ValidName(%q) = false, want true", s)
		}
	}
	bad := []string{"", "123table", "table name", "table;DROP", "a.b"}
	for _, s := range bad {
		if ValidName.MatchString(s) {
			t.Errorf("ValidName(%q) = true, want false", s)
		}
	}
}

func TestProtectedDatabases(t *testing.T) {
	protected := []string{"postgres", "template0", "template1", "pgmanager"}
	for _, db := range protected {
		if !ProtectedDatabases[db] {
			t.Errorf("expected %q to be protected", db)
		}
	}
	if ProtectedDatabases["mydb"] {
		t.Error("expected mydb to not be protected")
	}
}

func TestIsBlockedSQL(t *testing.T) {
	blocked := []string{
		"DROP DATABASE mydb",
		"DROP OWNED BY role1",
		"ALTER ROLE admin WITH SUPERUSER",
		"CREATE ROLE admin WITH LOGIN",
		"DROP ROLE admin",
		"GRANT ALL ON users TO public",
		"REVOKE ALL ON users FROM public",
		"TRUNCATE TABLE logs",
		"COMMENT ON DATABASE mydb IS 'test'",
	}
	for _, q := range blocked {
		if !IsBlockedSQL(q) {
			t.Errorf("IsBlockedSQL(%q) = false, want true", q)
		}
	}
	allowed := []string{
		"SELECT * FROM users",
		"INSERT INTO logs (msg) VALUES ('test')",
		"UPDATE users SET name='bob' WHERE id=1",
		"DELETE FROM logs WHERE id=1",
		"SELECT 1",
	}
	for _, q := range allowed {
		if IsBlockedSQL(q) {
			t.Errorf("IsBlockedSQL(%q) = true, want false", q)
		}
	}
}

func TestGeneratePassword(t *testing.T) {
	p1 := GeneratePassword(32)
	p2 := GeneratePassword(32)
	if len(p1) != 32 {
		t.Errorf("GeneratePassword(32) length = %d, want 32", len(p1))
	}
	if p1 == p2 {
		t.Error("GeneratePassword() should produce unique passwords")
	}
}

func TestValidPassword(t *testing.T) {
	good := []string{"password", "Password1", "abcdefg1", "Str0ngPass", "ALLUPPERCASE", "alllowercase", "12345678"}
	for _, p := range good {
		if !ValidPassword(p) {
			t.Errorf("ValidPassword(%q) = false, want true", p)
		}
	}
	bad := []string{"", "short1", "has space", "has@special", "has.dot"}
	for _, p := range bad {
		if ValidPassword(p) {
			t.Errorf("ValidPassword(%q) = true, want false", p)
		}
	}
}

func TestBuildWhereClauses(t *testing.T) {
	conditions := []WhereCondition{
		{Column: "name", Operator: "=", Value: "test"},
		{Column: "age", Operator: ">", Value: "25"},
	}
	clauses, args, err := BuildWhereClauses(conditions, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clauses) != 2 {
		t.Errorf("expected 2 clauses, got %d", len(clauses))
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}

	_, _, err = BuildWhereClauses(nil, 1)
	if err == nil {
		t.Error("expected error for nil conditions")
	}

	_, _, err = BuildWhereClauses([]WhereCondition{{}}, 1)
	if err == nil {
		t.Error("expected error for empty column")
	}

	_, _, err = BuildWhereClauses([]WhereCondition{{Column: "x", Operator: "BETWEEN", Value: "1"}}, 1)
	if err == nil {
		t.Error("expected error for unsupported operator")
	}
}
