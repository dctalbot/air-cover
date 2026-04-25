package cmd

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

		mux := http.NewServeMux()
		mux.HandleFunc("/health", healthHandler)

		port := viper.GetString("port")
		slog.Info("Listening on port", "port", port)

		server := &http.Server{
			Addr:              ":" + port,
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

	serverCmd.Flags().StringP("port", "p", "8080", "Port to listen on")
	_ = viper.BindPFlag("port", serverCmd.Flags().Lookup("port"))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("OK"))
	if err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}
