package main

import (
	"strings"
	"testing"
)

func TestGenerateEnvVarsMinimal(t *testing.T) {
	cfg := &Config{
		Provider:       "AWS S3",
		Bucket:         "my-bucket",
		AccessKeyID:    "AKIAIOSFODNN7EXAMPLE",
		SecretKey:      "wJalrXUtnFEMI/K7MDENG",
		Endpoint:       "",
		Region:         "us-east-1",
		ForcePathStyle: false,
	}

	lines := generateEnvVars(cfg)

	// Should have exactly 6 lines: type, bucket, access key, secret key, region, uri style
	if len(lines) != 6 {
		t.Errorf("expected 6 env vars for minimal AWS config, got %d: %v", len(lines), lines)
	}

	assertEnvVar(t, lines, "PGBACKREST_REPO1_TYPE", "s3")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_BUCKET", "my-bucket")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_KEY", "AKIAIOSFODNN7EXAMPLE")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_KEY_SECRET", "wJalrXUtnFEMI/K7MDENG")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_REGION", "us-east-1")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_URI_STYLE", "host")
}

func TestGenerateEnvVarsWithPathStyle(t *testing.T) {
	cfg := &Config{
		Provider:       "MinIO",
		Bucket:         "minio-bucket",
		AccessKeyID:    "minioadmin",
		SecretKey:      "minioadmin",
		Endpoint:       "http://minio:9000",
		Region:         "us-east-1",
		ForcePathStyle: true,
	}

	lines := generateEnvVars(cfg)

	// Should have 7 lines: type, bucket, access key, secret key, endpoint, region, uri style
	if len(lines) != 7 {
		t.Errorf("expected 7 env vars for MinIO config, got %d: %v", len(lines), lines)
	}

	assertEnvVar(t, lines, "PGBACKREST_REPO1_TYPE", "s3")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_BUCKET", "minio-bucket")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_KEY", "minioadmin")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_KEY_SECRET", "minioadmin")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_ENDPOINT", "http://minio:9000")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_REGION", "us-east-1")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_URI_STYLE", "path")
}

func TestGenerateEnvVarsWithEndpoint(t *testing.T) {
	cfg := &Config{
		Provider:       "Cloudflare R2",
		Bucket:         "r2-bucket",
		AccessKeyID:    "r2key123",
		SecretKey:      "r2secret456",
		Endpoint:       "https://abc123.r2.cloudflarestorage.com",
		Region:         "auto",
		ForcePathStyle: true,
	}

	lines := generateEnvVars(cfg)

	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_ENDPOINT", "https://abc123.r2.cloudflarestorage.com")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_REGION", "auto")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_URI_STYLE", "path")
}

func TestGenerateEnvVarsNoEndpointOmitted(t *testing.T) {
	cfg := &Config{
		Provider:       "AWS S3",
		Bucket:         "my-bucket",
		AccessKeyID:    "key",
		SecretKey:      "secret",
		Endpoint:       "",
		Region:         "us-east-1",
		ForcePathStyle: false,
	}

	lines := generateEnvVars(cfg)

	for _, line := range lines {
		if strings.HasPrefix(line, "PGBACKREST_REPO1_S3_ENDPOINT") {
			t.Errorf("empty endpoint should be omitted, but found: %s", line)
		}
	}
}

func TestGenerateEnvVarsSpecialCharacters(t *testing.T) {
	cfg := &Config{
		Provider:       "AWS S3",
		Bucket:         "my-bucket-with-dashes",
		AccessKeyID:    "AKIAIOSFODNN7EXAMPLE",
		SecretKey:      "wJalrXUtnFEMI/K7MDENG/BpxRsiCyF8TH+nVw=",
		Region:         "us-west-2",
	}

	lines := generateEnvVars(cfg)

	// Secret key with special characters should be preserved exactly
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_KEY_SECRET", "wJalrXUtnFEMI/K7MDENG/BpxRsiCyF8TH+nVw=")
}

func TestGenerateEnvVarsFormat(t *testing.T) {
	cfg := &Config{
		Provider:       "AWS S3",
		Bucket:         "test",
		AccessKeyID:    "key",
		SecretKey:      "secret",
		Region:         "us-east-1",
	}

	lines := generateEnvVars(cfg)

	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			t.Errorf("env var line should have format KEY=VALUE, got: %s", line)
			continue
		}
		if parts[0] == "" {
			t.Errorf("env var key should not be empty: %s", line)
		}
	}
}

