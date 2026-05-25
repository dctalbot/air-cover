package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"air-cover/internal/adapters/outbound/sqlite"
	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	appcatalog "air-cover/internal/app/catalog"
	subrequestsapp "air-cover/internal/app/subrequests"

	"github.com/getkin/kin-openapi/openapi3"
)

type routerTestCatalog interface {
	adminapp.Catalog
	subrequestsapp.Catalog
}

type routerTestShowsService struct {
	shows []appcatalog.Show
	err   error
}

func (f *routerTestShowsService) ListShows(ctx context.Context) ([]appcatalog.Show, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.shows, nil
}

func (f *routerTestShowsService) ListPersonas(ctx context.Context) ([]appcatalog.Persona, error) {
	return nil, f.err
}

func newRouterTestServer(repo *sqlite.Repository, authHandler *AuthHandler, catalog routerTestCatalog) *Server {
	var subRequests *subrequestsapp.Service
	var admin *adminapp.Service
	if repo != nil {
		subRequests = subrequestsapp.NewService(repo, catalog)
		admin = adminapp.NewService(repo, catalog)
	}
	return NewServer(authHandler, subRequests, admin)
}

func markSameOrigin(req *http.Request) {
	req.Header.Set("Sec-Fetch-Site", "same-origin")
}

func TestNewRouter(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer dbConn.Close()

	repo := sqlite.NewRepository(dbConn)
	auth := NewAuthHandler(authapp.NewService(repo, nil))
	server := newRouterTestServer(repo, auth, nil)

	r := NewRouter(server, auth)
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

	// Test invalid ID in sub-requests Get route (handled by generated wrapper)
	req = httptest.NewRequest(http.MethodGet, "/sub-requests/abc", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken_admin"})
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid detail ID, got %d", rr.Code)
	}
}

func TestNewRouter_RouteAuthorization(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer dbConn.Close()

	repo := sqlite.NewRepository(dbConn)
	auth := NewAuthHandler(authapp.NewService(repo, nil))
	server := newRouterTestServer(repo, auth, &routerTestShowsService{
		shows: []appcatalog.Show{
			{ID: "show-1", Title: "Authorization Test Show"},
		},
	})
	r := NewRouter(server, auth)

	ctx := context.Background()
	member, err := repo.CreateUser(ctx, "route-member@example.com", "member")
	if err != nil {
		t.Fatalf("failed to create member user: %v", err)
	}
	admin, err := repo.CreateUser(ctx, "route-admin@example.com", "admin")
	if err != nil {
		t.Fatalf("failed to create admin user: %v", err)
	}
	disabled, err := repo.CreateUser(ctx, "route-disabled@example.com", "member")
	if err != nil {
		t.Fatalf("failed to create disabled user: %v", err)
	}
	disabledEnabled := false
	if err := repo.UpdateUser(ctx, disabled.ID, nil, &disabledEnabled); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	expired, err := repo.CreateUser(ctx, "route-expired@example.com", "member")
	if err != nil {
		t.Fatalf("failed to create expired-session user: %v", err)
	}

	sessions := map[string]struct {
		userID    int
		rawToken  string
		expiresAt time.Time
	}{
		"member": {
			userID:    member.ID,
			rawToken:  "route-member-token",
			expiresAt: time.Now().Add(time.Hour),
		},
		"admin": {
			userID:    admin.ID,
			rawToken:  "route-admin-token",
			expiresAt: time.Now().Add(time.Hour),
		},
		"disabled": {
			userID:    disabled.ID,
			rawToken:  "route-disabled-token",
			expiresAt: time.Now().Add(time.Hour),
		},
		"expired": {
			userID:    expired.ID,
			rawToken:  "route-expired-token",
			expiresAt: time.Now().Add(-time.Hour),
		},
	}
	for name, session := range sessions {
		if err := repo.CreateSession(ctx, "route-"+name+"-session", authapp.HashToken(session.rawToken), session.userID, session.expiresAt); err != nil {
			t.Fatalf("failed to create %s session: %v", name, err)
		}
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
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.sessionToken != "" {
				req.AddCookie(&http.Cookie{Name: "session_id", Value: tt.sessionToken})
			}
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

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
	dbConn, err := sqlite.InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer dbConn.Close()

	repo := sqlite.NewRepository(dbConn)
	auth := NewAuthHandler(authapp.NewService(repo, nil))
	server := newRouterTestServer(repo, auth, nil)
	r := NewRouter(server, auth)

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
		NewRouter(nil, nil)
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
	server := newRouterTestServer(repo, auth, nil)

	r := NewRouter(server, auth)

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
