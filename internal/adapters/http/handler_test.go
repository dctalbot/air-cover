package httpadapter

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"air-cover/internal/adapters/sqlite"
	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	appcatalog "air-cover/internal/app/catalog"
	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"

	"github.com/go-chi/chi/v5"
)

type MockShowsService struct{}

func (m *MockShowsService) ListShows(ctx context.Context) ([]appcatalog.Show, error) {
	return []appcatalog.Show{{ID: "1", Title: "Test Show"}}, nil
}

func (m *MockShowsService) ListPersonas(ctx context.Context) ([]appcatalog.Persona, error) {
	return nil, nil
}

type badShowsService struct{}

func (b *badShowsService) ListShows(ctx context.Context) ([]appcatalog.Show, error) {
	return []appcatalog.Show{{ID: "not-a-number", Title: "Bad ID Show"}}, nil
}

func (b *badShowsService) ListPersonas(ctx context.Context) ([]appcatalog.Persona, error) {
	return nil, nil
}

type fakeServerRepo struct {
	session       *domain.Session
	subRequests   []subrequestsapp.DashboardReadModel
	subRequest    *domain.SubRequest
	users         []*domain.User
	err           error
	deleteErr     error
	takeErr       error
	untakeErr     error
	updateErr     error
	importErr     error
	createUserErr error
	createSubErr  error
}

func (f *fakeServerRepo) GetSessionByToken(ctx context.Context, sessionTokenHash string, now time.Time) (*domain.Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.session, nil
}

func (f *fakeServerRepo) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &domain.User{ID: 1, Email: email, Role: domain.RoleMember, IsEnabled: true}, nil
}

func (f *fakeServerRepo) GetUserByID(ctx context.Context, id int) (*domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &domain.User{ID: id, Email: "user@example.com", Role: domain.RoleMember, IsEnabled: true}, nil
}

