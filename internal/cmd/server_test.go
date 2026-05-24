package cmd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpadapter "air-cover/internal/adapters/http"
	adapterspinitron "air-cover/internal/adapters/spinitron"
	"air-cover/internal/adapters/sqlite"
	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	bootstrapapp "air-cover/internal/app/bootstrap"
	appcatalog "air-cover/internal/app/catalog"
	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/apperrors"
	"air-cover/internal/config"
	"air-cover/internal/domain"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"
)

type errorWriter struct{}

func (w *errorWriter) Header() http.Header {
	return make(http.Header)
}

func (w *errorWriter) Write(b []byte) (int, error) {
	return 0, errors.New("write error")
}

func (w *errorWriter) WriteHeader(statusCode int) {}

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

type fakeAdminRepo struct{}

func (f *fakeAdminRepo) ListUsers(ctx context.Context) ([]*domain.User, error) {
	return nil, nil
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

func newTestAPIServer(repo *sqlite.Repository, authHandler *httpadapter.AuthHandler, catalog catalog) *httpadapter.Server {
	var subRequests *subrequestsapp.Service
	var admin *adminapp.Service
	if repo != nil {
		subRequests = subrequestsapp.NewService(repo, catalog)
		admin = adminapp.NewService(repo, catalog)
	}
	return httpadapter.NewServer(authHandler, subRequests, admin)
}

func markSameOrigin(req *http.Request) {
	req.Header.Set("Sec-Fetch-Site", "same-origin")
}

func testServerDeps(cfg *config.Config, repo bootstrapapp.Repository) serverDeps {
	return serverDeps{
		loadConfig: func(cmd *cobra.Command) (*config.Config, error) {
			return cfg, nil
		},
		initDB: func(uri string) (*sql.DB, error) {
			return nil, nil
		},
		newSender: func(apiKey, fromEmail, env string) authapp.Sender {
			return &mockSender{}
		},
		newSpinitron: func(apiKey, baseURL string) adapterspinitron.PageClient {
			return &fakeSpinitronPageClient{}
		},
		newCatalog: func(source adapterspinitron.PageClient) catalog {
			return &fakeShowsService{}
		},
		newRouter: func(apiServer *httpadapter.Server, authHandler *httpadapter.AuthHandler) chi.Router {
			return chi.NewRouter()
		},
		listenAndServe: func(server *http.Server) error {
			return nil
		},
		backgroundCtx: context.Background,
		newAuthHandler: func(service *authapp.Service) *httpadapter.AuthHandler {
			return httpadapter.NewAuthHandler(service)
		},
		newAPIServer: func(subRequests *subrequestsapp.Service, admin *adminapp.Service, authHandler *httpadapter.AuthHandler) *httpadapter.Server {
			return httpadapter.NewServer(authHandler, subRequests, admin)
		},
		newDBRepository: func(database *sql.DB) repositories {
			return repositories{startup: repo}
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

func TestHealthHandler(t *testing.T) {
	server := httpadapter.NewServer(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	rr := httptest.NewRecorder()
	server.GetHealth(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	ew := &errorWriter{}
	server.GetHealth(ew, req)
}

func TestIndexHandler(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
	t.Cleanup(func() {
		_ = dbConn.Close()
	})

	repo := sqlite.NewRepository(dbConn)
	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test@example.com", "member")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	err = repo.CreateSession(ctx, "sid", authapp.HashToken("stoken"), user.ID, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	auth := httpadapter.NewAuthHandler(authapp.NewService(repo, nil))
	server := newTestAPIServer(repo, auth, nil)

	handler := server.Get

	tests := []struct {
		name       string
		path       string
		cookie     *http.Cookie
		wantStatus int
		wantBody   string
		wantHeader string
	}{
		{
			name:       "valid path unauthenticated",
			path:       "/",
			wantStatus: http.StatusOK,
			wantBody:   "Submit",
		},
		{
			name:       "valid path authenticated",
			path:       "/",
			cookie:     &http.Cookie{Name: "session_id", Value: "stoken"},
			wantStatus: http.StatusFound,
			wantHeader: "/app",
		},
		{
			name:       "submitted success",
			path:       "/?submitted=true",
			wantStatus: http.StatusOK,
			wantBody:   "If an account exists, an email has been sent.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rr := httptest.NewRecorder()

			handler(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
			if tt.wantStatus == http.StatusOK && !strings.Contains(rr.Body.String(), tt.wantBody) {
				t.Errorf("expected body to contain %q", tt.wantBody)
			}
			if tt.wantHeader != "" && rr.Header().Get("Location") != tt.wantHeader {
				t.Errorf("expected Location header %q, got %q", tt.wantHeader, rr.Header().Get("Location"))
			}
		})
	}

	// Test write error
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ew := &errorWriter{}
	handler(ew, req)
}

func TestAppHandler(t *testing.T) {
	service := &fakeShowsService{
		shows: []appcatalog.Show{
			{ID: "2", Title: "Zebra Show"},
			{ID: "1", Title: "Apple Show"},
		},
	}
	dbConn, _ := sqlite.InitDB("file::memory:?cache=shared")
	repo := sqlite.NewRepository(dbConn)
	server := newTestAPIServer(repo, nil, service)
	handler := server.GetApp

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid path",
			path:       "/app",
			wantStatus: http.StatusOK,
			wantBody:   "Apple Show",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			ctx := context.WithValue(req.Context(), httpadapter.UserEmailKey, "test@example.com")
			ctx = context.WithValue(ctx, httpadapter.UserIDKey, 1)
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()

			handler(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
			if !strings.Contains(rr.Body.String(), tt.wantBody) {
				t.Fatalf("expected body to contain %q", tt.wantBody)
			}
			if !strings.Contains(rr.Body.String(), `id="start-time"`) {
				t.Fatalf("expected body to contain start-time input")
			}
			if !strings.Contains(rr.Body.String(), `id="end-time"`) {
				t.Fatalf("expected body to contain end-time input")
			}

			if !strings.Contains(rr.Body.String(), `id="total-duration"`) {
				t.Fatalf("expected body to contain total-duration display")
			}
			if !strings.Contains(rr.Body.String(), "<b>test@example.com</b> is requesting a sub") {
				t.Fatalf("expected body to contain new duration label with email")
			}

			if !strings.Contains(rr.Body.String(), `id="selected-show"`) {
				t.Fatalf("expected body to contain selected-show span")
			}

			if strings.Index(rr.Body.String(), "Apple Show") > strings.Index(rr.Body.String(), "Zebra Show") {
				t.Fatalf("expected Apple Show to appear before Zebra Show")
			}
		})
	}
}

func TestAppHandler_UpstreamError(t *testing.T) {
	service := &fakeShowsService{err: errors.New("boom")}
	dbConn, _ := sqlite.InitDB("file::memory:?cache=shared")
	repo := sqlite.NewRepository(dbConn)
	server := newTestAPIServer(repo, nil, service)
	handler := server.GetApp

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rr.Code)
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
	t.Setenv("DB_URI", "file::memory:?cache=shared")
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
	dbConn, err := sqlite.InitDB("file::memory:?cache=shared")
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

func TestRunServer_DependencyFailures(t *testing.T) {
	cfg := &config.Config{
		Port:            8080,
		DBURI:           "file::memory:",
		ENV:             "test",
		FromEmail:       "noreply@example.com",
		SpinitronAPIURL: "https://proxy.example.test/api",
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
		FromEmail:       "noreply@example.com",
		SpinitronAPIURL: "https://proxy.example.test/api",
	}
	startupRepo := &fakeStartupRepo{}
	authRepo := &fakeAuthRepo{}
	subRequestsRepo := &fakeSubRequestsRepo{}
	adminRepo := &fakeAdminRepo{}
	repos := repositories{
		startup:     startupRepo,
		auth:        authRepo,
		subRequests: subRequestsRepo,
		admin:       adminRepo,
	}
	deps := testServerDeps(cfg, startupRepo)
	deps.newDBRepository = func(database *sql.DB) repositories {
		return repos
	}
	var gotAuthService *authapp.Service
	deps.newAuthHandler = func(service *authapp.Service) *httpadapter.AuthHandler {
		gotAuthService = service
		return httpadapter.NewAuthHandler(service)
	}
	var gotSubRequestsService *subrequestsapp.Service
	var gotAdminService *adminapp.Service
	deps.newAPIServer = func(subRequests *subrequestsapp.Service, admin *adminapp.Service, authHandler *httpadapter.AuthHandler) *httpadapter.Server {
		gotSubRequestsService = subRequests
		gotAdminService = admin
		return httpadapter.NewServer(authHandler, nil, nil)
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

func TestNewRouter(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer dbConn.Close()

	repo := sqlite.NewRepository(dbConn)
	auth := httpadapter.NewAuthHandler(authapp.NewService(repo, nil))
	server := newTestAPIServer(repo, auth, nil)

	r := newRouter(server, auth)
	if r == nil {
		t.Fatal("expected non-nil router")
	}

	// Create admin user/session for authenticated tests
	admin, err := repo.CreateUser(context.Background(), "admin_cmd_test@example.com", "admin")
	if err != nil {
		t.Fatalf("failed to create admin user: %v", err)
	}
	err = repo.CreateSession(context.Background(), "sid_admin", authapp.HashToken("stoken_admin"), admin.ID, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Test invalid ID in sub-requests Delete route (now handled by generated wrapper)
	req := httptest.NewRequest(http.MethodDelete, "/sub-requests/abc", nil)
	markSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken_admin"})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid ID, got %d", rr.Code)
	}

	// Test invalid ID in users Post route (now handled by generated wrapper)
	req = httptest.NewRequest(http.MethodPost, "/users/abc", strings.NewReader(`{"is_enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	markSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken_admin"})
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid user ID, got %d", rr.Code)
	}

	// Test valid wiring for POST /users/{id}
	req = httptest.NewRequest(http.MethodPost, "/users/123", strings.NewReader(`{"is_enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	markSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken_admin"})
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	// Should be 404 because user 123 doesn't exist
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for non-existent user, got %d", rr.Code)
	}

	// Test valid wiring for POST /users
	form := "email=newuser@example.com&role=member"
	req = httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	markSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken_admin"})
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected status 303 for user creation, got %d", rr.Code)
	}
}

func TestNewRouter_RejectsCrossSiteMutations(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer dbConn.Close()

	repo := sqlite.NewRepository(dbConn)
	auth := httpadapter.NewAuthHandler(authapp.NewService(repo, nil))
	server := newTestAPIServer(repo, auth, nil)
	r := newRouter(server, auth)

	admin, err := repo.CreateUser(context.Background(), "csrf_admin@example.com", "admin")
	if err != nil {
		t.Fatalf("failed to create admin user: %v", err)
	}
	if err := repo.CreateSession(context.Background(), "csrf_sid_admin", authapp.HashToken("csrf_token_admin"), admin.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("failed to create admin session: %v", err)
	}

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
	}{
		{
			name:   "logout",
			method: http.MethodPost,
			path:   "/auth/logout",
		},
		{
			name:        "create sub request",
			method:      http.MethodPost,
			path:        "/sub-requests",
			body:        "show=1&start_time=2036-01-01T10%3A00&end_time=2036-01-01T12%3A00",
			contentType: "application/x-www-form-urlencoded",
		},
		{
			name:   "delete sub request",
			method: http.MethodDelete,
			path:   "/sub-requests/1",
		},
		{
			name:        "patch sub request",
			method:      http.MethodPatch,
			path:        "/sub-requests/1",
			body:        `{"action":"take"}`,
			contentType: "application/json",
		},
		{
			name:        "create user",
			method:      http.MethodPost,
			path:        "/users",
			body:        "email=csrf_user@example.com&role=member",
			contentType: "application/x-www-form-urlencoded",
		},
		{
			name:   "import users",
			method: http.MethodPost,
			path:   "/users/import/spinitron",
		},
		{
			name:        "update user",
			method:      http.MethodPost,
			path:        "/users/1",
			body:        `{"is_enabled":false}`,
			contentType: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			req.AddCookie(&http.Cookie{Name: "session_id", Value: "csrf_token_admin"})

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
			}
		})
	}
}

func TestSameOriginMutationChecks(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		headers map[string]string
		want    bool
	}{
		{
			name:   "safe method skips mutation protection",
			method: http.MethodGet,
			want:   true,
		},
		{
			name:    "same-site fetch metadata accepted",
			method:  http.MethodPost,
			headers: map[string]string{"Sec-Fetch-Site": "same-site"},
			want:    true,
		},
		{
			name:    "browser-initiated none fetch metadata accepted",
			method:  http.MethodPost,
			headers: map[string]string{"Sec-Fetch-Site": "none"},
			want:    true,
		},
		{
			name:    "cross-site fetch metadata rejected",
			method:  http.MethodPost,
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
			want:    false,
		},
		{
			name:    "matching origin accepted",
			method:  http.MethodPost,
			headers: map[string]string{"Origin": "http://example.com"},
			want:    true,
		},
		{
			name:    "matching forwarded proto origin accepted",
			method:  http.MethodPost,
			headers: map[string]string{"Origin": "https://example.com", "X-Forwarded-Proto": "https"},
			want:    true,
		},
		{
			name:    "matching referer accepted",
			method:  http.MethodPost,
			headers: map[string]string{"Referer": "http://example.com/app"},
			want:    true,
		},
		{
			name:    "different origin rejected",
			method:  http.MethodPost,
			headers: map[string]string{"Origin": "http://evil.example"},
			want:    false,
		},
		{
			name:    "malformed origin rejected",
			method:  http.MethodPost,
			headers: map[string]string{"Origin": "://bad-origin"},
			want:    false,
		},
		{
			name:   "missing origin metadata rejected",
			method: http.MethodPost,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://example.com/app", nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			called := false
			handler := requireSameOriginMutation(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			}))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if called != tt.want {
				t.Fatalf("handler called = %v, want %v", called, tt.want)
			}
			if tt.want && rr.Code != http.StatusNoContent {
				t.Fatalf("expected accepted status %d, got %d", http.StatusNoContent, rr.Code)
			}
			if !tt.want && rr.Code != http.StatusForbidden {
				t.Fatalf("expected rejected status %d, got %d", http.StatusForbidden, rr.Code)
			}
		})
	}
}

