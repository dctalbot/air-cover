package cmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	adapteremail "air-cover/internal/adapters/email"
	adapterspinitron "air-cover/internal/adapters/spinitron"
	"air-cover/internal/adapters/sqlite"
	coreapp "air-cover/internal/app"
	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/apperrors"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	nethttp_middleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"air-cover/internal/api"
	"air-cover/internal/config"
	"air-cover/internal/logger"
)

var (
	osExit         = os.Exit
	listenAndServe = func(server *http.Server) error {
		return server.ListenAndServe()
	}
	getSwagger = api.GetSwagger
)

const (
	serverReadHeaderTimeout = 3 * time.Second
	serverReadTimeout       = 10 * time.Second
	serverWriteTimeout      = 30 * time.Second
	serverIdleTimeout       = 60 * time.Second
	serverShutdownTimeout   = 10 * time.Second
)

type serverDeps struct {
	loadConfig       func(*cobra.Command) (*config.Config, error)
	initDB           func(string) (*sql.DB, error)
	newSender        func(apiKey, fromEmail, env string) authapp.Sender
	newSpinitron     func(apiKey, baseURL string) adapterspinitron.PageClient
	newCatalog       func(adapterspinitron.PageClient) coreapp.Catalog
	newRouter        func(*api.Server, *api.AuthHandler) chi.Router
	listenAndServe   func(*http.Server) error
	backgroundCtx    func() context.Context
	newAuthHandler   func(authapp.Repository, authapp.Sender) *api.AuthHandler
	newAPIServer     func(coreapp.Repository, *api.AuthHandler, coreapp.Catalog) *api.Server
	newDBRepository  func(*sql.DB) repository
	setDefaultLogger func(*config.Config)
	signalContext    func(context.Context) (context.Context, context.CancelFunc)
	shutdownServer   func(*http.Server, context.Context) error
	shutdownTimeout  time.Duration
}

type repository interface {
	coreapp.Repository
}

func defaultServerDeps() serverDeps {
	return serverDeps{
		loadConfig: config.Load,
		initDB:     sqlite.InitDB,
		newSender: func(apiKey, fromEmail, env string) authapp.Sender {
			return adapteremail.NewSender(apiKey, fromEmail, env)
		},
		newSpinitron: func(apiKey, baseURL string) adapterspinitron.PageClient {
			return adapterspinitron.NewClient(apiKey, baseURL)
		},
		newCatalog:     func(source adapterspinitron.PageClient) coreapp.Catalog { return adapterspinitron.NewCatalog(source) },
		newRouter:      newRouter,
		listenAndServe: listenAndServe,
		backgroundCtx:  context.Background,
		newAuthHandler: func(repo authapp.Repository, sender authapp.Sender) *api.AuthHandler {
			return api.NewAuthHandler(repo, sender)
		},
		newAPIServer: func(repo coreapp.Repository, authHandler *api.AuthHandler, catalog coreapp.Catalog) *api.Server {
			return api.NewServer(
				authHandler,
				subrequestsapp.NewService(repo, catalog),
				adminapp.NewService(repo, catalog),
			)
		},
		newDBRepository: func(database *sql.DB) repository {
			return sqlite.NewRepository(database)
		},
		setDefaultLogger: func(cfg *config.Config) {
			slog.SetDefault(logger.NewLogger(cfg))
		},
		signalContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
		},
		shutdownServer: func(server *http.Server, ctx context.Context) error {
			return server.Shutdown(ctx)
		},
		shutdownTimeout: serverShutdownTimeout,
	}
}

func newRouter(apiServer *api.Server, authHandler *api.AuthHandler) chi.Router {
	swagger, err := getSwagger()
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
		if err := runServer(cmd, defaultServerDeps()); err != nil {
			slog.Error(err.Error())
			osExit(1)
		}
	},
}

func runServer(cmd *cobra.Command, deps serverDeps) error {
	cfg, err := deps.loadConfig(cmd)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	deps.setDefaultLogger(cfg)

	database, err := deps.initDB(cfg.DBURI)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	repo := deps.newDBRepository(database)

	if cfg.MasterEmail != "" {
		ctx := deps.backgroundCtx()
		_, err := repo.GetUserByEmail(ctx, cfg.MasterEmail)
		if errors.Is(err, apperrors.ErrNotFound) {
			slog.Info("Creating master admin user", "email", cfg.MasterEmail)
			_, err = repo.CreateUser(ctx, cfg.MasterEmail, "admin")
			if err != nil {
				return fmt.Errorf("failed to create master user: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check master user: %w", err)
		}
	}

	sender := deps.newSender(cfg.SendGridAPIKey, cfg.FromEmail, cfg.ENV)
	authHandler := deps.newAuthHandler(repo, sender)
	spinitronClient := deps.newSpinitron("", cfg.SpinitronAPIURL)
	spinitronCatalog := deps.newCatalog(spinitronClient)
	if prefetcher, ok := spinitronCatalog.(interface{ Prefetch(context.Context) error }); ok {
		go func() {
			ctx, cancel := context.WithTimeout(deps.backgroundCtx(), 5*time.Second)
			defer cancel()
			if err := prefetcher.Prefetch(ctx); err != nil {
				slog.Warn("Failed to prefetch spinitron catalog", "error", err)
			}
		}()
	}
	apiServer := deps.newAPIServer(repo, authHandler, spinitronCatalog)

	r := deps.newRouter(apiServer, authHandler)
	portStr := strconv.Itoa(cfg.Port)
	slog.Info("Listening on port", "port", portStr)

	server := &http.Server{
		Addr:              ":" + portStr,
		Handler:           r,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}

	if err := serveWithGracefulShutdown(server, deps); err != nil {
		return fmt.Errorf("server failed to start: %w", err)
	}
	return nil
}

func serveWithGracefulShutdown(server *http.Server, deps serverDeps) error {
	ctx, stop := deps.signalContext(deps.backgroundCtx())
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- deps.listenAndServe(server)
	}()

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(deps.backgroundCtx(), deps.shutdownTimeout)
	defer cancel()

	if err := deps.shutdownServer(server, shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-shutdownCtx.Done():
		return shutdownCtx.Err()
	}
}

func init() {
	openapi3filter.RegisterBodyDecoder("application/x-www-form-urlencoded", openapi3filter.UrlencodedBodyDecoder)

	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	_ = viper.BindPFlag("port", serverCmd.Flags().Lookup("port"))
}
