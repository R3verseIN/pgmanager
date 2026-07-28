package ssl

import (
	"crypto/elliptic"
	"crypto/x509"
	"testing"
)

func TestNew(t *testing.T) {
	sh := New("/tmp/test")
	if sh.DataDir != "/tmp/test" {
		t.Errorf("expected DataDir /tmp/test, got %q", sh.DataDir)
	}
}

func TestCertPath(t *testing.T) {
	sh := New("/data")
	got := sh.certPath("server.crt")
	expected := "/data/server.crt"
	if got != expected {
		t.Errorf("certPath(server.crt) = %q, want %q", got, expected)
	}
}

func TestGenerateCA(t *testing.T) {
	sh := New("/tmp")
	key, tmpl, certBytes, err := sh.generateCA("Test CA", 365)
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}
	if key == nil {
		t.Fatal("expected non-nil key")
	}
	if key.Curve != elliptic.P256() {
		t.Errorf("expected P256 curve, got %v", key.Curve)
	}
	if tmpl.Subject.CommonName != "Test CA" {
		t.Errorf("expected CN 'Test CA', got %q", tmpl.Subject.CommonName)
	}
	if !tmpl.IsCA {
		t.Error("expected IsCA=true")
	}
	if tmpl.BasicConstraintsValid != true {
		t.Error("expected BasicConstraintsValid=true")
	}
	if tmpl.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("expected KeyUsageCertSign")
	}
	if len(certBytes) == 0 {
		t.Error("expected non-empty cert bytes")
	}

	parsed, err := x509.ParseCertificate(certBytes)
	if err != nil {
		t.Fatalf("failed to parse generated CA cert: %v", err)
	}
	if !parsed.IsCA {
		t.Error("parsed cert should be CA")
	}
	if parsed.Subject.CommonName != "Test CA" {
		t.Errorf("parsed cert CN = %q, want 'Test CA'", parsed.Subject.CommonName)
	}
}

func TestGenerateServerCert(t *testing.T) {
	sh := New("/tmp")

	caKey, _, caCertBytes, err := sh.generateCA("Test CA", 365)
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}

	caCert, err := x509.ParseCertificate(caCertBytes)
	if err != nil {
		t.Fatalf("failed to parse CA cert: %v", err)
	}

	key, certBytes, err := sh.generateServerCert(caCert, caKey, "localhost", 365)
	if err != nil {
		t.Fatalf("generateServerCert failed: %v", err)
	}
	if key == nil {
		t.Fatal("expected non-nil key")
	}
	if len(certBytes) == 0 {
		t.Error("expected non-empty cert bytes")
	}

	parsed, err := x509.ParseCertificate(certBytes)
	if err != nil {
		t.Fatalf("failed to parse generated server cert: %v", err)
	}
	if parsed.IsCA {
		t.Error("server cert should not be CA")
	}
	if parsed.Subject.CommonName != "localhost" {
		t.Errorf("expected CN 'localhost', got %q", parsed.Subject.CommonName)
	}
	if parsed.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("expected KeyUsageDigitalSignature")
	}

	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	_, err = parsed.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		t.Errorf("server cert failed verification against CA: %v", err)
	}
}

func TestGenerateCA_DifferentValidity(t *testing.T) {
	sh := New("/tmp")
	_, tmpl, _, err := sh.generateCA("Short CA", 30)
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}
	hours := tmpl.NotAfter.Sub(tmpl.NotBefore).Hours()
	if hours < 29*24 || hours > 31*24 {
		t.Errorf("expected ~30 day validity, got %.1f hours (%.1f days)", hours, hours/24)
	}
}

func TestGenerateCA_KeyType(t *testing.T) {
	sh := New("/tmp")
	key, _, _, err := sh.generateCA("Key Test", 365)
	if err != nil {
		t.Fatalf("generateCA failed: %v", err)
	}
	if key.Curve != elliptic.P256() {
		t.Errorf("expected P256 curve, got %v", key.Curve)
	}
}
