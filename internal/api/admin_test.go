package api

import (
	"bytes"
	"context"
	"fmt"
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

	_, _ = repo.CreateUser(context.Background(), "admin@example.com", "admin")
	_, _ = repo.CreateUser(context.Background(), "member@example.com", "member")

	// Use full handler to cover api.gen.go wrappers
	h := Handler(s)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	// Add current user ID to context to cover CanDelete logic
	ctx := context.WithValue(req.Context(), UserIDKey, 1)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK, got %v", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("expected HTML content type, got %v", rr.Header().Get("Content-Type"))
	}

	if !bytes.Contains(rr.Body.Bytes(), []byte("admin@example.com")) {
		t.Error("expected body to contain admin@example.com")
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("member@example.com")) {
		t.Error("expected body to contain member@example.com")
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("role-admin")) {
		t.Error("expected body to contain role-admin badge")
	}

	// Test error case (e.g. database error)
	// We can close the DB to trigger error in ListUsers
	dbConn := repo.DB()
	dbConn.Close()
	rr = httptest.NewRecorder()
	s.GetAdmin(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected InternalServerError with closed DB, got %v", rr.Code)
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

func TestServer_DeleteUsersId(t *testing.T) {
	repo := setupTestDB(t)
	s := NewServer(repo, nil, nil)

	admin, _ := repo.CreateUser(context.Background(), "admin@example.com", "admin")
	member, _ := repo.CreateUser(context.Background(), "member@example.com", "member")

	// Use full handler to cover api.gen.go wrappers
	h := Handler(s)

	tests := []struct {
		name          string
		currentUserID any
		targetUserID  int
		closeDB       bool
		wantStatus    int
	}{
		{
			name:          "admin deletes member",
			currentUserID: admin.ID,
			targetUserID:  member.ID,
			wantStatus:    http.StatusNoContent,
		},
		{
			name:          "admin cannot delete self",
			currentUserID: admin.ID,
			targetUserID:  admin.ID,
			wantStatus:    http.StatusForbidden,
		},
		{
			name:          "delete non-existent user",
			currentUserID: admin.ID,
			targetUserID:  999,
			wantStatus:    http.StatusNotFound,
		},
		{
			name:          "unauthorized (no user id in context)",
			currentUserID: nil,
			targetUserID:  member.ID,
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "database error",
			currentUserID: admin.ID,
			targetUserID:  member.ID,
			closeDB:       true,
			wantStatus:    http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-setup DB for each subtest if we are closing it
			if tt.closeDB {
				repo = setupTestDB(t)
				s = NewServer(repo, nil, nil)
				admin, _ = repo.CreateUser(context.Background(), "admin@example.com", "admin")
				member, _ = repo.CreateUser(context.Background(), "member@example.com", "member")
				dbConn := repo.DB()
				dbConn.Close()
				h = Handler(s)
			}

			req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/users/%d", tt.targetUserID), nil)
			if tt.currentUserID != nil {
				ctx := context.WithValue(req.Context(), UserIDKey, tt.currentUserID)
				req = req.WithContext(ctx)
			}

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %v, got %v", tt.wantStatus, rr.Code)
			}

			if tt.wantStatus == http.StatusNoContent {
				_, err := repo.GetUserByID(context.Background(), tt.targetUserID)
				if err == nil {
					t.Error("expected user to be deleted from database")
				}
			}
		})
	}
}

func TestUnimplemented_Admin(t *testing.T) {
	u := Unimplemented{}
	rr := httptest.NewRecorder()
	u.GetAdmin(rr, nil)
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected NotImplemented, got %v", rr.Code)
	}

	rr = httptest.NewRecorder()
	u.DeleteUsersId(rr, nil, 1)
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected NotImplemented, got %v", rr.Code)
	}
}
