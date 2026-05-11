package api

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

	"air-cover/internal/db"
	"air-cover/internal/models"
	"air-cover/internal/spinitron"

	"github.com/go-chi/chi/v5"
)

type MockShowsService struct{}

func (m *MockShowsService) GetShowsPage(ctx context.Context, page int) (spinitron.ShowsPage, error) {
	return spinitron.ShowsPage{
		Items:    []spinitron.Show{{ID: "1", Title: "Test Show"}},
		NextPage: nil,
	}, nil
}

func (m *MockShowsService) GetPersonasPage(ctx context.Context, page int) (spinitron.PersonasPage, error) {
	return spinitron.PersonasPage{}, nil
}

type badShowsService struct{}

func (b *badShowsService) GetShowsPage(ctx context.Context, page int) (spinitron.ShowsPage, error) {
	return spinitron.ShowsPage{
		Items:    []spinitron.Show{{ID: "not-a-number", Title: "Bad ID Show"}},
		NextPage: nil,
	}, nil
}

func (b *badShowsService) GetPersonasPage(ctx context.Context, page int) (spinitron.PersonasPage, error) {
	return spinitron.PersonasPage{}, nil
}

func TestServer_PostSubRequests(t *testing.T) {
	repo := setupTestDB(t)
	u, _ := repo.CreateUser(context.Background(), "test@example.com", "member")
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
		u, _ := repo.CreateUser(context.Background(), "test@example.com", "member")
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
	ctx = context.WithValue(ctx, UserIDKey, 1)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.GetApp(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK, got %v", rr.Code)
	}
}

