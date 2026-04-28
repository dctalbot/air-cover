package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"air-cover/internal/models"
	"air-cover/internal/spinitron"
)

type MockShowsService struct{}

func (m *MockShowsService) GetShowsPage(ctx context.Context, page int) (spinitron.ShowsPage, error) {
	return spinitron.ShowsPage{
		Items:    []spinitron.Show{{ID: "1", Title: "Test Show"}},
		NextPage: nil,
	}, nil
}

func TestServer_PostSubRequests(t *testing.T) {
	repo := setupTestDB(t)
	u, _ := repo.CreateUser(context.Background(), "test@example.com")
	s := NewServer(repo, nil, &MockShowsService{})

	tests := []struct {
		name       string
		formData   url.Values
		userID     any
		wantStatus int
	}{
		{
			name: "valid request",
			formData: url.Values{
				"show":       {"1"},
				"start_time": {"2026-05-01T10:00"},
				"end_time":   {"2026-05-01T12:00"},
				"notes":      {"Help please!"},
			},
			userID:     u.ID,
			wantStatus: http.StatusSeeOther,
		},
		{
			name: "missing user id",
			formData: url.Values{
				"show":       {"1"},
				"start_time": {"2026-05-01T10:00"},
				"end_time":   {"2026-05-01T12:00"},
			},
			userID:     nil,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "invalid show id",
			formData: url.Values{
				"show":       {"abc"},
				"start_time": {"2026-05-01T10:00"},
				"end_time":   {"2026-05-01T12:00"},
			},
			userID:     u.ID,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "invalid start time",
			formData: url.Values{
				"show":       {"1"},
				"start_time": {"invalid"},
				"end_time":   {"2026-05-01T12:00"},
			},
			userID:     u.ID,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "invalid end time",
			formData: url.Values{
				"show":       {"1"},
				"start_time": {"2026-05-01T10:00"},
				"end_time":   {"invalid"},
			},
			userID:     u.ID,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/sub-requests", nil)
			req.PostForm = tt.formData
			if tt.userID != nil {
				ctx := context.WithValue(req.Context(), UserIDKey, tt.userID)
				req = req.WithContext(ctx)
			}
			rr := httptest.NewRecorder()
			s.PostSubRequests(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %v, got %v", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestServer_Get(t *testing.T) {
	repo := setupTestDB(t)
	s := NewServer(repo, nil, nil)

	t.Run("unauthenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		s.Get(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected OK, got %v", rr.Code)
		}
	})

	t.Run("authenticated", func(t *testing.T) {
		u, _ := repo.CreateUser(context.Background(), "test@example.com")
		_ = repo.CreateSession(context.Background(), "sid", "stoken", u.ID, time.Now().Add(1*time.Hour))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken"})
		rr := httptest.NewRecorder()
		s.Get(rr, req)
		if rr.Code != http.StatusFound {
			t.Errorf("expected redirect, got %v", rr.Code)
		}
	})

	t.Run("stale session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "stale"})
		rr := httptest.NewRecorder()
		s.Get(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected OK, got %v", rr.Code)
		}
		// check if cookie is cleared
		found := false
		for _, c := range rr.Result().Cookies() {
			if c.Name == "session_id" && c.MaxAge < 0 {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected stale session cookie to be cleared")
		}
	})
}

func TestServer_GetApp(t *testing.T) {
	repo := setupTestDB(t)
	s := NewServer(repo, nil, &MockShowsService{})

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "test@example.com")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.GetApp(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK, got %v", rr.Code)
	}
}

func TestServer_AuthDelegation(t *testing.T) {
	repo := setupTestDB(t)
	auth := NewAuthHandler(repo, &MockSender{})
	s := NewServer(repo, auth, nil)
	// These just call s.auth handlers, so we just check they don't panic and call the right thing (implicitly)
	// Since s.auth is not nil, it should try to handle it.
	// We'll use a nil repo in setup to trigger errors if they go too far.

	t.Run("PostAuthLogin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(`{}`))
		rr := httptest.NewRecorder()
		s.PostAuthLogin(rr, req)
		if rr.Code == http.StatusNotImplemented {
			t.Error("expected not implemented to be overridden")
		}
	})

	t.Run("PostAuthLogout", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		rr := httptest.NewRecorder()
		s.PostAuthLogout(rr, req)
		if rr.Code == http.StatusNotImplemented {
			t.Error("expected not implemented to be overridden")
		}
	})

	t.Run("GetAuthVerify", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auth/verify?token=foo", nil)
		rr := httptest.NewRecorder()
		s.GetAuthVerify(rr, req, GetAuthVerifyParams{Token: "foo"})
		if rr.Code == http.StatusNotImplemented {
			t.Error("expected not implemented to be overridden")
		}
	})
}

func TestServer_GetHealth(t *testing.T) {
	s := NewServer(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	s.GetHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK, got %v", rr.Code)
	}
	if rr.Body.String() != "OK" {
		t.Errorf("expected OK body, got %v", rr.Body.String())
	}
}

func TestServer_DeleteSubRequestsId(t *testing.T) {
	repo := setupTestDB(t)
	u1, _ := repo.CreateUser(context.Background(), "user1@example.com")
	u2, _ := repo.CreateUser(context.Background(), "user2@example.com")
	s := NewServer(repo, nil, nil)

	sr := &models.SubRequest{
		ID:        "sr1",
		ShowID:    1,
		UserID:    u1.ID,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(1 * time.Hour),
		Status:    "open",
	}
	_ = repo.CreateSubRequest(context.Background(), sr)

	tests := []struct {
		name       string
		id         string
		userID     any
		wantStatus int
	}{
		{
			name:       "delete own request",
			id:         "sr1",
			userID:     u1.ID,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "delete other request",
			id:         "sr1", // need to recreate it because first test deletes it
			userID:     u2.ID,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "delete non-existent",
			id:         "nonexistent",
			userID:     u1.ID,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unauthorized",
			id:         "sr1",
			userID:     nil,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Recreate if deleted
			if tt.name != "delete own request" {
				_ = repo.CreateSubRequest(context.Background(), sr)
			}

			req := httptest.NewRequest(http.MethodDelete, "/sub-requests/"+tt.id, nil)
			if tt.userID != nil {
				ctx := context.WithValue(req.Context(), UserIDKey, tt.userID)
				req = req.WithContext(ctx)
			}
			rr := httptest.NewRecorder()
			s.DeleteSubRequestsId(rr, req, tt.id)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %v, got %v", tt.wantStatus, rr.Code)
			}
		})
	}
}
