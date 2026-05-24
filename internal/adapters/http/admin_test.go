package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"air-cover/internal/adapters/sqlite"
	authapp "air-cover/internal/app/auth"
	"air-cover/internal/domain"
)

func TestAuthHandler_RequireAdmin(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(authapp.NewService(repo, nil))

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
		{"member access", "member", http.StatusFound},
		{"no role access", "", http.StatusFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			if tt.role != "" {
				ctx := context.WithValue(req.Context(), UserRoleKey, domain.Role(tt.role))
				req = req.WithContext(ctx)
			}
			rr := httptest.NewRecorder()
			mw.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %v, got %v", tt.wantStatus, rr.Code)
			}
			if tt.wantStatus == http.StatusFound && rr.Header().Get("Location") != "/" {
				t.Errorf("expected redirect to /, got %v", rr.Header().Get("Location"))
			}
		})
	}

	t.Run("json client", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req.Header.Set("Accept", "application/json")
		ctx := context.WithValue(req.Context(), UserRoleKey, domain.RoleMember)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 for JSON client, got %v", rr.Code)
		}
	})
}

func TestServer_GetAdmin(t *testing.T) {
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, nil)

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

func TestServer_GetAdmin_DeactivatedUsersSortedByEmail(t *testing.T) {
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, nil)

	_, _ = repo.CreateUser(context.Background(), "admin@example.com", "admin")
	zUser, _ := repo.CreateUser(context.Background(), "zeta@example.com", "member")
	aUser, _ := repo.CreateUser(context.Background(), "alpha@example.com", "admin")
	enabled := false
	if err := repo.UpdateUser(context.Background(), zUser.ID, nil, &enabled); err != nil {
		t.Fatalf("failed to deactivate zeta user: %v", err)
	}
	if err := repo.UpdateUser(context.Background(), aUser.ID, nil, &enabled); err != nil {
		t.Fatalf("failed to deactivate alpha user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, 1)
	ctx = context.WithValue(ctx, UserEmailKey, "admin@example.com")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	s.GetAdmin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected OK, got %v", rr.Code)
	}

	output := rr.Body.String()
	alphaIndex := strings.Index(output, "alpha@example.com")
	zetaIndex := strings.Index(output, "zeta@example.com")
	if alphaIndex == -1 || zetaIndex == -1 {
		t.Fatalf("expected deactivated users in admin output: %s", output)
	}
	if alphaIndex > zetaIndex {
		t.Error("expected deactivated users to be sorted by email")
	}
}

func TestServer_GetApp_AdminLinkVisibility(t *testing.T) {
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, &MockShowsService{})

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
			ctx = context.WithValue(ctx, UserRoleKey, domain.Role(tt.role))
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
	s := newTestServer(repo, nil, nil)
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
			name:       "missing role defaults to member",
			email:      "norole@example.com",
			role:       "",
			wantStatus: http.StatusSeeOther,
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
				expectedRole := tt.role
				if expectedRole == "" {
					expectedRole = "member"
				}
				if user.Role != domain.Role(expectedRole) {
					t.Errorf("expected role %s, got %s", expectedRole, user.Role)
				}
			}
		})
	}

	// Test database error
	t.Run("database error", func(t *testing.T) {
		repo = setupTestDB(t)
		s = newTestServer(repo, nil, nil)
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

func TestServer_PostUsersId(t *testing.T) {
	tests := []struct {
		name           string
		currentUser    string // email of current user
		targetUser     string // email of target user
		targetIDOffset int    // if target user is non-existent
		body           string
		wantStatus     int
		check          func(t *testing.T, repo *sqlite.Repository, targetID int)
	}{
		{
			name:        "admin deactivates member",
			currentUser: "admin@example.com",
			targetUser:  "member@example.com",
			body:        `{"is_enabled": false}`,
			wantStatus:  http.StatusNoContent,
			check: func(t *testing.T, repo *sqlite.Repository, targetID int) {
				user, _ := repo.GetUserByID(context.Background(), targetID)
				if user.IsEnabled != false {
					t.Error("expected is_enabled false")
				}
				if user.Role != domain.RoleMember {
					t.Error("expected role to remain member")
				}
			},
		},
		{
			name:        "admin promotes member",
			currentUser: "admin@example.com",
			targetUser:  "member@example.com",
			body:        `{"role": "admin"}`,
			wantStatus:  http.StatusNoContent,
			check: func(t *testing.T, repo *sqlite.Repository, targetID int) {
				user, _ := repo.GetUserByID(context.Background(), targetID)
				if user.Role != domain.RoleAdmin {
					t.Error("expected role admin")
				}
				if user.IsEnabled != true {
					t.Error("expected is_enabled to remain true")
				}
			},
		},
		{
			name:        "admin cannot deactivate self",
			currentUser: "admin@example.com",
			targetUser:  "admin@example.com",
			body:        `{"is_enabled": false}`,
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "admin can change own role",
			currentUser: "admin@example.com",
			targetUser:  "admin@example.com",
			body:        `{"role": "member"}`,
			wantStatus:  http.StatusNoContent,
			check: func(t *testing.T, repo *sqlite.Repository, targetID int) {
				user, _ := repo.GetUserByID(context.Background(), targetID)
				if user.Role != domain.RoleMember {
					t.Error("expected role member")
				}
			},
		},
		{
			name:        "invalid role",
			currentUser: "admin@example.com",
			targetUser:  "member@example.com",
			body:        `{"role": "superadmin"}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:           "deactivate non-existent user",
			currentUser:    "admin@example.com",
			targetIDOffset: 999,
			body:           `{"is_enabled": false}`,
			wantStatus:     http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestDB(t)
			s := newTestServer(repo, nil, nil)
			h := Handler(s)

			admin, _ := repo.CreateUser(context.Background(), "admin@example.com", "admin")
			member, _ := repo.CreateUser(context.Background(), "member@example.com", "member")

			var currentID int
			if tt.currentUser == "admin@example.com" {
				currentID = admin.ID
			} else {
				currentID = member.ID
			}

			var targetID int
			if tt.targetIDOffset != 0 {
				targetID = tt.targetIDOffset
			} else if tt.targetUser == "admin@example.com" {
				targetID = admin.ID
			} else {
				targetID = member.ID
			}

			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/users/%d", targetID), bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), UserIDKey, currentID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %v, got %v", tt.wantStatus, rr.Code)
			}

			if tt.wantStatus == http.StatusNoContent && tt.check != nil {
				tt.check(t, repo, targetID)
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
	u.PostUsersId(rr, nil, 1)
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected NotImplemented, got %v", rr.Code)
	}

	rr = httptest.NewRecorder()
	u.PostUsers(rr, nil)
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected NotImplemented, got %v", rr.Code)
	}
}