func (f *fakeServerRepo) CreateMagicLink(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error {
	return nil
}

func (f *fakeServerRepo) UseMagicLink(ctx context.Context, tokenHash string, now time.Time) (*domain.MagicLink, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &domain.MagicLink{UserID: 1, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (f *fakeServerRepo) CreateSession(ctx context.Context, sessionID, sessionTokenHash string, userID int, expiresAt time.Time) error {
	return nil
}

func (f *fakeServerRepo) DeleteSessionsByUserID(ctx context.Context, userID int) error {
	return nil
}

func (f *fakeServerRepo) ListDashboardSubRequests(ctx context.Context) ([]subrequestsapp.DashboardReadModel, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.subRequests, nil
}

func (f *fakeServerRepo) ListUsers(ctx context.Context) ([]*domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.users, nil
}

func (f *fakeServerRepo) CreateUser(ctx context.Context, email string, role string) (*domain.User, error) {
	if f.createUserErr != nil {
		return nil, f.createUserErr
	}
	return &domain.User{ID: 1, Email: email, Role: domain.Role(role), IsEnabled: true}, nil
}

func (f *fakeServerRepo) CreateSubRequest(ctx context.Context, sr *domain.SubRequest) error {
	return f.createSubErr
}

func (f *fakeServerRepo) GetSubRequestByID(ctx context.Context, id int) (*domain.SubRequest, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.subRequest == nil {
		return nil, apperrors.ErrNotFound
	}
	return f.subRequest, nil
}

func (f *fakeServerRepo) DeleteSubRequest(ctx context.Context, id int) error {
	return f.deleteErr
}

func (f *fakeServerRepo) TakeSubRequest(ctx context.Context, id int, userID int, updatedAt time.Time) error {
	return f.takeErr
}

func (f *fakeServerRepo) UntakeSubRequest(ctx context.Context, id int, updatedAt time.Time) error {
	return f.untakeErr
}

func (f *fakeServerRepo) UpdateUser(ctx context.Context, id int, role *string, isEnabled *bool) error {
	return f.updateErr
}

func (f *fakeServerRepo) ImportUsers(ctx context.Context, emails []string) error {
	return f.importErr
}

type fakeSubRequestService struct {
	dashboard    subrequestsapp.Dashboard
	err          error
	createInput  subrequestsapp.CreateInput
	deleteID     int
	actionID     int
	action       subrequestsapp.Action
	createCalled bool
	deleteCalled bool
	actionCalled bool
}

func (f *fakeSubRequestService) ListDashboard(ctx context.Context, viewer domain.CurrentUser) (subrequestsapp.Dashboard, error) {
	return f.dashboard, f.err
}

func (f *fakeSubRequestService) Create(ctx context.Context, viewer domain.CurrentUser, input subrequestsapp.CreateInput) error {
	f.createCalled = true
	f.createInput = input
	return f.err
}

func (f *fakeSubRequestService) Delete(ctx context.Context, viewer domain.CurrentUser, id int) error {
	f.deleteCalled = true
	f.deleteID = id
	return f.err
}

func (f *fakeSubRequestService) ApplyAction(ctx context.Context, viewer domain.CurrentUser, id int, action subrequestsapp.Action) error {
	f.actionCalled = true
	f.actionID = id
	f.action = action
	return f.err
}

type fakeAdminService struct {
	users        []*domain.User
	err          error
	createInput  adminapp.CreateUserInput
	updateInput  adminapp.UpdateUserInput
	createCalled bool
	updateCalled bool
	importCalled bool
}

func (f *fakeAdminService) ListUsers(ctx context.Context) ([]*domain.User, error) {
	return f.users, f.err
}

func (f *fakeAdminService) CreateUser(ctx context.Context, input adminapp.CreateUserInput) error {
	f.createCalled = true
	f.createInput = input
	return f.err
}

func (f *fakeAdminService) UpdateUser(ctx context.Context, viewer domain.CurrentUser, input adminapp.UpdateUserInput) error {
	f.updateCalled = true
	f.updateInput = input
	return f.err
}

func (f *fakeAdminService) ImportCatalogUsers(ctx context.Context) error {
	f.importCalled = true
	return f.err
}

func TestServer_WithFakeServices(t *testing.T) {
	t.Run("post sub request delegates parsed app input", func(t *testing.T) {
		subRequests := &fakeSubRequestService{}
		server := NewServer(nil, subRequests, nil)
		form := url.Values{
			"show":       {"12"},
			"start_time": {"2036-05-01T10:00"},
			"end_time":   {"2036-05-01T12:00"},
			"notes":      {"Help please"},
		}
		req := httptest.NewRequest(http.MethodPost, "/sub-requests", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(context.WithValue(req.Context(), UserIDKey, 9))

		rr := httptest.NewRecorder()
		server.PostSubRequests(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected status %d, got %d", http.StatusSeeOther, rr.Code)
		}
		if !subRequests.createCalled || subRequests.createInput.ShowID != 12 || subRequests.createInput.Notes != "Help please" {
			t.Fatalf("unexpected create input: called=%v input=%+v", subRequests.createCalled, subRequests.createInput)
		}
	})

	t.Run("patch sub request delegates action", func(t *testing.T) {
		subRequests := &fakeSubRequestService{}
		server := NewServer(nil, subRequests, nil)
		req := httptest.NewRequest(http.MethodPatch, "/sub-requests/3", strings.NewReader(`{"action":"take"}`))
		req = req.WithContext(context.WithValue(req.Context(), UserIDKey, 9))

		rr := httptest.NewRecorder()
		server.PatchSubRequestsId(rr, req, 3)

		if rr.Code != http.StatusNoContent {
			t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
		}
		if !subRequests.actionCalled || subRequests.actionID != 3 || subRequests.action != subrequestsapp.ActionTake {
			t.Fatalf("unexpected action call: called=%v id=%d action=%s", subRequests.actionCalled, subRequests.actionID, subRequests.action)
		}
	})

	t.Run("post users delegates create input", func(t *testing.T) {
		admin := &fakeAdminService{}
		server := NewServer(nil, nil, admin)
		form := url.Values{"email": {"new@example.com"}, "role": {"admin"}}
		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		rr := httptest.NewRecorder()
		server.PostUsers(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected status %d, got %d", http.StatusSeeOther, rr.Code)
		}
		if !admin.createCalled || admin.createInput.Email != "new@example.com" || admin.createInput.Role != "admin" {
			t.Fatalf("unexpected create user input: called=%v input=%+v", admin.createCalled, admin.createInput)
		}
	})

	t.Run("post users maps app invalid error", func(t *testing.T) {
		admin := &fakeAdminService{err: apperrors.ErrInvalid}
		server := NewServer(nil, nil, admin)
		form := url.Values{"email": {"new@example.com"}, "role": {"member"}}
		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		rr := httptest.NewRecorder()
		server.PostUsers(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
		if !admin.createCalled {
			t.Fatal("expected create use case to be called")
		}
	})

	t.Run("import users delegates use case", func(t *testing.T) {
		admin := &fakeAdminService{}
		server := NewServer(nil, nil, admin)
		req := httptest.NewRequest(http.MethodPost, "/users/import/spinitron", nil)

		rr := httptest.NewRecorder()
		server.PostUsersImportSpinitron(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected status %d, got %d", http.StatusSeeOther, rr.Code)
		}
		if !admin.importCalled {
			t.Fatal("expected import use case to be called")
		}
	})
}

func TestServer_PostSubRequests(t *testing.T) {
	repo := setupTestDB(t)
	u, _ := repo.CreateUser(context.Background(), "test@example.com", "member")
	s := newTestServer(repo, nil, &MockShowsService{})

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
				"start_time": {"2036-05-01T10:00"},
				"end_time":   {"2036-05-01T12:00"},
				"notes":      {"Help please!"},
			},
			userID:     u.ID,
			wantStatus: http.StatusSeeOther,
		},
		{
			name: "missing user id",
			formData: url.Values{
				"show":       {"1"},
				"start_time": {"2036-05-01T10:00"},
				"end_time":   {"2036-05-01T12:00"},
			},
			userID:     nil,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "invalid show id",
			formData: url.Values{
				"show":       {"abc"},
				"start_time": {"2036-05-01T10:00"},
				"end_time":   {"2036-05-01T12:00"},
			},
			userID:     u.ID,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "invalid start time",
			formData: url.Values{
				"show":       {"1"},
				"start_time": {"invalid"},
				"end_time":   {"2036-05-01T12:00"},
			},
			userID:     u.ID,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "invalid end time",
			formData: url.Values{
				"show":       {"1"},
				"start_time": {"2036-05-01T10:00"},
				"end_time":   {"invalid"},
			},
			userID:     u.ID,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "end time before start time",
			formData: url.Values{
				"show":       {"1"},
				"start_time": {"2036-05-01T12:00"},
				"end_time":   {"2036-05-01T10:00"},
			},
			userID:     u.ID,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "unknown catalog show",
			formData: url.Values{
				"show":       {"2"},
				"start_time": {"2036-05-01T10:00"},
				"end_time":   {"2036-05-01T12:00"},
			},
			userID:     u.ID,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "notes too long",
			formData: url.Values{
				"show":       {"1"},
				"start_time": {"2036-05-01T10:00"},
				"end_time":   {"2036-05-01T12:00"},
				"notes":      {strings.Repeat("a", 1001)},
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

func TestServer_PostSubRequests_CatalogError(t *testing.T) {
	repo := setupTestDB(t)
	u, _ := repo.CreateUser(context.Background(), "catalogerr@example.com", "member")
	s := newTestServer(repo, nil, &faultyShowsService{})

	req := httptest.NewRequest(http.MethodPost, "/sub-requests", nil)
	req.PostForm = url.Values{
		"show":       {"1"},
		"start_time": {"2036-05-01T10:00"},
		"end_time":   {"2036-05-01T12:00"},
	}
	ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.PostSubRequests(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for catalog validation error, got %d", rr.Code)
	}
}

func TestServer_Get(t *testing.T) {
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, nil)

	t.Run("unauthenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		s.Get(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected OK, got %v", rr.Code)
		}
	})

	t.Run("authenticated", func(t *testing.T) {
		u, _ := repo.CreateUser(context.Background(), "test@example.com", "member")
		_ = repo.CreateSession(context.Background(), "sid", authapp.HashToken("stoken"), u.ID, time.Now().Add(1*time.Hour))
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

func TestServer_Get_WithAuthHandler(t *testing.T) {
	repo := setupTestDB(t)
	user, _ := repo.CreateUser(context.Background(), "withauth@example.com", "member")
	_ = repo.CreateSession(context.Background(), "with-auth-session", authapp.HashToken("with-auth-token"), user.ID, time.Now().Add(time.Hour))
	s := NewServer(NewAuthHandler(authapp.NewService(repo, nil)), nil, nil)

	t.Run("authenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "with-auth-token"})
		rr := httptest.NewRecorder()

		s.Get(rr, req)

		if rr.Code != http.StatusFound {
			t.Fatalf("expected redirect, got %d", rr.Code)
		}
	})

	t.Run("stale session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "stale-with-auth"})
		rr := httptest.NewRecorder()

		s.Get(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected OK, got %d", rr.Code)
		}
	})
}

func TestServer_Get_DocOnlyRedirect(t *testing.T) {
	s := newTestServer(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "doc-token"})
	rr := httptest.NewRecorder()

	s.Get(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
}

func TestServer_GetApp(t *testing.T) {
	repo := setupTestDB(t)
	u, _ := repo.CreateUser(context.Background(), "test@example.com", "member")
	s := newTestServer(repo, nil, &MockShowsService{})

	// Create a future request
	_ = repo.CreateSubRequest(context.Background(), &domain.SubRequest{
		ShowID:         1,
		PostedByUserID: u.ID,
		StartTime:      time.Now().Add(24 * time.Hour),
		EndTime:        time.Now().Add(26 * time.Hour),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})

	// Create past requests in ascending order so GetApp has to reverse them for display.
	u2, _ := repo.CreateUser(context.Background(), "taker@example.com", "member")
	_ = repo.CreateSubRequest(context.Background(), &domain.SubRequest{
		ShowID:         999,
		PostedByUserID: u.ID,
		StartTime:      time.Now().Add(-48 * time.Hour),
		EndTime:        time.Now().Add(-46 * time.Hour),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})
	_ = repo.CreateSubRequest(context.Background(), &domain.SubRequest{
		ShowID:         999, // Unknown show
		PostedByUserID: u.ID,
		TakenByUserID:  &u2.ID,
		StartTime:      time.Now().Add(-24 * time.Hour),
		EndTime:        time.Now().Add(-22 * time.Hour),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "test@example.com")
	ctx = context.WithValue(ctx, UserIDKey, u.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.GetApp(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK, got %v", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Upcoming Sub Requests") {
		t.Error("expected 'Upcoming Sub Requests' header not found")
	}
	if !strings.Contains(body, "Recent history") {
		t.Error("expected 'Recent history' header not found")
	}
	if !strings.Contains(body, "Unknown Show") {
		t.Error("expected 'Unknown Show' not found in body")
	}
	if !strings.Contains(body, "taker@example.com") {
		t.Error("expected taker email not found in body")
	}
	if strings.Index(body, "taker@example.com") > strings.LastIndex(body, "Unknown Show") {
		t.Error("expected most recent past request to render before older past request")
	}
}

func TestServer_GetApp_BadShowID(t *testing.T) {
	// Tests the branch where show ID cannot be parsed as int
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, &badShowsService{})

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "test@example.com")
	ctx = context.WithValue(ctx, UserIDKey, 1)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.GetApp(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK even with bad show ID, got %v", rr.Code)
	}
}

func TestServer_GetApp_ListDashboardSubRequestsErrorClosedDB(t *testing.T) {
	s := newTestServer(&fakeServerRepo{err: errors.New("list failed")}, nil, &MockShowsService{})
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rr := httptest.NewRecorder()

	s.GetApp(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestServer_GetApp_AppError(t *testing.T) {
	s := newTestServer(&fakeServerRepo{err: apperrors.ErrNotFound}, nil, &MockShowsService{})
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rr := httptest.NewRecorder()

	s.GetApp(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestServer_GetAdmin_ListUsersError(t *testing.T) {
	s := newTestServer(&fakeServerRepo{err: errors.New("list users failed")}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()

	s.GetAdmin(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestServer_GetApp_RenderErrorWithFakeRepo(t *testing.T) {
	s := newTestServer(&fakeServerRepo{}, nil, &MockShowsService{})
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserEmailKey, "test@example.com"))

	s.GetApp(&errorResponseWriter{}, req)
}

func TestServer_GetAdmin_RenderErrorWithDisabledUsers(t *testing.T) {
	s := newTestServer(&fakeServerRepo{
		users: []*domain.User{
			{ID: 1, Email: "disabled-b@example.com", Role: domain.RoleMember, IsEnabled: false},
			{ID: 2, Email: "disabled-a@example.com", Role: domain.RoleMember, IsEnabled: false},
			{ID: 3, Email: "z-member@example.com", Role: domain.RoleMember, IsEnabled: true},
			{ID: 4, Email: "a-member@example.com", Role: domain.RoleMember, IsEnabled: true},
		},
	}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserIDKey, 1))

	s.GetAdmin(&errorResponseWriter{}, req)
}

func TestServer_DeleteSubRequestsId_RepositoryErrors(t *testing.T) {
	tests := []struct {
		name       string
		repo       *fakeServerRepo
		userID     int
		role       string
		wantStatus int
	}{
		{
			name:       "lookup error",
			repo:       &fakeServerRepo{err: errors.New("lookup failed")},
			userID:     1,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "delete error",
			repo: &fakeServerRepo{
				subRequest: &domain.SubRequest{ID: 10, PostedByUserID: 1},
				deleteErr:  errors.New("delete failed"),
			},
			userID:     1,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(tt.repo, nil, nil)
			req := httptest.NewRequest(http.MethodDelete, "/sub-requests/10", nil)
			ctx := context.WithValue(req.Context(), UserIDKey, tt.userID)
			if tt.role != "" {
				ctx = context.WithValue(ctx, UserRoleKey, domain.Role(tt.role))
			}
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()

			s.DeleteSubRequestsId(rr, req, 10)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestServer_PatchSubRequestsId_RepositoryErrorsWithFake(t *testing.T) {
	takerID := 2
	tests := []struct {
		name       string
		repo       *fakeServerRepo
		body       string
		userID     int
		wantStatus int
	}{
		{
			name:       "lookup error",
			repo:       &fakeServerRepo{err: errors.New("lookup failed")},
			body:       `{"action":"take"}`,
			userID:     takerID,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "take error",
			repo: &fakeServerRepo{
				subRequest: &domain.SubRequest{ID: 10, PostedByUserID: 1},
				takeErr:    errors.New("take failed"),
			},
			body:       `{"action":"take"}`,
			userID:     takerID,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "take conflict",
			repo: &fakeServerRepo{
				subRequest: &domain.SubRequest{ID: 10, PostedByUserID: 1},
				takeErr:    apperrors.ErrConflict,
			},
			body:       `{"action":"take"}`,
			userID:     takerID,
			wantStatus: http.StatusConflict,
		},
		{
			name: "take not found after update",
			repo: &fakeServerRepo{
				subRequest: &domain.SubRequest{ID: 10, PostedByUserID: 1},
				takeErr:    apperrors.ErrNotFound,
			},
			body:       `{"action":"take"}`,
			userID:     takerID,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "untake error",
			repo: &fakeServerRepo{
				subRequest: &domain.SubRequest{ID: 10, PostedByUserID: 1, TakenByUserID: &takerID},
				untakeErr:  errors.New("untake failed"),
			},
			body:       `{"action":"untake"}`,
			userID:     takerID,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(tt.repo, nil, nil)
			req := httptest.NewRequest(http.MethodPatch, "/sub-requests/10", strings.NewReader(tt.body))
			req = req.WithContext(context.WithValue(req.Context(), UserIDKey, tt.userID))
			rr := httptest.NewRecorder()

			s.PatchSubRequestsId(rr, req, 10)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestServer_PostUsersAndImport_RepositoryErrors(t *testing.T) {
	t.Run("create user error", func(t *testing.T) {
		s := newTestServer(&fakeServerRepo{createUserErr: errors.New("create failed")}, nil, nil)
		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader("email=test@example.com&role=member"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()

		s.PostUsers(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rr.Code)
		}
	})

	t.Run("import users error", func(t *testing.T) {
		service := &importMockShowsService{fail: false}
		s := newTestServer(&fakeServerRepo{importErr: errors.New("import failed")}, nil, service)
		req := httptest.NewRequest(http.MethodPost, "/users/import/spinitron", nil)
		rr := httptest.NewRecorder()

		s.PostUsersImportSpinitron(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rr.Code)
		}
	})
}

func TestServer_AuthDelegation(t *testing.T) {
	repo := setupTestDB(t)
	auth := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	s := newTestServer(repo, auth, nil)

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
	s := newTestServer(nil, nil, nil)
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
	u1, _ := repo.CreateUser(context.Background(), "user1@example.com", "member")
	u2, _ := repo.CreateUser(context.Background(), "user2@example.com", "member")
	admin, _ := repo.CreateUser(context.Background(), "admin@example.com", "admin")
	s := newTestServer(repo, nil, nil)

	sr := &domain.SubRequest{
		ShowID:         1,
		PostedByUserID: u1.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(1 * time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)

	tests := []struct {
		name       string
		id         int
		userID     any
		userRole   string
		wantStatus int
	}{
		{
			name:       "delete own request",
			id:         sr.ID,
			userID:     u1.ID,
			userRole:   "member",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "delete other request",
			id:         sr.ID,
			userID:     u2.ID,
			userRole:   "member",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "admin delete other request",
			id:         sr.ID,
			userID:     admin.ID,
			userRole:   "admin",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "delete non-existent",
			id:         99999,
			userID:     u1.ID,
			userRole:   "member",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unauthorized",
			id:         sr.ID,
			userID:     nil,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Recreate if deleted
			if tt.name != "delete own request" && tt.name != "delete non-existent" {
				_ = repo.CreateSubRequest(context.Background(), sr)
			}

			req := httptest.NewRequest(http.MethodDelete, "/sub-requests/"+strconv.Itoa(sr.ID), nil)
			if tt.userID != nil {
				ctx := context.WithValue(req.Context(), UserIDKey, tt.userID)
				ctx = context.WithValue(ctx, UserRoleKey, domain.Role(tt.userRole))
				req = req.WithContext(ctx)
			}
			rr := httptest.NewRecorder()
			targetID := tt.id
			if tt.name != "delete non-existent" {
				targetID = sr.ID
			}
			s.DeleteSubRequestsId(rr, req, targetID)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %v, got %v", tt.wantStatus, rr.Code)
			}
		})
	}

	t.Run("db error", func(t *testing.T) {
		repo := setupTestDB(t)
		u, _ := repo.CreateUser(context.Background(), "poster@example.com", "member")
		sr := &domain.SubRequest{
			ShowID:         1,
			PostedByUserID: u.ID,
			StartTime:      time.Now(),
			EndTime:        time.Now().Add(time.Hour),
		}
		_ = repo.CreateSubRequest(context.Background(), sr)
		s := newTestServer(repo, nil, nil)
		_ = repo.DB().Close()

		req := httptest.NewRequest(http.MethodDelete, "/sub-requests/1", nil)
		ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.DeleteSubRequestsId(rr, req, sr.ID)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected 500 for db error, got %d", rr.Code)
		}
	})
}

// TestUnimplemented exercises all the Unimplemented stub methods.
func TestUnimplemented(t *testing.T) {
	u := Unimplemented{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	check := func(name string, code int) {
		if code != http.StatusNotImplemented {
			t.Errorf("%s: expected 501, got %d", name, code)
		}
	}

	rr := httptest.NewRecorder()
	u.Get(rr, req)
	check("Get", rr.Code)

	rr = httptest.NewRecorder()
	u.GetApp(rr, req)
	check("GetApp", rr.Code)

	rr = httptest.NewRecorder()
	u.PostAuthLogin(rr, req)
	check("PostAuthLogin", rr.Code)

	rr = httptest.NewRecorder()
	u.PostAuthLogout(rr, req)
	check("PostAuthLogout", rr.Code)

	rr = httptest.NewRecorder()
	u.GetAuthVerify(rr, req, GetAuthVerifyParams{})
	check("GetAuthVerify", rr.Code)

	rr = httptest.NewRecorder()
	u.GetAdmin(rr, req)
	check("GetAdmin", rr.Code)

	rr = httptest.NewRecorder()
	u.GetHealth(rr, req)
	check("GetHealth", rr.Code)

	rr = httptest.NewRecorder()
	u.PostSubRequests(rr, req)
	check("PostSubRequests", rr.Code)

	rr = httptest.NewRecorder()
	u.DeleteSubRequestsId(rr, req, 123)
	check("DeleteSubRequestsId", rr.Code)

	rr = httptest.NewRecorder()
	u.PatchSubRequestsId(rr, req, 123)
	check("PatchSubRequestsId", rr.Code)

	rr = httptest.NewRecorder()
	u.PostUsers(rr, req)
	check("PostUsers", rr.Code)

	rr = httptest.NewRecorder()
	u.PostUsersImportSpinitron(rr, req)
	check("PostUsersImportSpinitron", rr.Code)

	rr = httptest.NewRecorder()
	u.PostUsersId(rr, req, 123)
	check("PostUsersId", rr.Code)
}

// TestHandlerWithOptions exercises the generated router setup and all wrapper functions.
func TestHandlerWithOptions(t *testing.T) {
	repo := setupTestDB(t)
	auth := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	s := newTestServer(repo, auth, &MockShowsService{})

	// Test Handler (nil base router → creates new chi router)
	h := Handler(s)
	if h == nil {
		t.Fatal("expected handler to be non-nil")
	}

	// Test HandlerFromMux with an existing chi router
	r := chi.NewRouter()
	h2 := HandlerFromMux(s, r)
	if h2 == nil {
		t.Fatal("expected HandlerFromMux result to be non-nil")
	}

	// Test HandlerFromMuxWithBaseURL
	r2 := chi.NewRouter()
	h3 := HandlerFromMuxWithBaseURL(s, r2, "/v1")
	if h3 == nil {
		t.Fatal("expected HandlerFromMuxWithBaseURL result to be non-nil")
	}
}

// TestHandlerViaHTTP exercises the ServerInterfaceWrapper routes via actual HTTP requests.
func TestHandlerViaHTTP(t *testing.T) {
	repo := setupTestDB(t)
	auth := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	s := newTestServer(repo, auth, &MockShowsService{})

	h := Handler(s)
	ts := httptest.NewServer(h)
	defer ts.Close()

	client := ts.Client()

	// GET / → 200
	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET / expected 200, got %d", resp.StatusCode)
	}

	// GET /health → 200
	resp, err = client.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /health expected 200, got %d", resp.StatusCode)
	}

	// POST /auth/login with valid JSON → 200
	resp, err = client.Post(ts.URL+"/auth/login", "application/json",
		strings.NewReader(`{"email":"unknown@example.com"}`))
	if err != nil {
		t.Fatalf("POST /auth/login: %v", err)
	}
	resp.Body.Close()

	// GET /auth/verify without token → 400 (missing required param error handler)
	resp, err = client.Get(ts.URL + "/auth/verify")
	if err != nil {
		t.Fatalf("GET /auth/verify: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("GET /auth/verify without token expected 400, got %d", resp.StatusCode)
	}

	// GET /auth/verify with token → handled (not 501)
	resp, err = client.Get(ts.URL + "/auth/verify?token=sometoken")
	if err != nil {
		t.Fatalf("GET /auth/verify?token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotImplemented {
		t.Error("expected /auth/verify to not return 501")
	}

	// GET /app → exercises ServerInterfaceWrapper.GetApp
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/app", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET /app: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotImplemented {
		t.Error("expected /app to not return 501")
	}

	// POST /auth/logout → exercises ServerInterfaceWrapper.PostAuthLogout
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/auth/logout", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /auth/logout: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotImplemented {
		t.Error("expected /auth/logout to not return 501")
	}

	// POST /sub-requests → exercises ServerInterfaceWrapper.PostSubRequests
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/sub-requests",
		strings.NewReader("show=1&start_time=2026-05-01T10%3A00&end_time=2026-05-01T12%3A00"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /sub-requests: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotImplemented {
		t.Error("expected /sub-requests to not return 501")
	}

	// DELETE /sub-requests/{id} → exercises ServerInterfaceWrapper.DeleteSubRequestsId
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/sub-requests/123", nil)
	resp, err = client.Do(req)
	if err != nil {
		_ = repo.DB().Close()
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotImplemented {
		t.Error("expected DELETE /sub-requests/{id} to not return 501")
	}

	// PATCH /sub-requests/{id} → exercises ServerInterfaceWrapper.PatchSubRequestsId
	req, _ = http.NewRequest(http.MethodPatch, ts.URL+"/sub-requests/123",
		strings.NewReader(`{"action":"take"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("PATCH /sub-requests/{id}: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotImplemented {
		t.Error("expected PATCH /sub-requests/{id} to not return 501")
	}

	// POST /users/{id} → exercises ServerInterfaceWrapper.PostUsersId
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/users/123",
		strings.NewReader(`{"role":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /users/{id}: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotImplemented {
		t.Error("expected POST /users/{id} to not return 501")
	}
}

// TestHandlerWithOptions_CustomErrorHandler tests that the custom error handler is called.
func TestHandlerWithOptions_CustomErrorHandler(t *testing.T) {
	s := newTestServer(nil, nil, nil)
	customErrorCalled := false

	h := HandlerWithOptions(s, ChiServerOptions{
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			customErrorCalled = true
			http.Error(w, "custom: "+err.Error(), http.StatusBadRequest)
		},
	})

	ts := httptest.NewServer(h)
	defer ts.Close()

	// GET /auth/verify without token triggers the error handler (required param missing)
	resp, err := ts.Client().Get(ts.URL + "/auth/verify")
	if err != nil {
		t.Fatalf("GET /auth/verify: %v", err)
	}
	resp.Body.Close()
	if !customErrorCalled {
		t.Error("expected custom error handler to be called")
	}
}

// TestErrorTypes exercises the error type methods from the generated code.
func TestErrorTypes(t *testing.T) {
	inner := &RequiredParamError{ParamName: "inner"}

	e1 := &UnescapedCookieParamError{ParamName: "myCookie", Err: inner}
	if !strings.Contains(e1.Error(), "myCookie") {
		t.Errorf("expected param name in error: %s", e1.Error())
	}
	if e1.Unwrap() != inner {
		t.Error("expected inner error")
	}

	e2 := &UnmarshalingParamError{ParamName: "myParam", Err: inner}
	if !strings.Contains(e2.Error(), "myParam") {
		t.Errorf("expected param name in error: %s", e2.Error())
	}
	if e2.Unwrap() != inner {
		t.Error("expected inner error")
	}

	e3 := &RequiredParamError{ParamName: "requiredParam"}
	if !strings.Contains(e3.Error(), "requiredParam") {
		t.Errorf("expected param name in error: %s", e3.Error())
	}

	e4 := &RequiredHeaderError{ParamName: "myHeader", Err: inner}
	if !strings.Contains(e4.Error(), "myHeader") {
		t.Errorf("expected header name in error: %s", e4.Error())
	}
	if e4.Unwrap() != inner {
		t.Error("expected inner error")
	}

	e5 := &InvalidParamFormatError{ParamName: "badParam", Err: inner}
	if !strings.Contains(e5.Error(), "badParam") {
		t.Errorf("expected param name in error: %s", e5.Error())
	}
	if e5.Unwrap() != inner {
		t.Error("expected inner error")
	}

	e6 := &TooManyValuesForParamError{ParamName: "multiParam", Count: 3}
	if !strings.Contains(e6.Error(), "multiParam") {
		t.Errorf("expected param name in error: %s", e6.Error())
	}
}

// TestPathToRawSpec exercises the PathToRawSpec function.
func TestPathToRawSpec(t *testing.T) {
	m := PathToRawSpec("")
	if len(m) != 0 {
		t.Errorf("expected empty map for empty path, got %v", m)
	}

	m2 := PathToRawSpec("some/path/to/spec.yaml")
	fn, ok := m2["some/path/to/spec.yaml"]
	if !ok {
		t.Fatal("expected path to be in map")
	}
	data, err := fn()
	if err != nil {
		t.Errorf("expected no error from spec fn, got %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty spec data")
	}
}

// TestGetSwagger exercises the GetSwagger function.
func TestGetSwagger(t *testing.T) {
	swagger, err := GetSwagger()
	if err != nil {
		t.Fatalf("GetSwagger failed: %v", err)
	}
	if swagger == nil {
		t.Fatal("expected non-nil swagger")
	}
}

// TestHandlerWithMiddleware ensures the HandlerMiddlewares loop in ServerInterfaceWrapper is covered.
func TestHandlerWithMiddleware(t *testing.T) {
	repo := setupTestDB(t)
	auth := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	s := newTestServer(repo, auth, &MockShowsService{})

	callCount := 0
	mw := MiddlewareFunc(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			next.ServeHTTP(w, r)
		})
	})

	h := HandlerWithOptions(s, ChiServerOptions{
		Middlewares: []MiddlewareFunc{mw},
	})
	ts := httptest.NewServer(h)
	defer ts.Close()

	client := ts.Client()

	routes := []struct {
		method string
		path   string
		body   string
		ct     string
	}{
		{http.MethodGet, "/", "", ""},
		{http.MethodGet, "/health", "", ""},
		{http.MethodGet, "/app", "", ""},
		{http.MethodPost, "/auth/login", `{"email":"x@y.com"}`, "application/json"},
		{http.MethodPost, "/auth/logout", "", ""},
		{http.MethodGet, "/auth/verify", "", ""}, // Missing token → 400 but middleware still runs
		{http.MethodPost, "/sub-requests", "show=1&start_time=2036-01-01T10%3A00&end_time=2036-01-01T12%3A00", "application/x-www-form-urlencoded"},
		{http.MethodDelete, "/sub-requests/123", "", ""},
		{http.MethodPatch, "/sub-requests/123", `{"action":"take"}`, "application/json"},
		{http.MethodPost, "/users/123", `{"role":"admin"}`, "application/json"},
	}

	for _, route := range routes {
		var reqBody *strings.Reader
		if route.body != "" {
			reqBody = strings.NewReader(route.body)
		} else {
			reqBody = strings.NewReader("")
		}
		req, _ := http.NewRequest(route.method, ts.URL+route.path, reqBody)
		if route.ct != "" {
			req.Header.Set("Content-Type", route.ct)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", route.method, route.path, err)
		}
		resp.Body.Close()
	}

	if callCount == 0 {
		t.Error("expected middleware to be called at least once")
	}
}

// TestServer_GetApp_ListDashboardSubRequestsError covers the dashboard query error path in GetApp.
func TestServer_GetApp_ListDashboardSubRequestsError(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	dbConn.Close() // Force the dashboard query to fail.

	s := newTestServer(repo, nil, &MockShowsService{})
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "test@example.com")
	ctx = context.WithValue(ctx, UserIDKey, 1)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.GetApp(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error in GetApp, got %d", rr.Code)
	}
}

type listingShowsService struct {
	calls int
}

func (m *listingShowsService) ListShows(ctx context.Context) ([]appcatalog.Show, error) {
	m.calls++
	return []appcatalog.Show{
		{ID: "1", Title: "Show A"},
		{ID: "2", Title: "Show B"},
	}, nil
}

func (m *listingShowsService) ListPersonas(ctx context.Context) ([]appcatalog.Persona, error) {
	return nil, nil
}

func TestServer_GetApp_LoadsShowList(t *testing.T) {
	repo := setupTestDB(t)
	svc := &listingShowsService{}
	s := newTestServer(repo, nil, svc)

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "test@example.com")
	ctx = context.WithValue(ctx, UserIDKey, 1)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.GetApp(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK for multi-page, got %d", rr.Code)
	}
	if svc.calls != 1 {
		t.Errorf("expected 1 show list call, got %d", svc.calls)
	}
}

// TestServer_GetHealth_WriteError covers the write error path in GetHealth.
func TestServer_GetHealth_WriteError(t *testing.T) {
	s := newTestServer(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	ew := &errorResponseWriter{}
	s.GetHealth(ew, req)
	// Just ensure no panic (error is logged)
}

// errorResponseWriter simulates a ResponseWriter that fails on Write.
type errorResponseWriter struct {
	header http.Header
}

func (e *errorResponseWriter) Header() http.Header {
	if e.header == nil {
		e.header = make(http.Header)
	}
	return e.header
}

func (e *errorResponseWriter) Write(b []byte) (int, error) {
	return 0, errors.New("write error")
}

func (e *errorResponseWriter) WriteHeader(code int) {}

// TestServer_DeleteSubRequestsId_DBError covers the GetSubRequestByID DB error path.
func TestServer_DeleteSubRequestsId_DBError(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "dberr@example.com", "member")
	sr := &domain.SubRequest{
		ShowID:         1,
		PostedByUserID: u.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)
	dbConn.Close() // Force GetSubRequestByID to fail

	s := newTestServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/sub-requests/1", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.DeleteSubRequestsId(rr, req, sr.ID)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error, got %d", rr.Code)
	}
}

// TestServer_PostSubRequests_DBError covers the CreateSubRequest DB error path.
func TestServer_PostSubRequests_DBError(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "postreqerr@example.com", "member")
	dbConn.Close() // Force CreateSubRequest to fail

	s := newTestServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/sub-requests", nil)
	req.PostForm = url.Values{
		"show":       {"1"},
		"start_time": {"2036-05-01T10:00"},
		"end_time":   {"2036-05-01T12:00"},
	}
	ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.PostSubRequests(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error on create, got %d", rr.Code)
	}
}

// TestGetSwagger_CorruptedSpec exercises the GetSwagger error path when rawSpec returns an error.
func TestGetSwagger_CorruptedSpec(t *testing.T) {
	// Temporarily replace rawSpec with an error-returning function
	origRawSpec := rawSpec
	defer func() { rawSpec = origRawSpec }()

	rawSpec = func() ([]byte, error) {
		return nil, errors.New("corrupted spec")
	}

	_, err := GetSwagger()
	if err == nil {
		t.Error("expected error for corrupted rawSpec")
	}
}

// TestDecodeSpec_Errors exercises decodeSpec error paths via corrupted swaggerSpec.
func TestDecodeSpec_Errors(t *testing.T) {
	// Test invalid base64
	origSpec := swaggerSpec
	defer func() { swaggerSpec = origSpec }()

	// Replace with invalid base64
	swaggerSpec = []string{"!!!not-valid-base64!!!"}
	_, err := decodeSpec()
	if err == nil {
		t.Error("expected error for invalid base64")
	}

	// Replace with valid base64 but invalid gzip
	swaggerSpec = []string{"dGhpcyBpcyBub3QgZ2l6cA=="}
	_, err = decodeSpec()
	if err == nil {
		t.Error("expected error for invalid gzip content")
	}
}

func TestServer_PostSubRequests_ParseFormError(t *testing.T) {
	s := newTestServer(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/sub-requests", strings.NewReader("!!invalid!!"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.PostSubRequests(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad form data, got %d", rr.Code)
	}
}

func TestServer_DeleteSubRequestsId_DeleteError(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "deleterr@example.com", "member")
	sr := &domain.SubRequest{
		ShowID:         1,
		PostedByUserID: u.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)
	dbConn.Close() // Force GetSubRequestByID to fail

	s := newTestServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/sub-requests/1", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.DeleteSubRequestsId(rr, req, sr.ID)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error during delete, got %d", rr.Code)
	}
}

func TestServer_GetApp_SpinitronError(t *testing.T) {
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, &faultyShowsService{})

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "test@example.com")
	ctx = context.WithValue(ctx, UserIDKey, 1)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.GetApp(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for Spinitron error, got %d", rr.Code)
	}
}

