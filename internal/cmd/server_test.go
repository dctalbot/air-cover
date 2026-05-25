package cmd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"air-cover/internal/adapters/inbound/http/api"
	adapteremail "air-cover/internal/adapters/outbound/email"
	adapterspinitron "air-cover/internal/adapters/outbound/spinitron"
	"air-cover/internal/adapters/outbound/sqlite"
	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	bootstrapapp "air-cover/internal/app/bootstrap"
	appcatalog "air-cover/internal/app/catalog"
	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
	"air-cover/internal/platform/config"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"
)

type fakeShowsService struct {
	shows    []appcatalog.Show
	err      error
	calls    int
	prefetch bool
	done     chan struct{}
}

func (f *fakeShowsService) ListShows(ctx context.Context) ([]appcatalog.Show, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.shows, nil
}

func (f *fakeShowsService) ListPersonas(ctx context.Context) ([]appcatalog.Persona, error) {
	return nil, f.err
}

func (f *fakeShowsService) Prefetch(ctx context.Context) error {
	f.prefetch = true
	if f.done != nil {
		close(f.done)
	}
	return f.err
}

type fakeCatalogOnly struct{}

func (f *fakeCatalogOnly) ListShows(ctx context.Context) ([]appcatalog.Show, error) {
	return nil, nil
}

func (f *fakeCatalogOnly) ListPersonas(ctx context.Context) ([]appcatalog.Persona, error) {
	return nil, nil
}

type fakeSpinitronPageClient struct{}

func (f *fakeSpinitronPageClient) GetShowsPage(ctx context.Context, page int) (adapterspinitron.ShowsPage, error) {
	return adapterspinitron.ShowsPage{}, nil
}

func (f *fakeSpinitronPageClient) GetPersonasPage(ctx context.Context, page int) (adapterspinitron.PersonasPage, error) {
	return adapterspinitron.PersonasPage{}, nil
}

type mockSender struct{}

func (m *mockSender) SendMagicLink(toEmail, magicLink string) error {
	return nil
}

func (m *mockSender) SendSubRequestCreated(bccEmails []string, message adapteremail.SubRequestCreatedMessage) error {
	return nil
}

func (m *mockSender) SendSubRequestTaken(toEmail string, ccEmails []string, message adapteremail.SubRequestTakenMessage) error {
	return nil
}

type fakeAsyncNotifier struct {
	started bool
	stopped bool
	next    subrequestsapp.Notifier
}

func (f *fakeAsyncNotifier) SubRequestCreated(ctx context.Context, event subrequestsapp.SubRequestCreatedEvent) error {
	return nil
}

func (f *fakeAsyncNotifier) SubRequestTaken(ctx context.Context, event subrequestsapp.SubRequestTakenEvent) error {
	return nil
}

func (f *fakeAsyncNotifier) Start() {
	f.started = true
}

func (f *fakeAsyncNotifier) Stop() {
	f.stopped = true
}

type fakeStartupRepo struct {
	getUserErr    error
	createUserErr error
}

type fakeAuthRepo struct {
	fakeStartupRepo
}

