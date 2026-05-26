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

	"air-cover/internal/adapters/inbound/api"
	adaptercasbin "air-cover/internal/adapters/outbound/authorization/casbin"
	adapteremail "air-cover/internal/adapters/outbound/email"
	adapterspinitron "air-cover/internal/adapters/outbound/spinitron"
	"air-cover/internal/adapters/outbound/sqlite"
	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	"air-cover/internal/app/authorization"
	bootstrapapp "air-cover/internal/app/bootstrap"
	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/platform/config"
	"air-cover/internal/platform/logger"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	osExit         = os.Exit
	listenAndServe = func(server *http.Server) error {
		return server.ListenAndServe()
	}
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
	newSender        func(resendAPIKey, sendGridAPIKey, fromEmail string) adapteremail.Sender
	newSpinitron     func(apiKey, baseURL string) adapterspinitron.PageClient
	newCatalog       func(adapterspinitron.PageClient) catalog
	newRouter        func(*api.Server, *api.AuthHandler, *config.Config) chi.Router
	listenAndServe   func(*http.Server) error
	backgroundCtx    func() context.Context
	newAuthorizer    func(context.Context, *sql.DB) (authorization.Authorizer, error)
	newAuthHandler   func(*authapp.Service) *api.AuthHandler
	newAPIServer     func(*subrequestsapp.Service, *adminapp.Service, *api.AuthHandler) *api.Server
	newDBRepository  func(*sql.DB) repositories
	newAsyncNotifier func(subrequestsapp.Notifier) asyncNotifier
	setDefaultLogger func(*config.Config)
	signalContext    func(context.Context) (context.Context, context.CancelFunc)
	shutdownServer   func(*http.Server, context.Context) error
	shutdownTimeout  time.Duration
}

type repositories struct {
	startup     bootstrapapp.Repository
	auth        authapp.Repository
	subRequests subrequestsapp.Repository
	activeUsers subrequestsapp.ActiveUserLister
	admin       adminapp.Repository
}

type infrastructure struct {
	database     *sql.DB
	repositories repositories
	catalog      catalog
	sender       adapteremail.Sender
	notifier     asyncNotifier
	authorizer   authorization.Authorizer
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

type asyncNotifier interface {
	subrequestsapp.Notifier
	Start()
	Stop()
}

func defaultServerDeps() serverDeps {
	return serverDeps{
		loadConfig: config.Load,
		initDB:     sqlite.InitDB,
		newSender: func(resendAPIKey, sendGridAPIKey, fromEmail string) adapteremail.Sender {
			return adapteremail.NewSender(resendAPIKey, sendGridAPIKey, fromEmail)
		},
		newSpinitron: func(apiKey, baseURL string) adapterspinitron.PageClient {
			return adapterspinitron.NewClient(apiKey, baseURL)
		},
		newCatalog: func(source adapterspinitron.PageClient) catalog { return adapterspinitron.NewCatalog(source) },
		newRouter: func(apiServer *api.Server, authHandler *api.AuthHandler, cfg *config.Config) chi.Router {
			return api.NewRouter(apiServer, authHandler, cfg.TrustedProxies)
		},
		listenAndServe: listenAndServe,
		backgroundCtx:  context.Background,
		newAuthorizer: func(ctx context.Context, database *sql.DB) (authorization.Authorizer, error) {
			return adaptercasbin.NewAuthorizer(ctx, database)
		},
		newAuthHandler: func(service *authapp.Service) *api.AuthHandler {
			return api.NewAuthHandler(service)
		},
		newAPIServer: func(subRequests *subrequestsapp.Service, admin *adminapp.Service, authHandler *api.AuthHandler) *api.Server {
			return api.NewServer(authHandler, subRequests, admin)
		},
		newDBRepository: func(database *sql.DB) repositories {
			repo := sqlite.NewRepository(database)
			return repositories{
				startup:     repo,
				auth:        repo,
				subRequests: repo,
				activeUsers: repo,
				admin:       repo,
			}
		},
		newAsyncNotifier: func(notifier subrequestsapp.Notifier) asyncNotifier {
			return adapteremail.NewAsyncNotifier(notifier, 0)
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
	if infra.database != nil {
		defer infra.database.Close()
	}
	if infra.notifier != nil {
		infra.notifier.Start()
		defer infra.notifier.Stop()
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
	sender := deps.newSender(cfg.ResendAPIKey, cfg.SendGridAPIKey, cfg.FromEmail)
	spinitronClient := deps.newSpinitron("", cfg.SpinitronAPIURL)
	authorizer, err := deps.newAuthorizer(context.Background(), database)
	if err != nil {
		return infrastructure{}, fmt.Errorf("failed to initialize authorization: %w", err)
	}
	notifier := deps.newAsyncNotifier(&adapteremail.SubRequestNotifier{
		Users:   repo.activeUsers,
		Sender:  sender,
		BaseURL: cfg.AppBaseURL,
	})
	return infrastructure{
		database:     database,
		repositories: repo,
		catalog:      deps.newCatalog(spinitronClient),
		sender:       sender,
		notifier:     notifier,
		authorizer:   authorizer,
	}, nil
}

func buildApplicationServices(infra infrastructure) applicationServices {
	subRequests := subrequestsapp.NewService(infra.repositories.subRequests, infra.catalog)
	subRequests.SetNotifier(infra.notifier)
	subRequests.SetAuthorizer(infra.authorizer)
	admin := adminapp.NewService(infra.repositories.admin, infra.catalog)
	admin.SetAuthorizer(infra.authorizer)
	return applicationServices{
		auth:        authapp.NewService(infra.repositories.auth, infra.sender),
		subRequests: subRequests,
		admin:       admin,
	}
}

func buildHTTPServer(cfg *config.Config, services applicationServices, deps serverDeps) *http.Server {
	authHandler := deps.newAuthHandler(services.auth)
	apiServer := deps.newAPIServer(services.subRequests, services.admin, authHandler)
	r := deps.newRouter(apiServer, authHandler, cfg)

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
