package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"pgmanager/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

// --- Audit Middleware ---

type auditEntry struct {
	Username  string      `json:"username"`
	Action    string      `json:"action"`
	Database  string      `json:"database"`
	TableName string      `json:"table_name,omitempty"`
	Detail    interface{} `json:"detail,omitempty"`
	IPAddress string      `json:"ip_address"`
}

func (h *Handler) writeAuditLog(ctx context.Context, entry auditEntry) {
	_, err := h.pool.Exec(ctx,
		`INSERT INTO audit_log (username, action, database, table_name, detail, ip_address)
		 VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)`,
		entry.Username, entry.Action, entry.Database,
		entry.TableName, entry.Detail, entry.IPAddress,
	)
	if err != nil {
		log.Printf("audit log write failed: %v", err)
	}
}

// --- Dynamic Database Connection ---

func (h *Handler) connectToDatabase(ctx context.Context, dbName string) (*pgxpool.Pool, func(), error) {
	dsn := h.baseDSN + "&dbname=" + dbName
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

// --- Dev Access Check ---

func (h *Handler) checkDevAccess(ctx context.Context, user *auth.SessionUser, dbName string) error {
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
		err := h.pool.QueryRow(ctx,
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

func (h *Handler) checkWriteAccess(ctx context.Context, user *auth.SessionUser) error {
	if user == nil {
		return fmt.Errorf("unauthorized")
	}
	if user.Role == "admin" || user.Role == "dev" {
		return nil
	}
	return fmt.Errorf("forbidden")
}

func clientIP(r *http.Request) string {
	remoteIPStr, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIPStr = r.RemoteAddr
	}

	remoteIP := net.ParseIP(remoteIPStr)

	// If it's a public IP, ignore all headers to prevent spoofing
	if remoteIP == nil || (!remoteIP.IsPrivate() && !remoteIP.IsLoopback()) {
		return remoteIPStr
	}

	// It's a private/local IP (e.g. from Docker proxy/Nginx), so we trust headers
	if cfip := r.Header.Get("CF-Connecting-IP"); cfip != "" {
		return strings.TrimSpace(cfip)
	}
	if trueClient := r.Header.Get("True-Client-IP"); trueClient != "" {
		return strings.TrimSpace(trueClient)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	return remoteIPStr
}

// --- Path Extraction Helpers ---
// The dispatcher doesn't use Go 1.22 method patterns for these routes,
// so r.PathValue() returns empty. We extract manually.

func extractDBName(path, suffix string) string {
	// /api/databases/{name}/suffix → {name}
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

func extractTableFromColumns(path string) (dbName, table string) {
	// /api/databases/{name}/tables/{table}/columns → name, table
	prefix := "/api/databases/"
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}
	rest := path[len(prefix):]
	parts := strings.Split(rest, "/")
	if len(parts) >= 4 && parts[1] == "tables" && parts[3] == "columns" {
		return parts[0], parts[2]
	}
	return "", ""
}

func extractGetColumns(path string) (dbName, table string) {
	// /api/databases/{name}/columns/{table} → name, table
	prefix := "/api/databases/"
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}
	rest := path[len(prefix):]
	parts := strings.Split(rest, "/")
	if len(parts) >= 3 && parts[1] == "columns" {
		return parts[0], parts[2]
	}
	return "", ""
}

func extractTableFromColumnsDrop(path string) (dbName, table, column string) {
	// /api/databases/{name}/tables/{table}/columns/{column} → name, table, column
	prefix := "/api/databases/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", ""
	}
	rest := path[len(prefix):]
	parts := strings.Split(rest, "/")
	if len(parts) >= 5 && parts[1] == "tables" && parts[3] == "columns" {
		return parts[0], parts[2], parts[4]
	}
	return "", "", ""
}

func extractTableFromData(path string) (dbName, table string) {
	// /api/databases/{name}/data/{table} → name, table
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

func extractDBNameFromQuery(path string) string {
	// /api/databases/{name}/query → name
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

// --- Blocked SQL Patterns ---

var blockedSQLPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*DROP\s+DATABASE`),
	regexp.MustCompile(`(?i)^\s*DROP\s+OWNED\s+BY`),
	regexp.MustCompile(`(?i)^\s*ALTER\s+ROLE`),
	regexp.MustCompile(`(?i)^\s*CREATE\s+ROLE`),
	regexp.MustCompile(`(?i)^\s*DROP\s+ROLE`),
	regexp.MustCompile(`(?i)^\s*GRANT\s+`),
	regexp.MustCompile(`(?i)^\s*REVOKE\s+`),
	regexp.MustCompile(`(?i)^\s*TRUNCATE\s+`),
	regexp.MustCompile(`(?i)^\s*COMMENT\s+ON\s+DATABASE`),
}

func isBlockedSQL(sql string) bool {
	trimmed := strings.TrimSpace(sql)
	for _, pattern := range blockedSQLPatterns {
		if pattern.MatchString(trimmed) {
			return true
		}
	}
	return false
}

// --- Request Types ---

type tableInfo struct {
	Name     string `json:"name"`
	RowCount int64  `json:"rowCount"`
}

type columnInfo struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Nullable     bool    `json:"nullable"`
	Default      *string `json:"default"`
	IsPrimaryKey bool    `json:"isPrimaryKey"`
}

type dataResult struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
	Total   int64           `json:"total"`
}

type whereCondition struct {
	Column   string      `json:"column"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value,omitempty"`
}

type insertRowRequest struct {
	Values map[string]interface{} `json:"values"`
}

type updateRowRequest struct {
	Values map[string]interface{} `json:"values"`
	Where  []whereCondition      `json:"where"`
}

type deleteRowRequest struct {
	Where []whereCondition `json:"where"`
}

type createTableRequest struct {
	Name    string      `json:"name"`
	Columns []columnDef `json:"columns"`
}

type columnDef struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Nullable     bool    `json:"nullable"`
	Default      *string `json:"default"`
	IsPrimaryKey bool    `json:"isPrimaryKey"`
}

type addColumnRequest struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Nullable     bool    `json:"nullable"`
	Default      *string `json:"default"`
}

