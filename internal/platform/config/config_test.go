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
	os.Unsetenv("ENV")
	os.Setenv("MASTER_EMAIL", "admin@example.com")
	defer os.Unsetenv("MASTER_EMAIL")
	os.Setenv("RESEND_API_KEY", "resend-key")
	defer os.Unsetenv("RESEND_API_KEY")
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
	if cfg.ENV != "production" {
		t.Errorf("expected env to default to production, got %s", cfg.ENV)
	}
	if cfg.MasterEmail != "admin@example.com" {
		t.Errorf("expected master email to be admin@example.com, got %s", cfg.MasterEmail)
	}
	if cfg.ResendAPIKey != "resend-key" {
		t.Errorf("expected resend API key to be loaded, got %s", cfg.ResendAPIKey)
	}
	if cfg.SpinitronAPIURL != "https://proxy.example.test/api" {
		t.Errorf("expected SPINITRON_API_URL to be loaded, got %s", cfg.SpinitronAPIURL)
	}
	if cfg.AppBaseURL != "http://localhost:8080" {
		t.Errorf("expected APP_BASE_URL to default to localhost, got %s", cfg.AppBaseURL)
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
	os.Setenv("MASTER_EMAIL", "admin@example.com")
	defer os.Unsetenv("MASTER_EMAIL")
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
	os.Setenv("MASTER_EMAIL", "admin@example.com")
	defer os.Unsetenv("MASTER_EMAIL")
	os.Setenv("FROM_EMAIL", "noreply@example.com")
	defer os.Unsetenv("FROM_EMAIL")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("expected port to be 9090, got %d", cfg.Port)
	}
	if cfg.AppBaseURL != "http://localhost:9090" {
		t.Errorf("expected app base URL to use configured port, got %s", cfg.AppBaseURL)
	}
}

func TestLoad_AppBaseURLFromEnv(t *testing.T) {
	viper.Reset()
	os.Setenv("DB_URI", "postgres://localhost/db")
	defer os.Unsetenv("DB_URI")
	os.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	defer os.Unsetenv("SPINITRON_API_URL")
	os.Setenv("MASTER_EMAIL", "admin@example.com")
	defer os.Unsetenv("MASTER_EMAIL")
	os.Setenv("FROM_EMAIL", "noreply@example.com")
	defer os.Unsetenv("FROM_EMAIL")
	os.Setenv("APP_BASE_URL", "https://aircover.example.com")
	defer os.Unsetenv("APP_BASE_URL")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.AppBaseURL != "https://aircover.example.com" {
		t.Errorf("expected configured app base URL, got %s", cfg.AppBaseURL)
	}
}

func TestLoad_TrustedProxiesFromEnv(t *testing.T) {
	viper.Reset()
	os.Setenv("DB_URI", "postgres://localhost/db")
	defer os.Unsetenv("DB_URI")
	os.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	defer os.Unsetenv("SPINITRON_API_URL")
	os.Setenv("MASTER_EMAIL", "admin@example.com")
	defer os.Unsetenv("MASTER_EMAIL")
	os.Setenv("FROM_EMAIL", "noreply@example.com")
	defer os.Unsetenv("FROM_EMAIL")
	os.Setenv("TRUSTED_PROXIES", "127.0.0.1, 10.0.0.0/8")
	defer os.Unsetenv("TRUSTED_PROXIES")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got := len(cfg.TrustedProxies); got != 2 {
		t.Fatalf("expected 2 trusted proxies, got %d", got)
	}
	if cfg.TrustedProxies[0].String() != "127.0.0.1/32" {
		t.Errorf("expected host proxy to become /32 prefix, got %s", cfg.TrustedProxies[0])
	}
	if cfg.TrustedProxies[1].String() != "10.0.0.0/8" {
		t.Errorf("expected CIDR proxy to be preserved, got %s", cfg.TrustedProxies[1])
	}
}