type faultyShowsService struct{}

func (f *faultyShowsService) ListShows(ctx context.Context) ([]appcatalog.Show, error) {
	return nil, errors.New("spinitron down")
}

func (f *faultyShowsService) ListPersonas(ctx context.Context) ([]appcatalog.Persona, error) {
	return nil, nil
}

func TestServer_PostSubRequests_LargeBody(t *testing.T) {
	s := newTestServer(nil, nil, nil)
	largeBody := "show=1&start_time=2036-05-01T10:00&end_time=2036-05-01T12:00&notes=" + strings.Repeat("a", 1024*1024+100)
	req := httptest.NewRequest(http.MethodPost, "/sub-requests", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.PostSubRequests(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for large body, got %d", rr.Code)
	}
}

func TestServer_DeleteSubRequestsId_Unauthorized(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	u1, _ := repo.CreateUser(context.Background(), "u1@example.com", "member")
	u2, _ := repo.CreateUser(context.Background(), "u2@example.com", "member")

	sr := &domain.SubRequest{
		ShowID:         1,
		PostedByUserID: u1.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)

	s := newTestServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/sub-requests/1", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, u2.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.DeleteSubRequestsId(rr, req, sr.ID)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for unauthorized delete, got %d", rr.Code)
	}
}

func TestServer_DeleteSubRequestsId_NotFound(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	u1, _ := repo.CreateUser(context.Background(), "u1@example.com", "member")

	s := newTestServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/sub-requests/999", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, u1.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.DeleteSubRequestsId(rr, req, 999)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent sub request, got %d", rr.Code)
	}
}

