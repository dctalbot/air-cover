package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"air-cover/internal/adapters/outbound/sqlite"
	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	appcatalog "air-cover/internal/app/catalog"
	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"

	"github.com/getkin/kin-openapi/openapi3"
)

type routerAuthService struct {
	users map[string]routerAuthResult
}

func (f routerAuthService) RequestLogin(ctx context.Context, input authapp.LoginInput) (authapp.LoginResult, error) {
	return authapp.LoginResult{}, nil
}

func (f routerAuthService) VerifyMagicLink(ctx context.Context, rawToken string) (authapp.VerifiedSession, error) {
	return authapp.VerifiedSession{}, nil
}

func (f routerAuthService) Logout(ctx context.Context, userID int) error {
	return nil
}

func (f routerAuthService) AuthenticateSession(ctx context.Context, sessionToken string) (domain.CurrentUser, error) {
	result, ok := f.users[sessionToken]
	if !ok {
		return domain.CurrentUser{}, apperrors.ErrNotFound
	}
	if result.err != nil {
		return domain.CurrentUser{}, result.err
	}
	return result.user, nil
}

type routerAuthResult struct {
	user domain.CurrentUser
	err  error
}

type routerSubRequestService struct {
	dashboard subrequestsapp.Dashboard
	detailErr error
}

func (f routerSubRequestService) ListDashboard(ctx context.Context, viewer domain.CurrentUser) (subrequestsapp.Dashboard, error) {
	return f.dashboard, nil
}

func (f routerSubRequestService) Get(ctx context.Context, viewer domain.CurrentUser, id int) (subrequestsapp.Detail, error) {
	return subrequestsapp.Detail{}, f.detailErr
}

func (f routerSubRequestService) Create(ctx context.Context, viewer domain.CurrentUser, input subrequestsapp.CreateInput) error {
	return nil
}

func (f routerSubRequestService) Delete(ctx context.Context, viewer domain.CurrentUser, id int) error {
	return nil
}

func (f routerSubRequestService) ApplyAction(ctx context.Context, viewer domain.CurrentUser, id int, action subrequestsapp.Action) error {
	return nil
}

type routerAdminService struct {
	users []adminapp.UserReadModel
}

func (f routerAdminService) ListUsers(ctx context.Context, viewer domain.CurrentUser) ([]adminapp.UserReadModel, error) {
	return f.users, nil
}

func (f routerAdminService) CreateUser(ctx context.Context, viewer domain.CurrentUser, input adminapp.CreateUserInput) error {
	return nil
}

func (f routerAdminService) UpdateUser(ctx context.Context, viewer domain.CurrentUser, input adminapp.UpdateUserInput) error {
	return apperrors.ErrNotFound
}

func (f routerAdminService) ImportCatalogUsers(ctx context.Context, viewer domain.CurrentUser) error {
	return nil
}

func markSameOrigin(req *http.Request) {
	req.Header.Set("Sec-Fetch-Site", "same-origin")
}

func TestNewRouter(t *testing.T) {
	// Arrange
	auth := NewAuthHandler(routerAuthService{users: map[string]routerAuthResult{
		"stoken_admin": {user: domain.CurrentUser{ID: 1, Email: "admin_cmd_test@example.com", Role: domain.RoleAdmin}},
	}})
	server := NewServer(auth, routerSubRequestService{}, routerAdminService{})
	r := NewRouter(server, auth, nil)
	if r == nil {
		t.Fatal("expected non-nil router")
	}

	// Act
	req := httptest.NewRequest(http.MethodDelete, "/sub-requests/abc", nil)
	markSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken_admin"})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid ID, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/users/abc", strings.NewReader(`{"is_enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	markSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken_admin"})
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid user ID, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/users/123", strings.NewReader(`{"is_enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	markSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken_admin"})
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for non-existent user, got %d", rr.Code)
	}

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

	req = httptest.NewRequest(http.MethodGet, "/sub-requests/abc", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken_admin"})
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid detail ID, got %d", rr.Code)
	}
}

