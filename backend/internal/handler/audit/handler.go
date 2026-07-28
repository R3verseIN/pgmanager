package audit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"pgmanager/internal/handler/core"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ListLogs(pool *pgxpool.Pool, w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	action := r.URL.Query().Get("action")
	database := r.URL.Query().Get("database")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	limit := 100
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	where := []string{}
	args := []interface{}{}
	argIdx := 1

	if username != "" {
		where = append(where, fmt.Sprintf("username = $%d", argIdx))
		args = append(args, username)
		argIdx++
	}
	if action != "" {
		where = append(where, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, action)
		argIdx++
	}
	if database != "" {
		where = append(where, fmt.Sprintf("database = $%d", argIdx))
		args = append(args, database)
		argIdx++
	}
	if from != "" {
		where = append(where, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, from)
		argIdx++
	}
	if to != "" {
		where = append(where, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, to)
		argIdx++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM audit_log" + whereClause
	err := pool.QueryRow(r.Context(), countSQL, args...).Scan(&total)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to count logs: "+err.Error())
		return
	}

	querySQL := fmt.Sprintf(
		"SELECT id, username, action, database, NULLIF(table_name, ''), detail, NULLIF(ip_address, ''), created_at FROM audit_log%s ORDER BY created_at DESC LIMIT %d OFFSET %d",
		whereClause, limit, offset,
	)
	rows, err := pool.Query(r.Context(), querySQL, args...)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to list logs: "+err.Error())
		return
	}
	defer rows.Close()

	entries := make([]core.AuditLogEntry, 0)
	for rows.Next() {
		var e core.AuditLogEntry
		var detailJSON []byte
		var createdAt time.Time
		err := rows.Scan(&e.ID, &e.Username, &e.Action, &e.Database, &e.TableName, &detailJSON, &e.IPAddress, &createdAt)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to scan log: "+err.Error())
			return
		}
		e.CreatedAt = createdAt.Format(time.RFC3339)
		if len(detailJSON) > 0 {
			_ = json.Unmarshal(detailJSON, &e.Detail)
		}
		entries = append(entries, e)
	}

	core.WriteJSON(w, http.StatusOK, core.AuditLogResponse{
		Entries: entries,
		Total:   total,
	})
}
