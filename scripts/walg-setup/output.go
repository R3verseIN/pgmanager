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
	Interval       int
	RetentionDays  int
	SaveToFile     bool
}

// generateEnvVars produces the WAL-G environment variables from the config.
func generateEnvVars(cfg *Config) []string {
	var lines []string

	lines = append(lines, fmt.Sprintf("WALG_S3_PREFIX=s3://%s", cfg.Bucket))
	lines = append(lines, fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", cfg.AccessKeyID))
	lines = append(lines, fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", cfg.SecretKey))

	if cfg.Endpoint != "" {
		lines = append(lines, fmt.Sprintf("AWS_ENDPOINT=%s", cfg.Endpoint))
	}

	lines = append(lines, fmt.Sprintf("AWS_REGION=%s", cfg.Region))

	if cfg.ForcePathStyle {
		lines = append(lines, "AWS_S3_FORCE_PATH_STYLE=true")
	}

	if cfg.Interval > 0 && cfg.Interval != 3600 {
		lines = append(lines, fmt.Sprintf("WALG_BACKUP_INTERVAL=%d", cfg.Interval))
	}

	if cfg.RetentionDays > 0 && cfg.RetentionDays != 7 {
		lines = append(lines, fmt.Sprintf("WALG_BACKUP_RETENTION_DAYS=%d", cfg.RetentionDays))
	}

	return lines
}
