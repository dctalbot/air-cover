package cmd

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	nethttp_middleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"air-cover/internal/api"
	"air-cover/internal/config"
	"air-cover/internal/db"
	"air-cover/internal/email"
	"air-cover/internal/logger"
	"air-cover/internal/spinitron"
)

var (
	osExit         = os.Exit
	listenAndServe = func(server *http.Server) error {
		return server.ListenAndServe()
	}
)

func newRouter(apiServer *api.Server, authHandler *api.AuthHandler) chi.Router {
	swagger, err := api.GetSwagger()
	if err != nil {
		slog.Error("Failed to load swagger spec", "error", err)
		osExit(1)
	}

	// Disable server name validation so the validator doesn't
	// reject requests based on the Host header.
	swagger.Servers = nil

	wrapper := api.NewWrapper(apiServer)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(nethttp_middleware.OapiRequestValidator(swagger))

	// Public endpoints
	r.Get("/", wrapper.Get)
	r.Get("/health", wrapper.GetHealth)

	// Auth endpoints with rate limiting
	r.Group(func(r chi.Router) {
		r.Use(httprate.LimitByIP(5, time.Minute))
		r.Post("/auth/login", wrapper.PostAuthLogin)
		r.Get("/auth/verify", wrapper.GetAuthVerify)
	})

	// Authenticated endpoints
	r.Group(func(r chi.Router) {
		r.Use(authHandler.AuthMiddleware)
		r.Get("/app", wrapper.GetApp)
		r.Post("/auth/logout", wrapper.PostAuthLogout)
		r.Post("/sub-requests", wrapper.PostSubRequests)
		r.Delete("/sub-requests/{id}", wrapper.DeleteSubRequestsId)
		r.Patch("/sub-requests/{id}", wrapper.PatchSubRequestsId)
	})

	// Admin endpoints
	r.Group(func(r chi.Router) {
		r.Use(authHandler.AuthMiddleware)
		r.Use(authHandler.RequireAdmin)
		r.Get("/admin", wrapper.GetAdmin)
		r.Post("/users", wrapper.PostUsers)
		r.Post("/users/import/spinitron", wrapper.PostUsersImportSpinitron)
		r.Post("/users/{id}", wrapper.PostUsersId)
	})

	return r
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the web server",
	Long:  `Start the Air Cover web server.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load(cmd)
		if err != nil {
			slog.Error("Failed to load configuration", "error", err)
			osExit(1)
		}

		slog.SetDefault(logger.NewLogger(cfg))

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
				_, err = repo.CreateUser(context.Background(), cfg.MasterEmail, "admin")
				if err != nil {
					slog.Error("Failed to create master user", "error", err)
					osExit(1)
				}
			} else if err != nil {
				slog.Error("Failed to check master user", "error", err)
				osExit(1)
			}
		}

		sender := email.NewSender(cfg.SendGridAPIKey, cfg.FromEmail, cfg.ENV)
		authHandler := api.NewAuthHandler(repo, sender)
		spinitronClient := spinitron.NewClient("", cfg.SpinitronAPIURL)
		apiServer := api.NewServer(repo, authHandler, spinitronClient)

		r := newRouter(apiServer, authHandler)
		portStr := strconv.Itoa(cfg.Port)
		slog.Info("Listening on port", "port", portStr)

		server := &http.Server{
			Addr:              ":" + portStr,
			Handler:           r,
			ReadHeaderTimeout: 3 * time.Second,
		}

		if err := listenAndServe(server); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server failed to start", "error", err)
			osExit(1)
		}
	},
}

func init() {
	openapi3filter.RegisterBodyDecoder("application/x-www-form-urlencoded", openapi3filter.UrlencodedBodyDecoder)

	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	_ = viper.BindPFlag("port", serverCmd.Flags().Lookup("port"))
}
