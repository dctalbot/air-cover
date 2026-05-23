package cmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	"air-cover/internal/models"
	"air-cover/internal/spinitron"
)

var (
	osExit         = os.Exit
	listenAndServe = func(server *http.Server) error {
		return server.ListenAndServe()
	}
	getSwagger = api.GetSwagger
)

type serverDeps struct {
	loadConfig       func(*cobra.Command) (*config.Config, error)
	initDB           func(string) (*sql.DB, error)
	newSender        func(apiKey, fromEmail, env string) email.Sender
	newSpinitron     func(apiKey, baseURL string) api.ShowsService
	newRouter        func(*api.Server, *api.AuthHandler) chi.Router
	listenAndServe   func(*http.Server) error
	backgroundCtx    func() context.Context
	newAuthHandler   func(apiAuthRepository, email.Sender) *api.AuthHandler
	newAPIServer     func(apiServerRepository, *api.AuthHandler, api.ShowsService) *api.Server
	newDBRepository  func(*sql.DB) repository
	setDefaultLogger func(*config.Config)
}

type repository interface {
	apiAuthRepository
	apiServerRepository
}

type apiAuthRepository interface {
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	GetUserByID(ctx context.Context, id int) (*models.User, error)
	CreateMagicLink(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error
	UseMagicLink(ctx context.Context, tokenHash string) (*models.MagicLink, error)
	CreateSession(ctx context.Context, sessionID, sessionToken string, userID int, expiresAt time.Time) error
	GetSessionByToken(ctx context.Context, sessionToken string) (*models.Session, error)
	DeleteSessionsByUserID(ctx context.Context, userID int) error
}

type apiServerRepository interface {
	GetSessionByToken(ctx context.Context, sessionToken string) (*models.Session, error)
	ListSubRequests(ctx context.Context) ([]*models.SubRequest, error)
	ListUsers(ctx context.Context) ([]*models.User, error)
	CreateUser(ctx context.Context, email string, role string) (*models.User, error)
	CreateSubRequest(ctx context.Context, sr *models.SubRequest) error
	GetSubRequestByID(ctx context.Context, id int) (*models.SubRequest, error)
	DeleteSubRequest(ctx context.Context, id int) error
	TakeSubRequest(ctx context.Context, id int, userID int) error
	UntakeSubRequest(ctx context.Context, id int) error
	UpdateUser(ctx context.Context, id int, role *string, isEnabled *bool) error
	ImportUsers(ctx context.Context, emails []string) error
}

func defaultServerDeps() serverDeps {
	return serverDeps{
		loadConfig: config.Load,
		initDB:     db.InitDB,
		newSender:  email.NewSender,
		newSpinitron: func(apiKey, baseURL string) api.ShowsService {
			return spinitron.NewClient(apiKey, baseURL)
		},
		newRouter:      newRouter,
		listenAndServe: listenAndServe,
		backgroundCtx:  context.Background,
		newAuthHandler: func(repo apiAuthRepository, sender email.Sender) *api.AuthHandler {
			return api.NewAuthHandler(repo, sender)
		},
		newAPIServer: func(repo apiServerRepository, authHandler *api.AuthHandler, spinitronClient api.ShowsService) *api.Server {
			return api.NewServer(repo, authHandler, spinitronClient)
		},
		newDBRepository: func(database *sql.DB) repository {
			return db.NewRepository(database)
		},
		setDefaultLogger: func(cfg *config.Config) {
			slog.SetDefault(logger.NewLogger(cfg))
		},
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
		if errors.Is(err, db.ErrNotFound) {
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
	apiServer := deps.newAPIServer(repo, authHandler, spinitronClient)

	r := deps.newRouter(apiServer, authHandler)
	portStr := strconv.Itoa(cfg.Port)
	slog.Info("Listening on port", "port", portStr)

	server := &http.Server{
		Addr:              ":" + portStr,
		Handler:           r,
		ReadHeaderTimeout: 3 * time.Second,
	}

	if err := deps.listenAndServe(server); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server failed to start: %w", err)
	}
	return nil
}

func init() {
	openapi3filter.RegisterBodyDecoder("application/x-www-form-urlencoded", openapi3filter.UrlencodedBodyDecoder)

	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	_ = viper.BindPFlag("port", serverCmd.Flags().Lookup("port"))
}
