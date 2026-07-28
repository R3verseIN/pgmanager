package data

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"pgmanager/internal/auth"
	"pgmanager/internal/handler/core"

	"github.com/jackc/pgx/v5/pgxpool"
)

func extractTableFromData(path string) (dbName, table string) {
	prefix := "/api/databases/"
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}
	rest := path[len(prefix):]
	parts := strings.Split(rest, "/")
	if len(parts) >= 3 && parts[1] == "data" {
		return parts[0], parts[2]
	}
	return "", ""
}

func ListData(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	dbName, table := extractTableFromData(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := core.CheckDevAccess(r.Context(), pool, user, dbName); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	dbPool, cleanup, err := core.ConnectToDatabase(r.Context(), baseDSN, dbName)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	limit := 100
	offset := 0
	sort := r.URL.Query().Get("sort")
	order := strings.ToUpper(r.URL.Query().Get("order"))
	if order != "DESC" {
		order = "ASC"
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}
	if limit < 1 || limit > 10000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	colRows, err := dbPool.Query(r.Context(),
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = $1
		 ORDER BY ordinal_position`, table)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to get columns: "+err.Error())
		return
	}
	defer colRows.Close()

	var columns []string
	for colRows.Next() {
		var col string
		if err := colRows.Scan(&col); err == nil {
			columns = append(columns, col)
		}
	}

	if len(columns) == 0 {
		core.WriteError(w, http.StatusNotFound, "table not found or has no columns")
		return
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM " + core.QuoteIdent(table)
	err = dbPool.QueryRow(r.Context(), countSQL).Scan(&total)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to count rows: "+err.Error())
		return
	}

	querySQL := "SELECT * FROM " + core.QuoteIdent(table)
	if sort != "" {
		validSort := false
		for _, c := range columns {
			if c == sort {
				validSort = true
				break
			}
		}
		if validSort {
			querySQL += " ORDER BY " + core.QuoteIdent(sort) + " " + order
		}
	}
	querySQL += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	dataRows, err := dbPool.Query(r.Context(), querySQL)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to query data: "+err.Error())
		return
	}
	defer dataRows.Close()

	fieldDescriptions := dataRows.FieldDescriptions()
	colNames := make([]string, len(fieldDescriptions))
	for i, fd := range fieldDescriptions {
		colNames[i] = string(fd.Name)
	}

	var resultRows [][]interface{}
	for dataRows.Next() {
		values, err := dataRows.Values()
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to scan row: "+err.Error())
			return
		}
		for i, v := range values {
			switch val := v.(type) {
			case []byte:
				values[i] = string(val)
			case time.Time:
				values[i] = val.Format(time.RFC3339)
			}
		}
		resultRows = append(resultRows, values)
	}

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  user.Username,
		Action:    "view_data",
		Database:  dbName,
		TableName: table,
		Detail: map[string]interface{}{
			"limit":  limit,
			"offset": offset,
			"sort":   sort,
			"order":  order,
			"rows":   len(resultRows),
			"total":  total,
		},
		IPAddress: core.ClientIP(r),
	})

	core.WriteJSON(w, http.StatusOK, core.DataResult{
		Columns: colNames,
		Rows:    resultRows,
		Total:   total,
	})
}

func InsertRow(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	dbName, table := extractTableFromData(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := core.CheckDevAccess(r.Context(), pool, user, dbName); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := core.CheckWriteAccess(r.Context(), user); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	var req core.InsertRowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Values) == 0 {
		core.WriteError(w, http.StatusBadRequest, "at least one column value is required")
		return
	}

	dbPool, cleanup, err := core.ConnectToDatabase(r.Context(), baseDSN, dbName)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	cols := make([]string, 0, len(req.Values))
	vals := make([]interface{}, 0, len(req.Values))
	args := make([]string, 0, len(req.Values))
	argIdx := 1
	for col, val := range req.Values {
		cols = append(cols, core.QuoteIdent(col))
		vals = append(vals, val)
		args = append(args, fmt.Sprintf("$%d", argIdx))
		argIdx++
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		core.QuoteIdent(table),
		strings.Join(cols, ", "),
		strings.Join(args, ", "),
	)

	_, err = dbPool.Exec(r.Context(), sql, vals...)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to insert row: "+err.Error())
		return
	}

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  user.Username,
		Action:    "insert_row",
		Database:  dbName,
		TableName: table,
		Detail:    req.Values,
		IPAddress: core.ClientIP(r),
	})

	core.WriteJSON(w, http.StatusCreated, map[string]string{"status": "inserted"})
}

func UpdateRow(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	dbName, table := extractTableFromData(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := core.CheckDevAccess(r.Context(), pool, user, dbName); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := core.CheckWriteAccess(r.Context(), user); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	var req core.UpdateRowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Values) == 0 {
		core.WriteError(w, http.StatusBadRequest, "at least one column value to update is required")
		return
	}
	if len(req.Where) == 0 {
		core.WriteError(w, http.StatusBadRequest, "WHERE conditions are required for safety")
		return
	}

	dbPool, cleanup, err := core.ConnectToDatabase(r.Context(), baseDSN, dbName)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	setClauses := make([]string, 0, len(req.Values))
	args := make([]interface{}, 0)
	argIdx := 1
	for col, val := range req.Values {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", core.QuoteIdent(col), argIdx))
		args = append(args, val)
		argIdx++
	}

	whereClauses, newArgs, err := core.BuildWhereClauses(req.Where, argIdx)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid WHERE condition: "+err.Error())
		return
	}
	args = append(args, newArgs...)

	sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		core.QuoteIdent(table),
		strings.Join(setClauses, ", "),
		strings.Join(whereClauses, " AND "),
	)

	result, err := dbPool.Exec(r.Context(), sql, args...)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to update row: "+err.Error())
		return
	}

	rowsAffected := result.RowsAffected()

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  user.Username,
		Action:    "update_row",
		Database:  dbName,
		TableName: table,
		Detail: map[string]interface{}{
			"values":        req.Values,
			"where":         req.Where,
			"rows_affected": rowsAffected,
		},
		IPAddress: core.ClientIP(r),
	})

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{"status": "updated", "rowsAffected": rowsAffected})
}

func DeleteRow(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	dbName, table := extractTableFromData(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := core.CheckDevAccess(r.Context(), pool, user, dbName); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := core.CheckWriteAccess(r.Context(), user); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	var req core.DeleteRowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Where) == 0 {
		core.WriteError(w, http.StatusBadRequest, "WHERE conditions are required for safety")
		return
	}

	dbPool, cleanup, err := core.ConnectToDatabase(r.Context(), baseDSN, dbName)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	whereClauses, args, err := core.BuildWhereClauses(req.Where, 1)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid WHERE condition: "+err.Error())
		return
	}

	sql := fmt.Sprintf("DELETE FROM %s WHERE %s",
		core.QuoteIdent(table),
		strings.Join(whereClauses, " AND "),
	)

	result, err := dbPool.Exec(r.Context(), sql, args...)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to delete row: "+err.Error())
		return
	}

	rowsAffected := result.RowsAffected()

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  user.Username,
		Action:    "delete_row",
		Database:  dbName,
		TableName: table,
		Detail: map[string]interface{}{
			"where":         req.Where,
			"rows_affected": rowsAffected,
		},
		IPAddress: core.ClientIP(r),
	})

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{"status": "deleted", "rowsAffected": rowsAffected})
}
