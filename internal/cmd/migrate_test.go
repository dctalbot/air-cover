package cmd

import (
	"github.com/spf13/cobra"
	"os"
	"testing"
)

func TestMigrateCmds(t *testing.T) {
	// Setup DB for migration
	f, err := os.CreateTemp("", "testdb-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())
	t.Setenv("DB_URI", "file:"+f.Name())
	t.Setenv("FROM_EMAIL", "test@example.com")
	t.Setenv("SPINITRON_API_URL", "http://example.com")

	tests := []struct {
		name    string
		command *cobra.Command
	}{
		{"status", migrateStatusCmd},
		{"up", migrateUpCmd},
		{"down", migrateDownCmd},
		{"reset", migrateResetCmd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.command.Run(tt.command, nil)
		})
	}
}

func TestMigrateCmds_ConfigError(t *testing.T) {
	originalOsExit := osExit
	defer func() { osExit = originalOsExit }()

	exited := false
	osExit = func(code int) {
		exited = true
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		panic("osExit")
	}

	t.Setenv("DB_URI", "") // Invalid config
	t.Setenv("FROM_EMAIL", "test@example.com")
	t.Setenv("SPINITRON_API_URL", "http://example.com")

	func() {
		defer func() {
			if r := recover(); r != nil && r != "osExit" {
				panic(r)
			}
		}()
		migrateStatusCmd.Run(migrateStatusCmd, nil)
	}()

	if !exited {
		t.Errorf("expected osExit to be called")
	}
}

func TestMigrateCmds_DBError(t *testing.T) {
	originalOsExit := osExit
	defer func() { osExit = originalOsExit }()

	exited := false
	osExit = func(code int) {
		exited = true
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		panic("osExit")
	}

	t.Setenv("DB_URI", "invalid-uri://")
	t.Setenv("FROM_EMAIL", "test@example.com")
	t.Setenv("SPINITRON_API_URL", "http://example.com")

	func() {
		defer func() {
			if r := recover(); r != nil && r != "osExit" {
				panic(r)
			}
		}()
		migrateStatusCmd.Run(migrateStatusCmd, nil)
	}()

	if !exited {
		t.Errorf("expected osExit to be called")
	}
}

func TestMigrateCmds_MigrationError(t *testing.T) {
	originalOsExit := osExit
	defer func() { osExit = originalOsExit }()

	exited := false
	osExit = func(code int) {
		exited = true
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		panic("osExit")
	}

	f, err := os.CreateTemp("", "testdb-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	t.Setenv("DB_URI", "file:"+f.Name())
	t.Setenv("FROM_EMAIL", "test@example.com")
	t.Setenv("SPINITRON_API_URL", "http://example.com")

	func() {
		defer func() {
			if r := recover(); r != nil && r != "osExit" {
				panic(r)
			}
		}()
		runMigrate(migrateStatusCmd, "invalid")
	}()

	if !exited {
		t.Errorf("expected osExit to be called")
	}
}
