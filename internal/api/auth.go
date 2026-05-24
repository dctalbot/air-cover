package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	authapp "air-cover/internal/app/auth"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

var randomRead = rand.Read

type AuthHandler struct {
	auth           *authapp.Service
	tokenGenerator func(int) (string, error)
}

type authRepository interface {
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUserByID(ctx context.Context, id int) (*domain.User, error)
	CreateMagicLink(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error
	UseMagicLink(ctx context.Context, tokenHash string) (*domain.MagicLink, error)
	CreateSession(ctx context.Context, sessionID, sessionToken string, userID int, expiresAt time.Time) error
	GetSessionByToken(ctx context.Context, sessionToken string) (*domain.Session, error)
	DeleteSessionsByUserID(ctx context.Context, userID int) error
}

func NewAuthHandler(repo authRepository, sender authapp.Sender) *AuthHandler {
	return NewAuthHandlerWithService(authapp.NewService(repo, sender))
}

func NewAuthHandlerWithService(service *authapp.Service) *AuthHandler {
	h := &AuthHandler{auth: service, tokenGenerator: generateRandomToken}
	h.syncTokenGenerator()
	return h
}

func generateRandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := randomRead(b); err != nil {
		return "", err
	}
	return authapp.EncodeRandomToken(b), nil
}

func hashToken(token string) string {
	return authapp.HashToken(token)
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
	h.syncTokenGenerator()
	_, err := h.auth.RequestLogin(ctx, authapp.LoginInput{
		Email: emailVal,
		MagicLinkURL: func(rawToken string) string {
			return fmt.Sprintf("%s://%s/auth/verify?token=%s", requestScheme(r), r.Host, rawToken)
		},
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			slog.Info("Login attempt with unknown email", "email", strconv.Quote(emailVal))
			h.sendLoginResponse(w, r, "If an account exists, an email has been sent.")
			return
		}
		slog.Error("Failed during login", "error", err)
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

	http.Redirect(w, r, "/?submitted=true", http.StatusSeeOther)
}

func (h *AuthHandler) HandleVerify(w http.ResponseWriter, r *http.Request, rawToken string) {
	if rawToken == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	h.syncTokenGenerator()
	session, err := h.auth.VerifyMagicLink(ctx, rawToken)
	if err != nil {
		slog.Info("Invalid or expired magic link used", "error", err)
		if errors.Is(err, authapp.ErrInvalidMagicLink) {
			http.Error(w, "Invalid or expired link", http.StatusUnauthorized)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    session.Token,
		Path:     "/",
		Expires:  session.ExpiresAt,
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
	if err := h.auth.Logout(ctx, userID); err != nil {
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
	UserRoleKey  contextKey = "user_role"
)

func (h *AuthHandler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_id")
		if err != nil {
			h.handleAuthError(w, r, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		user, err := h.auth.AuthenticateSession(ctx, cookie.Value)
		if err != nil {
			h.handleAuthError(w, r, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx = context.WithValue(ctx, UserIDKey, user.ID)
		ctx = context.WithValue(ctx, UserEmailKey, user.Email)
		ctx = context.WithValue(ctx, UserRoleKey, user.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *AuthHandler) syncTokenGenerator() {
	if h == nil || h.auth == nil {
		return
	}
	h.auth.WithTokenGenerator(h.tokenGenerator)
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		return "https"
	}
	return "http"
}

func (h *AuthHandler) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, ok := r.Context().Value(UserRoleKey).(string)
		if !ok || role != "admin" {
			h.handleAuthError(w, r, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *AuthHandler) handleAuthError(w http.ResponseWriter, r *http.Request, message string, code int) {
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		http.Error(w, message, code)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}
