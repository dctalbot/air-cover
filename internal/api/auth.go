package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"air-cover/internal/db"
	"air-cover/internal/email"
	"air-cover/internal/ui"
)

type AuthHandler struct {
	repo   *db.Repository
	sender email.Sender
}

func NewAuthHandler(repo *db.Repository, sender email.Sender) *AuthHandler {
	return &AuthHandler{
		repo:   repo,
		sender: sender,
	}
}

func generateRandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func hashToken(token string) string {
	h := sha256.New()
	h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}

func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Limit request body size to 1MB to prevent memory exhaustion (G120)
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var emailVal string
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "application/json") {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		emailVal = string(req.Email)
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}
		emailVal = r.FormValue("email")
	}

	if emailVal == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	user, err := h.repo.GetUserByEmail(ctx, emailVal)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			slog.Info("Login attempt with unknown email", "email", strconv.Quote(emailVal))
			h.sendLoginResponse(w, r, "If an account exists, an email has been sent.")
			return
		}
		slog.Error("Database error during login", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	rawToken, err := generateRandomToken(32)
	if err != nil {
		slog.Error("Failed to generate token", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	tokenHash := hashToken(rawToken)
	expiresAt := time.Now().Add(15 * time.Minute)

	if err := h.repo.CreateMagicLink(ctx, user.ID, tokenHash, expiresAt); err != nil {
		slog.Error("Failed to save magic link", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	magicLink := fmt.Sprintf("%s://%s/auth/verify?token=%s", scheme, host, rawToken)

	if err := h.sender.SendMagicLink(user.Email, magicLink); err != nil {
		slog.Error("Failed to send magic link", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.sendLoginResponse(w, r, "If an account exists, an email has been sent.")
}

func (h *AuthHandler) sendLoginResponse(w http.ResponseWriter, r *http.Request, message string) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
		return
	}

	ui.RenderUnauthenticated(w, map[string]any{
		"Message": message,
	})
}

func (h *AuthHandler) HandleVerify(w http.ResponseWriter, r *http.Request) {
	rawToken := r.URL.Query().Get("token")
	if rawToken == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	tokenHash := hashToken(rawToken)
	ctx := r.Context()

	ml, err := h.repo.UseMagicLink(ctx, tokenHash)
	if err != nil {
		slog.Info("Invalid or expired magic link used", "error", err)
		http.Error(w, "Invalid or expired link", http.StatusUnauthorized)
		return
	}

	sessionID, err := generateRandomToken(32)
	if err != nil {
		slog.Error("Failed to generate session id", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	sessionToken, err := generateRandomToken(32)
	if err != nil {
		slog.Error("Failed to generate session token", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	if err := h.repo.CreateSession(ctx, sessionID, sessionToken, ml.UserID, expiresAt); err != nil {
		slog.Error("Failed to create session", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	if err := h.repo.DeleteSessionsByUserID(ctx, userID); err != nil {
		slog.Error("Failed to delete sessions", "error", err, "user_id", userID) // #nosec G706
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

type contextKey string

const (
	UserIDKey    contextKey = "user_id"
	UserEmailKey contextKey = "user_email"
)

func (h *AuthHandler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_id")
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		session, err := h.repo.GetSessionByToken(ctx, cookie.Value)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := h.repo.GetUserByID(ctx, session.UserID)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx = context.WithValue(ctx, UserIDKey, user.ID)
		ctx = context.WithValue(ctx, UserEmailKey, user.Email)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
