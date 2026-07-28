package main

// Provider defines the default S3 configuration for a cloud provider.
// Fields use Go template syntax: {account_id}, {region}, {endpoint}.
type Provider struct {
	Name        string // Display name for the provider
	Endpoint    string // Default endpoint template (empty = use AWS default)
	Region      string // Default region
	PathStyle   bool   // Force path-style addressing
	AccountID   bool   // Requires account ID input (for R2)
	RegionEdit  bool   // Allow user to edit region (false = auto-filled)
	RegionLabel string // Label for region prompt (e.g., "Space region")
}

var providers = []Provider{
	{
		Name:      "AWS S3",
		Endpoint:  "",
		Region:    "us-east-1",
		PathStyle: false,
	},
	{
		Name:        "Cloudflare R2",
		Endpoint:    "https://{account_id}.r2.cloudflarestorage.com",
		Region:      "auto",
		PathStyle:   true,
		AccountID:   true,
		RegionEdit:  false,
		RegionLabel: "",
	},
	{
		Name:        "DigitalOcean Spaces",
		Endpoint:    "https://{region}.digitaloceanspaces.com",
		Region:      "nyc3",
		PathStyle:   false,
		RegionEdit:  true,
		RegionLabel: "Space region (e.g., nyc3, sfo3, ams3)",
	},
	{
		Name:        "Wasabi",
		Endpoint:    "s3.wasabisys.com",
		Region:      "us-east-1",
		PathStyle:   false,
		RegionEdit:  true,
		RegionLabel: "Region (e.g., us-east-1, us-west-1, eu-central-1)",
	},
	{
		Name:        "Backblaze B2",
		Endpoint:    "s3.{region}.backblazeb2.com",
		Region:      "us-west-000",
		PathStyle:   false,
		RegionEdit:  true,
		RegionLabel: "Region (e.g., us-west-000, us-west-001, eu-central-003)",
	},
	{
		Name:        "Google Cloud Storage",
		Endpoint:    "storage.googleapis.com",
		Region:      "us-central1",
		PathStyle:   false,
		RegionEdit:  true,
		RegionLabel: "Region (e.g., us-central1, us-east1, europe-west1)",
	},
	{
		Name:        "Alibaba Cloud OSS",
		Endpoint:    "oss-{region}.aliyuncs.com",
		Region:      "cn-hangzhou",
		PathStyle:   false,
		RegionEdit:  true,
		RegionLabel: "Region (e.g., cn-hangzhou, us-west-1, eu-central-1)",
	},
	{
		Name:        "Scaleway",
		Endpoint:    "s3.{region}.scw.cloud",
		Region:      "fr-par",
		PathStyle:   false,
		RegionEdit:  true,
		RegionLabel: "Region (e.g., fr-par, nl-ams, de-cd)",
	},
	{
		Name:        "MinIO",
		Endpoint:    "",
		Region:      "us-east-1",
		PathStyle:   true,
		RegionEdit:  false,
		RegionLabel: "",
	},
	{
		Name:        "Ceph",
		Endpoint:    "",
		Region:      "us-east-1",
		PathStyle:   true,
		RegionEdit:  false,
		RegionLabel: "",
	},
	{
		Name:        "Custom S3-Compatible",
		Endpoint:    "",
		Region:      "us-east-1",
		PathStyle:   true,
		RegionEdit:  true,
		RegionLabel: "Region",
	},
}