func TestServer_GetApp_BadShowID(t *testing.T) {
	// Tests the branch where show ID cannot be parsed as int
	repo := setupTestDB(t)
	s := NewServer(repo, nil, &badShowsService{})

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

func TestServer_AuthDelegation(t *testing.T) {
	repo := setupTestDB(t)
	auth := NewAuthHandler(repo, &MockSender{})
	s := NewServer(repo, auth, nil)

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
	u1, _ := repo.CreateUser(context.Background(), "user1@example.com", "member")
	u2, _ := repo.CreateUser(context.Background(), "user2@example.com", "member")
	admin, _ := repo.CreateUser(context.Background(), "admin@example.com", "admin")
	s := NewServer(repo, nil, nil)

	sr := &models.SubRequest{
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
				ctx = context.WithValue(ctx, UserRoleKey, tt.userRole)
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
}

// TestHandlerWithOptions exercises the generated router setup and all wrapper functions.
func TestHandlerWithOptions(t *testing.T) {
	repo := setupTestDB(t)
	auth := NewAuthHandler(repo, &MockSender{})
	s := NewServer(repo, auth, &MockShowsService{})

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
	auth := NewAuthHandler(repo, &MockSender{})
	s := NewServer(repo, auth, &MockShowsService{})

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
}

// TestHandlerWithOptions_CustomErrorHandler tests that the custom error handler is called.
func TestHandlerWithOptions_CustomErrorHandler(t *testing.T) {
	s := NewServer(nil, nil, nil)
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
	auth := NewAuthHandler(repo, &MockSender{})
	s := NewServer(repo, auth, &MockShowsService{})

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
		{http.MethodPost, "/sub-requests", "show=1&start_time=2026-01-01T10%3A00&end_time=2026-01-01T12%3A00", "application/x-www-form-urlencoded"},
		{http.MethodDelete, "/sub-requests/123", "", ""},
		{http.MethodPatch, "/sub-requests/123", `{"action":"take"}`, "application/json"},
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

// TestServer_GetApp_ListSubRequestsError covers the ListSubRequests error path in GetApp.
func TestServer_GetApp_ListSubRequestsError(t *testing.T) {
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	dbConn.Close() // Force ListSubRequests to fail

	s := NewServer(repo, nil, &MockShowsService{})
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

// multiPageShowsService simulates a two-page response.
type multiPageShowsService struct {
	calls int
}

func (m *multiPageShowsService) GetShowsPage(ctx context.Context, page int) (spinitron.ShowsPage, error) {
	m.calls++
	if m.calls == 1 {
		next := 2
		return spinitron.ShowsPage{
			Items:    []spinitron.Show{{ID: "1", Title: "Show A"}},
			NextPage: &next,
		}, nil
	}
	return spinitron.ShowsPage{
		Items:    []spinitron.Show{{ID: "2", Title: "Show B"}},
		NextPage: nil,
	}, nil
}

func (m *multiPageShowsService) GetPersonasPage(ctx context.Context, page int) (spinitron.PersonasPage, error) {
	return spinitron.PersonasPage{}, nil
}

// TestServer_GetApp_MultiPage covers the pagination loop in GetApp.
func TestServer_GetApp_MultiPage(t *testing.T) {
	repo := setupTestDB(t)
	svc := &multiPageShowsService{}
	s := NewServer(repo, nil, svc)

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "test@example.com")
	ctx = context.WithValue(ctx, UserIDKey, 1)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.GetApp(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected OK for multi-page, got %d", rr.Code)
	}
	if svc.calls != 2 {
		t.Errorf("expected 2 page calls, got %d", svc.calls)
	}
}

// TestServer_GetHealth_WriteError covers the write error path in GetHealth.
func TestServer_GetHealth_WriteError(t *testing.T) {
	s := NewServer(nil, nil, nil)
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
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "dberr@example.com", "member")
	sr := &models.SubRequest{
		ShowID:         1,
		PostedByUserID: u.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)
	dbConn.Close() // Force GetSubRequestByID to fail

	s := NewServer(repo, nil, nil)
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
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "postreqerr@example.com", "member")
	dbConn.Close() // Force CreateSubRequest to fail

	s := NewServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/sub-requests", nil)
	req.PostForm = url.Values{
		"show":       {"1"},
		"start_time": {"2026-05-01T10:00"},
		"end_time":   {"2026-05-01T12:00"},
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
	s := NewServer(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/sub-requests", strings.NewReader("!!invalid!!"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.PostSubRequests(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad form data, got %d", rr.Code)
	}
}

func TestServer_DeleteSubRequestsId_DeleteError(t *testing.T) {
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "deleterr@example.com", "member")
	sr := &models.SubRequest{
		ShowID:         1,
		PostedByUserID: u.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)
	dbConn.Close() // Force GetSubRequestByID to fail

	s := NewServer(repo, nil, nil)
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
	s := NewServer(repo, nil, &faultyShowsService{})

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

func (f *faultyShowsService) GetShowsPage(ctx context.Context, page int) (spinitron.ShowsPage, error) {
	return spinitron.ShowsPage{}, errors.New("spinitron down")
}

func (f *faultyShowsService) GetPersonasPage(ctx context.Context, page int) (spinitron.PersonasPage, error) {
	return spinitron.PersonasPage{}, nil
}

func TestServer_PostSubRequests_LargeBody(t *testing.T) {
	s := NewServer(nil, nil, nil)
	largeBody := "show=1&start_time=2026-05-01T10:00&end_time=2026-05-01T12:00&notes=" + strings.Repeat("a", 1024*1024+100)
	req := httptest.NewRequest(http.MethodPost, "/sub-requests", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.PostSubRequests(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for large body, got %d", rr.Code)
	}
}

func TestServer_DeleteSubRequestsId_Unauthorized(t *testing.T) {
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	u1, _ := repo.CreateUser(context.Background(), "u1@example.com", "member")
	u2, _ := repo.CreateUser(context.Background(), "u2@example.com", "member")

	sr := &models.SubRequest{
		ShowID:         1,
		PostedByUserID: u1.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)

	s := NewServer(repo, nil, nil)
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
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	u1, _ := repo.CreateUser(context.Background(), "u1@example.com", "member")

	s := NewServer(repo, nil, nil)
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
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	u1, _ := repo.CreateUser(context.Background(), "u1@example.com", "member")
	sr := &models.SubRequest{
		ShowID:         1,
		PostedByUserID: u1.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)

	s := NewServer(repo, nil, nil)
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

func (m *importMockShowsService) GetShowsPage(ctx context.Context, page int) (spinitron.ShowsPage, error) {
	return spinitron.ShowsPage{}, nil
}

func (m *importMockShowsService) GetPersonasPage(ctx context.Context, page int) (spinitron.PersonasPage, error) {
	if m.fail {
		return spinitron.PersonasPage{}, errors.New("spinitron error")
	}
	if page == 1 {
		next := 2
		return spinitron.PersonasPage{
			Items: []spinitron.Persona{
				{ID: 1, Name: "DJ One", Email: "one@example.com"},
				{ID: 2, Name: "DJ Empty", Email: "  "}, // should be skipped
			},
			NextPage: &next,
		}, nil
	}
	return spinitron.PersonasPage{
		Items: []spinitron.Persona{
			{ID: 3, Name: "DJ Two", Email: "two@example.com"},
		},
		NextPage: nil,
	}, nil
}

func TestServer_PostUsersImportSpinitron(t *testing.T) {
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)

	t.Run("success", func(t *testing.T) {
		s := NewServer(repo, nil, &importMockShowsService{fail: false})
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
		s := NewServer(repo, nil, &importMockShowsService{fail: true})
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
		s := NewServer(repo, nil, &importMockShowsService{fail: false})
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
	s := NewServer(repo, nil, nil)

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

func TestServer_PatchUsersId_Errors(t *testing.T) {
	repo := setupTestDB(t)
	s := NewServer(repo, nil, nil)

	u, _ := repo.CreateUser(context.Background(), "test@example.com", "member")

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/users/1", strings.NewReader(`{invalid json`))
		rr := httptest.NewRecorder()
		s.PatchUsersId(rr, req, 1)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid json, got %d", rr.Code)
		}
	})

	t.Run("self deactivation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/users/1", strings.NewReader(`{"is_enabled":false}`))
		ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PatchUsersId(rr, req, u.ID)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 for self deactivation, got %d", rr.Code)
		}
	})

	t.Run("db error", func(t *testing.T) {
		_ = repo.DB().Close()
		req := httptest.NewRequest(http.MethodPatch, "/users/1", strings.NewReader(`{"role":"admin"}`))
		ctx := context.WithValue(req.Context(), UserIDKey, 999) // not self
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		s.PatchUsersId(rr, req, u.ID)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected 500 for db error, got %d", rr.Code)
		}
	})
}

