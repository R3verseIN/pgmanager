package core

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditEntry struct {
	Username  string      `json:"username"`
	Action    string      `json:"action"`
	Database  string      `json:"database"`
	TableName string      `json:"table_name,omitempty"`
	Detail    interface{} `json:"detail,omitempty"`
	IPAddress string      `json:"ip_address"`
}

func WriteAuditLog(pool *pgxpool.Pool, ctx context.Context, entry AuditEntry) {
	_, err := pool.Exec(ctx,
		`INSERT INTO audit_log (username, action, database, table_name, detail, ip_address)
		 VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)`,
		entry.Username, entry.Action, entry.Database,
		entry.TableName, entry.Detail, entry.IPAddress,
	)
	if err != nil {
		log.Printf("audit log write failed: %v", err)
	}
}
