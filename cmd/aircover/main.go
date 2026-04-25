package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"
)

var (
	osExit         = os.Exit
	listenAndServe = func(server *http.Server) error {
		return server.ListenAndServe()
	}
)

func main() {
	slog.Info("Starting Air Cover server...")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)

	port := "8080"
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
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("OK"))
	if err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}
