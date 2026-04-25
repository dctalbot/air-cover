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
	os.Setenv("PORT", "8080")
	defer os.Unsetenv("PORT")

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
}

func TestLoad_ValidationError(t *testing.T) {
	viper.Reset()
	os.Unsetenv("DB_URI")

	_, err := Load(nil)
	if err == nil {
		t.Fatalf("expected validation error for missing DB_URI, got nil")
	}
}

func TestLoad_WithCmd(t *testing.T) {
	viper.Reset()
	os.Setenv("DB_URI", "postgres://localhost/db")
	defer os.Unsetenv("DB_URI")

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
