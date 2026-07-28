package backup

type BackupCreateRequest struct {
	Database string   `json:"database"`
	Tables   []string `json:"tables,omitempty"`
}

type BackupDatabaseEntry struct {
	Name string `json:"name"`
}

type BackupTableEntry struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

type BackupTableListResponse struct {
	Database string             `json:"database"`
	Tables   []BackupTableEntry `json:"tables"`
}

type BackupInspectResponse struct {
	Database string             `json:"database"`
	Format   string             `json:"format"`
	Tables   []BackupTableEntry `json:"tables"`
	Size     int64              `json:"size"`
}

type BackupRestoreRequest struct {
	Database  string `json:"database"`
	DropFirst bool   `json:"dropFirst"`
}
