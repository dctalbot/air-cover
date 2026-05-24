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
	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	bootstrapapp "air-cover/internal/app/bootstrap"
	subrequestsapp "air-cover/internal/app/subrequests"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	httpadapter "air-cover/internal/adapters/http"
	"air-cover/internal/config"
	"air-cover/internal/logger"
)

var (
	osExit         = os.Exit
	listenAndServe = func(server *http.Server) error {
		return server.ListenAndServe()
	}
	getSwagger = httpadapter.GetSwagger
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
	newCatalog       func(adapterspinitron.PageClient) catalog
	newRouter        func(*httpadapter.Server, *httpadapter.AuthHandler) chi.Router
	listenAndServe   func(*http.Server) error
	backgroundCtx    func() context.Context
	newAuthHandler   func(*authapp.Service) *httpadapter.AuthHandler
	newAPIServer     func(*subrequestsapp.Service, *adminapp.Service, *httpadapter.AuthHandler) *httpadapter.Server
	newDBRepository  func(*sql.DB) repositories
	setDefaultLogger func(*config.Config)
	signalContext    func(context.Context) (context.Context, context.CancelFunc)
	shutdownServer   func(*http.Server, context.Context) error
	shutdownTimeout  time.Duration
}

type repositories struct {
	startup     bootstrapapp.Repository
	auth        authapp.Repository
	subRequests subrequestsapp.Repository
	admin       adminapp.Repository
}

type infrastructure struct {
	repositories repositories
	catalog      catalog
	sender       authapp.Sender
}

type applicationServices struct {
	auth        *authapp.Service
	subRequests *subrequestsapp.Service
	admin       *adminapp.Service
}

type catalog interface {
	adminapp.Catalog
	subrequestsapp.Catalog
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
		newCatalog:     func(source adapterspinitron.PageClient) catalog { return adapterspinitron.NewCatalog(source) },
		newRouter:      newRouter,
		listenAndServe: listenAndServe,
		backgroundCtx:  context.Background,
		newAuthHandler: func(service *authapp.Service) *httpadapter.AuthHandler {
			return httpadapter.NewAuthHandler(service)
		},
		newAPIServer: func(subRequests *subrequestsapp.Service, admin *adminapp.Service, authHandler *httpadapter.AuthHandler) *httpadapter.Server {
			return httpadapter.NewServer(authHandler, subRequests, admin)
		},
		newDBRepository: func(database *sql.DB) repositories {
			repo := sqlite.NewRepository(database)
			return repositories{
				startup:     repo,
				auth:        repo,
				subRequests: repo,
				admin:       repo,
			}
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

	infra, err := buildInfrastructure(cfg, deps)
	if err != nil {
		return err
	}

	if err := bootstrapapp.NewService(infra.repositories.startup).EnsureMasterUser(deps.backgroundCtx(), cfg.MasterEmail); err != nil {
		return err
	}

	services := buildApplicationServices(infra)
	startCatalogPrefetch(infra.catalog, deps.backgroundCtx)
	server := buildHTTPServer(cfg, services, deps)

	if err := serveWithGracefulShutdown(server, deps); err != nil {
		return fmt.Errorf("server failed to start: %w", err)
	}
	return nil
}

func buildInfrastructure(cfg *config.Config, deps serverDeps) (infrastructure, error) {
	database, err := deps.initDB(cfg.DBURI)
	if err != nil {
		return infrastructure{}, fmt.Errorf("failed to initialize database: %w", err)
	}
	repo := deps.newDBRepository(database)
	spinitronClient := deps.newSpinitron("", cfg.SpinitronAPIURL)
	return infrastructure{
		repositories: repo,
		catalog:      deps.newCatalog(spinitronClient),
		sender:       deps.newSender(cfg.SendGridAPIKey, cfg.FromEmail, cfg.ENV),
	}, nil
}

func buildApplicationServices(infra infrastructure) applicationServices {
	return applicationServices{
		auth:        authapp.NewService(infra.repositories.auth, infra.sender),
		subRequests: subrequestsapp.NewService(infra.repositories.subRequests, infra.catalog),
		admin:       adminapp.NewService(infra.repositories.admin, infra.catalog),
	}
}

func buildHTTPServer(cfg *config.Config, services applicationServices, deps serverDeps) *http.Server {
	authHandler := deps.newAuthHandler(services.auth)
	apiServer := deps.newAPIServer(services.subRequests, services.admin, authHandler)
	r := deps.newRouter(apiServer, authHandler)

	portStr := strconv.Itoa(cfg.Port)
	slog.Info("Listening on port", "port", portStr)
	return &http.Server{
		Addr:              ":" + portStr,
		Handler:           r,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}
}

func startCatalogPrefetch(catalog catalog, backgroundCtx func() context.Context) {
	prefetcher, ok := catalog.(interface{ Prefetch(context.Context) error })
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(backgroundCtx(), 5*time.Second)
		defer cancel()
		if err := prefetcher.Prefetch(ctx); err != nil {
			slog.Warn("Failed to prefetch spinitron catalog", "error", err)
		}
	}()
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
