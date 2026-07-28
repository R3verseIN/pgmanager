package pgbouncer

import "testing"

func TestNew(t *testing.T) {
	h := New(nil, "postgres://pgmanager:pgmanager@localhost:5433/pgmanager?sslmode=disable")
	if h.pool != nil {
		t.Error("expected nil pool")
	}
	if h.baseDSN != "postgres://pgmanager:pgmanager@localhost:5433/pgmanager?sslmode=disable" {
		t.Errorf("unexpected baseDSN: %q", h.baseDSN)
	}
}
