package config

import (
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strconv"
	"strings"

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
	TrustedProxies  []netip.Prefix
}

var viperBindPFlags = viper.BindPFlags

// Load reads environment variables and validates the configuration.
// CLI flags take precedence over environment variables.
// Sensible defaults are applied for missing values.
func Load(cmd *cobra.Command) (*Config, error) {
	if err := godotenv.Load(); err != nil {
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
	trustedProxies := os.Getenv("TRUSTED_PROXIES")       // nolint:forbidigo

	if osEnvPort != "" {
		p, err := strconv.Atoi(osEnvPort)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT: %w", err)
		}
		cfg.Port = p
	}

	if trustedProxies != "" {
		var err error
		cfg.TrustedProxies, err = parseTrustedProxies(trustedProxies)
		if err != nil {
			return nil, err
		}
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

	if cfg.ENV == "" {
		cfg.ENV = "production"
	}

	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.AppBaseURL == "" {
		cfg.AppBaseURL = fmt.Sprintf("http://localhost:%d", cfg.Port)
	}

	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

func parseTrustedProxies(raw string) ([]netip.Prefix, error) {
	parts := strings.Split(raw, ",")
	proxies := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}

		prefix, err := netip.ParsePrefix(value)
		if err == nil {
			proxies = append(proxies, prefix)
			continue
		}

		addr, addrErr := netip.ParseAddr(value)
		if addrErr != nil {
			return nil, fmt.Errorf("invalid TRUSTED_PROXIES entry %q: %w", value, addrErr)
		}
		proxies = append(proxies, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return proxies, nil
}