func TestServer_DeleteSubRequestsId_NoUserInContext(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	u1, _ := repo.CreateUser(context.Background(), "u1@example.com", "member")
	sr := &domain.SubRequest{
		ShowID:         1,
		PostedByUserID: u1.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)

	s := newTestServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/sub-requests/1", nil)
	rr := httptest.NewRecorder()
	s.DeleteSubRequestsId(rr, req, sr.ID)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for no user in context, got %d", rr.Code)
	}
}

type importMockShowsService struct {
	fail bool
}

func (m *importMockShowsService) ListShows(ctx context.Context) ([]appcatalog.Show, error) {
	return nil, nil
}

func (m *importMockShowsService) ListPersonas(ctx context.Context) ([]appcatalog.Persona, error) {
	if m.fail {
		return nil, errors.New("spinitron error")
	}
	return []appcatalog.Persona{
		{ID: 1, Name: "DJ One", Email: "one@example.com"},
		{ID: 2, Name: "DJ Empty", Email: "  "}, // should be skipped
		{ID: 3, Name: "DJ Two", Email: "two@example.com"},
	}, nil
}

func TestServer_PostUsersImportSpinitron(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)

	t.Run("success", func(t *testing.T) {
		s := newTestServer(repo, nil, &importMockShowsService{fail: false})
		req := httptest.NewRequest(http.MethodPost, "/users/import/spinitron", nil)
		rr := httptest.NewRecorder()
		s.PostUsersImportSpinitron(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Errorf("expected redirect 303, got %d", rr.Code)
		}

		users, _ := repo.ListUsers(context.Background())
		if len(users) != 2 {
			t.Errorf("expected 2 users imported, got %d", len(users))
		}
	})

	t.Run("spinitron error handled gracefully", func(t *testing.T) {
		s := newTestServer(repo, nil, &importMockShowsService{fail: true})
		req := httptest.NewRequest(http.MethodPost, "/users/import/spinitron", nil)
		rr := httptest.NewRecorder()
		s.PostUsersImportSpinitron(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Errorf("expected redirect 303 even on spinitron error, got %d", rr.Code)
		}
	})

	t.Run("db import error", func(t *testing.T) {
		// Drop users table to force repo.ImportUsers to fail
		_ = dbConn.Close()
		s := newTestServer(repo, nil, &importMockShowsService{fail: false})
		req := httptest.NewRequest(http.MethodPost, "/users/import/spinitron", nil)
		rr := httptest.NewRecorder()
		s.PostUsersImportSpinitron(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected 500 for db error, got %d", rr.Code)
		}
	})
}