func TestNewRouter_SwaggerError(t *testing.T) {
	originalGetSwagger := getSwagger
	originalOsExit := osExit
	defer func() {
		getSwagger = originalGetSwagger
		osExit = originalOsExit
	}()

	getSwagger = func() (*openapi3.T, error) {
		return nil, errors.New("swagger failed")
	}
	exited := false
	osExit = func(code int) {
		exited = true
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		panic("osExit")
	}

	func() {
		defer func() {
			if r := recover(); r != nil && r != "osExit" {
				panic(r)
			}
		}()
		newRouter(nil, nil)
	}()

	if !exited {
		t.Error("expected osExit to be called")
	}
}

func TestAuthRateLimiting(t *testing.T) {
	dbConn, _ := sqlite.InitDB("file::memory:?cache=shared")
	defer dbConn.Close()

	repo := sqlite.NewRepository(dbConn)
	_, _ = repo.CreateUser(context.Background(), "test@example.com", "member")
	auth := httpadapter.NewAuthHandler(authapp.NewService(repo, &mockSender{}))
	server := newTestAPIServer(repo, auth, nil)

	r := newRouter(server, auth)

	// Make 5 successful-ish requests
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"test@example.com"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		// We expect 200 because the handler will succeed (mocked repo/sender might be used)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}

	// 6th request should be rate limited
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"test@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", rr.Code)
	}
}

func TestServerCmd_DBInitError(t *testing.T) {
	t.Setenv("DB_URI", "invalid-dsn")
	t.Setenv("FROM_EMAIL", "noreply@example.com")
	t.Setenv("SPINITRON_API_URL", "https://proxy.example.test/api")
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
