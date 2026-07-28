package platform

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func StartAuditLogRetention(ctx context.Context, pool *pgxpool.Pool) {
	select {
	case <-time.After(1 * time.Hour):
	case <-ctx.Done():
		return
	}

	cleanupAuditLog(ctx, pool)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cleanupAuditLog(ctx, pool)
		case <-ctx.Done():
			return
		}
	}
}

func cleanupAuditLog(ctx context.Context, pool *pgxpool.Pool) {
	var val string
	err := pool.QueryRow(ctx,
		`SELECT value FROM system_config WHERE key = 'audit_log_retention_days'`).
		Scan(&val)
	if err != nil {
		return
	}

	days, err := strconv.Atoi(val)
	if err != nil || days <= 0 {
		return
	}

	tag, err := pool.Exec(ctx,
		`DELETE FROM audit_log WHERE created_at < NOW() - ($1 || ' days')::INTERVAL`, val)
	if err != nil {
		log.Printf("audit log cleanup failed: %v", err)
		return
	}
	if tag.RowsAffected() > 0 {
		log.Printf("audit log cleanup: deleted %d rows older than %d days", tag.RowsAffected(), days)
	}
}