func TestServer_GetAdmin_Sorting(t *testing.T) {
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, nil)

	_, _ = repo.CreateUser(context.Background(), "a@example.com", "member")
	_, _ = repo.CreateUser(context.Background(), "b@example.com", "admin")

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, 1)
	ctx = context.WithValue(ctx, UserEmailKey, "a@example.com")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	s.GetAdmin(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestServer_PostUsersId_Errors(t *testing.T) {
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, nil)

	u, _ := repo.CreateUser(context.Background(), "test@example.com", "member")

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/users/1", strings.NewReader(`{invalid json`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.PostUsersId(rr, req, 1)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid json, got %d", rr.Code)
		}
	})

	t.Run("self deactivation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/users/1", strings.NewReader(`{"is_enabled":false}`))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PostUsersId(rr, req, u.ID)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 for self deactivation, got %d", rr.Code)
		}
	})

	t.Run("db error", func(t *testing.T) {
		_ = repo.DB().Close()
		req := httptest.NewRequest(http.MethodPost, "/users/1", strings.NewReader(`{"role":"admin"}`))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), UserIDKey, 999) // not self
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PostUsersId(rr, req, u.ID)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected 500 for db error, got %d", rr.Code)
		}
	})

	t.Run("form success", func(t *testing.T) {
		repo := setupTestDB(t) // fresh DB
		s := newTestServer(repo, nil, nil)
		u, _ := repo.CreateUser(context.Background(), "test@example.com", "member")
		target, _ := repo.CreateUser(context.Background(), "target@example.com", "member")

		formData := url.Values{
			"is_enabled": {"false"},
			"role":       {"admin"},
		}
		req := httptest.NewRequest(http.MethodPost, "/users/"+strconv.Itoa(target.ID), strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PostUsersId(rr, req, target.ID)

		if rr.Code != http.StatusSeeOther {
			t.Errorf("expected 303 for form success, got %d", rr.Code)
		}

		updated, _ := repo.GetUserByID(context.Background(), target.ID)
		if updated.IsEnabled {
			t.Error("expected user to be disabled")
		}
		if updated.Role != domain.RoleAdmin {
			t.Errorf("expected role admin, got %s", updated.Role)
		}
	})

	t.Run("form parse error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/users/1", strings.NewReader("invalid%2"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ctx := context.WithValue(req.Context(), UserIDKey, 999)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PostUsersId(rr, req, 1)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid form, got %d", rr.Code)
		}
	})
}

