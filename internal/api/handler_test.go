package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

type badShowsService struct{}

func (b *badShowsService) GetShowsPage(ctx context.Context, page int) (spinitron.ShowsPage, error) {
	return spinitron.ShowsPage{
		Items:    []spinitron.Show{{ID: "not-a-number", Title: "Bad ID Show"}},
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
			id:         "sr1",
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
	u.DeleteSubRequestsId(rr, req, "test-id")
	check("DeleteSubRequestsId", rr.Code)
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
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/sub-requests/some-id", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("DELETE /sub-requests/some-id: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotImplemented {
		t.Error("expected DELETE /sub-requests/{id} to not return 501")
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
		{http.MethodDelete, "/sub-requests/some-id", "", ""},
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
	u, _ := repo.CreateUser(context.Background(), "dberr@example.com")
	sr := &models.SubRequest{
		ID:        "sr-db-err",
		ShowID:    1,
		UserID:    u.ID,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Hour),
		Status:    "open",
	}
	_ = repo.CreateSubRequest(context.Background(), sr)
	dbConn.Close() // Force GetSubRequestByID to fail

	s := NewServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/sub-requests/sr-db-err", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.DeleteSubRequestsId(rr, req, "sr-db-err")

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
	u, _ := repo.CreateUser(context.Background(), "postreqerr@example.com")
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
	u, _ := repo.CreateUser(context.Background(), "deleterr@example.com")
	sr := &models.SubRequest{
		ID:        "sr-del-err",
		ShowID:    1,
		UserID:    u.ID,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Hour),
		Status:    "open",
	}
	_ = repo.CreateSubRequest(context.Background(), sr)

	s := NewServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/sub-requests/sr-del-err", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
	req = req.WithContext(ctx)

	dbConn.Close() // Force DeleteSubRequest to fail

	rr := httptest.NewRecorder()
	s.DeleteSubRequestsId(rr, req, "sr-del-err")

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
	u1, _ := repo.CreateUser(context.Background(), "u1@example.com")
	u2, _ := repo.CreateUser(context.Background(), "u2@example.com")
	
	sr := &models.SubRequest{
		ID:        "sr-unauth",
		ShowID:    1,
		UserID:    u1.ID,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Hour),
		Status:    "open",
	}
	_ = repo.CreateSubRequest(context.Background(), sr)
	
	s := NewServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/sub-requests/sr-unauth", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, u2.ID) // Logged in as u2, but owner is u1
	req = req.WithContext(ctx)
	
	rr := httptest.NewRecorder()
	s.DeleteSubRequestsId(rr, req, "sr-unauth")

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
	u1, _ := repo.CreateUser(context.Background(), "u1@example.com")
	
	s := NewServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/sub-requests/sr-notfound", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, u1.ID)
	req = req.WithContext(ctx)
	
	rr := httptest.NewRecorder()
	s.DeleteSubRequestsId(rr, req, "sr-notfound")

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
	u1, _ := repo.CreateUser(context.Background(), "u1@example.com")
	sr := &models.SubRequest{
		ID:        "sr-noctx",
		ShowID:    1,
		UserID:    u1.ID,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Hour),
		Status:    "open",
	}
	_ = repo.CreateSubRequest(context.Background(), sr)
	
	s := NewServer(repo, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/sub-requests/sr-noctx", nil)
	// No UserIDKey in context
	
	rr := httptest.NewRecorder()
	s.DeleteSubRequestsId(rr, req, "sr-noctx")

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for no user in context, got %d", rr.Code)
	}
}

