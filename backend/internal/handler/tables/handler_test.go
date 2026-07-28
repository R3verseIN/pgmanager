package tables

import "testing"

func TestExtractDBName(t *testing.T) {
	tests := []struct {
		path     string
		suffix   string
		expected string
	}{
		{"/api/databases/mydb/tables", "/tables", "mydb"},
		{"/api/databases/test_db/tables", "/tables", "test_db"},
		{"/api/databases/mydb/tables/users/columns", "/tables", "mydb"},
		{"/api/other/mydb/tables", "/tables", ""},
		{"/api/databases/", "/tables", ""},
		{"", "/tables", ""},
	}
	for _, tt := range tests {
		got := extractDBName(tt.path, tt.suffix)
		if got != tt.expected {
			t.Errorf("extractDBName(%q, %q) = %q, want %q", tt.path, tt.suffix, got, tt.expected)
		}
	}
}

func TestExtractTableFromColumns(t *testing.T) {
	tests := []struct {
		path         string
		expectedDB   string
		expectedTable string
	}{
		{"/api/databases/mydb/tables/users/columns", "mydb", "users"},
		{"/api/databases/testdb/tables/orders/columns", "testdb", "orders"},
		{"/api/databases/mydb/tables", "", ""},
		{"/api/databases/mydb/tables/users", "", ""},
		{"/api/other/mydb/tables/users/columns", "", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		db, table := extractTableFromColumns(tt.path)
		if db != tt.expectedDB || table != tt.expectedTable {
			t.Errorf("extractTableFromColumns(%q) = (%q, %q), want (%q, %q)",
				tt.path, db, table, tt.expectedDB, tt.expectedTable)
		}
	}
}

func TestExtractGetColumns(t *testing.T) {
	tests := []struct {
		path         string
		expectedDB   string
		expectedTable string
	}{
		{"/api/databases/mydb/columns/users", "mydb", "users"},
		{"/api/databases/testdb/columns/orders", "testdb", "orders"},
		{"/api/databases/mydb/columns", "", ""},
		{"/api/other/mydb/columns/users", "", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		db, table := extractGetColumns(tt.path)
		if db != tt.expectedDB || table != tt.expectedTable {
			t.Errorf("extractGetColumns(%q) = (%q, %q), want (%q, %q)",
				tt.path, db, table, tt.expectedDB, tt.expectedTable)
		}
	}
}

func TestExtractTableFromColumnsDrop(t *testing.T) {
	tests := []struct {
		path           string
		expectedDB     string
		expectedTable  string
		expectedColumn string
	}{
		{"/api/databases/mydb/tables/users/columns/email", "mydb", "users", "email"},
		{"/api/databases/testdb/tables/orders/columns/total", "testdb", "orders", "total"},
		{"/api/databases/mydb/tables/users/columns", "", "", ""},
		{"/api/databases/mydb/tables/users", "", "", ""},
		{"/api/other/mydb/tables/users/columns/email", "", "", ""},
		{"", "", "", ""},
	}
	for _, tt := range tests {
		db, table, column := extractTableFromColumnsDrop(tt.path)
		if db != tt.expectedDB || table != tt.expectedTable || column != tt.expectedColumn {
			t.Errorf("extractTableFromColumnsDrop(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.path, db, table, column, tt.expectedDB, tt.expectedTable, tt.expectedColumn)
		}
	}
}
