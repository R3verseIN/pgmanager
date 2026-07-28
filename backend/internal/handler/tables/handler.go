package tables

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"pgmanager/internal/auth"
	"pgmanager/internal/handler/core"

	"github.com/jackc/pgx/v5/pgxpool"
)

func extractDBName(path, suffix string) string {
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

func ListTables(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	dbName := extractDBName(r.URL.Path, "/tables")
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

	rows, err := dbPool.Query(r.Context(), `
		SELECT t.tablename, COALESCE(c.reltuples, 0)::bigint AS row_estimate
		FROM pg_catalog.pg_tables t
		LEFT JOIN pg_class c ON c.relname = t.tablename AND c.relnamespace = (SELECT oid FROM pg_namespace WHERE nspname = 'public')
		WHERE t.schemaname = 'public'
		ORDER BY t.tablename
	`)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to list tables: "+err.Error())
		return
	}
	defer rows.Close()

	tables := make([]core.TableInfo, 0)
	for rows.Next() {
		var t core.TableInfo
		if err := rows.Scan(&t.Name, &t.RowCount); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to scan: "+err.Error())
			return
		}
		tables = append(tables, t)
	}

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  user.Username,
		Action:    "list_tables",
		Database:  dbName,
		Detail:    map[string]interface{}{"count": len(tables)},
		IPAddress: core.ClientIP(r),
	})

	core.WriteJSON(w, http.StatusOK, tables)
}

func GetColumns(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	dbName, table := extractGetColumns(r.URL.Path)
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

	rows, err := dbPool.Query(r.Context(), `
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
		core.WriteError(w, http.StatusInternalServerError, "failed to get columns: "+err.Error())
		return
	}
	defer rows.Close()

	columns := make([]core.ColumnInfo, 0)
	for rows.Next() {
		var c core.ColumnInfo
		var nullableStr string
		if err := rows.Scan(&c.Name, &c.Type, &nullableStr, &c.Default, &c.IsPrimaryKey); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to scan: "+err.Error())
			return
		}
		c.Nullable = nullableStr == "YES"
		columns = append(columns, c)
	}

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  user.Username,
		Action:    "view_columns",
		Database:  dbName,
		TableName: table,
		Detail:    map[string]interface{}{"column_count": len(columns)},
		IPAddress: core.ClientIP(r),
	})

	core.WriteJSON(w, http.StatusOK, columns)
}

func CreateTable(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	dbName := extractDBName(r.URL.Path, "/tables")
	user := auth.GetUserFromContext(r.Context())

	if err := core.CheckDevAccess(r.Context(), pool, user, dbName); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := core.CheckWriteAccess(r.Context(), user); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	var req core.CreateTableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		core.WriteError(w, http.StatusBadRequest, "table name is required")
		return
	}
	if !core.ValidName.MatchString(req.Name) {
		core.WriteError(w, http.StatusBadRequest, "invalid table name: must start with letter or underscore, alphanumeric only")
		return
	}
	if len(req.Columns) == 0 {
		core.WriteError(w, http.StatusBadRequest, "at least one column is required")
		return
	}

	dbPool, cleanup, err := core.ConnectToDatabase(r.Context(), baseDSN, dbName)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	var exists bool
	err = dbPool.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
		req.Name).Scan(&exists)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to check table: "+err.Error())
		return
	}
	if exists {
		core.WriteError(w, http.StatusConflict, "table already exists: "+req.Name)
		return
	}

	colDefs := make([]string, 0, len(req.Columns))
	var pkCols []string
	for _, col := range req.Columns {
		col.Name = strings.TrimSpace(col.Name)
		if col.Name == "" {
			core.WriteError(w, http.StatusBadRequest, "column name is required")
			return
		}
		if !core.ValidName.MatchString(col.Name) {
			core.WriteError(w, http.StatusBadRequest, "invalid column name: "+col.Name)
			return
		}
		def := core.QuoteIdent(col.Name) + " " + col.Type
		if !col.Nullable {
			def += " NOT NULL"
		}
		if col.Default != nil {
			def += " DEFAULT " + *col.Default
		}
		colDefs = append(colDefs, def)
		if col.IsPrimaryKey {
			pkCols = append(pkCols, core.QuoteIdent(col.Name))
		}
	}
	if len(pkCols) > 0 {
		colDefs = append(colDefs, "PRIMARY KEY ("+strings.Join(pkCols, ", ")+")")
	}

	sql := fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", core.QuoteIdent(req.Name), strings.Join(colDefs, ",\n  "))
	_, err = dbPool.Exec(r.Context(), sql)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to create table: "+err.Error())
		return
	}

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  user.Username,
		Action:    "create_table",
		Database:  dbName,
		TableName: req.Name,
		Detail:    req.Columns,
		IPAddress: core.ClientIP(r),
	})

	core.WriteJSON(w, http.StatusCreated, map[string]string{"status": "created", "table": req.Name})
}

func AddColumn(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	dbName, table := extractTableFromColumns(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := core.CheckDevAccess(r.Context(), pool, user, dbName); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := core.CheckWriteAccess(r.Context(), user); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	var req core.AddColumnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		core.WriteError(w, http.StatusBadRequest, "column name is required")
		return
	}
	if !core.ValidName.MatchString(req.Name) {
		core.WriteError(w, http.StatusBadRequest, "invalid column name")
		return
	}
	if req.Type == "" {
		core.WriteError(w, http.StatusBadRequest, "column type is required")
		return
	}

	dbPool, cleanup, err := core.ConnectToDatabase(r.Context(), baseDSN, dbName)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s",
		core.QuoteIdent(table), core.QuoteIdent(req.Name), req.Type)
	if !req.Nullable {
		sql += " NOT NULL"
	}
	if req.Default != nil {
		sql += " DEFAULT " + *req.Default
	}

	_, err = dbPool.Exec(r.Context(), sql)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to add column: "+err.Error())
		return
	}

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  user.Username,
		Action:    "add_column",
		Database:  dbName,
		TableName: table,
		Detail:    req,
		IPAddress: core.ClientIP(r),
	})

	core.WriteJSON(w, http.StatusCreated, map[string]string{"status": "column added"})
}

func DropColumn(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	dbName, table, column := extractTableFromColumnsDrop(r.URL.Path)
	user := auth.GetUserFromContext(r.Context())

	if err := core.CheckDevAccess(r.Context(), pool, user, dbName); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := core.CheckWriteAccess(r.Context(), user); err != nil {
		core.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	dbPool, cleanup, err := core.ConnectToDatabase(r.Context(), baseDSN, dbName)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to connect: "+err.Error())
		return
	}
	defer cleanup()

	var isPK bool
	err = dbPool.QueryRow(r.Context(),
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
		core.WriteError(w, http.StatusInternalServerError, "failed to check column: "+err.Error())
		return
	}
	if isPK {
		core.WriteError(w, http.StatusBadRequest, "cannot drop primary key column")
		return
	}

	sql := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", core.QuoteIdent(table), core.QuoteIdent(column))
	_, err = dbPool.Exec(r.Context(), sql)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to drop column: "+err.Error())
		return
	}

	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  user.Username,
		Action:    "drop_column",
		Database:  dbName,
		TableName: table,
		Detail:    map[string]string{"column": column},
		IPAddress: core.ClientIP(r),
	})

	core.WriteJSON(w, http.StatusOK, map[string]string{"status": "column dropped"})
}