func TestGenerateEnvVarsAllFields(t *testing.T) {
	cfg := &Config{
		Provider:       "Custom S3-Compatible",
		Bucket:         "custom-bucket",
		AccessKeyID:    "custom-key",
		SecretKey:      "custom-secret",
		Endpoint:       "http://custom:9000",
		Region:         "custom-region",
		ForcePathStyle: true,
	}

	lines := generateEnvVars(cfg)

	// Should have 7 env vars
	if len(lines) != 7 {
		t.Errorf("expected 7 env vars for full custom config, got %d: %v", len(lines), lines)
	}

	assertEnvVar(t, lines, "PGBACKREST_REPO1_TYPE", "s3")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_BUCKET", "custom-bucket")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_KEY", "custom-key")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_KEY_SECRET", "custom-secret")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_ENDPOINT", "http://custom:9000")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_REGION", "custom-region")
	assertEnvVar(t, lines, "PGBACKREST_REPO1_S3_URI_STYLE", "path")
}

func TestGenerateEnvVarsPreservesOrder(t *testing.T) {
	cfg := &Config{
		Provider:       "MinIO",
		Bucket:         "test",
		AccessKeyID:    "key",
		SecretKey:      "secret",
		Endpoint:       "http://minio:9000",
		Region:         "us-east-1",
		ForcePathStyle: true,
	}

	lines := generateEnvVars(cfg)

	// Verify order
	expectedOrder := []string{
		"PGBACKREST_REPO1_TYPE",
		"PGBACKREST_REPO1_S3_BUCKET",
		"PGBACKREST_REPO1_S3_KEY",
		"PGBACKREST_REPO1_S3_KEY_SECRET",
		"PGBACKREST_REPO1_S3_ENDPOINT",
		"PGBACKREST_REPO1_S3_REGION",
		"PGBACKREST_REPO1_S3_URI_STYLE",
	}

	if len(lines) != len(expectedOrder) {
		t.Fatalf("expected %d lines, got %d", len(expectedOrder), len(lines))
	}

	for i, line := range lines {
		key := strings.SplitN(line, "=", 2)[0]
		if key != expectedOrder[i] {
			t.Errorf("line %d: expected key %q, got %q", i, expectedOrder[i], key)
		}
	}
}

func TestConfigStructFields(t *testing.T) {
	cfg := &Config{
		Provider:       "AWS S3",
		Bucket:         "my-bucket",
		AccessKeyID:    "AKIAIOSFODNN7EXAMPLE",
		SecretKey:      "secret",
		Endpoint:       "https://example.com",
		Region:         "us-east-1",
		ForcePathStyle: true,
		SaveToFile:     true,
	}

	if cfg.Provider != "AWS S3" {
		t.Error("Provider field not set correctly")
	}
	if cfg.Bucket != "my-bucket" {
		t.Error("Bucket field not set correctly")
	}
	if cfg.AccessKeyID != "AKIAIOSFODNN7EXAMPLE" {
		t.Error("AccessKeyID field not set correctly")
	}
	if cfg.SecretKey != "secret" {
		t.Error("SecretKey field not set correctly")
	}
	if cfg.Endpoint != "https://example.com" {
		t.Error("Endpoint field not set correctly")
	}
	if cfg.Region != "us-east-1" {
		t.Error("Region field not set correctly")
	}
	if !cfg.ForcePathStyle {
		t.Error("ForcePathStyle field not set correctly")
	}
	if !cfg.SaveToFile {
		t.Error("SaveToFile field not set correctly")
	}
}

// assertEnvVar checks that a specific env var is present in the lines with the expected value.
func assertEnvVar(t *testing.T, lines []string, key, expectedValue string) {
	t.Helper()
	prefix := key + "="
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			actual := strings.TrimPrefix(line, prefix)
			if actual != expectedValue {
				t.Errorf("env var %s: expected %q, got %q", key, expectedValue, actual)
			}
			return
		}
	}
	t.Errorf("env var %s not found in output: %v", key, lines)
}