func TestLoad_TrustedProxiesSkipsEmptyEntries(t *testing.T) {
	viper.Reset()
	os.Setenv("DB_URI", "postgres://localhost/db")
	defer os.Unsetenv("DB_URI")
	os.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	defer os.Unsetenv("SPINITRON_API_URL")
	os.Setenv("MASTER_EMAIL", "admin@example.com")
	defer os.Unsetenv("MASTER_EMAIL")
	os.Setenv("FROM_EMAIL", "noreply@example.com")
	defer os.Unsetenv("FROM_EMAIL")
	os.Setenv("TRUSTED_PROXIES", "127.0.0.1, ,")
	defer os.Unsetenv("TRUSTED_PROXIES")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got := len(cfg.TrustedProxies); got != 1 {
		t.Fatalf("expected 1 trusted proxy, got %d", got)
	}
}

func TestLoad_InvalidTrustedProxies(t *testing.T) {
	viper.Reset()
	os.Setenv("DB_URI", "postgres://localhost/db")
	defer os.Unsetenv("DB_URI")
	os.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	defer os.Unsetenv("SPINITRON_API_URL")
	os.Setenv("MASTER_EMAIL", "admin@example.com")
	defer os.Unsetenv("MASTER_EMAIL")
	os.Setenv("FROM_EMAIL", "noreply@example.com")
	defer os.Unsetenv("FROM_EMAIL")
	os.Setenv("TRUSTED_PROXIES", "not-an-ip")
	defer os.Unsetenv("TRUSTED_PROXIES")

	_, err := Load(nil)
	if err == nil {
		t.Fatalf("expected error for invalid TRUSTED_PROXIES, got nil")
	}
}

func TestLoad_EnvFromEnv(t *testing.T) {
	viper.Reset()
	os.Setenv("DB_URI", "postgres://localhost/db")
	defer os.Unsetenv("DB_URI")
	os.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	defer os.Unsetenv("SPINITRON_API_URL")
	os.Setenv("ENV", "development")
	defer os.Unsetenv("ENV")
	os.Setenv("MASTER_EMAIL", "admin@example.com")
	defer os.Unsetenv("MASTER_EMAIL")
	os.Setenv("FROM_EMAIL", "noreply@example.com")
	defer os.Unsetenv("FROM_EMAIL")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.ENV != "development" {
		t.Errorf("expected env to be development, got %s", cfg.ENV)
	}
}

func TestLoad_ValidationError(t *testing.T) {
	viper.Reset()
	os.Unsetenv("DB_URI")
	os.Unsetenv("SPINITRON_API_URL")
	os.Unsetenv("MASTER_EMAIL")

	_, err := Load(nil)
	if err == nil {
		t.Fatalf("expected validation error for missing required env vars, got nil")
	}
}

func TestLoad_MissingMasterEmail(t *testing.T) {
	viper.Reset()
	os.Setenv("DB_URI", "postgres://localhost/db")
	defer os.Unsetenv("DB_URI")
	os.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	defer os.Unsetenv("SPINITRON_API_URL")
	os.Unsetenv("MASTER_EMAIL")
	os.Setenv("FROM_EMAIL", "noreply@example.com")
	defer os.Unsetenv("FROM_EMAIL")

	_, err := Load(nil)
	if err == nil {
		t.Fatalf("expected validation error for missing MASTER_EMAIL, got nil")
	}
}

func TestLoad_WithCmd(t *testing.T) {
	viper.Reset()
	os.Setenv("DB_URI", "postgres://localhost/db")
	defer os.Unsetenv("DB_URI")
	os.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	defer os.Unsetenv("SPINITRON_API_URL")
	os.Setenv("MASTER_EMAIL", "admin@example.com")
	defer os.Unsetenv("MASTER_EMAIL")
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
	os.Setenv("MASTER_EMAIL", "admin@example.com")
	defer os.Unsetenv("MASTER_EMAIL")
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