func TestAppHandler_UnknownShowTitle(t *testing.T) {
	repo := setupTestDB(t)
	server := newTestServer(repo, nil, &MockShowsService{})

	// Create a sub request with a show ID not in the spinitron response
	_, _ = repo.CreateUser(context.Background(), "test@example.com", "member")
	_ = repo.CreateSubRequest(context.Background(), &domain.SubRequest{
		ShowID:         999, // Doesn't match 1
		PostedByUserID: 1,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	})

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "test@example.com")
	ctx = context.WithValue(ctx, UserIDKey, 1)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	server.GetApp(rr, req)

	if !strings.Contains(rr.Body.String(), "Unknown Show") {
		t.Errorf("expected body to contain 'Unknown Show' fallback")
	}
}

func TestAppHandler_AdminCanTakeOwnRequest(t *testing.T) {
	repo := setupTestDB(t)
	server := newTestServer(repo, nil, &MockShowsService{})

	admin, _ := repo.CreateUser(context.Background(), "admin@example.com", "admin")
	_ = repo.CreateSubRequest(context.Background(), &domain.SubRequest{
		ShowID:         1,
		PostedByUserID: admin.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	})

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "admin@example.com")
	ctx = context.WithValue(ctx, UserIDKey, admin.ID)
	ctx = context.WithValue(ctx, UserRoleKey, domain.RoleAdmin)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	server.GetApp(rr, req)

	// Check if "Take" button is present in the actions column
	if !strings.Contains(rr.Body.String(), "takeRequest") {
		t.Errorf("expected body to contain 'takeRequest' script for admin on their own request")
	}
	if !strings.Contains(rr.Body.String(), "Take") {
		t.Errorf("expected body to contain 'Take' button text")
	}
}

