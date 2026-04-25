package cmd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"air-cover/internal/db"
)

type errorWriter struct{}

func (w *errorWriter) Header() http.Header {
	return make(http.Header)
}

func (w *errorWriter) Write(b []byte) (int, error) {
	return 0, errors.New("write error")
}

func (w *errorWriter) WriteHeader(statusCode int) {}

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	rr := httptest.NewRecorder()
	healthHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	ew := &errorWriter{}
	healthHandler(ew, req)
}

func TestIndexHandler(t *testing.T) {
	dbConn, err := db.InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
	t.Cleanup(func() {
		_ = dbConn.Close()
	})

	repo := db.NewRepository(dbConn)
	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	err = repo.CreateSession(ctx, "sid", "stoken", user.ID, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	handler := indexHandler(repo)

	tests := []struct {
		name       string
		path       string
		cookie     *http.Cookie
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid path unauthenticated",
			path:       "/",
			wantStatus: http.StatusOK,
			wantBody:   "Send magic link",
		},
		{
			name:       "valid path authenticated",
			path:       "/",
			cookie:     &http.Cookie{Name: "session_id", Value: "stoken"},
			wantStatus: http.StatusOK,
			wantBody:   "Welcome to Air Cover",
		},
		{
			name:       "invalid path",
			path:       "/unknown",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
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
		})
	}

	// Test write error
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ew := &errorWriter{}
	handler(ew, req)
}

func TestServerCmd_Success(t *testing.T) {
	t.Setenv("DB_URI", "file::memory:?cache=shared")
	originalListenAndServe := listenAndServe
	defer func() { listenAndServe = originalListenAndServe }()

	listenAndServe = func(server *http.Server) error {
		return nil
	}

	serverCmd.Run(serverCmd, nil)
}

func TestServerCmd_Error(t *testing.T) {
	t.Setenv("DB_URI", "file::memory:?cache=shared")
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
		ReadHeaderTimeout: 3 * time.Second,
	}
	err := listenAndServe(server)
	if err == nil {
		t.Error("expected error, got nil")
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
