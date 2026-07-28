package testutil

import (
	"context"
	"testing"
)

func TestContainerSmoke(t *testing.T) {
	pool, _ := TestContainer(t)
	ctx := context.Background()

	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count < 6 {
		t.Errorf("expected at least 6 tables, got %d", count)
	}

	var result string
	err = pool.QueryRow(ctx, "SELECT value FROM system_config WHERE key = 'setup_completed'").Scan(&result)
	if err != nil {
		t.Fatalf("system_config query failed: %v", err)
	}
	if result != "true" {
		t.Errorf("expected setup_completed=true, got %q", result)
	}
}