func TestNewRouter_RouteAuthorization(t *testing.T) {
	// Arrange
	auth := NewAuthHandler(routerAuthService{users: map[string]routerAuthResult{
		"route-member-token":   {user: domain.CurrentUser{ID: 1, Email: "route-member@example.com", Role: domain.RoleMember}},
		"route-admin-token":    {user: domain.CurrentUser{ID: 2, Email: "route-admin@example.com", Role: domain.RoleAdmin}},
		"route-disabled-token": {err: apperrors.ErrForbidden},
	}})
	server := NewServer(auth, routerSubRequestService{
		dashboard: subrequestsapp.Dashboard{
			Shows: []appcatalog.Show{{ID: "show-1", Title: "Authorization Test Show"}},
		},
		detailErr: apperrors.ErrNotFound,
	}, routerAdminService{
		users: []adminapp.UserReadModel{
			{User: &domain.User{ID: 2, Email: "route-admin@example.com", Role: domain.RoleAdmin, IsEnabled: true}},
		},
	})
	r := NewRouter(server, auth, nil)

	sessions := map[string]struct {
		rawToken string
	}{
		"member": {
			rawToken: "route-member-token",
		},
		"admin": {
			rawToken: "route-admin-token",
		},
		"disabled": {
			rawToken: "route-disabled-token",
		},
		"expired": {
			rawToken: "route-expired-token",
		},
	}

	tests := []struct {
		name         string
		path         string
		sessionToken string
		wantStatus   int
		wantLocation string
		wantBody     string
	}{
		{
			name:         "app unauthenticated",
			path:         "/app",
			wantStatus:   http.StatusFound,
			wantLocation: "/",
		},
		{
			name:         "app member",
			path:         "/app",
			sessionToken: sessions["member"].rawToken,
			wantStatus:   http.StatusOK,
			wantBody:     "Authorization Test Show",
		},
		{
			name:         "app admin",
			path:         "/app",
			sessionToken: sessions["admin"].rawToken,
			wantStatus:   http.StatusOK,
			wantBody:     "Authorization Test Show",
		},
		{
			name:         "sub request detail member",
			path:         "/sub-requests/999",
			sessionToken: sessions["member"].rawToken,
			wantStatus:   http.StatusNotFound,
		},
		{
			name:         "sub request detail unauthenticated",
			path:         "/sub-requests/999",
			wantStatus:   http.StatusFound,
			wantLocation: "/",
		},
		{
			name:         "app disabled user",
			path:         "/app",
			sessionToken: sessions["disabled"].rawToken,
			wantStatus:   http.StatusFound,
			wantLocation: "/",
		},
		{
			name:         "app expired session",
			path:         "/app",
			sessionToken: sessions["expired"].rawToken,
			wantStatus:   http.StatusFound,
			wantLocation: "/",
		},
		{
			name:         "admin unauthenticated",
			path:         "/admin",
			wantStatus:   http.StatusFound,
			wantLocation: "/",
		},
		{
			name:         "admin member",
			path:         "/admin",
			sessionToken: sessions["member"].rawToken,
			wantStatus:   http.StatusFound,
			wantLocation: "/",
		},
		{
			name:         "admin admin",
			path:         "/admin",
			sessionToken: sessions["admin"].rawToken,
			wantStatus:   http.StatusOK,
			wantBody:     "route-admin@example.com",
		},
		{
			name:         "admin disabled user",
			path:         "/admin",
			sessionToken: sessions["disabled"].rawToken,
			wantStatus:   http.StatusFound,
			wantLocation: "/",
		},
		{
			name:         "admin expired session",
			path:         "/admin",
			sessionToken: sessions["expired"].rawToken,
			wantStatus:   http.StatusFound,
			wantLocation: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.sessionToken != "" {
				req.AddCookie(&http.Cookie{Name: "session_id", Value: tt.sessionToken})
			}
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

			// Assert
			if rr.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
			if tt.wantLocation != "" && rr.Header().Get("Location") != tt.wantLocation {
				t.Fatalf("expected Location header %q, got %q", tt.wantLocation, rr.Header().Get("Location"))
			}
			if tt.wantBody != "" && !strings.Contains(rr.Body.String(), tt.wantBody) {
				t.Fatalf("expected body to contain %q", tt.wantBody)
			}
		})
	}
}

func TestNewRouter_RejectsCrossSiteMutations(t *testing.T) {
	// Arrange
	auth := NewAuthHandler(routerAuthService{users: map[string]routerAuthResult{
		"csrf_token_admin": {user: domain.CurrentUser{ID: 1, Email: "csrf_admin@example.com", Role: domain.RoleAdmin}},
	}})
	server := NewServer(auth, routerSubRequestService{}, routerAdminService{})
	r := NewRouter(server, auth, nil)

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
			// Act
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			req.AddCookie(&http.Cookie{Name: "session_id", Value: "csrf_token_admin"})

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			// Assert
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
			name:    "untrusted forwarded proto origin rejected",
			method:  http.MethodPost,
			headers: map[string]string{"Origin": "https://example.com", "X-Forwarded-Proto": "https"},
			want:    false,
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

func TestSameOriginMutationChecksTrustForwardedProtoFromConfiguredProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://example.com/app", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("X-Forwarded-Proto", "https")

	called := false
	handler := TrustForwardedHeaders([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})(
		requireSameOriginMutation(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		})),
	)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatal("expected handler to be called for trusted forwarded proto")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
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
		NewRouter(nil, nil, nil)
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
	auth := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	server := newTestServer(repo, auth, nil)

	r := NewRouter(server, auth, nil)

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
