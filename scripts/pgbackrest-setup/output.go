package main

import "fmt"

// Config holds the user's S3 backup configuration.
type Config struct {
	Provider       string
	Bucket         string
	AccessKeyID    string
	SecretKey      string
	Endpoint       string
	Region         string
	ForcePathStyle bool
	SaveToFile     bool
}

// generateEnvVars produces the pgBackRest environment variables from the config.
func generateEnvVars(cfg *Config) []string {
	var lines []string

	lines = append(lines, "PGBACKREST_REPO1_TYPE=s3")
	lines = append(lines, fmt.Sprintf("PGBACKREST_REPO1_S3_BUCKET=%s", cfg.Bucket))
	lines = append(lines, fmt.Sprintf("PGBACKREST_REPO1_S3_KEY=%s", cfg.AccessKeyID))
	lines = append(lines, fmt.Sprintf("PGBACKREST_REPO1_S3_KEY_SECRET=%s", cfg.SecretKey))

	if cfg.Endpoint != "" {
		lines = append(lines, fmt.Sprintf("PGBACKREST_REPO1_S3_ENDPOINT=%s", cfg.Endpoint))
	}

	lines = append(lines, fmt.Sprintf("PGBACKREST_REPO1_S3_REGION=%s", cfg.Region))

	if cfg.ForcePathStyle {
		lines = append(lines, "PGBACKREST_REPO1_S3_URI_STYLE=path")
	} else {
		lines = append(lines, "PGBACKREST_REPO1_S3_URI_STYLE=host")
	}

	return lines
}
