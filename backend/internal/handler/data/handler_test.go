package data

import "testing"

func TestExtractTableFromData(t *testing.T) {
	tests := []struct {
		path         string
		expectedDB   string
		expectedTable string
	}{
		{"/api/databases/mydb/data/users", "mydb", "users"},
		{"/api/databases/testdb/data/orders", "testdb", "orders"},
		{"/api/databases/mydb/data", "", ""},
		{"/api/databases/mydb/tables/users", "", ""},
		{"/api/other/mydb/data/users", "", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		db, table := extractTableFromData(tt.path)
		if db != tt.expectedDB || table != tt.expectedTable {
			t.Errorf("extractTableFromData(%q) = (%q, %q), want (%q, %q)",
				tt.path, db, table, tt.expectedDB, tt.expectedTable)
		}
	}
}
