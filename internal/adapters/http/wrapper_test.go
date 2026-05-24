package httpadapter

import (
	"net/http"
	"net/http/httptest"
	"testing"
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

	// Test that the error handler returns 400
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	wrapper.ErrorHandlerFunc(rr, req, http.ErrBodyNotAllowed)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}
