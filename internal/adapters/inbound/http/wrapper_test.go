package httpadapter

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"air-cover/internal/apperrors"
)

func TestNewWrapper(t *testing.T) {
	server := newTestServer(nil, nil, nil)
	wrapper := NewWrapper(server)

	if wrapper == nil {
		t.Fatal("expected non-nil wrapper")
	}
	if wrapper.Handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if wrapper.ErrorHandlerFunc == nil {
		t.Fatal("expected non-nil error handler func")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	wrapper.ErrorHandlerFunc(rr, req, http.ErrBodyNotAllowed)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), http.ErrBodyNotAllowed.Error()) {
		t.Errorf("expected stable response body, got %q", rr.Body.String())
	}
	if body := rr.Body.String(); body != "Invalid request\n" {
		t.Errorf("expected invalid request response, got %q", body)
	}
}

func TestNewWrapper_JSONErrorResponse(t *testing.T) {
	server := newTestServer(nil, nil, nil)
	wrapper := NewWrapper(server)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/json")
	wrapper.ErrorHandlerFunc(rr, req, http.ErrBodyNotAllowed)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("expected JSON content type, got %q", contentType)
	}
	if body := rr.Body.String(); body != "{\"error\":\"Invalid request\"}\n" {
		t.Errorf("expected JSON error body, got %q", body)
	}
	if strings.Contains(rr.Body.String(), http.ErrBodyNotAllowed.Error()) {
		t.Errorf("expected stable response body, got %q", rr.Body.String())
	}
}

func TestWriteAppError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{"not found", apperrors.ErrNotFound, http.StatusNotFound, "Not found\n"},
		{"conflict", apperrors.ErrConflict, http.StatusConflict, "Conflict\n"},
		{"forbidden", apperrors.ErrForbidden, http.StatusForbidden, "Forbidden\n"},
		{"invalid", apperrors.ErrInvalid, http.StatusBadRequest, "Invalid request\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			if !writeAppError(rr, req, tt.err) {
				t.Fatal("expected app error to be handled")
			}
			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
			if body := rr.Body.String(); body != tt.wantBody {
				t.Errorf("expected body %q, got %q", tt.wantBody, body)
			}
		})
	}
}

func TestWriteAppError_JSONResponse(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/sub-requests/1", nil)
	req.Header.Set("Content-Type", "application/json")

	if !writeAppError(rr, req, apperrors.ErrForbidden) {
		t.Fatal("expected app error to be handled")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("expected JSON content type, got %q", contentType)
	}
	if body := rr.Body.String(); body != "{\"error\":\"Forbidden\"}\n" {
		t.Errorf("expected JSON error body, got %q", body)
	}
}

func TestWriteAppError_UnknownError(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if writeAppError(rr, req, http.ErrAbortHandler) {
		t.Fatal("expected unknown error to remain unhandled")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected recorder to remain untouched, got %d", rr.Code)
	}
}

func TestHeaderContainsMediaType(t *testing.T) {
	if !headerContainsMediaType("text/html, application/json; charset=utf-8", "application/json") {
		t.Fatal("expected media type match with parameters")
	}
	if headerContainsMediaType("text/html, application/problem+json", "application/json") {
		t.Fatal("expected distinct media type not to match")
	}
}
