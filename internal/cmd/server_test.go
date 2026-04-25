package cmd

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid path",
			path:       "/",
			wantStatus: http.StatusOK,
			wantBody:   "<!DOCTYPE html>",
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
			rr := httptest.NewRecorder()

			indexHandler(rr, req)

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
	indexHandler(ew, req)
}

func TestServerCmd_Success(t *testing.T) {
	t.Setenv("DB_URI", "sqlite://test.db")
	originalListenAndServe := listenAndServe
	defer func() { listenAndServe = originalListenAndServe }()

	listenAndServe = func(server *http.Server) error {
		return nil
	}

	serverCmd.Run(serverCmd, nil)
}

func TestServerCmd_Error(t *testing.T) {
	t.Setenv("DB_URI", "sqlite://test.db")
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
