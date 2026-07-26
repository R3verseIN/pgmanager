package main

import (
	"strings"
	"testing"
)

// TestProviderResolutionAWS tests the full config generation for AWS S3.
func TestProviderResolutionAWS(t *testing.T) {
	cfg := resolveConfig(t, "AWS S3", "", "us-east-1", "")

	if cfg.Endpoint != "" {
		t.Errorf("AWS endpoint should be empty, got %q", cfg.Endpoint)
	}
	if cfg.Region != "us-east-1" {
		t.Errorf("AWS region should be us-east-1, got %q", cfg.Region)
	}
	if cfg.ForcePathStyle {
		t.Error("AWS should not force path style")
	}
}

// TestProviderResolutionR2 tests the full config generation for Cloudflare R2.
func TestProviderResolutionR2(t *testing.T) {
	cfg := resolveConfig(t, "Cloudflare R2", "abc123", "auto", "")

	expected := "https://abc123.r2.cloudflarestorage.com"
	if cfg.Endpoint != expected {
		t.Errorf("R2 endpoint should be %q, got %q", expected, cfg.Endpoint)
	}
	if cfg.Region != "auto" {
		t.Errorf("R2 region should be auto, got %q", cfg.Region)
	}
	if !cfg.ForcePathStyle {
		t.Error("R2 should force path style")
	}
}

// TestProviderResolutionMinIO tests the full config generation for MinIO.
func TestProviderResolutionMinIO(t *testing.T) {
	cfg := resolveConfig(t, "MinIO", "", "us-east-1", "http://minio:9000")

	if cfg.Endpoint != "http://minio:9000" {
		t.Errorf("MinIO endpoint should be user-provided, got %q", cfg.Endpoint)
	}
	if cfg.Region != "us-east-1" {
		t.Errorf("MinIO region should be us-east-1, got %q", cfg.Region)
	}
	if !cfg.ForcePathStyle {
		t.Error("MinIO should force path style")
	}
}

// TestProviderResolutionDO tests the full config generation for DigitalOcean Spaces.
func TestProviderResolutionDO(t *testing.T) {
	cfg := resolveConfig(t, "DigitalOcean Spaces", "", "sfo3", "")

	expected := "https://sfo3.digitaloceanspaces.com"
	if cfg.Endpoint != expected {
		t.Errorf("DO endpoint should be %q, got %q", expected, cfg.Endpoint)
	}
	if cfg.Region != "sfo3" {
		t.Errorf("DO region should be sfo3, got %q", cfg.Region)
	}
	if cfg.ForcePathStyle {
		t.Error("DO should not force path style")
	}
}

// TestProviderResolutionWasabi tests the full config generation for Wasabi.
func TestProviderResolutionWasabi(t *testing.T) {
	cfg := resolveConfig(t, "Wasabi", "", "eu-central-1", "")

	if cfg.Endpoint != "s3.wasabisys.com" {
		t.Errorf("Wasabi endpoint should be s3.wasabisys.com, got %q", cfg.Endpoint)
	}
	if cfg.Region != "eu-central-1" {
		t.Errorf("Wasabi region should be eu-central-1, got %q", cfg.Region)
	}
}

// TestProviderResolutionB2 tests the full config generation for Backblaze B2.
func TestProviderResolutionB2(t *testing.T) {
	cfg := resolveConfig(t, "Backblaze B2", "", "us-west-001", "")

	expected := "s3.us-west-001.backblazeb2.com"
	if cfg.Endpoint != expected {
		t.Errorf("B2 endpoint should be %q, got %q", expected, cfg.Endpoint)
	}
	if cfg.Region != "us-west-001" {
		t.Errorf("B2 region should be us-west-001, got %q", cfg.Region)
	}
}

// TestProviderResolutionGCS tests the full config generation for Google Cloud Storage.
func TestProviderResolutionGCS(t *testing.T) {
	cfg := resolveConfig(t, "Google Cloud Storage", "", "europe-west1", "")

	if cfg.Endpoint != "storage.googleapis.com" {
		t.Errorf("GCS endpoint should be storage.googleapis.com, got %q", cfg.Endpoint)
	}
	if cfg.Region != "europe-west1" {
		t.Errorf("GCS region should be europe-west1, got %q", cfg.Region)
	}
}

// TestProviderResolutionAlibaba tests the full config generation for Alibaba Cloud OSS.
func TestProviderResolutionAlibaba(t *testing.T) {
	cfg := resolveConfig(t, "Alibaba Cloud OSS", "", "us-west-1", "")

	expected := "oss-us-west-1.aliyuncs.com"
	if cfg.Endpoint != expected {
		t.Errorf("Alibaba endpoint should be %q, got %q", expected, cfg.Endpoint)
	}
	if cfg.Region != "us-west-1" {
		t.Errorf("Alibaba region should be us-west-1, got %q", cfg.Region)
	}
}