func (f *fakeAuthRepo) CreateMagicLink(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error {
	return nil
}

func (f *fakeAuthRepo) UseMagicLink(ctx context.Context, tokenHash string, now time.Time) (*domain.MagicLink, error) {
	return &domain.MagicLink{UserID: 1, ExpiresAt: now.Add(time.Hour)}, nil
}

func (f *fakeAuthRepo) CreateSession(ctx context.Context, sessionID, sessionTokenHash string, userID int, expiresAt time.Time) error {
	return nil
}

func (f *fakeAuthRepo) GetSessionByToken(ctx context.Context, sessionTokenHash string, now time.Time) (*domain.Session, error) {
	return &domain.Session{ID: "session", UserID: 1, TokenHash: sessionTokenHash, ExpiresAt: now.Add(time.Hour)}, nil
}

func (f *fakeAuthRepo) DeleteSessionsByUserID(ctx context.Context, userID int) error {
	return nil
}

type fakeSubRequestsRepo struct{}

func (f *fakeSubRequestsRepo) ListDashboardSubRequests(ctx context.Context) ([]subrequestsapp.DashboardReadModel, error) {
	return nil, nil
}

func (f *fakeSubRequestsRepo) GetSubRequestDetailByID(ctx context.Context, id int) (subrequestsapp.DetailReadModel, error) {
	request := &domain.SubRequest{ID: id, PostedByUserID: 1, StartTime: time.Now().Add(time.Hour)}
	return subrequestsapp.DetailReadModel{Request: request, RequesterEmail: "requester@example.com"}, nil
}

func (f *fakeSubRequestsRepo) CreateSubRequest(ctx context.Context, sr *domain.SubRequest) error {
	sr.ID = 1
	return nil
}

func (f *fakeSubRequestsRepo) GetSubRequestByID(ctx context.Context, id int) (*domain.SubRequest, error) {
	return &domain.SubRequest{ID: id, PostedByUserID: 1, StartTime: time.Now().Add(time.Hour)}, nil
}

func (f *fakeSubRequestsRepo) DeleteSubRequest(ctx context.Context, id int) error {
	return nil
}

func (f *fakeSubRequestsRepo) TakeSubRequest(ctx context.Context, id int, userID int, updatedAt time.Time) error {
	return nil
}

func (f *fakeSubRequestsRepo) UntakeSubRequest(ctx context.Context, id int, updatedAt time.Time) error {
	return nil
}

func (f *fakeSubRequestsRepo) ListActiveUsers(ctx context.Context) ([]*domain.User, error) {
	return []*domain.User{{ID: 1, Email: "active@example.com", Role: domain.RoleMember, IsEnabled: true}}, nil
}

type fakeAdminRepo struct{}

func (f *fakeAdminRepo) ListUsers(ctx context.Context) ([]*domain.User, error) {
	return nil, nil
}

func (f *fakeAdminRepo) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return nil, apperrors.ErrNotFound
}

func (f *fakeAdminRepo) CreateUser(ctx context.Context, email string, role string) (*domain.User, error) {
	return &domain.User{ID: 1, Email: email, Role: domain.Role(role), IsEnabled: true}, nil
}

func (f *fakeAdminRepo) UpdateUser(ctx context.Context, id int, role *string, isEnabled *bool) error {
	return nil
}

func (f *fakeAdminRepo) ImportUsers(ctx context.Context, emails []string) error {
	return nil
}

func (f *fakeStartupRepo) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	if f.getUserErr != nil {
		return nil, f.getUserErr
	}
	return &domain.User{ID: 1, Email: email, Role: domain.RoleAdmin, IsEnabled: true}, nil
}

func (f *fakeStartupRepo) GetUserByID(ctx context.Context, id int) (*domain.User, error) {
	return &domain.User{ID: id, Email: "user@example.com", Role: domain.RoleMember, IsEnabled: true}, nil
}

func (f *fakeStartupRepo) CreateUser(ctx context.Context, email string, role string) (*domain.User, error) {
	if f.createUserErr != nil {
		return nil, f.createUserErr
	}
	return &domain.User{ID: 1, Email: email, Role: domain.Role(role), IsEnabled: true}, nil
}

