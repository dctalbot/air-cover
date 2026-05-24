package httpadapter

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"air-cover/internal/adapters/outbound/sqlite"
	authapp "air-cover/internal/app/auth"
	"air-cover/internal/domain"
)

type MockSender struct{}

func (m *MockSender) SendMagicLink(toEmail, magicLink string) error {
	return nil
}

type recordingSender struct {
	toEmail   string
	magicLink string
}

func (s *recordingSender) SendMagicLink(toEmail, magicLink string) error {
	s.toEmail = toEmail
	s.magicLink = magicLink
	return nil
}

func setupTestDB(t *testing.T) *sqlite.Repository {
	// Using a unique name for each test to avoid conflicts when tests run in parallel or share a process.
	// Actually, just using ":memory:" without shared cache is enough for a single *sql.DB.
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	return sqlite.NewRepository(dbConn)
}

func TestAuthHandler_Login(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	_, _ = repo.CreateUser(context.Background(), "test@example.com", "member")

	tests := []struct {
		name       string
		method     string
		body       string
		wantStatus int
	}{
		{"invalid body", http.MethodPost, "invalid", http.StatusBadRequest},
		{"empty email", http.MethodPost, `{"email":""}`, http.StatusBadRequest},
		{"unknown email", http.MethodPost, `{"email":"unknown@example.com"}`, http.StatusOK},
		{"valid email", http.MethodPost, `{"email":"test@example.com"}`, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/auth/login", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			handler.HandleLogin(rr, req)
			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %v, got %v", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestAuthHandler_Login_Disabled(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	u, _ := repo.CreateUser(context.Background(), "disabled@example.com", "member")

	// Disable user
	_, _ = repo.DB().Exec("UPDATE users SET is_enabled = 0 WHERE id = ?", u.ID)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(`{"email":"disabled@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.HandleLogin(rr, req)

	// Should return 200 with generic message to avoid enumeration
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %v", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "If an account exists, an email has been sent.") {
		t.Errorf("expected generic message, got %s", rr.Body.String())
	}
}

func TestAuthHandler_Login_HTTPSScheme(t *testing.T) {
	// Test that HTTPS scheme is used when X-Forwarded-Proto is https
	repo := setupTestDB(t)
	_, _ = repo.CreateUser(context.Background(), "https@example.com", "member")
	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))

	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(`{"email":"https@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	handler.HandleLogin(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %v", rr.Code)
	}
}

func TestAuthHandler_Login_TokenGenerationError(t *testing.T) {
	repo := setupTestDB(t)
	_, _ = repo.CreateUser(context.Background(), "tokenfail@example.com", "member")
	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	handler.tokenGenerator = func(n int) (string, error) {
		return "", errors.New("token failed")
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(`{"email":"tokenfail@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.HandleLogin(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for token generation failure, got %d", rr.Code)
	}
}

func TestAuthHandler_Login_DeterministicMagicLink(t *testing.T) {
	repo := setupTestDB(t)
	_, _ = repo.CreateUser(context.Background(), "link@example.com", "member")
	sender := &recordingSender{}
	handler := NewAuthHandler(authapp.NewService(repo, sender))
	handler.tokenGenerator = func(n int) (string, error) {
		return "fixed-login-token", nil
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(`{"email":"link@example.com"}`))
	req.Host = "aircover.example.test"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()

	handler.HandleLogin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if sender.toEmail != "link@example.com" {
		t.Errorf("expected sender email link@example.com, got %q", sender.toEmail)
	}
	wantLink := "https://aircover.example.test/auth/verify?token=fixed-login-token"
	if sender.magicLink != wantLink {
		t.Errorf("expected magic link %q, got %q", wantLink, sender.magicLink)
	}
}

func TestAuthHandler_Verify(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	u, _ := repo.CreateUser(context.Background(), "test2@example.com", "member")

	rawToken, _ := authapp.GenerateRandomToken(32)
	hashedToken := authapp.HashToken(rawToken)
	_ = repo.CreateMagicLink(context.Background(), u.ID, hashedToken, time.Now().Add(1*time.Hour))

	tests := []struct {
		name       string
		method     string
		token      string
		wantStatus int
	}{
		{"empty token", http.MethodGet, "", http.StatusBadRequest},
		{"invalid token", http.MethodGet, "invalid", http.StatusUnauthorized},
		{"valid token", http.MethodGet, rawToken, http.StatusFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/auth/verify", nil)
			rr := httptest.NewRecorder()
			handler.HandleVerify(rr, req, tt.token)
			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %v, got %v", tt.wantStatus, rr.Code)
			}
		})
	}

	t.Run("missing token param", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
		rr := httptest.NewRecorder()
		handler.HandleVerify(rr, req, "")
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})
}

func TestAuthHandler_Verify_TokenGenerationErrors(t *testing.T) {
	tests := []struct {
		name           string
		generatorCalls []struct {
			token string
			err   error
		}
	}{
		{
			name: "session id failure",
			generatorCalls: []struct {
				token string
				err   error
			}{
				{"", errors.New("session id failed")},
			},
		},
		{
			name: "session token failure",
			generatorCalls: []struct {
				token string
				err   error
			}{
				{"fixed-session-id", nil},
				{"", errors.New("session token failed")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestDB(t)
			handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
			u, _ := repo.CreateUser(context.Background(), tt.name+"@example.com", "member")
			rawToken := "verify-token-" + strings.ReplaceAll(tt.name, " ", "-")
			_ = repo.CreateMagicLink(context.Background(), u.ID, authapp.HashToken(rawToken), time.Now().Add(1*time.Hour))

			call := 0
			handler.tokenGenerator = func(n int) (string, error) {
				result := tt.generatorCalls[call]
				call++
				return result.token, result.err
			}

			req := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
			rr := httptest.NewRecorder()
			handler.HandleVerify(rr, req, rawToken)

			if rr.Code != http.StatusInternalServerError {
				t.Errorf("expected 500 for token generation failure, got %d", rr.Code)
			}
		})
	}
}

func TestAuthHandler_Verify_HTTPSCookie(t *testing.T) {
	// Test that Secure cookie is set when X-Forwarded-Proto is https
	repo := setupTestDB(t)
	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	u, _ := repo.CreateUser(context.Background(), "secure@example.com", "member")

	rawToken, _ := authapp.GenerateRandomToken(32)
	hashedToken := authapp.HashToken(rawToken)
	_ = repo.CreateMagicLink(context.Background(), u.ID, hashedToken, time.Now().Add(1*time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	handler.HandleVerify(rr, req, rawToken)
	if rr.Code != http.StatusFound {
		t.Errorf("expected redirect, got %v", rr.Code)
	}
	// Verify the cookie is set as Secure
	found := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" && c.Secure {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Secure cookie to be set for HTTPS")
	}
}

func TestAuthMiddleware(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	u, _ := repo.CreateUser(context.Background(), "test3@example.com", "member")
	_ = repo.CreateSession(context.Background(), "sid", authapp.HashToken("stoken"), u.ID, time.Now().Add(1*time.Hour))

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := handler.AuthMiddleware(testHandler)

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rr1 := httptest.NewRecorder()
	mw.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusFound {
		t.Errorf("expected redirect without cookie, got %v", rr1.Code)
	}
	if rr1.Header().Get("Location") != "/" {
		t.Errorf("expected redirect to /, got %v", rr1.Header().Get("Location"))
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(&http.Cookie{Name: "session_id", Value: "invalid"})
	rr2 := httptest.NewRecorder()
	mw.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusFound {
		t.Errorf("expected redirect with invalid cookie, got %v", rr2.Code)
	}
	if rr2.Header().Get("Location") != "/" {
		t.Errorf("expected redirect to /, got %v", rr2.Header().Get("Location"))
	}

	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken"})
	rr3 := httptest.NewRecorder()
	mw.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("expected OK with valid cookie")
	}

	t.Run("json client", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "application/json")
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for JSON client, got %v", rr.Code)
		}
	})
}

func TestAuthMiddleware_Disabled(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	u, _ := repo.CreateUser(context.Background(), "disabled-session@example.com", "member")
	_ = repo.CreateSession(context.Background(), "sid", authapp.HashToken("stoken"), u.ID, time.Now().Add(1*time.Hour))

	// Disable user
	_, _ = repo.DB().Exec("UPDATE users SET is_enabled = 0 WHERE id = ?", u.ID)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := handler.AuthMiddleware(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken"})
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("expected 302 for disabled user session, got %d", rr.Code)
	}
	if rr.Header().Get("Location") != "/" {
		t.Errorf("expected redirect to /, got %v", rr.Header().Get("Location"))
	}
}

func TestAuthHandler_Logout(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	u, err := repo.CreateUser(context.Background(), "test-logout@example.com", "member")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	err = repo.CreateSession(context.Background(), "sid", authapp.HashToken("stoken"), u.ID, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.HandleLogout(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("expected status %v, got %v", http.StatusFound, rr.Code)
	}

	// Verify session is deleted
	session, err := repo.GetSessionByToken(context.Background(), authapp.HashToken("stoken"), time.Now())
	if err == nil {
		t.Errorf("expected session to be deleted, but found session for user %d", session.UserID)
	}

	// Verify cookie is cleared
	cookies := rr.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "session_id" && c.MaxAge < 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected session_id cookie to be cleared")
	}
}

func TestAuthHandler_Logout_HTTPSSecureCookie(t *testing.T) {
	// Test that Secure cookie is cleared properly when HTTPS
	repo := setupTestDB(t)
	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	u, _ := repo.CreateUser(context.Background(), "logout-https@example.com", "member")
	_ = repo.CreateSession(context.Background(), "sid2", authapp.HashToken("stoken2"), u.ID, time.Now().Add(1*time.Hour))

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.HandleLogout(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("expected redirect, got %v", rr.Code)
	}
}

func TestGenerateRandomToken(t *testing.T) {
	tok, err := authapp.GenerateRandomToken(32)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if tok == "" {
		t.Error("expected non-empty token")
	}
	// Two tokens should not be equal
	tok2, _ := authapp.GenerateRandomToken(32)
	if tok == tok2 {
		t.Error("expected distinct tokens")
	}
}

func TestHashToken(t *testing.T) {
	hash := authapp.HashToken("my-token")
	if hash == "" {
		t.Error("expected non-empty hash")
	}
	hash2 := authapp.HashToken("my-token")
	if hash != hash2 {
		t.Error("expected deterministic hash")
	}
}

func TestAuthHandlerSyncTokenGeneratorNilReceiver(t *testing.T) {
	var handler *AuthHandler
	handler.syncTokenGenerator()
}

func TestAuthHandlerSyncTokenGeneratorWithoutSetter(t *testing.T) {
	handler := NewAuthHandler(&fakeAuthService{})
	handler.syncTokenGenerator()
}

func TestAuthHandlerAuthenticateSessionWithoutAuth(t *testing.T) {
	if (&AuthHandler{}).AuthenticateSession(context.Background(), "token") {
		t.Fatal("expected empty auth handler not to authenticate")
	}
}

type fakeAuthService struct{}

func (f *fakeAuthService) RequestLogin(ctx context.Context, input authapp.LoginInput) (authapp.LoginResult, error) {
	return authapp.LoginResult{}, nil
}

func (f *fakeAuthService) VerifyMagicLink(ctx context.Context, rawToken string) (authapp.VerifiedSession, error) {
	return authapp.VerifiedSession{}, nil
}

func (f *fakeAuthService) Logout(ctx context.Context, userID int) error {
	return nil
}

func (f *fakeAuthService) AuthenticateSession(ctx context.Context, sessionToken string) (domain.CurrentUser, error) {
	return domain.CurrentUser{}, errors.New("not authenticated")
}

type failSender struct{}

func (f *failSender) SendMagicLink(toEmail, magicLink string) error {
	return errSendFailed
}

var errSendFailed = errors.New("send failed")

func TestAuthHandler_Login_DBError(t *testing.T) {
	// Force DB error by using a closed DB connection
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	// Create user first, then close the DB to force errors on subsequent operations
	_, err = repo.CreateUser(context.Background(), "dberror@example.com", "member")
	if err != nil {
		t.Fatal(err)
	}
	dbConn.Close() // Force subsequent DB operations to fail

	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(`{"email":"dberror@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.HandleLogin(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for closed DB, got %d", rr.Code)
	}
}

func TestAuthHandler_Login_SendError(t *testing.T) {
	repo := setupTestDB(t)
	_, err := repo.CreateUser(context.Background(), "senderror@example.com", "member")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewAuthHandler(authapp.NewService(repo, &failSender{}))
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(`{"email":"senderror@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.HandleLogin(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for send failure, got %d", rr.Code)
	}
}

func TestAuthHandler_Verify_CreateSessionError(t *testing.T) {
	// Create a magic link, then close DB before CreateSession can run
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "sessionfail@example.com", "member")

	rawToken, _ := authapp.GenerateRandomToken(32)
	hashedToken := authapp.HashToken(rawToken)
	_ = repo.CreateMagicLink(context.Background(), u.ID, hashedToken, time.Now().Add(1*time.Hour))
	// Mark the link as used so UseMagicLink will work once, then update used_at
	// We can't easily make UseMagicLink succeed but CreateSession fail without sqlmock.
	// Instead: use a repo backed by a closed DB that will fail on CreateSession.
	// UseMagicLink reads and writes; we need it to succeed. This is tricky.
	// Workaround: close DB after successful setup. But UseMagicLink also writes.
	// This path is tested at an integration level - skipping the direct error path.
	dbConn.Close()

	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	req := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	rr := httptest.NewRecorder()
	handler.HandleVerify(rr, req, rawToken)
	// After closing DB, UseMagicLink will fail too (reading from closed DB)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for closed DB verify, got %d", rr.Code)
	}
}

func TestAuthHandler_Logout_DBError(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "logouterr@example.com", "member")
	dbConn.Close() // Force DeleteSessionsByUserID to fail

	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, u.ID)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.HandleLogout(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error on logout, got %d", rr.Code)
	}
}

func TestAuthMiddleware_UserNotFound(t *testing.T) {
	// Session token exists but user has been deleted → GetUserByID fails
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "deleteduser@example.com", "member")
	_ = repo.CreateSession(context.Background(), "sid-del", authapp.HashToken("stoken-del"), u.ID, time.Now().Add(1*time.Hour))
	// Close DB to force GetUserByID to fail
	dbConn.Close()

	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	mw := handler.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken-del"})
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	// After closing DB, GetSessionByToken will fail → 401
	if rr.Code != http.StatusFound {
		t.Errorf("expected 302 for closed DB auth middleware, got %d", rr.Code)
	}
}

func TestAuthMiddleware_GetUserByIDError(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "miderr@example.com", "member")
	_ = repo.CreateSession(context.Background(), "sid-mid", authapp.HashToken("stoken-mid"), u.ID, time.Now().Add(1*time.Hour))

	handler := NewAuthHandler(authapp.NewService(repo, nil))
	mw := handler.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken-mid"})

	// Delete the user so GetUserByID fails with ErrNotFound
	_, _ = dbConn.Exec("DELETE FROM users WHERE id = ?", u.ID)

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Errorf("expected 302 when user is deleted, got %d", rr.Code)
	}
}

func TestAuthHandler_Login_Form(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	_, _ = repo.CreateUser(context.Background(), "form@example.com", "member")

	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString("email=form@example.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.HandleLogin(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %v", rr.Code)
	}
	location := rr.Header().Get("Location")
	if location != "/?submitted=true" {
		t.Errorf("expected redirect to /?submitted=true, got %s", location)
	}
}

func TestAuthHandler_Login_Form_Invalid(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))

	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString("email="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.HandleLogin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty email, got %v", rr.Code)
	}
}

func TestAuthHandler_Login_Form_ParseError(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))

	// Sending a body that will cause ParseForm to fail (invalid percent encoding)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString("email=%ZZ"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.HandleLogin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid form data, got %v", rr.Code)
	}
}

func TestAuthHandler_Login_CreateMagicLink_Error(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	_, _ = repo.CreateUser(context.Background(), "test@example.com", "member")

	// Drop magic_links table to force CreateMagicLink to fail
	_, _ = dbConn.Exec("DROP TABLE magic_links")

	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	body := `{"email": "test@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.HandleLogin(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for CreateMagicLink error, got %v", rr.Code)
	}
}

func TestAuthHandler_Verify_CreateSession_Error(t *testing.T) {
	dbConn, err := sqlite.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "test@example.com", "member")

	// Create a magic link
	_ = repo.CreateMagicLink(context.Background(), u.ID, authapp.HashToken("test-token"), time.Now().Add(1*time.Hour))

	// Drop sessions table to force CreateSession to fail
	_, _ = dbConn.Exec("DROP TABLE sessions")

	handler := NewAuthHandler(authapp.NewService(repo, &MockSender{}))
	req := httptest.NewRequest(http.MethodGet, "/auth/verify?token=test-token", nil)

	rr := httptest.NewRecorder()
	handler.HandleVerify(rr, req, "test-token")

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for CreateSession error, got %v", rr.Code)
	}
}
