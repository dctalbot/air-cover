package ui

import (
	"net/http/httptest"
	"testing"
)

func TestRenderUnauthenticated(t *testing.T) {
	rr := httptest.NewRecorder()
	RenderUnauthenticated(rr)
	if rr.Code != 200 {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRenderAuthenticated(t *testing.T) {
	rr := httptest.NewRecorder()
	RenderAuthenticated(rr, nil)
	if rr.Code != 200 {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
