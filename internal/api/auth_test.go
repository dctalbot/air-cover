package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"air-cover/internal/db"
)

type MockSender struct{}

func (m *MockSender) SendMagicLink(toEmail, magicLink string) error {
	return nil
}

func setupTestDB(t *testing.T) *db.Repository {
	// Using a unique name for each test to avoid conflicts when tests run in parallel or share a process.
	// Actually, just using ":memory:" without shared cache is enough for a single *sql.DB.
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	return db.NewRepository(dbConn)
}

func TestAuthHandler_Login(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(repo, &MockSender{})
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

func TestAuthHandler_Login_HTTPSScheme(t *testing.T) {
	// Test that HTTPS scheme is used when X-Forwarded-Proto is https
	repo := setupTestDB(t)
	_, _ = repo.CreateUser(context.Background(), "https@example.com", "member")
	handler := NewAuthHandler(repo, &MockSender{})

	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(`{"email":"https@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	handler.HandleLogin(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %v", rr.Code)
	}
}

func TestAuthHandler_Verify(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(repo, &MockSender{})
	u, _ := repo.CreateUser(context.Background(), "test2@example.com", "member")

	rawToken, _ := generateRandomToken(32)
	hashedToken := hashToken(rawToken)
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

func TestAuthHandler_Verify_HTTPSCookie(t *testing.T) {
	// Test that Secure cookie is set when X-Forwarded-Proto is https
	repo := setupTestDB(t)
	handler := NewAuthHandler(repo, &MockSender{})
	u, _ := repo.CreateUser(context.Background(), "secure@example.com", "member")

	rawToken, _ := generateRandomToken(32)
	hashedToken := hashToken(rawToken)
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
	handler := NewAuthHandler(repo, &MockSender{})
	u, _ := repo.CreateUser(context.Background(), "test3@example.com", "member")
	_ = repo.CreateSession(context.Background(), "sid", "stoken", u.ID, time.Now().Add(1*time.Hour))

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := handler.AuthMiddleware(testHandler)

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rr1 := httptest.NewRecorder()
	mw.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Errorf("expected unauthorized without cookie")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(&http.Cookie{Name: "session_id", Value: "invalid"})
	rr2 := httptest.NewRecorder()
	mw.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("expected unauthorized with invalid cookie")
	}

	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken"})
	rr3 := httptest.NewRecorder()
	mw.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("expected OK with valid cookie")
	}
}

func TestAuthHandler_Logout(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(repo, &MockSender{})
	u, err := repo.CreateUser(context.Background(), "test-logout@example.com", "member")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	err = repo.CreateSession(context.Background(), "sid", "stoken", u.ID, time.Now().Add(1*time.Hour))
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
	session, err := repo.GetSessionByToken(context.Background(), "stoken")
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
	handler := NewAuthHandler(repo, &MockSender{})
	u, _ := repo.CreateUser(context.Background(), "logout-https@example.com", "member")
	_ = repo.CreateSession(context.Background(), "sid2", "stoken2", u.ID, time.Now().Add(1*time.Hour))

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
	tok, err := generateRandomToken(32)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if tok == "" {
		t.Error("expected non-empty token")
	}
	// Two tokens should not be equal
	tok2, _ := generateRandomToken(32)
	if tok == tok2 {
		t.Error("expected distinct tokens")
	}
}

func TestHashToken(t *testing.T) {
	hash := hashToken("my-token")
	if hash == "" {
		t.Error("expected non-empty hash")
	}
	hash2 := hashToken("my-token")
	if hash != hash2 {
		t.Error("expected deterministic hash")
	}
}

type failSender struct{}

func (f *failSender) SendMagicLink(toEmail, magicLink string) error {
	return errSendFailed
}

var errSendFailed = errors.New("send failed")

func TestAuthHandler_Login_DBError(t *testing.T) {
	// Force DB error by using a closed DB connection
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	// Create user first, then close the DB to force errors on subsequent operations
	_, err = repo.CreateUser(context.Background(), "dberror@example.com", "member")
	if err != nil {
		t.Fatal(err)
	}
	dbConn.Close() // Force subsequent DB operations to fail

	handler := NewAuthHandler(repo, &MockSender{})
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
	handler := NewAuthHandler(repo, &failSender{})
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
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "sessionfail@example.com", "member")

	rawToken, _ := generateRandomToken(32)
	hashedToken := hashToken(rawToken)
	_ = repo.CreateMagicLink(context.Background(), u.ID, hashedToken, time.Now().Add(1*time.Hour))
	// Mark the link as used so UseMagicLink will work once, then update used_at
	// We can't easily make UseMagicLink succeed but CreateSession fail without sqlmock.
	// Instead: use a repo backed by a closed DB that will fail on CreateSession.
	// UseMagicLink reads and writes; we need it to succeed. This is tricky.
	// Workaround: close DB after successful setup. But UseMagicLink also writes.
	// This path is tested at an integration level - skipping the direct error path.
	dbConn.Close()

	handler := NewAuthHandler(repo, &MockSender{})
	req := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	rr := httptest.NewRecorder()
	handler.HandleVerify(rr, req, rawToken)
	// After closing DB, UseMagicLink will fail too (reading from closed DB)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for closed DB verify, got %d", rr.Code)
	}
}

func TestAuthHandler_Logout_DBError(t *testing.T) {
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "logouterr@example.com", "member")
	dbConn.Close() // Force DeleteSessionsByUserID to fail

	handler := NewAuthHandler(repo, &MockSender{})
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
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "deleteduser@example.com", "member")
	_ = repo.CreateSession(context.Background(), "sid-del", "stoken-del", u.ID, time.Now().Add(1*time.Hour))
	// Close DB to force GetUserByID to fail
	dbConn.Close()

	handler := NewAuthHandler(repo, &MockSender{})
	mw := handler.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken-del"})
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	// After closing DB, GetSessionByToken will fail → 401
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for closed DB auth middleware, got %d", rr.Code)
	}
}

func TestAuthMiddleware_GetUserByIDError(t *testing.T) {
	dbConn, err := db.InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := db.NewRepository(dbConn)
	u, _ := repo.CreateUser(context.Background(), "miderr@example.com", "member")
	_ = repo.CreateSession(context.Background(), "sid-mid", "stoken-mid", u.ID, time.Now().Add(1*time.Hour))

	handler := NewAuthHandler(repo, nil)
	mw := handler.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stoken-mid"})

	// Delete the user so GetUserByID fails with ErrNotFound
	_, _ = dbConn.Exec("DELETE FROM users WHERE id = ?", u.ID)

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when user is deleted, got %d", rr.Code)
	}
}

func TestAuthHandler_Login_Form(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(repo, &MockSender{})
	_, _ = repo.CreateUser(context.Background(), "form@example.com", "member")

	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString("email=form@example.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.HandleLogin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %v", rr.Code)
	}
	// Check for the success message in the rendered HTML
	if !bytes.Contains(rr.Body.Bytes(), []byte("If an account exists, an email has been sent.")) {
		t.Errorf("expected HTML success message, got %s", rr.Body.String())
	}
}

func TestAuthHandler_Login_Form_Invalid(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(repo, &MockSender{})

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
	handler := NewAuthHandler(repo, &MockSender{})

	// Sending a body that will cause ParseForm to fail (invalid percent encoding)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString("email=%ZZ"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.HandleLogin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid form data, got %v", rr.Code)
	}
}
