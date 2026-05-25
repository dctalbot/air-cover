package api

import (
	"context"
	"log/slog"
	"net/http"

	"air-cover/internal/adapters/inbound/http/ui"
	adminapp "air-cover/internal/app/admin"
	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/domain"
)

const (
	maxFormBodyBytes  = 1024 * 1024
	sessionCookieName = "session_id"
)

type Server struct {
	auth        *AuthHandler
	subRequests subRequestService
	admin       adminService
}

type subRequestService interface {
	ListDashboard(context.Context, domain.CurrentUser) (subrequestsapp.Dashboard, error)
	Get(context.Context, domain.CurrentUser, int) (subrequestsapp.Detail, error)
	Create(context.Context, domain.CurrentUser, subrequestsapp.CreateInput) error
	Delete(context.Context, domain.CurrentUser, int) error
	ApplyAction(context.Context, domain.CurrentUser, int, subrequestsapp.Action) error
}

type adminService interface {
	ListUsers(context.Context) ([]*domain.User, error)
	CreateUser(context.Context, adminapp.CreateUserInput) error
	UpdateUser(context.Context, domain.CurrentUser, adminapp.UpdateUserInput) error
	ImportCatalogUsers(context.Context) error
}

func NewServer(auth *AuthHandler, subRequests subRequestService, admin adminService) *Server {
	return &Server{
		auth:        auth,
		subRequests: subRequests,
		admin:       admin,
	}
}

func (s *Server) hasValidSession(ctx context.Context, token string) bool {
	if s.auth == nil {
		return s.subRequests == nil && s.admin == nil
	}
	return s.auth.AuthenticateSession(ctx, token)
}

// Home page or login page
// (GET /)
func (s *Server) Get(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if s.hasValidSession(r.Context(), cookie.Value) {
			http.Redirect(w, r, "/app", http.StatusFound)
			return
		}

		clearSessionCookie(w, r)
	}

	if err := ui.Unauthenticated(r.URL.Query().Get("submitted") == "true").Render(r.Context(), w); err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}

// Request a magic link for login
// (POST /auth/login)
func (s *Server) PostAuthLogin(w http.ResponseWriter, r *http.Request) {
	s.auth.HandleLogin(w, r)
}

// Logout and invalidate sessions
// (POST /auth/logout)
func (s *Server) PostAuthLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.HandleLogout(w, r)
}

// Verify magic link and create session
// (GET /auth/verify)
func (s *Server) GetAuthVerify(w http.ResponseWriter, r *http.Request, params GetAuthVerifyParams) {
	s.auth.HandleVerify(w, r, params.Token)
}

// Health check
// (GET /health)
func (s *Server) GetHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("OK"))
	if err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}