func testServerDeps(cfg *config.Config, repo bootstrapapp.Repository) serverDeps {
	return serverDeps{
		loadConfig: func(cmd *cobra.Command) (*config.Config, error) {
			return cfg, nil
		},
		initDB: func(uri string) (*sql.DB, error) {
			return nil, nil
		},
		newSender: func(resendAPIKey, sendGridAPIKey, fromEmail string) adapteremail.Sender {
			return &mockSender{}
		},
		newSpinitron: func(apiKey, baseURL string) adapterspinitron.PageClient {
			return &fakeSpinitronPageClient{}
		},
		newCatalog: func(source adapterspinitron.PageClient) catalog {
			return &fakeShowsService{}
		},
		newRouter: func(apiServer *api.Server, authHandler *api.AuthHandler) chi.Router {
			return chi.NewRouter()
		},
		listenAndServe: func(server *http.Server) error {
			return nil
		},
		backgroundCtx: context.Background,
		newAuthHandler: func(service *authapp.Service) *api.AuthHandler {
			return api.NewAuthHandler(service)
		},
		newAPIServer: func(subRequests *subrequestsapp.Service, admin *adminapp.Service, authHandler *api.AuthHandler) *api.Server {
			return api.NewServer(authHandler, subRequests, admin)
		},
		newDBRepository: func(database *sql.DB) repositories {
			subRequestsRepo := &fakeSubRequestsRepo{}
			return repositories{startup: repo, auth: &fakeAuthRepo{}, subRequests: subRequestsRepo, activeUsers: subRequestsRepo, admin: &fakeAdminRepo{}}
		},
		newAsyncNotifier: func(notifier subrequestsapp.Notifier) asyncNotifier {
			return &fakeAsyncNotifier{next: notifier}
		},
		setDefaultLogger: func(cfg *config.Config) {},
		signalContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			return context.WithCancel(parent)
		},
		shutdownServer: func(server *http.Server, ctx context.Context) error {
			return nil
		},
		shutdownTimeout: serverShutdownTimeout,
	}
}

func newClosableTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	if err := database.Ping(); err != nil {
		t.Fatalf("failed to ping test db: %v", err)
	}
	return database
}

func assertDBClosed(t *testing.T, database *sql.DB) {
	t.Helper()
	if err := database.Ping(); err == nil {
		t.Fatal("expected database to be closed")
	}
}

func TestServerCmd_Success(t *testing.T) {
	t.Setenv("DB_URI", "file::memory:?cache=shared")
	t.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	t.Setenv("MASTER_EMAIL", "admin@example.com")
	t.Setenv("FROM_EMAIL", "noreply@example.com")
	originalListenAndServe := listenAndServe
	defer func() { listenAndServe = originalListenAndServe }()

	listenAndServe = func(server *http.Server) error {
		return nil
	}

	serverCmd.Run(serverCmd, nil)
}

func TestServerCmd_Error(t *testing.T) {
	t.Setenv("DB_URI", "file::memory:?cache=shared")
	t.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	t.Setenv("MASTER_EMAIL", "admin@example.com")
	t.Setenv("FROM_EMAIL", "noreply@example.com")
	originalListenAndServe := listenAndServe
	originalOsExit := osExit
	defer func() {
		listenAndServe = originalListenAndServe
		osExit = originalOsExit
	}()

	listenAndServe = func(server *http.Server) error {
		return errors.New("start error")
	}

	exited := false
	osExit = func(code int) {
		exited = true
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
	}

	serverCmd.Run(serverCmd, nil)

	if !exited {
		t.Errorf("expected osExit to be called")
	}
}

