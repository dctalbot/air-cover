package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config holds the application configuration.
type Config struct {
	Port            int    `validate:"gte=1,lte=65535"`
	DBURI           string `mapstructure:"db_uri" validate:"required"`
	ENV             string `validate:"oneof=production development test"`
	MasterEmail     string `validate:"required,email"`
	ResendAPIKey    string `validate:"omitempty"`
	SendGridAPIKey  string `validate:"omitempty"`
	FromEmail       string `mapstructure:"from_email" validate:"required,email"`
	AppBaseURL      string `mapstructure:"app_base_url" validate:"required,url"`
	SpinitronAPIURL string `mapstructure:"spinitron_api_url" validate:"required,url"`
}

var viperBindPFlags = viper.BindPFlags

// Load reads environment variables and validates the configuration.
func Load(cmd *cobra.Command) (*Config, error) {
	// Load .env file if exists
	if err := godotenv.Load(); err != nil {
		// No .env in prod
		slog.Warn("No .env file found")
	}

	cfg := &Config{}
	osEnvPort := os.Getenv("PORT")                       // nolint:forbidigo
	cfg.DBURI = os.Getenv("DB_URI")                      // nolint:forbidigo
	cfg.ENV = os.Getenv("ENV")                           // nolint:forbidigo
	cfg.MasterEmail = os.Getenv("MASTER_EMAIL")          // nolint:forbidigo
	cfg.ResendAPIKey = os.Getenv("RESEND_API_KEY")       // nolint:forbidigo
	cfg.SendGridAPIKey = os.Getenv("SENDGRID_API_KEY")   // nolint:forbidigo
	cfg.FromEmail = os.Getenv("FROM_EMAIL")              // nolint:forbidigo
	cfg.AppBaseURL = os.Getenv("APP_BASE_URL")           // nolint:forbidigo
	cfg.SpinitronAPIURL = os.Getenv("SPINITRON_API_URL") // nolint:forbidigo

	if osEnvPort != "" {
		p, err := strconv.Atoi(osEnvPort)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT: %w", err)
		}
		cfg.Port = p
	}

	// Default values
	if cfg.ENV == "" {
		cfg.ENV = "production"
	}

	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.AppBaseURL == "" {
		cfg.AppBaseURL = fmt.Sprintf("http://localhost:%d", cfg.Port)
	}

	if cmd != nil {
		// Bind flags to viper
		if err := viperBindPFlags(cmd.Flags()); err != nil {
			return nil, fmt.Errorf("error binding flags: %w", err)
		}

		// Flags override config if needed
		if cmd.Flags().Changed("port") {
			cfg.Port = viper.GetInt("port")
		}
		if cmd.Flags().Changed("db-uri") {
			cfg.DBURI = viper.GetString("db-uri")
		}
	}

	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}
