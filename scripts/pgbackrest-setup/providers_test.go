package main

import (
	"strings"
	"testing"
)

func TestProvidersNotEmpty(t *testing.T) {
	if len(providers) == 0 {
		t.Fatal("providers list is empty")
	}
}

func TestProviderNamesUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i, p := range providers {
		if p.Name == "" {
			t.Errorf("provider [%d] has empty name", i)
		}
		if seen[p.Name] {
			t.Errorf("provider name %q is duplicated", p.Name)
		}
		seen[p.Name] = true
	}
}

func TestProviderRegionsNotEmpty(t *testing.T) {
	for i, p := range providers {
		if p.Region == "" {
			t.Errorf("provider [%d] %q has empty region", i, p.Name)
		}
	}
}

func TestAWSDefaults(t *testing.T) {
	aws := findProvider(t, "AWS S3")

	if aws.Endpoint != "" {
		t.Errorf("AWS S3 endpoint should be empty (uses SDK default), got %q", aws.Endpoint)
	}
	if aws.Region != "us-east-1" {
		t.Errorf("AWS S3 default region should be us-east-1, got %q", aws.Region)
	}
	if aws.PathStyle {
		t.Error("AWS S3 should not force path style")
	}
	if aws.AccountID {
		t.Error("AWS S3 should not require account ID")
	}
}

func TestR2Defaults(t *testing.T) {
	r2 := findProvider(t, "Cloudflare R2")

	if r2.Endpoint != "https://{account_id}.r2.cloudflarestorage.com" {
		t.Errorf("R2 endpoint template wrong, got %q", r2.Endpoint)
	}
	if r2.Region != "auto" {
		t.Errorf("R2 region should be auto, got %q", r2.Region)
	}
	if !r2.PathStyle {
		t.Error("R2 should force path style")
	}
	if !r2.AccountID {
		t.Error("R2 should require account ID")
	}
	if r2.RegionEdit {
		t.Error("R2 region should not be editable (it's always auto)")
	}
}

func TestMinIODefaults(t *testing.T) {
	minio := findProvider(t, "MinIO")

	if minio.Endpoint != "" {
		t.Errorf("MinIO endpoint should be empty (user provides), got %q", minio.Endpoint)
	}
	if minio.Region != "us-east-1" {
		t.Errorf("MinIO default region should be us-east-1, got %q", minio.Region)
	}
	if !minio.PathStyle {
		t.Error("MinIO should force path style")
	}
	if minio.AccountID {
		t.Error("MinIO should not require account ID")
	}
}

func TestDigitalOceanDefaults(t *testing.T) {
	do := findProvider(t, "DigitalOcean Spaces")

	if do.Endpoint != "https://{region}.digitaloceanspaces.com" {
		t.Errorf("DO endpoint template wrong, got %q", do.Endpoint)
	}
	if do.Region != "nyc3" {
		t.Errorf("DO default region should be nyc3, got %q", do.Region)
	}
	if do.PathStyle {
		t.Error("DO should not force path style")
	}
	if !do.RegionEdit {
		t.Error("DO should allow region editing")
	}
}

func TestWasabiDefaults(t *testing.T) {
	wasabi := findProvider(t, "Wasabi")

	if wasabi.Endpoint != "s3.wasabisys.com" {
		t.Errorf("Wasabi endpoint wrong, got %q", wasabi.Endpoint)
	}
	if wasabi.Region != "us-east-1" {
		t.Errorf("Wasabi default region should be us-east-1, got %q", wasabi.Region)
	}
	if wasabi.PathStyle {
		t.Error("Wasabi should not force path style")
	}
}

func TestBackblazeDefaults(t *testing.T) {
	b2 := findProvider(t, "Backblaze B2")

	if b2.Endpoint != "s3.{region}.backblazeb2.com" {
		t.Errorf("B2 endpoint template wrong, got %q", b2.Endpoint)
	}
	if b2.Region != "us-west-000" {
		t.Errorf("B2 default region should be us-west-000, got %q", b2.Region)
	}
	if b2.PathStyle {
		t.Error("B2 should not force path style")
	}
}

func TestGCSDefaults(t *testing.T) {
	gcs := findProvider(t, "Google Cloud Storage")

	if gcs.Endpoint != "storage.googleapis.com" {
		t.Errorf("GCS endpoint wrong, got %q", gcs.Endpoint)
	}
	if gcs.Region != "us-central1" {
		t.Errorf("GCS default region should be us-central1, got %q", gcs.Region)
	}
	if gcs.PathStyle {
		t.Error("GCS should not force path style")
	}
}

func TestAlibabaDefaults(t *testing.T) {
	ali := findProvider(t, "Alibaba Cloud OSS")

	if ali.Endpoint != "oss-{region}.aliyuncs.com" {
		t.Errorf("Alibaba endpoint template wrong, got %q", ali.Endpoint)
	}
	if ali.Region != "cn-hangzhou" {
		t.Errorf("Alibaba default region should be cn-hangzhou, got %q", ali.Region)
	}
	if ali.PathStyle {
		t.Error("Alibaba should not force path style")
	}
}

func TestScalewayDefaults(t *testing.T) {
	sw := findProvider(t, "Scaleway")

	if sw.Endpoint != "s3.{region}.scw.cloud" {
		t.Errorf("Scaleway endpoint template wrong, got %q", sw.Endpoint)
	}
	if sw.Region != "fr-par" {
		t.Errorf("Scaleway default region should be fr-par, got %q", sw.Region)
	}
	if sw.PathStyle {
		t.Error("Scaleway should not force path style")
	}
}

