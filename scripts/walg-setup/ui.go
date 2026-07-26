package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

var errCancelled = errors.New("cancelled")

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)
	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A0A0A0"))
	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF4444")).
			Bold(true)
)

func runWizard() (*Config, error) {
	var providerIdx int

	providerOptions := make([]huh.Option[int], len(providers))
	for i, p := range providers {
		providerOptions[i] = huh.NewOption(p.Name, i)
	}

	// Step 1: Select provider
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("WAL-G S3 Backup Configuration").
				Description("This wizard generates environment variables for WAL-G S3 backups.\n"+
					"Select your provider and enter credentials."),
			huh.NewSelect[int]().
				Title("Pick your S3 provider").
				Options(providerOptions...).
				Value(&providerIdx),
		),
	)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errCancelled
		}
		return nil, err
	}
	provider := providers[providerIdx]

	// Step 2: Bucket + credentials
	var bucket, accessKey, secretKey string
	form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Bucket name").
				Placeholder("my-postgres-backups").
				Validate(huh.ValidateMinLength(1)).
				Value(&bucket),
			huh.NewInput().
				Title("Access Key ID").
				Placeholder("AKIAIOSFODNN7EXAMPLE").
				Validate(huh.ValidateMinLength(1)).
				Value(&accessKey),
			huh.NewInput().
				Title("Secret Access Key").
				Password(true).
				Validate(huh.ValidateMinLength(1)).
				Value(&secretKey),
		),
	)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errCancelled
		}
		return nil, err
	}

	// Step 3: Provider-specific fields
	var accountID, endpoint, region string
	var specificFields []huh.Field

	if provider.AccountID {
		specificFields = append(specificFields,
			huh.NewInput().
				Title("Account ID").
				Placeholder("abc123").
				Validate(huh.ValidateMinLength(1)).
				Value(&accountID),
		)
	}

	if provider.Endpoint == "" && provider.Name != "AWS S3" {
		specificFields = append(specificFields,
			huh.NewInput().
				Title("Endpoint URL").
				Placeholder("http://minio:9000").
				Validate(huh.ValidateMinLength(1)).
				Value(&endpoint),
		)
	}

	if provider.RegionEdit {
		regionLabel := provider.RegionLabel
		if regionLabel == "" {
			regionLabel = "Region"
		}
		specificFields = append(specificFields,
			huh.NewInput().
				Title(regionLabel).
				Placeholder(provider.Region).
				Value(&region),
		)
	}

	if len(specificFields) > 0 {
		form = huh.NewForm(huh.NewGroup(specificFields...))
		if err := form.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil, errCancelled
			}
			return nil, err
		}
	}

	// Set region default
	if region == "" {
		region = provider.Region
	}

	// Resolve endpoint
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

	// Step 4: Advanced settings
	var intervalStr, retentionStr string
	var saveFile bool
	form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Backup interval (seconds)").
				Placeholder("3600").
				Value(&intervalStr),
			huh.NewInput().
				Title("Retention days").
				Placeholder("7").
				Value(&retentionStr),
			huh.NewConfirm().
				Title("Save to file (walg.env)?").
				Affirmative("Yes").
				Negative("No, print to stdout").
				Value(&saveFile),
		),
	)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errCancelled
		}
		return nil, err
	}

	interval := 3600
	retention := 7
	if intervalStr != "" {
		fmt.Sscanf(intervalStr, "%d", &interval)
	}
	if retentionStr != "" {
		fmt.Sscanf(retentionStr, "%d", &retention)
	}

	return &Config{
		Provider:       provider.Name,
		Bucket:         bucket,
		AccessKeyID:    accessKey,
		SecretKey:      secretKey,
		Endpoint:       resolvedEndpoint,
		Region:         region,
		ForcePathStyle: provider.PathStyle,
		Interval:       interval,
		RetentionDays:  retention,
		SaveToFile:     saveFile,
	}, nil
}

func printBanner() {
	fmt.Println(titleStyle.Render(" WAL-G S3 Backup Configuration Generator "))
	fmt.Println()
}

func printConfig(cfg *Config) {
	fmt.Println()
	fmt.Println(successStyle.Render("Generated WAL-G environment variables:"))
	fmt.Println(strings.Repeat("-", 50))

	envVars := generateEnvVars(cfg)
	for _, v := range envVars {
		fmt.Println(v)
	}

	fmt.Println(strings.Repeat("-", 50))
	fmt.Println()

	if cfg.SaveToFile {
		filename := "walg.env"
		content := strings.Join(envVars, "\n") + "\n"
		if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
			fmt.Println(errorStyle.Render(fmt.Sprintf("Failed to save file: %v", err)))
			os.Exit(1)
		}
		fmt.Println(successStyle.Render(fmt.Sprintf("Saved to: %s", filename)))
	} else {
		fmt.Println(subtitleStyle.Render("Copy the above into your docker-compose.yml under app.environment"))
	}
}
