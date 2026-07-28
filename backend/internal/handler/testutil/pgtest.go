package testutil

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"pgmanager/internal/handler/core"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestContainer(t *testing.T) (pool *pgxpool.Pool, baseDSN string) {
	t.Helper()

	ctx := context.Background()

	_, filename, _, _ := runtime.Caller(0)
	schemaPath := filename[:strings.LastIndex(filename, "/")] + "/schema.sql"

	ctr, err := postgres.Run(ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("pgmanager"),
		postgres.WithUsername("pgmanager"),
		postgres.WithPassword("pgmanager"),
		postgres.WithInitScripts(schemaPath),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	testcontainers.CleanupContainer(t, ctr)

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("failed to connect to test container: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to ping test container: %v", err)
	}

	baseDSN = connStr

	return pool, baseDSN
}

func SetupHandler(t *testing.T) *core.Handler {
	t.Helper()
	pool, baseDSN := TestContainer(t)
	return core.NewWithDSN(pool, baseDSN)
}