func TestCephDefaults(t *testing.T) {
	ceph := findProvider(t, "Ceph")

	if ceph.Endpoint != "" {
		t.Errorf("Ceph endpoint should be empty (user provides), got %q", ceph.Endpoint)
	}
	if ceph.Region != "us-east-1" {
		t.Errorf("Ceph default region should be us-east-1, got %q", ceph.Region)
	}
	if !ceph.PathStyle {
		t.Error("Ceph should force path style")
	}
}

func TestCustomDefaults(t *testing.T) {
	custom := findProvider(t, "Custom S3-Compatible")

	if custom.Endpoint != "" {
		t.Errorf("Custom endpoint should be empty (user provides), got %q", custom.Endpoint)
	}
	if custom.Region != "us-east-1" {
		t.Errorf("Custom default region should be us-east-1, got %q", custom.Region)
	}
	if !custom.PathStyle {
		t.Error("Custom should force path style by default")
	}
	if !custom.RegionEdit {
		t.Error("Custom should allow region editing")
	}
}

func TestEndpointTemplateAccountID(t *testing.T) {
	r2 := findProvider(t, "Cloudflare R2")
	resolved := strings.ReplaceAll(r2.Endpoint, "{account_id}", "myaccount123")
	if resolved != "https://myaccount123.r2.cloudflarestorage.com" {
		t.Errorf("R2 endpoint resolution failed, got %q", resolved)
	}
}

func TestEndpointTemplateRegion(t *testing.T) {
	do := findProvider(t, "DigitalOcean Spaces")
	resolved := strings.ReplaceAll(do.Endpoint, "{region}", "sfo3")
	if resolved != "https://sfo3.digitaloceanspaces.com" {
		t.Errorf("DO endpoint resolution failed, got %q", resolved)
	}
}

func TestEndpointTemplateRegionBackblaze(t *testing.T) {
	b2 := findProvider(t, "Backblaze B2")
	resolved := strings.ReplaceAll(b2.Endpoint, "{region}", "us-west-001")
	if resolved != "s3.us-west-001.backblazeb2.com" {
		t.Errorf("B2 endpoint resolution failed, got %q", resolved)
	}
}

func TestEndpointTemplateRegionAlibaba(t *testing.T) {
	ali := findProvider(t, "Alibaba Cloud OSS")
	resolved := strings.ReplaceAll(ali.Endpoint, "{region}", "us-west-1")
	if resolved != "oss-us-west-1.aliyuncs.com" {
		t.Errorf("Alibaba endpoint resolution failed, got %q", resolved)
	}
}

func TestEndpointTemplateRegionScaleway(t *testing.T) {
	sw := findProvider(t, "Scaleway")
	resolved := strings.ReplaceAll(sw.Endpoint, "{region}", "nl-ams")
	if resolved != "s3.nl-ams.scw.cloud" {
		t.Errorf("Scaleway endpoint resolution failed, got %q", resolved)
	}
}

func TestProvidersHaveRequiredFields(t *testing.T) {
	for i, p := range providers {
		if p.Name == "" {
			t.Errorf("provider [%d] missing Name", i)
		}
		if p.Region == "" {
			t.Errorf("provider [%d] %q missing Region", i, p.Name)
		}
		// Endpoint can be empty for user-provided providers (MinIO, Ceph, Custom)
		// PathStyle is a bool, always has a value
		// AccountID is a bool, always has a value
	}
}

func TestProvidersThatRequireEndpoint(t *testing.T) {
	// These providers have no endpoint template — user must provide one
	userProvidedEndpoint := []string{"MinIO", "Ceph", "Custom S3-Compatible"}
	for _, name := range userProvidedEndpoint {
		p := findProvider(t, name)
		if p.Endpoint != "" {
			t.Errorf("provider %q should have empty endpoint (user provides it)", name)
		}
	}
}

func TestProvidersWithFixedEndpoint(t *testing.T) {
	// These providers have fixed endpoints (no template variables)
	fixedEndpoint := []string{"Wasabi", "Google Cloud Storage"}
	for _, name := range fixedEndpoint {
		p := findProvider(t, name)
		if p.Endpoint == "" {
			t.Errorf("provider %q should have a fixed endpoint", name)
		}
		if strings.Contains(p.Endpoint, "{") {
			t.Errorf("provider %q endpoint should not contain template variables", name)
		}
	}
}

func TestProvidersWithTemplateEndpoint(t *testing.T) {
	// These providers have template endpoints with variables
	templateEndpoint := []string{"Cloudflare R2", "DigitalOcean Spaces", "Backblaze B2", "Alibaba Cloud OSS", "Scaleway"}
	for _, name := range templateEndpoint {
		p := findProvider(t, name)
		if p.Endpoint == "" {
			t.Errorf("provider %q should have a template endpoint", name)
		}
		if !strings.Contains(p.Endpoint, "{") {
			t.Errorf("provider %q endpoint should contain template variables", name)
		}
	}
}

// findProvider is a test helper that finds a provider by name.
func findProvider(t *testing.T, name string) Provider {
	t.Helper()
	for _, p := range providers {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("provider %q not found", name)
	return Provider{}
}