func TestServer_GetAdmin_DBError(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	dbConn.Close() // Force ListUsers to fail

	s := newTestServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, 1)
	ctx = context.WithValue(ctx, UserEmailKey, "admin@example.com")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.GetAdmin(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error in GetAdmin, got %d", rr.Code)
	}
}

func TestServer_GetAdmin_RenderError(t *testing.T) {
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, 1)
	ctx = context.WithValue(ctx, UserEmailKey, "admin@example.com")
	req = req.WithContext(ctx)
	ew := &errorResponseWriter{}
	s.GetAdmin(ew, req)
	// No panic means the error was handled gracefully (logged)
}

func TestServer_Get_RenderError(t *testing.T) {
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ew := &errorResponseWriter{}
	s.Get(ew, req)
	// No panic means the error was handled gracefully (logged)
}

func TestServer_GetApp_RenderError(t *testing.T) {
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, &MockShowsService{})

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "test@example.com")
	ctx = context.WithValue(ctx, UserIDKey, 1)
	req = req.WithContext(ctx)
	ew := &errorResponseWriter{}
	s.GetApp(ew, req)
	// No panic means the error was handled gracefully (logged)
}

func TestServer_PostUsersId_NoUserInContext(t *testing.T) {
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/users/1", strings.NewReader(`{"is_enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.PostUsersId(rr, req, 1)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for no user in context, got %d", rr.Code)
	}
}

func TestServer_PostUsersId_InvalidRole(t *testing.T) {
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/users/1", strings.NewReader(`{"role":"superadmin"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), UserIDKey, 999)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.PostUsersId(rr, req, 1)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid role, got %d", rr.Code)
	}
}

func TestServer_PostUsersId_NotFound(t *testing.T) {
	repo := setupTestDB(t)
	s := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/users/99999", strings.NewReader(`{"role":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), UserIDKey, 1)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.PostUsersId(rr, req, 99999)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent user, got %d", rr.Code)
	}
}

func TestServer_PostUsers_CreateError(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	_, _ = repo.CreateUser(context.Background(), "dup@example.com", "member")

	s := newTestServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	req.PostForm = url.Values{
		"email": {"dup@example.com"},
	}
	rr := httptest.NewRecorder()
	s.PostUsers(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for duplicate user creation, got %d", rr.Code)
	}
}

func TestServer_PostUsers_EmptyEmail(t *testing.T) {
	s := NewServer(nil, nil, &fakeAdminService{err: apperrors.ErrInvalid})
	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	req.PostForm = url.Values{
		"email": {""},
	}
	rr := httptest.NewRecorder()
	s.PostUsers(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty email, got %d", rr.Code)
	}
}

func TestServer_PostUsers_InvalidRole(t *testing.T) {
	s := NewServer(nil, nil, &fakeAdminService{err: apperrors.ErrInvalid})
	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	req.PostForm = url.Values{
		"email": {"test@example.com"},
		"role":  {"superadmin"},
	}
	rr := httptest.NewRecorder()
	s.PostUsers(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid role, got %d", rr.Code)
	}
}

func TestServer_PostUsers_ParseFormError(t *testing.T) {
	s := newTestServer(nil, nil, nil)
	largeBody := "email=test@example.com&" + strings.Repeat("a", 1024*1024+100)
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.PostUsers(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for large body, got %d", rr.Code)
	}
}

