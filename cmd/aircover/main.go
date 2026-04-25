package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	slog.Info("Starting Air Cover server...")

	// Basic health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("OK"))
		if err != nil {
			slog.Error("Failed to write response", "error", err)
		}
	})

	port := "8080"
	slog.Info("Listening on port", "port", port)

	server := &http.Server{
		Addr:              ":" + port,
		ReadHeaderTimeout: 3 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		slog.Error("Server failed to start", "error", err)
		os.Exit(1)
	}
}
