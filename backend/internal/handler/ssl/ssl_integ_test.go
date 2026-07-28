//go:build integration

package ssl

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestSSLStatus_NoCerts(t *testing.T) {
	tmpDir := t.TempDir()
	sh := New(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ssl/status", nil)
	sh.GetStatus(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status SSLStatus
	json.Unmarshal(w.Body.Bytes(), &status)
	if status.Enabled {
		t.Error("expected enabled=false")
	}
	if status.HasCerts {
		t.Error("expected hasCerts=false")
	}
}

func TestGenerateCerts(t *testing.T) {
	tmpDir := t.TempDir()
	sh := newTestHandler(tmpDir)

	body := bytes.NewBufferString(`{"commonName":"localhost","validityDays":365}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ssl/generate", body)
	req.Header.Set("Content-Type", "application/json")
	sh.GenerateCerts(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "generated" {
		t.Errorf("expected status=generated, got %q", resp["status"])
	}
}

func TestSSLStatus_WithCerts(t *testing.T) {
	tmpDir := t.TempDir()
	sh := newTestHandler(tmpDir)

	body := bytes.NewBufferString(`{"commonName":"localhost","validityDays":365}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ssl/generate", body)
	req.Header.Set("Content-Type", "application/json")
	sh.GenerateCerts(w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/ssl/status", nil)
	sh.GetStatus(w2, req2)

	var status SSLStatus
	json.Unmarshal(w2.Body.Bytes(), &status)
	if !status.HasCerts {
		t.Error("expected hasCerts=true after generation")
	}
	if status.Expiry == "" {
		t.Error("expected expiry to be set")
	}
}

func TestDeleteCerts(t *testing.T) {
	tmpDir := t.TempDir()
	sh := newTestHandler(tmpDir)

	body := bytes.NewBufferString(`{"commonName":"localhost","validityDays":365}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ssl/generate", body)
	req.Header.Set("Content-Type", "application/json")
	sh.GenerateCerts(w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodDelete, "/api/ssl/certs", nil)
	sh.DeleteCerts(w2, req2)

	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/api/ssl/status", nil)
	sh.GetStatus(w3, req3)

	var status SSLStatus
	json.Unmarshal(w3.Body.Bytes(), &status)
	if status.HasCerts {
		t.Error("expected hasCerts=false after delete")
	}
}

func TestDownloadCA(t *testing.T) {
	tmpDir := t.TempDir()
	sh := newTestHandler(tmpDir)

	body := bytes.NewBufferString(`{"commonName":"TestCA","validityDays":365}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ssl/generate", body)
	req.Header.Set("Content-Type", "application/json")
	sh.GenerateCerts(w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/ssl/download/ca", nil)
	sh.DownloadCA(w2, req2)

	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestEnableCerts(t *testing.T) {
	tmpDir := t.TempDir()
	sh := newTestHandler(tmpDir)

	body := bytes.NewBufferString(`{"commonName":"localhost","validityDays":365}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ssl/generate", body)
	req.Header.Set("Content-Type", "application/json")
	sh.GenerateCerts(w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/ssl/enable", nil)
	sh.EnableCerts(w2, req2)

	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestDisableCerts(t *testing.T) {
	tmpDir := t.TempDir()
	sh := newTestHandler(tmpDir)

	body := bytes.NewBufferString(`{"commonName":"localhost","validityDays":365}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ssl/generate", body)
	req.Header.Set("Content-Type", "application/json")
	sh.GenerateCerts(w, req)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/ssl/disable", nil)
	sh.DisableCerts(w2, req2)

	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestGenerateCerts_DefaultValues(t *testing.T) {
	tmpDir := t.TempDir()
	sh := newTestHandler(tmpDir)

	body := bytes.NewBufferString(`{}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ssl/generate", body)
	req.Header.Set("Content-Type", "application/json")
	sh.GenerateCerts(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status SSLStatus
	req2 := httptest.NewRequest(http.MethodGet, "/api/ssl/status", nil)
	w2 := httptest.NewRecorder()
	sh.GetStatus(w2, req2)
	json.Unmarshal(w2.Body.Bytes(), &status)
	if !status.HasCerts {
		t.Error("expected certs with default CN")
	}
}

func TestUploadCerts_InvalidPEM(t *testing.T) {
	tmpDir := t.TempDir()
	sh := New(tmpDir)

	body := bytes.NewBufferString(`not a multipart form`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ssl/upload", body)
	req.Header.Set("Content-Type", "application/json")
	sh.UploadCerts(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func newTestHandler(dataDir string) *SSLHandler {
	sh := New(dataDir)
	sh.PgBouncerSSLPrefPath = filepath.Join(dataDir, "pgmanager-pgbouncer-ssl")
	sh.PgBouncerRestartPath = filepath.Join(dataDir, "pgbouncer-restart-signal")
	return sh
}
