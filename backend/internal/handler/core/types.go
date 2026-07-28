package core

type TableInfo struct {
	Name     string `json:"name"`
	RowCount int64  `json:"rowCount"`
}

type ColumnInfo struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Nullable     bool    `json:"nullable"`
	Default      *string `json:"default"`
	IsPrimaryKey bool    `json:"isPrimaryKey"`
}

type DataResult struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
	Total   int64           `json:"total"`
}

type InsertRowRequest struct {
	Values map[string]interface{} `json:"values"`
}

type UpdateRowRequest struct {
	Values map[string]interface{} `json:"values"`
	Where  []WhereCondition      `json:"where"`
}

type DeleteRowRequest struct {
	Where []WhereCondition `json:"where"`
}

type CreateTableRequest struct {
	Name    string      `json:"name"`
	Columns []ColumnDef `json:"columns"`
}

type ColumnDef struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Nullable     bool    `json:"nullable"`
	Default      *string `json:"default"`
	IsPrimaryKey bool    `json:"isPrimaryKey"`
}

type AddColumnRequest struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Nullable     bool    `json:"nullable"`
	Default      *string `json:"default"`
}

type ExecuteQueryRequest struct {
	SQL string `json:"sql"`
}

type QueryResult struct {
	Columns  []string        `json:"columns"`
	Rows     [][]interface{} `json:"rows"`
	RowCount int64           `json:"rowCount"`
	Duration int64           `json:"duration"`
	Error    string          `json:"error,omitempty"`
}

type AuditLogEntry struct {
	ID        int64       `json:"id"`
	Username  string      `json:"username"`
	Action    string      `json:"action"`
	Database  string      `json:"database"`
	TableName *string     `json:"tableName,omitempty"`
	Detail    interface{} `json:"detail,omitempty"`
	IPAddress *string     `json:"ipAddress,omitempty"`
	CreatedAt string      `json:"createdAt"`
}

type AuditLogResponse struct {
	Entries []AuditLogEntry `json:"entries"`
	Total   int64           `json:"total"`
}
