package api

import (
	"bytes"
	"context"
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
	_, _ = repo.CreateUser(context.Background(), "test@example.com")

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
			rr := httptest.NewRecorder()
			handler.HandleLogin(rr, req)
			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %v, got %v", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestAuthHandler_Verify(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(repo, &MockSender{})
	u, _ := repo.CreateUser(context.Background(), "test2@example.com")

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
			req := httptest.NewRequest(tt.method, "/auth/verify?token="+tt.token, nil)
			rr := httptest.NewRecorder()
			handler.HandleVerify(rr, req)
			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %v, got %v", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	repo := setupTestDB(t)
	handler := NewAuthHandler(repo, &MockSender{})
	u, _ := repo.CreateUser(context.Background(), "test3@example.com")
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
	u, err := repo.CreateUser(context.Background(), "test-logout@example.com")
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