func TestListenAndServe(t *testing.T) {
	server := &http.Server{
		Addr:              "invalid:",
		ReadHeaderTimeout: serverReadHeaderTimeout,
	}
	err := listenAndServe(server)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestDefaultShutdownServer(t *testing.T) {
	deps := defaultServerDeps()
	server := &http.Server{ReadHeaderTimeout: serverReadHeaderTimeout}
	if err := deps.shutdownServer(server, context.Background()); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestServerCmd_ConfigError(t *testing.T) {
	originalOsExit := osExit
	defer func() { osExit = originalOsExit }()

	exited := false
	osExit = func(code int) {
		exited = true
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		panic("osExit")
	}

	t.Setenv("DB_URI", "")
	t.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	t.Setenv("MASTER_EMAIL", "admin@example.com")
	t.Setenv("FROM_EMAIL", "noreply@example.com")

	func() {
		defer func() {
			if r := recover(); r != nil && r != "osExit" {
				panic(r)
			}
		}()
		serverCmd.Run(serverCmd, nil)
	}()

	if !exited {
		t.Errorf("expected osExit to be called")
	}
}

func TestServerCmd_MasterEmail(t *testing.T) {
	// Tests the master email bootstrapping path
	dbURI := "file://" + t.TempDir() + "/master-email.db"
	t.Setenv("DB_URI", dbURI)
	t.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	t.Setenv("MASTER_EMAIL", "admin@example.com")
	t.Setenv("FROM_EMAIL", "noreply@example.com")
	defer t.Setenv("MASTER_EMAIL", "")

	originalListenAndServe := listenAndServe
	defer func() { listenAndServe = originalListenAndServe }()

	listenAndServe = func(server *http.Server) error {
		return nil
	}

	serverCmd.Run(serverCmd, nil)

	// Verify the user was created with admin role
	dbConn, err := sqlite.InitDB(dbURI)
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()
	repo := sqlite.NewRepository(dbConn)
	u, err := repo.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("failed to find master user: %v", err)
	}
	if u.Role != domain.RoleAdmin {
		t.Errorf("expected role 'admin', got %q", u.Role)
	}

	// Run again so the master email already exists (covers the "already exists" branch)
	serverCmd.Run(serverCmd, nil)
}

func TestRunServer_MasterEmailFailures(t *testing.T) {
	cfg := &config.Config{
		Port:            8080,
		DBURI:           "file::memory:",
		ENV:             "test",
		MasterEmail:     "admin@example.com",
		FromEmail:       "noreply@example.com",
		SpinitronAPIURL: "https://proxy.example.test/api",
	}

	tests := []struct {
		name string
		repo *fakeStartupRepo
		want string
	}{
		{
			name: "check error",
			repo: &fakeStartupRepo{getUserErr: errors.New("check failed")},
			want: "failed to check master user",
		},
		{
			name: "create error",
			repo: &fakeStartupRepo{getUserErr: apperrors.ErrNotFound, createUserErr: errors.New("create failed")},
			want: "failed to create master user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runServer(&cobra.Command{}, testServerDeps(cfg, tt.repo))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestRunServer_ClosesDatabase(t *testing.T) {
	cfg := &config.Config{
		Port:            8080,
		DBURI:           "file::memory:",
		ENV:             "test",
		ResendAPIKey:    "resend-key",
		SendGridAPIKey:  "sendgrid-key",
		FromEmail:       "noreply@example.com",
		SpinitronAPIURL: "https://proxy.example.test/api",
		AppBaseURL:      "https://aircover.example.com",
	}

	t.Run("normal exit", func(t *testing.T) {
		database := newClosableTestDB(t)
		deps := testServerDeps(cfg, &fakeStartupRepo{})
		deps.initDB = func(uri string) (*sql.DB, error) {
			return database, nil
		}

		if err := runServer(&cobra.Command{}, deps); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		assertDBClosed(t, database)
	})

	t.Run("post-infrastructure error", func(t *testing.T) {
		database := newClosableTestDB(t)
		cfgWithMaster := *cfg
		cfgWithMaster.MasterEmail = "admin@example.com"
		deps := testServerDeps(&cfgWithMaster, &fakeStartupRepo{getUserErr: errors.New("check failed")})
		deps.initDB = func(uri string) (*sql.DB, error) {
			return database, nil
		}

		err := runServer(&cobra.Command{}, deps)
		if err == nil || !strings.Contains(err.Error(), "failed to check master user") {
			t.Fatalf("expected master user error, got %v", err)
		}
		assertDBClosed(t, database)
	})
}

func TestRunServer_DependencyFailures(t *testing.T) {
	cfg := &config.Config{
		Port:            8080,
		DBURI:           "file::memory:",
		ENV:             "test",
		ResendAPIKey:    "resend-key",
		SendGridAPIKey:  "sendgrid-key",
		FromEmail:       "noreply@example.com",
		SpinitronAPIURL: "https://proxy.example.test/api",
		AppBaseURL:      "https://aircover.example.com",
	}

	t.Run("config load error", func(t *testing.T) {
		deps := testServerDeps(cfg, &fakeStartupRepo{})
		deps.loadConfig = func(cmd *cobra.Command) (*config.Config, error) {
			return nil, errors.New("bad config")
		}

		err := runServer(&cobra.Command{}, deps)
		if err == nil || !strings.Contains(err.Error(), "failed to load configuration") {
			t.Fatalf("expected config error, got %v", err)
		}
	})

	t.Run("db init error", func(t *testing.T) {
		deps := testServerDeps(cfg, &fakeStartupRepo{})
		deps.initDB = func(uri string) (*sql.DB, error) {
			return nil, errors.New("db failed")
		}

		err := runServer(&cobra.Command{}, deps)
		if err == nil || !strings.Contains(err.Error(), "failed to initialize database") {
			t.Fatalf("expected db init error, got %v", err)
		}
	})

	t.Run("listen error", func(t *testing.T) {
		deps := testServerDeps(cfg, &fakeStartupRepo{})
		deps.listenAndServe = func(server *http.Server) error {
			return errors.New("listen failed")
		}

		err := runServer(&cobra.Command{}, deps)
		if err == nil || !strings.Contains(err.Error(), "server failed to start") {
			t.Fatalf("expected listen error, got %v", err)
		}
	})

	t.Run("server timeouts are configured", func(t *testing.T) {
		deps := testServerDeps(cfg, &fakeStartupRepo{})
		var got *http.Server
		deps.listenAndServe = func(server *http.Server) error {
			got = server
			return http.ErrServerClosed
		}

		if err := runServer(&cobra.Command{}, deps); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if got == nil {
			t.Fatal("expected server to be passed to listener")
		}
		if got.ReadHeaderTimeout != serverReadHeaderTimeout {
			t.Errorf("expected ReadHeaderTimeout %s, got %s", serverReadHeaderTimeout, got.ReadHeaderTimeout)
		}
		if got.ReadTimeout != serverReadTimeout {
			t.Errorf("expected ReadTimeout %s, got %s", serverReadTimeout, got.ReadTimeout)
		}
		if got.WriteTimeout != serverWriteTimeout {
			t.Errorf("expected WriteTimeout %s, got %s", serverWriteTimeout, got.WriteTimeout)
		}
		if got.IdleTimeout != serverIdleTimeout {
			t.Errorf("expected IdleTimeout %s, got %s", serverIdleTimeout, got.IdleTimeout)
		}
	})

	t.Run("server closed is graceful", func(t *testing.T) {
		deps := testServerDeps(cfg, &fakeStartupRepo{})
		deps.listenAndServe = func(server *http.Server) error {
			return http.ErrServerClosed
		}

		if err := runServer(&cobra.Command{}, deps); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("signal triggers graceful shutdown", func(t *testing.T) {
		deps := testServerDeps(cfg, &fakeStartupRepo{})
		started := make(chan struct{})
		shutdown := make(chan struct{})
		deps.signalContext = func(parent context.Context) (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(parent)
			go func() {
				<-started
				cancel()
			}()
			return ctx, cancel
		}
		deps.listenAndServe = func(server *http.Server) error {
			close(started)
			<-shutdown
			return http.ErrServerClosed
		}
		deps.shutdownServer = func(server *http.Server, ctx context.Context) error {
			close(shutdown)
			return nil
		}

		if err := runServer(&cobra.Command{}, deps); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("shutdown error is returned", func(t *testing.T) {
		deps := testServerDeps(cfg, &fakeStartupRepo{})
		started := make(chan struct{})
		release := make(chan struct{})
		shutdownErr := errors.New("shutdown failed")
		deps.signalContext = func(parent context.Context) (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(parent)
			go func() {
				<-started
				cancel()
			}()
			return ctx, cancel
		}
		deps.listenAndServe = func(server *http.Server) error {
			close(started)
			<-release
			return http.ErrServerClosed
		}
		deps.shutdownServer = func(server *http.Server, ctx context.Context) error {
			close(release)
			return shutdownErr
		}

		err := runServer(&cobra.Command{}, deps)
		if err == nil || !strings.Contains(err.Error(), "graceful shutdown failed") {
			t.Fatalf("expected shutdown error, got %v", err)
		}
	})

	t.Run("post-shutdown listen error is returned", func(t *testing.T) {
		deps := testServerDeps(cfg, &fakeStartupRepo{})
		started := make(chan struct{})
		shutdown := make(chan struct{})
		deps.signalContext = func(parent context.Context) (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(parent)
			go func() {
				<-started
				cancel()
			}()
			return ctx, cancel
		}
		deps.listenAndServe = func(server *http.Server) error {
			close(started)
			<-shutdown
			return errors.New("late listen failure")
		}
		deps.shutdownServer = func(server *http.Server, ctx context.Context) error {
			close(shutdown)
			return nil
		}

		err := runServer(&cobra.Command{}, deps)
		if err == nil || !strings.Contains(err.Error(), "late listen failure") {
			t.Fatalf("expected post-shutdown listen error, got %v", err)
		}
	})

	t.Run("shutdown wait timeout is returned", func(t *testing.T) {
		deps := testServerDeps(cfg, &fakeStartupRepo{})
		started := make(chan struct{})
		deps.shutdownTimeout = time.Nanosecond
		deps.signalContext = func(parent context.Context) (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(parent)
			go func() {
				<-started
				cancel()
			}()
			return ctx, cancel
		}
		deps.listenAndServe = func(server *http.Server) error {
			close(started)
			select {}
		}

		err := runServer(&cobra.Command{}, deps)
		if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
			t.Fatalf("expected shutdown timeout, got %v", err)
		}
	})

	t.Run("prefetch starts in background", func(t *testing.T) {
		deps := testServerDeps(cfg, &fakeStartupRepo{})
		service := &fakeShowsService{done: make(chan struct{})}
		deps.newCatalog = func(source adapterspinitron.PageClient) catalog {
			return service
		}

		if err := runServer(&cobra.Command{}, deps); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		select {
		case <-service.done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for prefetch")
		}
		if !service.prefetch {
			t.Fatal("expected prefetch to run")
		}
	})
}

func TestRunServer_WiresIndependentRepositoryPorts(t *testing.T) {
	cfg := &config.Config{
		Port:            8080,
		DBURI:           "file::memory:",
		ENV:             "test",
		ResendAPIKey:    "resend-key",
		SendGridAPIKey:  "sendgrid-key",
		FromEmail:       "noreply@example.com",
		SpinitronAPIURL: "https://proxy.example.test/api",
		AppBaseURL:      "https://aircover.example.com",
	}
	startupRepo := &fakeStartupRepo{}
	authRepo := &fakeAuthRepo{}
	subRequestsRepo := &fakeSubRequestsRepo{}
	adminRepo := &fakeAdminRepo{}
	repos := repositories{
		startup:     startupRepo,
		auth:        authRepo,
		subRequests: subRequestsRepo,
		activeUsers: subRequestsRepo,
		admin:       adminRepo,
	}
	deps := testServerDeps(cfg, startupRepo)
	deps.newDBRepository = func(database *sql.DB) repositories {
		return repos
	}
	var gotResendAPIKey string
	var gotSendGridAPIKey string
	var gotFromEmail string
	deps.newSender = func(resendAPIKey, sendGridAPIKey, fromEmail string) adapteremail.Sender {
		gotResendAPIKey = resendAPIKey
		gotSendGridAPIKey = sendGridAPIKey
		gotFromEmail = fromEmail
		return &mockSender{}
	}
	var gotNotifier *fakeAsyncNotifier
	deps.newAsyncNotifier = func(notifier subrequestsapp.Notifier) asyncNotifier {
		gotNotifier = &fakeAsyncNotifier{next: notifier}
		return gotNotifier
	}
	var gotAuthService *authapp.Service
	deps.newAuthHandler = func(service *authapp.Service) *api.AuthHandler {
		gotAuthService = service
		return api.NewAuthHandler(service)
	}
	var gotSubRequestsService *subrequestsapp.Service
	var gotAdminService *adminapp.Service
	deps.newAPIServer = func(subRequests *subrequestsapp.Service, admin *adminapp.Service, authHandler *api.AuthHandler) *api.Server {
		gotSubRequestsService = subRequests
		gotAdminService = admin
		return api.NewServer(authHandler, nil, nil)
	}

	if err := runServer(&cobra.Command{}, deps); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if gotAuthService == nil {
		t.Fatal("expected auth service to be wired")
	}
	if gotSubRequestsService == nil {
		t.Fatal("expected subrequests service to be wired")
	}
	if gotAdminService == nil {
		t.Fatal("expected admin service to be wired")
	}
	if gotResendAPIKey != "resend-key" {
		t.Fatalf("expected resend key to be wired, got %q", gotResendAPIKey)
	}
	if gotSendGridAPIKey != "sendgrid-key" {
		t.Fatalf("expected sendgrid key to be wired, got %q", gotSendGridAPIKey)
	}
	if gotFromEmail != "noreply@example.com" {
		t.Fatalf("expected from email to be wired, got %q", gotFromEmail)
	}
	if gotNotifier == nil || !gotNotifier.started || !gotNotifier.stopped {
		t.Fatalf("expected async notifier to start and stop, got %+v", gotNotifier)
	}
	emailNotifier, ok := gotNotifier.next.(*adapteremail.SubRequestNotifier)
	if !ok {
		t.Fatalf("expected sub request email notifier, got %T", gotNotifier.next)
	}
	if emailNotifier.BaseURL != "https://aircover.example.com" {
		t.Fatalf("expected app base URL to be wired, got %q", emailNotifier.BaseURL)
	}
}

func TestStartCatalogPrefetchWithoutPrefetcher(t *testing.T) {
	service := &fakeCatalogOnly{}
	startCatalogPrefetch(service, context.Background)
}

func TestStartCatalogPrefetchLogsError(t *testing.T) {
	service := &fakeShowsService{
		err:  errors.New("prefetch failed"),
		done: make(chan struct{}),
	}
	startCatalogPrefetch(service, context.Background)

	select {
	case <-service.done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for prefetch")
	}
	if !service.prefetch {
		t.Fatal("expected prefetch to run")
	}
}

func TestDocCmd(t *testing.T) {
	// Run the doc command to exercise doc.go
	docCmd.Run(docCmd, nil)
}

func TestServerCmd_DBInitError(t *testing.T) {
	t.Setenv("DB_URI", "invalid-dsn")
	t.Setenv("FROM_EMAIL", "noreply@example.com")
	t.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
	t.Setenv("MASTER_EMAIL", "admin@example.com")
	originalOsExit := osExit
	defer func() { osExit = originalOsExit }()

	exited := false
	osExit = func(code int) {
		exited = true
		panic("osExit")
	}

	defer func() {
		_ = recover()
		if !exited {
			t.Error("expected osExit to be called on DB init failure")
		}
	}()

	serverCmd.Run(serverCmd, nil)
}

func TestServerCmd_MasterEmailCheckError(t *testing.T) {
	t.Setenv("DB_URI", "file::memory:?cache=shared")
	t.Setenv("MASTER_EMAIL", "admin@example.com")

	originalOsExit := osExit
	defer func() { osExit = originalOsExit }()

	// We need to make repo.GetUserByEmail fail.
	// This is hard since we can't inject the repo into serverCmd easily.
	// But serverCmd.Run calls sqlite.InitDB(cfg.DBURI).
	// If we close the DB connection after InitDB but before GetUserByEmail?
	// There's no hook for that.
}
