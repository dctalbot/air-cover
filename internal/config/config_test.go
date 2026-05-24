package config

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func TestLoad_Success(t *testing.T) {
	viper.Reset()
	os.Setenv("DB_URI", "postgres://localhost/db")
	defer os.Unsetenv("DB_URI")
	os.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	defer os.Unsetenv("SPINITRON_API_URL")
	os.Unsetenv("PORT")
	os.Setenv("FROM_EMAIL", "noreply@example.com")
	defer os.Unsetenv("FROM_EMAIL")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.DBURI != "postgres://localhost/db" {
		t.Errorf("expected db_uri to be postgres://localhost/db, got %s", cfg.DBURI)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected port to be 8080, got %d", cfg.Port)
	}
	if cfg.SpinitronAPIURL != "https://proxy.example.test/api" {
		t.Errorf("expected SPINITRON_API_URL to be loaded, got %s", cfg.SpinitronAPIURL)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	viper.Reset()
	os.Setenv("DB_URI", "postgres://localhost/db")
	defer os.Unsetenv("DB_URI")
	os.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	defer os.Unsetenv("SPINITRON_API_URL")
	os.Setenv("PORT", "invalid")
	defer os.Unsetenv("PORT")
	os.Setenv("FROM_EMAIL", "noreply@example.com")
	defer os.Unsetenv("FROM_EMAIL")

	_, err := Load(nil)
	if err == nil {
		t.Fatalf("expected error for invalid PORT, got nil")
	}
}

func TestLoad_PortFromEnv(t *testing.T) {
	viper.Reset()
	os.Setenv("DB_URI", "postgres://localhost/db")
	defer os.Unsetenv("DB_URI")
	os.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	defer os.Unsetenv("SPINITRON_API_URL")
	os.Setenv("PORT", "9090")
	defer os.Unsetenv("PORT")
	os.Setenv("FROM_EMAIL", "noreply@example.com")
	defer os.Unsetenv("FROM_EMAIL")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("expected port to be 9090, got %d", cfg.Port)
	}
}

func TestLoad_ValidationError(t *testing.T) {
	viper.Reset()
	os.Unsetenv("DB_URI")
	os.Unsetenv("SPINITRON_API_URL")

	_, err := Load(nil)
	if err == nil {
		t.Fatalf("expected validation error for missing required env vars, got nil")
	}
}

func TestLoad_WithCmd(t *testing.T) {
	viper.Reset()
	os.Setenv("DB_URI", "postgres://localhost/db")
	defer os.Unsetenv("DB_URI")
	os.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	defer os.Unsetenv("SPINITRON_API_URL")
	os.Setenv("FROM_EMAIL", "noreply@example.com")
	defer os.Unsetenv("FROM_EMAIL")

	cmd := &cobra.Command{}
	cmd.Flags().Int("port", 8080, "")
	cmd.Flags().String("db-uri", "", "")

	_ = cmd.Flags().Set("port", "9090")
	_ = cmd.Flags().Set("db-uri", "mysql://localhost/db")

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("expected port to be 9090, got %d", cfg.Port)
	}
	if cfg.DBURI != "mysql://localhost/db" {
		t.Errorf("expected db_uri to be mysql, got %s", cfg.DBURI)
	}
}

func TestLoad_BindFlagsError(t *testing.T) {
	viper.Reset()
	os.Setenv("DB_URI", "postgres://localhost/db")
	defer os.Unsetenv("DB_URI")
	os.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	defer os.Unsetenv("SPINITRON_API_URL")
	os.Setenv("FROM_EMAIL", "noreply@example.com")
	defer os.Unsetenv("FROM_EMAIL")

	cmd := &cobra.Command{}
	cmd.Flags().Int("port", 8080, "")

	originalViperBindPFlags := viperBindPFlags
	defer func() { viperBindPFlags = originalViperBindPFlags }()
	viperBindPFlags = func(flags *pflag.FlagSet) error {
		return os.ErrInvalid
	}

	_, err := Load(cmd)
	if err == nil {
		t.Fatalf("expected error from BindPFlags, got nil")
	}
}
