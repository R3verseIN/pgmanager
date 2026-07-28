package settings

import (
	"encoding/json"
	"net/http"

	"pgmanager/internal/handler/core"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `SELECT key, value FROM system_config`)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to read settings")
		return
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to scan row")
			return
		}
		settings[key] = value
	}

	core.WriteJSON(w, http.StatusOK, settings)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var updates map[string]string
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	for key, value := range updates {
		_, err := h.pool.Exec(r.Context(),
			`INSERT INTO system_config (key, value) VALUES ($1, $2)
			 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
			key, value)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to save setting: "+key)
			return
		}
	}

	core.WriteJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}