// TestProviderResolutionScaleway tests the full config generation for Scaleway.
func TestProviderResolutionScaleway(t *testing.T) {
	cfg := resolveConfig(t, "Scaleway", "", "nl-ams", "")

	expected := "s3.nl-ams.scw.cloud"
	if cfg.Endpoint != expected {
		t.Errorf("Scaleway endpoint should be %q, got %q", expected, cfg.Endpoint)
	}
	if cfg.Region != "nl-ams" {
		t.Errorf("Scaleway region should be nl-ams, got %q", cfg.Region)
	}
}

// TestProviderResolutionCeph tests the full config generation for Ceph.
func TestProviderResolutionCeph(t *testing.T) {
	cfg := resolveConfig(t, "Ceph", "", "us-east-1", "http://ceph:7480")

	if cfg.Endpoint != "http://ceph:7480" {
		t.Errorf("Ceph endpoint should be user-provided, got %q", cfg.Endpoint)
	}
	if !cfg.ForcePathStyle {
		t.Error("Ceph should force path style")
	}
}

// TestProviderResolutionCustom tests the full config generation for Custom S3-Compatible.
func TestProviderResolutionCustom(t *testing.T) {
	cfg := resolveConfig(t, "Custom S3-Compatible", "", "my-region", "http://custom:9000")

	if cfg.Endpoint != "http://custom:9000" {
		t.Errorf("Custom endpoint should be user-provided, got %q", cfg.Endpoint)
	}
	if cfg.Region != "my-region" {
		t.Errorf("Custom region should be my-region, got %q", cfg.Region)
	}
	if !cfg.ForcePathStyle {
		t.Error("Custom should force path style")
	}
}

