package sql

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"pgmanager/internal/auth"
	"pgmanager/internal/handler/core"

	"github.com/jackc/pgx/v5/pgxpool"
)

type executeQueryRequest struct {
	SQL string `json:"sql"`
}

func extractDBNameFromQuery(path string) string {
	prefix := "/api/databases/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	idx := strings.Index(rest, "/")
	if idx < 0 {
		return ""
	}
	return rest[:idx]
}

func ExecuteQuery(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	dbName := extractDBNameFromQuery(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := core.CheckDevAccess(r.Context(), pool, user, dbName); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := core.CheckWriteAccess(r.Context(), user); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	var req executeQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.SQL = strings.TrimSpace(req.SQL)
	if req.SQL == "" {
		core.WriteError(w, http.StatusBadRequest, "SQL query is required")
		return
	}

	if core.IsBlockedSQL(req.SQL) {
		core.WriteError(w, http.StatusForbidden, "this SQL statement is not allowed")
		return
	}

	dbPool, cleanup, err := core.ConnectToDatabase(r.Context(), baseDSN, dbName)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	_, _ = dbPool.Exec(r.Context(), "SET statement_timeout = '10000'")

	start := time.Now()
	var result core.QueryResult

	upperSQL := strings.ToUpper(req.SQL)
	isQuery := strings.HasPrefix(upperSQL, "SELECT") ||
		strings.HasPrefix(upperSQL, "WITH") ||
		strings.HasPrefix(upperSQL, "EXPLAIN") ||
		strings.HasPrefix(upperSQL, "SHOW")

	if isQuery {
		rows, err := dbPool.Query(r.Context(), req.SQL)
		if err != nil {
			result.Error = err.Error()
			result.Duration = time.Since(start).Milliseconds()
			core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
				Username:  user.Username,
				Action:    "raw_query",
				Database:  dbName,
				Detail:    map[string]interface{}{"query": req.SQL, "duration_ms": result.Duration, "error": result.Error},
				IPAddress: core.ClientIP(r),
			})
			core.WriteJSON(w, http.StatusOK, result)
			return
		}
		defer rows.Close()

		fieldDescriptions := rows.FieldDescriptions()
		result.Columns = make([]string, len(fieldDescriptions))
		for i, fd := range fieldDescriptions {
			result.Columns[i] = string(fd.Name)
		}

		count := 0
		for rows.Next() && count < 10000 {
			values, err := rows.Values()
			if err != nil {
				result.Error = err.Error()
				break
			}
			for i, v := range values {
				switch val := v.(type) {
				case []byte:
					values[i] = string(val)
				case time.Time:
					values[i] = val.Format(time.RFC3339)
				}
			}
			result.Rows = append(result.Rows, values)
			count++
		}
		result.RowCount = int64(count)
	} else {
		tag, err := dbPool.Exec(r.Context(), req.SQL)
		if err != nil {
			result.Error = err.Error()
			result.Duration = time.Since(start).Milliseconds()
			core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
				Username:  user.Username,
				Action:    "raw_query",
				Database:  dbName,
				Detail:    map[string]interface{}{"query": req.SQL, "duration_ms": result.Duration, "error": result.Error},
				IPAddress: core.ClientIP(r),
			})
			core.WriteJSON(w, http.StatusOK, result)
			return
		}
		result.RowCount = tag.RowsAffected()
		result.Columns = []string{"rows_affected"}
		result.Rows = [][]interface{}{{result.RowCount}}
	}

	result.Duration = time.Since(start).Milliseconds()

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  user.Username,
		Action:    "raw_query",
		Database:  dbName,
		Detail:    map[string]interface{}{"query": req.SQL, "duration_ms": result.Duration, "rows": result.RowCount, "error": result.Error},
		IPAddress: core.ClientIP(r),
	})

	core.WriteJSON(w, http.StatusOK, result)
}