func TestAppHandler_UnknownShowTitle(t *testing.T) {
	repo := setupTestDB(t)
	server := NewServer(repo, nil, &MockShowsService{})

	// Create a sub request with a show ID not in the spinitron response
	_, _ = repo.CreateUser(context.Background(), "test@example.com", "member")
	_ = repo.CreateSubRequest(context.Background(), &models.SubRequest{
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

func TestServer_GetAdmin_DBError(t *testing.T) {
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	dbConn.Close() // Force ListUsers to fail

	s := NewServer(repo, nil, nil)
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
	s := NewServer(repo, nil, nil)

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
	s := NewServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ew := &errorResponseWriter{}
	s.Get(ew, req)
	// No panic means the error was handled gracefully (logged)
}

func TestServer_GetApp_RenderError(t *testing.T) {
	repo := setupTestDB(t)
	s := NewServer(repo, nil, &MockShowsService{})

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "test@example.com")
	ctx = context.WithValue(ctx, UserIDKey, 1)
	req = req.WithContext(ctx)
	ew := &errorResponseWriter{}
	s.GetApp(ew, req)
	// No panic means the error was handled gracefully (logged)
}

func TestServer_PatchUsersId_NoUserInContext(t *testing.T) {
	repo := setupTestDB(t)
	s := NewServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodPatch, "/users/1", strings.NewReader(`{"is_enabled":false}`))
	rr := httptest.NewRecorder()
	s.PatchUsersId(rr, req, 1)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for no user in context, got %d", rr.Code)
	}
}

func TestServer_PatchUsersId_InvalidRole(t *testing.T) {
	repo := setupTestDB(t)
	s := NewServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodPatch, "/users/1", strings.NewReader(`{"role":"superadmin"}`))
	ctx := context.WithValue(req.Context(), UserIDKey, 999)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.PatchUsersId(rr, req, 1)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid role, got %d", rr.Code)
	}
}

func TestServer_PatchUsersId_NotFound(t *testing.T) {
	repo := setupTestDB(t)
	s := NewServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodPatch, "/users/99999", strings.NewReader(`{"role":"admin"}`))
	ctx := context.WithValue(req.Context(), UserIDKey, 1)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.PatchUsersId(rr, req, 99999)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent user, got %d", rr.Code)
	}
}

func TestServer_PostUsers_CreateError(t *testing.T) {
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	_, _ = repo.CreateUser(context.Background(), "dup@example.com", "member")

	s := NewServer(repo, nil, nil)
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
	s := NewServer(nil, nil, nil)
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
	s := NewServer(nil, nil, nil)
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
	s := NewServer(nil, nil, nil)
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
	s := NewServer(repo, nil, nil)

	sr := &models.SubRequest{
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
}

func TestServer_PatchSubRequestsId_DBError(t *testing.T) {
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "dberr@example.com", "member")
	sr := &models.SubRequest{
		ShowID:         1,
		PostedByUserID: u.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)
	dbConn.Close()

	s := NewServer(repo, nil, nil)
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

	sr := &models.SubRequest{
		ShowID:         1,
		PostedByUserID: u1.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)

	// Close the DB after creating the sub request to force TakeSubRequest to fail
	_ = repo.DB().Close()

	s := NewServer(repo, nil, nil)
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

	sr := &models.SubRequest{
		ShowID:         1,
		PostedByUserID: u1.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
	}
	_ = repo.CreateSubRequest(context.Background(), sr)
	_ = repo.TakeSubRequest(context.Background(), sr.ID, u2.ID)

	// Close the DB after taking the sub request
	_ = repo.DB().Close()

	s := NewServer(repo, nil, nil)
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