type executeQueryRequest struct {
	SQL string `json:"sql"`
}

type queryResult struct {
	Columns  []string        `json:"columns"`
	Rows     [][]interface{} `json:"rows"`
	RowCount int64           `json:"rowCount"`
	Duration int64           `json:"duration"`
	Error    string          `json:"error,omitempty"`
}

type auditLogEntry struct {
	ID        int64       `json:"id"`
	Username  string      `json:"username"`
	Action    string      `json:"action"`
	Database  string      `json:"database"`
	TableName *string     `json:"tableName,omitempty"`
	Detail    interface{} `json:"detail,omitempty"`
	IPAddress *string     `json:"ipAddress,omitempty"`
	CreatedAt string      `json:"createdAt"`
}

type auditLogResponse struct {
	Entries []auditLogEntry `json:"entries"`
	Total   int64           `json:"total"`
}

// --- Handlers ---

func (h *Handler) ListTables(w http.ResponseWriter, r *http.Request) {
	dbName := extractDBName(r.URL.Path, "/tables")
	user := auth.GetUserFromContext(r.Context())

	if err := h.checkDevAccess(r.Context(), user, dbName); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	pool, cleanup, err := h.connectToDatabase(r.Context(), dbName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	rows, err := pool.Query(r.Context(), `
		SELECT t.tablename, COALESCE(c.reltuples, 0)::bigint AS row_estimate
		FROM pg_catalog.pg_tables t
		LEFT JOIN pg_class c ON c.relname = t.tablename AND c.relnamespace = (SELECT oid FROM pg_namespace WHERE nspname = 'public')
		WHERE t.schemaname = 'public'
		ORDER BY t.tablename
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tables: "+err.Error())
		return
	}
	defer rows.Close()

	tables := make([]tableInfo, 0)
	for rows.Next() {
		var t tableInfo
		if err := rows.Scan(&t.Name, &t.RowCount); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan: "+err.Error())
			return
		}
		tables = append(tables, t)
	}

	h.writeAuditLog(r.Context(), auditEntry{
		Username: user.Username,
		Action:   "list_tables",
		Database: dbName,
		Detail:   map[string]interface{}{"count": len(tables)},
		IPAddress: clientIP(r),
	})

	writeJSON(w, http.StatusOK, tables)
}

func (h *Handler) GetColumns(w http.ResponseWriter, r *http.Request) {
	dbName, table := extractGetColumns(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := h.checkDevAccess(r.Context(), user, dbName); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	pool, cleanup, err := h.connectToDatabase(r.Context(), dbName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	rows, err := pool.Query(r.Context(), `
		SELECT
			c.column_name,
			c.data_type,
			c.is_nullable,
			c.column_default,
			CASE WHEN tc.constraint_type = 'PRIMARY KEY' THEN true ELSE false END AS is_pk
		FROM information_schema.columns c
		LEFT JOIN information_schema.constraint_column_usage ccu
			ON ccu.column_name = c.column_name AND ccu.table_name = c.table_name AND ccu.table_schema = c.table_schema
		LEFT JOIN information_schema.table_constraints tc
			ON tc.constraint_name = ccu.constraint_name AND tc.constraint_schema = ccu.constraint_schema AND tc.constraint_type = 'PRIMARY KEY'
		WHERE c.table_schema = 'public' AND c.table_name = $1
		ORDER BY c.ordinal_position
	`, table)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get columns: "+err.Error())
		return
	}
	defer rows.Close()

	columns := make([]columnInfo, 0)
	for rows.Next() {
		var c columnInfo
		var nullableStr string
		if err := rows.Scan(&c.Name, &c.Type, &nullableStr, &c.Default, &c.IsPrimaryKey); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan: "+err.Error())
			return
		}
		c.Nullable = nullableStr == "YES"
		columns = append(columns, c)
	}

	h.writeAuditLog(r.Context(), auditEntry{
		Username:  user.Username,
		Action:    "view_columns",
		Database:  dbName,
		TableName: table,
		Detail:    map[string]interface{}{"column_count": len(columns)},
		IPAddress: clientIP(r),
	})

	writeJSON(w, http.StatusOK, columns)
}

func (h *Handler) ListData(w http.ResponseWriter, r *http.Request) {
	dbName, table := extractTableFromData(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := h.checkDevAccess(r.Context(), user, dbName); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	pool, cleanup, err := h.connectToDatabase(r.Context(), dbName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
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

	// Get column names first
	colRows, err := pool.Query(r.Context(),
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = $1
		 ORDER BY ordinal_position`, table)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get columns: "+err.Error())
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
		writeError(w, http.StatusNotFound, "table not found or has no columns")
		return
	}

	// Count total
	var total int64
	countSQL := "SELECT COUNT(*) FROM " + quoteIdent(table)
	err = pool.QueryRow(r.Context(), countSQL).Scan(&total)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count rows: "+err.Error())
		return
	}

	// Build query
	querySQL := "SELECT * FROM " + quoteIdent(table)
	if sort != "" {
		// Validate sort column exists
		validSort := false
		for _, c := range columns {
			if c == sort {
				validSort = true
				break
			}
		}
		if validSort {
			querySQL += " ORDER BY " + quoteIdent(sort) + " " + order
		}
	}
	querySQL += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	dataRows, err := pool.Query(r.Context(), querySQL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query data: "+err.Error())
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
			writeError(w, http.StatusInternalServerError, "failed to scan row: "+err.Error())
			return
		}
		// Convert []byte to string for JSON serialization
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

	h.writeAuditLog(r.Context(), auditEntry{
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
		IPAddress: clientIP(r),
	})

	writeJSON(w, http.StatusOK, dataResult{
		Columns: colNames,
		Rows:    resultRows,
		Total:   total,
	})
}

