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
	Port            int    `validate:"required,gte=1,lte=65535"`
	DBURI           string `mapstructure:"db_uri" validate:"required"`
	ENV             string `validate:"oneof=production development test"`
	MasterEmail     string `validate:"omitempty,email"`
	SendGridAPIKey  string `validate:"omitempty"`
	FromEmail       string `mapstructure:"from_email" validate:"required,email"`
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
	cfg.SendGridAPIKey = os.Getenv("SENDGRID_API_KEY")   // nolint:forbidigo
	cfg.FromEmail = os.Getenv("FROM_EMAIL")              // nolint:forbidigo
	cfg.SpinitronAPIURL = os.Getenv("SPINITRON_API_URL") // nolint:forbidigo

	if osEnvPort != "" {
		if p, err := strconv.Atoi(osEnvPort); err == nil {
			cfg.Port = p
		}
	}

	// Default values
	if cfg.ENV == "" {
		cfg.ENV = "production"
	}

	if cfg.Port == 0 {
		cfg.Port = 8080
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
