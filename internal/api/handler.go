package api

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"air-cover/internal/db"
	"air-cover/internal/spinitron"
	"air-cover/internal/ui"
)

type ShowsService interface {
	GetShowsPage(ctx context.Context, page int) (spinitron.ShowsPage, error)
}

type Server struct {
	repo            *db.Repository
	auth            *AuthHandler
	spinitronClient ShowsService
}

func NewServer(repo *db.Repository, auth *AuthHandler, spinitronClient ShowsService) *Server {
	return &Server{
		repo:            repo,
		auth:            auth,
		spinitronClient: spinitronClient,
	}
}

// Home page or login page
// (GET /)
func (s *Server) Get(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session_id"); err == nil && cookie.Value != "" {
		if _, err := s.repo.GetSessionByToken(r.Context(), cookie.Value); err == nil {
			http.Redirect(w, r, "/app", http.StatusFound)
			return
		}

		// Clear stale session cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	ui.RenderUnauthenticated(w)
}

// Authenticated application page
// (GET /app)
func (s *Server) GetApp(w http.ResponseWriter, r *http.Request) {
	var allShows []spinitron.Show
	page := 1

	for page > 0 {
		showsPage, err := s.spinitronClient.GetShowsPage(r.Context(), page)
		if err != nil {
			slog.Error("Failed to load shows from spinitron", "error", err)
			http.Error(w, "Unable to load shows", http.StatusBadGateway)
			return
		}

		allShows = append(allShows, showsPage.Items...)

		if showsPage.NextPage != nil {
			page = *showsPage.NextPage
		} else {
			break
		}
	}

	sort.Slice(allShows, func(i, j int) bool {
		return strings.ToLower(allShows[i].Title) < strings.ToLower(allShows[j].Title)
	})

	email, _ := r.Context().Value(UserEmailKey).(string)

	ui.RenderAuthenticated(w, map[string]any{
		"Shows": allShows,
		"Email": email,
	})
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
	s.auth.HandleVerify(w, r)
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