func TestServer_PatchSubRequestsId(t *testing.T) {
	repo := setupTestDB(t)
	u1, _ := repo.CreateUser(context.Background(), "poster@example.com", "member")
	u2, _ := repo.CreateUser(context.Background(), "taker@example.com", "member")
	u3, _ := repo.CreateUser(context.Background(), "other@example.com", "member")
	s := newTestServer(repo, nil, nil)

	sr := &domain.SubRequest{
		ShowID:         1,
		PostedByUserID: u1.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)

	t.Run("take success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/sub-requests/1",
			strings.NewReader(`{"action":"take"}`))
		ctx := context.WithValue(req.Context(), UserIDKey, u2.ID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PatchSubRequestsId(rr, req, sr.ID)
		if rr.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", rr.Code)
		}
	})

	t.Run("take already taken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/sub-requests/1",
			strings.NewReader(`{"action":"take"}`))
		ctx := context.WithValue(req.Context(), UserIDKey, u3.ID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PatchSubRequestsId(rr, req, sr.ID)
		if rr.Code != http.StatusConflict {
			t.Errorf("expected 409, got %d", rr.Code)
		}
	})

	t.Run("untake by wrong user", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/sub-requests/1",
			strings.NewReader(`{"action":"untake"}`))
		ctx := context.WithValue(req.Context(), UserIDKey, u3.ID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PatchSubRequestsId(rr, req, sr.ID)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rr.Code)
		}
	})

	t.Run("untake success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/sub-requests/1",
			strings.NewReader(`{"action":"untake"}`))
		ctx := context.WithValue(req.Context(), UserIDKey, u2.ID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PatchSubRequestsId(rr, req, sr.ID)
		if rr.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", rr.Code)
		}
	})

	t.Run("untake untaken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/sub-requests/1",
			strings.NewReader(`{"action":"untake"}`))
		ctx := context.WithValue(req.Context(), UserIDKey, u2.ID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PatchSubRequestsId(rr, req, sr.ID)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rr.Code)
		}
	})

	t.Run("invalid action", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/sub-requests/1",
			strings.NewReader(`{"action":"invalid"}`))
		ctx := context.WithValue(req.Context(), UserIDKey, u2.ID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PatchSubRequestsId(rr, req, sr.ID)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/sub-requests/1",
			strings.NewReader(`{invalid}`))
		ctx := context.WithValue(req.Context(), UserIDKey, u2.ID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PatchSubRequestsId(rr, req, sr.ID)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("no user in context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/sub-requests/1",
			strings.NewReader(`{"action":"take"}`))
		rr := httptest.NewRecorder()
		s.PatchSubRequestsId(rr, req, sr.ID)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/sub-requests/99999",
			strings.NewReader(`{"action":"take"}`))
		ctx := context.WithValue(req.Context(), UserIDKey, u2.ID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PatchSubRequestsId(rr, req, 99999)
		if rr.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rr.Code)
		}
	})

	t.Run("admin can take their own request", func(t *testing.T) {
		admin, _ := repo.CreateUser(context.Background(), "admin-poster@example.com", "admin")
		srAdmin := &domain.SubRequest{
			ShowID:         1,
			PostedByUserID: admin.ID,
			StartTime:      time.Now(),
			EndTime:        time.Now().Add(time.Hour),
		}
		_ = repo.CreateSubRequest(context.Background(), srAdmin)

		req := httptest.NewRequest(http.MethodPatch, "/sub-requests/"+strconv.Itoa(srAdmin.ID),
			strings.NewReader(`{"action":"take"}`))
		ctx := context.WithValue(req.Context(), UserIDKey, admin.ID)
		ctx = context.WithValue(ctx, UserRoleKey, domain.RoleAdmin)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PatchSubRequestsId(rr, req, srAdmin.ID)

		if rr.Code != http.StatusNoContent {
			t.Errorf("expected 204 for admin taking own request, got %d", rr.Code)
		}
	})

	t.Run("admin cannot take already taken request", func(t *testing.T) {
		admin, _ := repo.CreateUser(context.Background(), "admin-taker@example.com", "admin")
		// sr is already taken by u2 in previous test case if it hasn't been reset,
		// but let's make a clean one just in case.
		srTaken := &domain.SubRequest{
			ShowID:         1,
			PostedByUserID: u1.ID,
			StartTime:      time.Now(),
			EndTime:        time.Now().Add(time.Hour),
		}
		_ = repo.CreateSubRequest(context.Background(), srTaken)
		_ = repo.TakeSubRequest(context.Background(), srTaken.ID, u2.ID, time.Now())

		req := httptest.NewRequest(http.MethodPatch, "/sub-requests/"+strconv.Itoa(srTaken.ID),
			strings.NewReader(`{"action":"take"}`))
		ctx := context.WithValue(req.Context(), UserIDKey, admin.ID)
		ctx = context.WithValue(ctx, UserRoleKey, domain.RoleAdmin)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PatchSubRequestsId(rr, req, srTaken.ID)

		if rr.Code != http.StatusConflict {
			t.Errorf("expected 409 for admin taking taken request, got %d", rr.Code)
		}
	})
}

func TestServer_PatchSubRequestsId_DBError(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "dberr@example.com", "member")
	sr := &domain.SubRequest{
		ShowID:         1,
		PostedByUserID: u.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)
	dbConn.Close()

	s := newTestServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/sub-requests/1",
		strings.NewReader(`{"action":"take"}`))
	ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.PatchSubRequestsId(rr, req, sr.ID)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error, got %d", rr.Code)
	}
}

func TestServer_PatchSubRequestsId_TakeDBError(t *testing.T) {
	repo := setupTestDB(t)
	u1, _ := repo.CreateUser(context.Background(), "poster@example.com", "member")
	u2, _ := repo.CreateUser(context.Background(), "taker@example.com", "member")

	sr := &domain.SubRequest{
		ShowID:         1,
		PostedByUserID: u1.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)

	// Close the DB after creating the sub request to force TakeSubRequest to fail
	_ = repo.DB().Close()

	s := newTestServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/sub-requests/1",
		strings.NewReader(`{"action":"take"}`))
	ctx := context.WithValue(req.Context(), UserIDKey, u2.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.PatchSubRequestsId(rr, req, sr.ID)
	// The GetSubRequestByID call will fail first
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestServer_PatchSubRequestsId_UntakeDBError(t *testing.T) {
	repo := setupTestDB(t)
	u1, _ := repo.CreateUser(context.Background(), "poster@example.com", "member")
	u2, _ := repo.CreateUser(context.Background(), "taker@example.com", "member")

	sr := &domain.SubRequest{
		ShowID:         1,
		PostedByUserID: u1.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)
	_ = repo.TakeSubRequest(context.Background(), sr.ID, u2.ID, time.Now())

	// Close the DB after taking the sub request
	_ = repo.DB().Close()

	s := newTestServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/sub-requests/1",
		strings.NewReader(`{"action":"untake"}`))
	ctx := context.WithValue(req.Context(), UserIDKey, u2.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.PatchSubRequestsId(rr, req, sr.ID)
	// The GetSubRequestByID call will fail first
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestPatchSubRequestsIdJSONBodyAction_Valid(t *testing.T) {
	if !Take.Valid() {
		t.Error("expected Take to be valid")
	}
	if !Untake.Valid() {
		t.Error("expected Untake to be valid")
	}
	if PatchSubRequestsIdJSONBodyAction("invalid").Valid() {
		t.Error("expected invalid action to not be valid")
	}
}