func (h *Handler) InsertRow(w http.ResponseWriter, r *http.Request) {
	dbName, table := extractTableFromData(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := h.checkDevAccess(r.Context(), user, dbName); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := h.checkWriteAccess(r.Context(), user); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	var req insertRowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Values) == 0 {
		writeError(w, http.StatusBadRequest, "at least one column value is required")
		return
	}

	pool, cleanup, err := h.connectToDatabase(r.Context(), dbName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	cols := make([]string, 0, len(req.Values))
	vals := make([]interface{}, 0, len(req.Values))
	args := make([]string, 0, len(req.Values))
	argIdx := 1
	for col, val := range req.Values {
		cols = append(cols, quoteIdent(col))
		vals = append(vals, val)
		args = append(args, fmt.Sprintf("$%d", argIdx))
		argIdx++
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		quoteIdent(table),
		strings.Join(cols, ", "),
		strings.Join(args, ", "),
	)

	_, err = pool.Exec(r.Context(), sql, vals...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to insert row: "+err.Error())
		return
	}

	h.writeAuditLog(r.Context(), auditEntry{
		Username:  user.Username,
		Action:    "insert_row",
		Database:  dbName,
		TableName: table,
		Detail:    req.Values,
		IPAddress: clientIP(r),
	})

	writeJSON(w, http.StatusCreated, map[string]string{"status": "inserted"})
}

func (h *Handler) UpdateRow(w http.ResponseWriter, r *http.Request) {
	dbName, table := extractTableFromData(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := h.checkDevAccess(r.Context(), user, dbName); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := h.checkWriteAccess(r.Context(), user); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	var req updateRowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Values) == 0 {
		writeError(w, http.StatusBadRequest, "at least one column value to update is required")
		return
	}
	if len(req.Where) == 0 {
		writeError(w, http.StatusBadRequest, "WHERE conditions are required for safety")
		return
	}

	pool, cleanup, err := h.connectToDatabase(r.Context(), dbName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	setClauses := make([]string, 0, len(req.Values))
	args := make([]interface{}, 0)
	argIdx := 1
	for col, val := range req.Values {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", quoteIdent(col), argIdx))
		args = append(args, val)
		argIdx++
	}

	whereClauses, newArgs, err := buildWhereClauses(req.Where, argIdx)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid WHERE condition: "+err.Error())
		return
	}
	args = append(args, newArgs...)

	sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		quoteIdent(table),
		strings.Join(setClauses, ", "),
		strings.Join(whereClauses, " AND "),
	)

	result, err := pool.Exec(r.Context(), sql, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update row: "+err.Error())
		return
	}

	rowsAffected := result.RowsAffected()

	h.writeAuditLog(r.Context(), auditEntry{
		Username:  user.Username,
		Action:    "update_row",
		Database:  dbName,
		TableName: table,
		Detail: map[string]interface{}{
			"values":        req.Values,
			"where":         req.Where,
			"rows_affected": rowsAffected,
		},
		IPAddress: clientIP(r),
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "updated", "rowsAffected": rowsAffected})
}

