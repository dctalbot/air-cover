package cmd

import (
	"database/sql"
	"log/slog"

	"github.com/spf13/cobra"

	"air-cover/internal/config"
	"air-cover/internal/db"
	"air-cover/internal/logger"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Manage database migrations",
	Long:  `Manage database migrations (up, down, reset, status).`,
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Run migrations up",
	Run: func(cmd *cobra.Command, args []string) {
		runMigrate(cmd, "up")
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Rollback a single migration",
	Run: func(cmd *cobra.Command, args []string) {
		runMigrate(cmd, "down")
	},
}

var migrateResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Rollback all migrations",
	Run: func(cmd *cobra.Command, args []string) {
		runMigrate(cmd, "reset")
	},
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status",
	Run: func(cmd *cobra.Command, args []string) {
		runMigrate(cmd, "status")
	},
}

func runMigrate(cmd *cobra.Command, action string) {
	cfg, err := config.Load(cmd)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		osExit(1)
	}

	slog.SetDefault(logger.NewLogger(cfg))

	database, err := sql.Open("libsql", cfg.DBURI)
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		osExit(1)
	}
	defer database.Close()

	if err := db.RunMigration(database, action); err != nil {
		slog.Error("Migration failed", "action", action, "error", err)
		osExit(1)
	}

	slog.Info("Migration completed successfully", "action", action)
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	migrateCmd.AddCommand(migrateResetCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
}
