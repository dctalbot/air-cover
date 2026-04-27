package cmd

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"air-cover/internal/api"
	"air-cover/internal/config"
	"air-cover/internal/db"
	"air-cover/internal/email"
	"air-cover/internal/spinitron"
	"air-cover/internal/ui"
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
		spinitronClient := spinitron.NewClient("", cfg.SpinitronAPIURL)

		mux := http.NewServeMux()
		mux.HandleFunc("GET /{$}", indexHandler(repo))
		mux.Handle("GET /app", authHandler.AuthMiddleware(appHandler(spinitronClient)))
		mux.HandleFunc("GET /health", healthHandler)
		mux.HandleFunc("POST /auth/login", authHandler.HandleLogin)
		mux.HandleFunc("GET /auth/verify", authHandler.HandleVerify)

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

func indexHandler(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie("session_id"); err == nil && cookie.Value != "" {
			if _, err := repo.GetSessionByToken(r.Context(), cookie.Value); err == nil {
				http.Redirect(w, r, "/app", http.StatusFound)
				return
			}

			// Clear stale session cookie so users can request a fresh login link.
			http.SetCookie(w, &http.Cookie{
				Name:     "session_id",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}

		ui.RenderUnauthenticated(w)
	}
}

type showsService interface {
	GetShowsPage(ctx context.Context, page int) (spinitron.ShowsPage, error)
}

func appHandler(client showsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var allShows []spinitron.Show
		page := 1

		for page > 0 {
			showsPage, err := client.GetShowsPage(r.Context(), page)
			if err != nil {
				slog.Error("Failed to load shows from spinitron", "error", err)
				http.Error(w, "Unable to load shows", http.StatusBadGateway)
				return
			}

			allShows = append(allShows, showsPage.Items...)

			if showsPage.NextPage != nil {
				page = *showsPage.NextPage
			} else {
				break
			}
		}

		sort.Slice(allShows, func(i, j int) bool {
			return strings.ToLower(allShows[i].Title) < strings.ToLower(allShows[j].Title)
		})

		ui.RenderAuthenticated(w, allShows)
	}
}
