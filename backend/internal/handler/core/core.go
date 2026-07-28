package core

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	Pool             *pgxpool.Pool
	BaseDSN          string
	OnDatabaseChange func()
}

func New(pool *pgxpool.Pool) *Handler {
	return &Handler{Pool: pool}
}

func NewWithDSN(pool *pgxpool.Pool, baseDSN string) *Handler {
	return &Handler{Pool: pool, BaseDSN: baseDSN}
}

func (h *Handler) NotifyDatabaseChange() {
	if h.OnDatabaseChange != nil {
		go h.OnDatabaseChange()
	}
}