// TestEnvVarsIntegrationR2 tests the full env var generation for R2.
func TestEnvVarsIntegrationR2(t *testing.T) {
	cfg := &Config{
		Provider:       "Cloudflare R2",
		Bucket:         "my-r2-bucket",
		AccessKeyID:    "r2key",
		SecretKey:      "r2secret",
		Endpoint:       "https://abc123.r2.cloudflarestorage.com",
		Region:         "auto",
		ForcePathStyle: true,
		Interval:       3600,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	assertEnvVar(t, lines, "WALG_S3_PREFIX", "s3://my-r2-bucket")
	assertEnvVar(t, lines, "AWS_ACCESS_KEY_ID", "r2key")
	assertEnvVar(t, lines, "AWS_SECRET_ACCESS_KEY", "r2secret")
	assertEnvVar(t, lines, "AWS_ENDPOINT", "https://abc123.r2.cloudflarestorage.com")
	assertEnvVar(t, lines, "AWS_REGION", "auto")
	assertEnvVar(t, lines, "AWS_S3_FORCE_PATH_STYLE", "true")
}

// TestEnvVarsIntegrationMinIO tests the full env var generation for MinIO.
func TestEnvVarsIntegrationMinIO(t *testing.T) {
	cfg := &Config{
		Provider:       "MinIO",
		Bucket:         "minio-test",
		AccessKeyID:    "minioadmin",
		SecretKey:      "minioadmin",
		Endpoint:       "http://minio:9000",
		Region:         "us-east-1",
		ForcePathStyle: true,
		Interval:       1800,
		RetentionDays:  14,
	}

	lines := generateEnvVars(cfg)

	assertEnvVar(t, lines, "WALG_S3_PREFIX", "s3://minio-test")
	assertEnvVar(t, lines, "AWS_ENDPOINT", "http://minio:9000")
	assertEnvVar(t, lines, "AWS_S3_FORCE_PATH_STYLE", "true")
	assertEnvVar(t, lines, "WALG_BACKUP_INTERVAL", "1800")
	assertEnvVar(t, lines, "WALG_BACKUP_RETENTION_DAYS", "14")
}

// TestEnvVarsIntegrationDO tests the full env var generation for DigitalOcean Spaces.
func TestEnvVarsIntegrationDO(t *testing.T) {
	cfg := &Config{
		Provider:       "DigitalOcean Spaces",
		Bucket:         "my-space",
		AccessKeyID:    "do-key",
		SecretKey:      "do-secret",
		Endpoint:       "https://sfo3.digitaloceanspaces.com",
		Region:         "sfo3",
		ForcePathStyle: false,
		Interval:       3600,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	assertEnvVar(t, lines, "AWS_ENDPOINT", "https://sfo3.digitaloceanspaces.com")
	assertEnvVar(t, lines, "AWS_REGION", "sfo3")

	// Path style should NOT be present
	for _, line := range lines {
		if strings.HasPrefix(line, "AWS_S3_FORCE_PATH_STYLE") {
			t.Errorf("DO should not have path style, but found: %s", line)
		}
	}
}

// TestEnvVarsIntegrationWasabi tests the full env var generation for Wasabi.
func TestEnvVarsIntegrationWasabi(t *testing.T) {
	cfg := &Config{
		Provider:       "Wasabi",
		Bucket:         "wasabi-bucket",
		AccessKeyID:    "wasabi-key",
		SecretKey:      "wasabi-secret",
		Endpoint:       "s3.wasabisys.com",
		Region:         "eu-central-1",
		ForcePathStyle: false,
		Interval:       3600,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	assertEnvVar(t, lines, "AWS_ENDPOINT", "s3.wasabisys.com")
	assertEnvVar(t, lines, "AWS_REGION", "eu-central-1")
}

// TestEnvVarsIntegrationB2 tests the full env var generation for Backblaze B2.
func TestEnvVarsIntegrationB2(t *testing.T) {
	cfg := &Config{
		Provider:       "Backblaze B2",
		Bucket:         "b2-bucket",
		AccessKeyID:    "b2-key",
		SecretKey:      "b2-secret",
		Endpoint:       "s3.us-west-001.backblazeb2.com",
		Region:         "us-west-001",
		ForcePathStyle: false,
		Interval:       3600,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	assertEnvVar(t, lines, "AWS_ENDPOINT", "s3.us-west-001.backblazeb2.com")
	assertEnvVar(t, lines, "AWS_REGION", "us-west-001")
}

// TestEnvVarsIntegrationCustom tests the full env var generation for Custom provider.
func TestEnvVarsIntegrationCustom(t *testing.T) {
	cfg := &Config{
		Provider:       "Custom S3-Compatible",
		Bucket:         "custom-bucket",
		AccessKeyID:    "custom-key",
		SecretKey:      "custom-secret",
		Endpoint:       "http://custom-storage:9000",
		Region:         "custom-region",
		ForcePathStyle: true,
		Interval:       900,
		RetentionDays:  30,
	}

	lines := generateEnvVars(cfg)

	assertEnvVar(t, lines, "AWS_ENDPOINT", "http://custom-storage:9000")
	assertEnvVar(t, lines, "AWS_REGION", "custom-region")
	assertEnvVar(t, lines, "AWS_S3_FORCE_PATH_STYLE", "true")
	assertEnvVar(t, lines, "WALG_BACKUP_INTERVAL", "900")
	assertEnvVar(t, lines, "WALG_BACKUP_RETENTION_DAYS", "30")
}

// TestEnvVarsIntegrationDefaults tests that default values are omitted.
func TestEnvVarsIntegrationDefaults(t *testing.T) {
	cfg := &Config{
		Provider:       "AWS S3",
		Bucket:         "test",
		AccessKeyID:    "key",
		SecretKey:      "secret",
		Endpoint:       "",
		Region:         "us-east-1",
		ForcePathStyle: false,
		Interval:       3600,
		RetentionDays:  7,
	}

	lines := generateEnvVars(cfg)

	// Should only have 4 lines (prefix, access key, secret key, region)
	if len(lines) != 4 {
		t.Errorf("expected 4 lines for defaults, got %d: %v", len(lines), lines)
	}

	// No endpoint, no path style, no interval, no retention
	for _, line := range lines {
		key := strings.SplitN(line, "=", 2)[0]
		switch key {
		case "AWS_ENDPOINT", "AWS_S3_FORCE_PATH_STYLE", "WALG_BACKUP_INTERVAL", "WALG_BACKUP_RETENTION_DAYS":
			t.Errorf("default value should be omitted, but found: %s", line)
		}
	}
}

// resolveConfig is a test helper that simulates the provider resolution logic
// without the interactive UI. It mirrors the logic in ui.go's runWizard.
func resolveConfig(t *testing.T, providerName, accountID, region, endpoint string) *Config {
	t.Helper()

	var provider Provider
	for _, p := range providers {
		if p.Name == providerName {
			provider = p
			break
		}
	}
	if provider.Name == "" {
		t.Fatalf("provider %q not found", providerName)
	}

	// Resolve endpoint (same logic as ui.go)
	resolvedEndpoint := provider.Endpoint
	if provider.AccountID && accountID != "" {
		resolvedEndpoint = strings.ReplaceAll(resolvedEndpoint, "{account_id}", accountID)
	}
	if provider.Endpoint != "" && strings.Contains(provider.Endpoint, "{region}") {
		resolvedEndpoint = strings.ReplaceAll(resolvedEndpoint, "{region}", region)
	}
	if endpoint != "" {
		resolvedEndpoint = endpoint
	}

	// Set region default
	if region == "" {
		region = provider.Region
	}

	return &Config{
		Provider:       provider.Name,
		Bucket:         "test-bucket",
		AccessKeyID:    "test-key",
		SecretKey:      "test-secret",
		Endpoint:       resolvedEndpoint,
		Region:         region,
		ForcePathStyle: provider.PathStyle,
		Interval:       3600,
		RetentionDays:  7,
	}
}