func (h *Handler) DeleteRow(w http.ResponseWriter, r *http.Request) {
	dbName, table := extractTableFromData(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := h.checkDevAccess(r.Context(), user, dbName); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := h.checkWriteAccess(r.Context(), user); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	var req deleteRowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Where) == 0 {
		writeError(w, http.StatusBadRequest, "WHERE conditions are required for safety")
		return
	}

	pool, cleanup, err := h.connectToDatabase(r.Context(), dbName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	whereClauses, args, err := buildWhereClauses(req.Where, 1)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid WHERE condition: "+err.Error())
		return
	}

	sql := fmt.Sprintf("DELETE FROM %s WHERE %s",
		quoteIdent(table),
		strings.Join(whereClauses, " AND "),
	)

	result, err := pool.Exec(r.Context(), sql, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete row: "+err.Error())
		return
	}

	rowsAffected := result.RowsAffected()

	h.writeAuditLog(r.Context(), auditEntry{
		Username:  user.Username,
		Action:    "delete_row",
		Database:  dbName,
		TableName: table,
		Detail: map[string]interface{}{
			"where":         req.Where,
			"rows_affected": rowsAffected,
		},
		IPAddress: clientIP(r),
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "deleted", "rowsAffected": rowsAffected})
}

func (h *Handler) CreateTable(w http.ResponseWriter, r *http.Request) {
	dbName := extractDBName(r.URL.Path, "/tables")
	user := auth.GetUserFromContext(r.Context())

	if err := h.checkDevAccess(r.Context(), user, dbName); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := h.checkWriteAccess(r.Context(), user); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	var req createTableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "table name is required")
		return
	}
	if !validName.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "invalid table name: must start with letter or underscore, alphanumeric only")
		return
	}
	if len(req.Columns) == 0 {
		writeError(w, http.StatusBadRequest, "at least one column is required")
		return
	}

	pool, cleanup, err := h.connectToDatabase(r.Context(), dbName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	// Check table doesn't already exist
	var exists bool
	err = pool.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
		req.Name).Scan(&exists)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check table: "+err.Error())
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "table already exists: "+req.Name)
		return
	}

	colDefs := make([]string, 0, len(req.Columns))
	var pkCols []string
	for _, col := range req.Columns {
		col.Name = strings.TrimSpace(col.Name)
		if col.Name == "" {
			writeError(w, http.StatusBadRequest, "column name is required")
			return
		}
		if !validName.MatchString(col.Name) {
			writeError(w, http.StatusBadRequest, "invalid column name: "+col.Name)
			return
		}
		def := quoteIdent(col.Name) + " " + col.Type
		if !col.Nullable {
			def += " NOT NULL"
		}
		if col.Default != nil {
			def += " DEFAULT " + *col.Default
		}
		colDefs = append(colDefs, def)
		if col.IsPrimaryKey {
			pkCols = append(pkCols, quoteIdent(col.Name))
		}
	}
	if len(pkCols) > 0 {
		colDefs = append(colDefs, "PRIMARY KEY ("+strings.Join(pkCols, ", ")+")")
	}

	sql := fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", quoteIdent(req.Name), strings.Join(colDefs, ",\n  "))
	_, err = pool.Exec(r.Context(), sql)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create table: "+err.Error())
		return
	}

	h.writeAuditLog(r.Context(), auditEntry{
		Username:  user.Username,
		Action:    "create_table",
		Database:  dbName,
		TableName: req.Name,
		Detail:    req.Columns,
		IPAddress: clientIP(r),
	})

	writeJSON(w, http.StatusCreated, map[string]string{"status": "created", "table": req.Name})
}

