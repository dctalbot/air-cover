package cmd

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"air-cover/internal/api"
	"air-cover/internal/config"
	"air-cover/internal/db"
	"air-cover/internal/email"
)

var (
	osExit         = os.Exit
	listenAndServe = func(server *http.Server) error {
		return server.ListenAndServe()
	}
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the web server",
	Long:  `Start the Air Cover web server.`,
	Run: func(cmd *cobra.Command, args []string) {
		slog.Info("Starting Air Cover server...")

		cfg, err := config.Load(cmd)
		if err != nil {
			slog.Error("Failed to load configuration", "error", err)
			osExit(1)
		}

		database, err := db.InitDB(cfg.DBURI)
		if err != nil {
			slog.Error("Failed to initialize database", "error", err)
			osExit(1)
		}
		repo := db.NewRepository(database)

		if cfg.MasterEmail != "" {
			_, err := repo.GetUserByEmail(context.Background(), cfg.MasterEmail)
			if errors.Is(err, db.ErrNotFound) {
				slog.Info("Creating master admin user", "email", cfg.MasterEmail)
				_, err = repo.CreateUser(context.Background(), cfg.MasterEmail)
				if err != nil {
					slog.Error("Failed to create master user", "error", err)
					osExit(1)
				}
			} else if err != nil {
				slog.Error("Failed to check master user", "error", err)
				osExit(1)
			}
		}

		sender := email.NewSender(cfg.SendGridAPIKey, cfg.ENV)
		authHandler := api.NewAuthHandler(repo, sender)

		mux := http.NewServeMux()
		mux.HandleFunc("/", indexHandler)
		mux.HandleFunc("/health", healthHandler)
		mux.HandleFunc("/auth/login", authHandler.HandleLogin)
		mux.HandleFunc("/auth/verify", authHandler.HandleVerify)

		portStr := strconv.Itoa(cfg.Port)
		slog.Info("Listening on port", "port", portStr)

		server := &http.Server{
			Addr:              ":" + portStr,
			Handler:           mux,
			ReadHeaderTimeout: 3 * time.Second,
		}

		if err := listenAndServe(server); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server failed to start", "error", err)
			osExit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	_ = viper.BindPFlag("port", serverCmd.Flags().Lookup("port"))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("OK"))
	if err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Air Cover</title>
</head>
<body>
    <h1>Welcome to Air Cover</h1>
    <p>Air Cover web server is running.</p>
</body>
</html>`))
	if err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}
