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

func TestServer_PostUsers(t *testing.T) {
	repo := setupTestDB(t)
	s := NewServer(repo, nil, nil)
	h := Handler(s)

	tests := []struct {
		name       string
		email      string
		role       string
		wantStatus int
	}{
		{
			name:       "valid user creation",
			email:      "new@example.com",
			role:       "member",
			wantStatus: http.StatusSeeOther,
		},
		{
			name:       "valid admin creation",
			email:      "newadmin@example.com",
			role:       "admin",
			wantStatus: http.StatusSeeOther,
		},
		{
			name:       "missing email",
			email:      "",
			role:       "member",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing role",
			email:      "norole@example.com",
			role:       "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid role",
			email:      "badrole@example.com",
			role:       "superadmin",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := fmt.Sprintf("email=%s&role=%s", tt.email, tt.role)
			req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %v, got %v", tt.wantStatus, rr.Code)
			}

			if tt.wantStatus == http.StatusSeeOther {
				if rr.Header().Get("Location") != "/admin" {
					t.Errorf("expected redirect to /admin, got %v", rr.Header().Get("Location"))
				}
				// Verify user exists
				user, err := repo.GetUserByEmail(context.Background(), tt.email)
				if err != nil {
					t.Errorf("expected user %s to be created, got error %v", tt.email, err)
				}
				if user.Role != tt.role {
					t.Errorf("expected role %s, got %s", tt.role, user.Role)
				}
			}
		})
	}

	// Test database error
	t.Run("database error", func(t *testing.T) {
		repo = setupTestDB(t)
		s = NewServer(repo, nil, nil)
		dbConn := repo.DB()
		dbConn.Close()

		form := "email=error@example.com&role=member"
		req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		rr := httptest.NewRecorder()
		s.PostUsers(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected InternalServerError, got %v", rr.Code)
		}
	})

	// Test form parse error
	t.Run("form parse error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString("invalid%2"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		rr := httptest.NewRecorder()
		s.PostUsers(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected BadRequest for invalid form, got %v", rr.Code)
		}
	})
}

func TestServer_PatchUsersId(t *testing.T) {
	repo := setupTestDB(t)
	s := NewServer(repo, nil, nil)
	h := Handler(s)

	admin, _ := repo.CreateUser(context.Background(), "admin@example.com", "admin")
	member, _ := repo.CreateUser(context.Background(), "member@example.com", "member")

	tests := []struct {
		name          string
		currentUserID int
		targetUserID  int
		isEnabled     bool
		wantStatus    int
	}{
		{
			name:          "admin bans member",
			currentUserID: admin.ID,
			targetUserID:  member.ID,
			isEnabled:     false,
			wantStatus:    http.StatusNoContent,
		},
		{
			name:          "admin reinstates member",
			currentUserID: admin.ID,
			targetUserID:  member.ID,
			isEnabled:     true,
			wantStatus:    http.StatusNoContent,
		},
		{
			name:          "admin cannot ban self",
			currentUserID: admin.ID,
			targetUserID:  admin.ID,
			isEnabled:     false,
			wantStatus:    http.StatusForbidden,
		},
		{
			name:          "ban non-existent user",
			currentUserID: admin.ID,
			targetUserID:  999,
			isEnabled:     false,
			wantStatus:    http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"is_enabled": %v}`, tt.isEnabled)
			req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/users/%d", tt.targetUserID), bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), UserIDKey, tt.currentUserID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %v, got %v", tt.wantStatus, rr.Code)
			}

			if tt.wantStatus == http.StatusNoContent {
				user, _ := repo.GetUserByID(context.Background(), tt.targetUserID)
				if user.IsEnabled != tt.isEnabled {
					t.Errorf("expected is_enabled %v, got %v", tt.isEnabled, user.IsEnabled)
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
	u.PatchUsersId(rr, nil, 1)
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected NotImplemented, got %v", rr.Code)
	}

	rr = httptest.NewRecorder()
	u.PostUsers(rr, nil)
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected NotImplemented, got %v", rr.Code)
	}
}
