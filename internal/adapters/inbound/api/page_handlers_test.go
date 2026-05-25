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
)

type errorWriter struct{}

func (w *errorWriter) Header() http.Header {
	return make(http.Header)
}

func (w *errorWriter) Write(b []byte) (int, error) {
	return 0, errors.New("write error")
}

func (w *errorWriter) WriteHeader(statusCode int) {}

type pageHandlerTestCatalog interface {
	adminapp.Catalog
	subrequestsapp.Catalog
}

type pageHandlerShowsService struct {
	shows []appcatalog.Show
	err   error
}

func (f *pageHandlerShowsService) ListShows(ctx context.Context) ([]appcatalog.Show, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.shows, nil
}

func (f *pageHandlerShowsService) ListPersonas(ctx context.Context) ([]appcatalog.Persona, error) {
	return nil, f.err
}

func newPageHandlerTestServer(repo *sqlite.Repository, authHandler *AuthHandler, catalog pageHandlerTestCatalog) *Server {
	var subRequests *subrequestsapp.Service
	var admin *adminapp.Service
	if repo != nil {
		subRequests = subrequestsapp.NewService(repo, catalog)
		admin = adminapp.NewService(repo, catalog)
	}
	return NewServer(authHandler, subRequests, admin)
}

func TestHealthHandler(t *testing.T) {
	server := NewServer(nil, nil, nil)
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

	auth := NewAuthHandler(authapp.NewService(repo, nil))
	server := newPageHandlerTestServer(repo, auth, nil)

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
	service := &pageHandlerShowsService{
		shows: []appcatalog.Show{
			{ID: "2", Title: "Zebra Show"},
			{ID: "1", Title: "Apple Show"},
		},
	}
	dbConn, _ := sqlite.InitDB("file::memory:?cache=shared")
	repo := sqlite.NewRepository(dbConn)
	server := newPageHandlerTestServer(repo, nil, service)
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
			ctx := context.WithValue(req.Context(), UserEmailKey, "test@example.com")
			ctx = context.WithValue(ctx, UserIDKey, 1)
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
	service := &pageHandlerShowsService{err: errors.New("boom")}
	dbConn, _ := sqlite.InitDB("file::memory:?cache=shared")
	repo := sqlite.NewRepository(dbConn)
	server := newPageHandlerTestServer(repo, nil, service)
	handler := server.GetApp

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rr.Code)
	}
}