func (h *Handler) AddColumn(w http.ResponseWriter, r *http.Request) {
	dbName, table := extractTableFromColumns(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := h.checkDevAccess(r.Context(), user, dbName); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := h.checkWriteAccess(r.Context(), user); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	var req addColumnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "column name is required")
		return
	}
	if !validName.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "invalid column name")
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "column type is required")
		return
	}

	pool, cleanup, err := h.connectToDatabase(r.Context(), dbName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s",
		quoteIdent(table), quoteIdent(req.Name), req.Type)
	if !req.Nullable {
		sql += " NOT NULL"
	}
	if req.Default != nil {
		sql += " DEFAULT " + *req.Default
	}

	_, err = pool.Exec(r.Context(), sql)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add column: "+err.Error())
		return
	}

	h.writeAuditLog(r.Context(), auditEntry{
		Username:  user.Username,
		Action:    "add_column",
		Database:  dbName,
		TableName: table,
		Detail:    req,
		IPAddress: clientIP(r),
	})

	writeJSON(w, http.StatusCreated, map[string]string{"status": "column added"})
}

func (h *Handler) DropColumn(w http.ResponseWriter, r *http.Request) {
	dbName, table, column := extractTableFromColumnsDrop(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := h.checkDevAccess(r.Context(), user, dbName); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := h.checkWriteAccess(r.Context(), user); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	pool, cleanup, err := h.connectToDatabase(r.Context(), dbName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	// Check if column is a primary key
	var isPK bool
	err = pool.QueryRow(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
				ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
			WHERE tc.constraint_type = 'PRIMARY KEY'
				AND tc.table_schema = 'public'
				AND tc.table_name = $1
				AND kcu.column_name = $2
		)`, table, column).Scan(&isPK)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check column: "+err.Error())
		return
	}
	if isPK {
		writeError(w, http.StatusBadRequest, "cannot drop primary key column")
		return
	}

	sql := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", quoteIdent(table), quoteIdent(column))
	_, err = pool.Exec(r.Context(), sql)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to drop column: "+err.Error())
		return
	}

	h.writeAuditLog(r.Context(), auditEntry{
		Username:  user.Username,
		Action:    "drop_column",
		Database:  dbName,
		TableName: table,
		Detail:    map[string]string{"column": column},
		IPAddress: clientIP(r),
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "column dropped"})
}

func (h *Handler) ExecuteQuery(w http.ResponseWriter, r *http.Request) {
	dbName := extractDBNameFromQuery(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := h.checkDevAccess(r.Context(), user, dbName); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := h.checkWriteAccess(r.Context(), user); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	var req executeQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.SQL = strings.TrimSpace(req.SQL)
	if req.SQL == "" {
		writeError(w, http.StatusBadRequest, "SQL query is required")
		return
	}

	if isBlockedSQL(req.SQL) {
		writeError(w, http.StatusForbidden, "this SQL statement is not allowed")
		return
	}

	pool, cleanup, err := h.connectToDatabase(r.Context(), dbName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	// Set statement timeout
	_, _ = pool.Exec(r.Context(), "SET statement_timeout = '10000'")

	start := time.Now()
	var result queryResult

	// Detect if it's a SELECT-like query (returns rows)
	upperSQL := strings.ToUpper(req.SQL)
	isQuery := strings.HasPrefix(upperSQL, "SELECT") ||
		strings.HasPrefix(upperSQL, "WITH") ||
		strings.HasPrefix(upperSQL, "EXPLAIN") ||
		strings.HasPrefix(upperSQL, "SHOW")

	if isQuery {
		rows, err := pool.Query(r.Context(), req.SQL)
		if err != nil {
			result.Error = err.Error()
			result.Duration = time.Since(start).Milliseconds()
			h.writeAuditLog(r.Context(), auditEntry{
				Username:  user.Username,
				Action:    "raw_query",
				Database:  dbName,
				Detail:    map[string]interface{}{"query": req.SQL, "duration_ms": result.Duration, "error": result.Error},
				IPAddress: clientIP(r),
			})
			writeJSON(w, http.StatusOK, result)
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
		tag, err := pool.Exec(r.Context(), req.SQL)
		if err != nil {
			result.Error = err.Error()
			result.Duration = time.Since(start).Milliseconds()
			h.writeAuditLog(r.Context(), auditEntry{
				Username:  user.Username,
				Action:    "raw_query",
				Database:  dbName,
				Detail:    map[string]interface{}{"query": req.SQL, "duration_ms": result.Duration, "error": result.Error},
				IPAddress: clientIP(r),
			})
			writeJSON(w, http.StatusOK, result)
			return
		}
		result.RowCount = tag.RowsAffected()
		result.Columns = []string{"rows_affected"}
		result.Rows = [][]interface{}{{result.RowCount}}
	}

	result.Duration = time.Since(start).Milliseconds()

	h.writeAuditLog(r.Context(), auditEntry{
		Username:  user.Username,
		Action:    "raw_query",
		Database:  dbName,
		Detail:    map[string]interface{}{"query": req.SQL, "duration_ms": result.Duration, "rows": result.RowCount, "error": result.Error},
		IPAddress: clientIP(r),
	})

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) ListLogs(w http.ResponseWriter, r *http.Request) {
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
	err := h.pool.QueryRow(r.Context(), countSQL, args...).Scan(&total)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count logs: "+err.Error())
		return
	}

	querySQL := fmt.Sprintf(
		"SELECT id, username, action, database, NULLIF(table_name, ''), detail, NULLIF(ip_address, ''), created_at FROM audit_log%s ORDER BY created_at DESC LIMIT %d OFFSET %d",
		whereClause, limit, offset,
	)
	rows, err := h.pool.Query(r.Context(), querySQL, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list logs: "+err.Error())
		return
	}
	defer rows.Close()

	entries := make([]auditLogEntry, 0)
	for rows.Next() {
		var e auditLogEntry
		var detailJSON []byte
		var createdAt time.Time
		err := rows.Scan(&e.ID, &e.Username, &e.Action, &e.Database, &e.TableName, &detailJSON, &e.IPAddress, &createdAt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan log: "+err.Error())
			return
		}
		e.CreatedAt = createdAt.Format(time.RFC3339)
		if len(detailJSON) > 0 {
			_ = json.Unmarshal(detailJSON, &e.Detail)
		}
		entries = append(entries, e)
	}

	writeJSON(w, http.StatusOK, auditLogResponse{
		Entries: entries,
		Total:   total,
	})
}

// --- WHERE clause builder ---

func buildWhereClauses(conditions []whereCondition, startIdx int) ([]string, []interface{}, error) {
	clauses := make([]string, 0, len(conditions))
	args := make([]interface{}, 0)
	argIdx := startIdx

	for _, c := range conditions {
		if c.Column == "" {
			return nil, nil, fmt.Errorf("column name is required in WHERE condition")
		}
		if !validName.MatchString(c.Column) {
			return nil, nil, fmt.Errorf("invalid column name in WHERE: %s", c.Column)
		}

		switch strings.ToUpper(c.Operator) {
		case "=", "!=", ">", "<", ">=", "<=", "LIKE":
			clauses = append(clauses, fmt.Sprintf("%s %s $%d", quoteIdent(c.Column), c.Operator, argIdx))
			args = append(args, c.Value)
			argIdx++
		case "IS NULL":
			clauses = append(clauses, fmt.Sprintf("%s IS NULL", quoteIdent(c.Column)))
		case "IS NOT NULL":
			clauses = append(clauses, fmt.Sprintf("%s IS NOT NULL", quoteIdent(c.Column)))
		default:
			return nil, nil, fmt.Errorf("unsupported operator: %s", c.Operator)
		}
	}

	if len(clauses) == 0 {
		return nil, nil, fmt.Errorf("at least one WHERE condition is required")
	}

	return clauses, args, nil
}
