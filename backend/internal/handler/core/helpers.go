package core

import (
	"context"
	"fmt"
	"time"

	"pgmanager/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ConnectToDatabase(ctx context.Context, baseDSN string, dbName string) (*pgxpool.Pool, func(), error) {
	dsn := baseDSN + "&dbname=" + dbName
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse dsn: %w", err)
	}
	config.MaxConns = 2
	config.MinConns = 0
	config.MaxConnLifetime = 30 * time.Second
	config.MaxConnIdleTime = 10 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	cleanup := func() { pool.Close() }
	return pool, cleanup, nil
}

func CheckDevAccess(ctx context.Context, pool *pgxpool.Pool, user *auth.SessionUser, dbName string) error {
	if user == nil {
		return fmt.Errorf("unauthorized")
	}
	if user.Role == "admin" {
		return nil
	}
	if user.Role == "viewer" {
		return nil
	}
	if user.Role == "dev" {
		var allowed bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM dev_databases WHERE auth_user_id = $1 AND database_name = $2)",
			user.ID, dbName,
		).Scan(&allowed)
		if err != nil || !allowed {
			return fmt.Errorf("access denied to database: %s", dbName)
		}
		return nil
	}
	return fmt.Errorf("forbidden")
}

func CheckWriteAccess(ctx context.Context, user *auth.SessionUser) error {
	if user == nil {
		return fmt.Errorf("unauthorized")
	}
	if user.Role == "admin" || user.Role == "dev" {
		return nil
	}
	return fmt.Errorf("forbidden")
}
