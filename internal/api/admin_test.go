package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthHandler_RequireAdmin(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(repo, nil)

	_, _ = repo.CreateUser(context.Background(), "admin@example.com", "admin")
	_, _ = repo.CreateUser(context.Background(), "member@example.com", "member")

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := handler.RequireAdmin(testHandler)

	tests := []struct {
		name       string
		role       string
		wantStatus int
	}{
		{"admin access", "admin", http.StatusOK},
		{"member access", "member", http.StatusForbidden},
		{"no role access", "", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			if tt.role != "" {
				ctx := context.WithValue(req.Context(), UserRoleKey, tt.role)
				req = req.WithContext(ctx)
			}
			rr := httptest.NewRecorder()
			mw.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %v, got %v", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestServer_GetAdmin(t *testing.T) {
	repo := setupTestDB(t)
	s := NewServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()
	s.GetAdmin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK, got %v", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("expected HTML content type, got %v", rr.Header().Get("Content-Type"))
	}
}

func TestServer_GetApp_AdminLinkVisibility(t *testing.T) {
	repo := setupTestDB(t)
	s := NewServer(repo, nil, &MockShowsService{})

	tests := []struct {
		name     string
		role     string
		wantLink bool
	}{
		{"admin sees link", "admin", true},
		{"member hides link", "member", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/app", nil)
			ctx := context.WithValue(req.Context(), UserEmailKey, "test@example.com")
			ctx = context.WithValue(ctx, UserIDKey, 1)
			ctx = context.WithValue(ctx, UserRoleKey, tt.role)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			s.GetApp(rr, req)

			containsLink := bytes.Contains(rr.Body.Bytes(), []byte("/admin\">Admin</a>"))
			if containsLink != tt.wantLink {
				t.Errorf("expected admin link presence to be %v, got %v", tt.wantLink, containsLink)
			}
		})
	}
}
