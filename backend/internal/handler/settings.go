package handler

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SettingsHandler struct {
	pool *pgxpool.Pool
}

func NewSettingsHandler(pool *pgxpool.Pool) *SettingsHandler {
	return &SettingsHandler{pool: pool}
}

// GET /api/settings — returns all system_config key/value pairs
func (sh *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := sh.pool.Query(r.Context(), `SELECT key, value FROM system_config`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read settings")
		return
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan row")
			return
		}
		settings[key] = value
	}

	writeJSON(w, http.StatusOK, settings)
}

// PUT /api/settings — upserts key/value pairs into system_config
func (sh *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var updates map[string]string
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	for key, value := range updates {
		_, err := sh.pool.Exec(r.Context(),
			`INSERT INTO system_config (key, value) VALUES ($1, $2)
			 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
			key, value)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save setting: "+key)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}
