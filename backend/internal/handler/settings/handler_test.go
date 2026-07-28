package settings

import "testing"

func TestNew(t *testing.T) {
	h := New(nil)
	if h.pool != nil {
		t.Error("expected nil pool")
	}
}
