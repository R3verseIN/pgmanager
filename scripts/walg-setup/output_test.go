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
		Interval:       3600,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	// Should have exactly 4 lines: prefix, access key, secret key, region
	if len(lines) != 4 {
		t.Errorf("expected 4 env vars for minimal AWS config, got %d: %v", len(lines), lines)
	}

	assertEnvVar(t, lines, "WALG_S3_PREFIX", "s3://my-bucket")
	assertEnvVar(t, lines, "AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	assertEnvVar(t, lines, "AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG")
	assertEnvVar(t, lines, "AWS_REGION", "us-east-1")
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
		Interval:       3600,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	// Should have 6 lines: prefix, access key, secret key, endpoint, region, path style
	if len(lines) != 6 {
		t.Errorf("expected 6 env vars for MinIO config, got %d: %v", len(lines), lines)
	}

	assertEnvVar(t, lines, "WALG_S3_PREFIX", "s3://minio-bucket")
	assertEnvVar(t, lines, "AWS_ACCESS_KEY_ID", "minioadmin")
	assertEnvVar(t, lines, "AWS_SECRET_ACCESS_KEY", "minioadmin")
	assertEnvVar(t, lines, "AWS_ENDPOINT", "http://minio:9000")
	assertEnvVar(t, lines, "AWS_REGION", "us-east-1")
	assertEnvVar(t, lines, "AWS_S3_FORCE_PATH_STYLE", "true")
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
		Interval:       3600,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	assertEnvVar(t, lines, "AWS_ENDPOINT", "https://abc123.r2.cloudflarestorage.com")
	assertEnvVar(t, lines, "AWS_REGION", "auto")
	assertEnvVar(t, lines, "AWS_S3_FORCE_PATH_STYLE", "true")
}

func TestGenerateEnvVarsCustomInterval(t *testing.T) {
	cfg := &Config{
		Provider:       "AWS S3",
		Bucket:         "my-bucket",
		AccessKeyID:    "key",
		SecretKey:      "secret",
		Region:         "us-east-1",
		Interval:       1800,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	// Should include custom interval
	found := false
	for _, line := range lines {
		if line == "WALG_BACKUP_INTERVAL=1800" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("custom interval 1800 not found in env vars: %v", lines)
	}
}

func TestGenerateEnvVarsCustomRetention(t *testing.T) {
	cfg := &Config{
		Provider:       "AWS S3",
		Bucket:         "my-bucket",
		AccessKeyID:    "key",
		SecretKey:      "secret",
		Region:         "us-east-1",
		Interval:       3600,
		RetentionDays:  14,
	}

	lines := generateEnvVars(cfg)

	found := false
	for _, line := range lines {
		if line == "WALG_BACKUP_RETENTION_DAYS=14" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("custom retention 14 not found in env vars: %v", lines)
	}
}

func TestGenerateEnvVarsDefaultIntervalOmitted(t *testing.T) {
	cfg := &Config{
		Provider:       "AWS S3",
		Bucket:         "my-bucket",
		AccessKeyID:    "key",
		SecretKey:      "secret",
		Region:         "us-east-1",
		Interval:       3600,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	// Default interval (3600) should be omitted
	for _, line := range lines {
		if strings.HasPrefix(line, "WALG_BACKUP_INTERVAL") {
			t.Errorf("default interval should be omitted, but found: %s", line)
		}
	}
}

func TestGenerateEnvVarsDefaultRetentionOmitted(t *testing.T) {
	cfg := &Config{
		Provider:       "AWS S3",
		Bucket:         "my-bucket",
		AccessKeyID:    "key",
		SecretKey:      "secret",
		Region:         "us-east-1",
		Interval:       3600,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	// Default retention (7) should be omitted
	for _, line := range lines {
		if strings.HasPrefix(line, "WALG_BACKUP_RETENTION_DAYS") {
			t.Errorf("default retention should be omitted, but found: %s", line)
		}
	}
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
		Interval:       3600,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	for _, line := range lines {
		if strings.HasPrefix(line, "AWS_ENDPOINT") {
			t.Errorf("empty endpoint should be omitted, but found: %s", line)
		}
	}
}

func TestGenerateEnvVarsPathStyleFalseOmitted(t *testing.T) {
	cfg := &Config{
		Provider:       "AWS S3",
		Bucket:         "my-bucket",
		AccessKeyID:    "key",
		SecretKey:      "secret",
		Region:         "us-east-1",
		ForcePathStyle: false,
		Interval:       3600,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	for _, line := range lines {
		if strings.HasPrefix(line, "AWS_S3_FORCE_PATH_STYLE") {
			t.Errorf("path style false should be omitted, but found: %s", line)
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
		Interval:       3600,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	// Secret key with special characters should be preserved exactly
	assertEnvVar(t, lines, "AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/BpxRsiCyF8TH+nVw=")
}

func TestGenerateEnvVarsEmptyBucket(t *testing.T) {
	cfg := &Config{
		Provider:    "AWS S3",
		Bucket:      "",
		AccessKeyID: "key",
		SecretKey:   "secret",
		Region:      "us-east-1",
	}

	lines := generateEnvVars(cfg)

	// Should still generate the prefix, even if empty
	assertEnvVar(t, lines, "WALG_S3_PREFIX", "s3://")
}

func TestGenerateEnvVarsFormat(t *testing.T) {
	cfg := &Config{
		Provider:       "AWS S3",
		Bucket:         "test",
		AccessKeyID:    "key",
		SecretKey:      "secret",
		Region:         "us-east-1",
		Interval:       3600,
		RetentionDays:  7,
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
		Interval:       900,
		RetentionDays:  30,
	}

	lines := generateEnvVars(cfg)

	// Should have all 8 env vars
	if len(lines) != 8 {
		t.Errorf("expected 8 env vars for full custom config, got %d: %v", len(lines), lines)
	}

	assertEnvVar(t, lines, "WALG_S3_PREFIX", "s3://custom-bucket")
	assertEnvVar(t, lines, "AWS_ACCESS_KEY_ID", "custom-key")
	assertEnvVar(t, lines, "AWS_SECRET_ACCESS_KEY", "custom-secret")
	assertEnvVar(t, lines, "AWS_ENDPOINT", "http://custom:9000")
	assertEnvVar(t, lines, "AWS_REGION", "custom-region")
	assertEnvVar(t, lines, "AWS_S3_FORCE_PATH_STYLE", "true")
	assertEnvVar(t, lines, "WALG_BACKUP_INTERVAL", "900")
	assertEnvVar(t, lines, "WALG_BACKUP_RETENTION_DAYS", "30")
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
		Interval:       1800,
		RetentionDays:  14,
	}

	lines := generateEnvVars(cfg)

	// Verify order: prefix, access key, secret key, endpoint, region, path style, interval, retention
	expectedOrder := []string{
		"WALG_S3_PREFIX",
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_ENDPOINT",
		"AWS_REGION",
		"AWS_S3_FORCE_PATH_STYLE",
		"WALG_BACKUP_INTERVAL",
		"WALG_BACKUP_RETENTION_DAYS",
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
		Interval:       3600,
		RetentionDays:  7,
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
	if cfg.Interval != 3600 {
		t.Error("Interval field not set correctly")
	}
	if cfg.RetentionDays != 7 {
		t.Error("RetentionDays field not set correctly")
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
