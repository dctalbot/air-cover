package api

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"air-cover/internal/db"
	"air-cover/internal/models"
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

// Create a new sub request
// (POST /sub-requests)
func (s *Server) PostSubRequests(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse form", "error", err)
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	showIDStr := r.FormValue("show")
	showID, err := strconv.Atoi(showIDStr)
	if err != nil {
		slog.Error("Invalid show ID", "show", showIDStr, "error", err)
		http.Error(w, "Invalid show ID", http.StatusBadRequest)
		return
	}

	startTimeStr := r.FormValue("start_time")
	startTime, err := time.Parse("2006-01-02T15:04", startTimeStr)
	if err != nil {
		slog.Error("Invalid start time", "start_time", startTimeStr, "error", err)
		http.Error(w, "Invalid start time", http.StatusBadRequest)
		return
	}

	endTimeStr := r.FormValue("end_time")
	endTime, err := time.Parse("2006-01-02T15:04", endTimeStr)
	if err != nil {
		slog.Error("Invalid end time", "end_time", endTimeStr, "error", err)
		http.Error(w, "Invalid end time", http.StatusBadRequest)
		return
	}

	notes := r.FormValue("notes")

	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		slog.Error("User ID not found in context")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	id, err := generateRandomToken(32)
	if err != nil {
		slog.Error("Failed to generate sub request ID", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	sr := &models.SubRequest{
		ID:        id,
		ShowID:    showID,
		UserID:    userID,
		StartTime: startTime,
		EndTime:   endTime,
		Notes:     notes,
		Status:    "open",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.repo.CreateSubRequest(r.Context(), sr); err != nil {
		slog.Error("Failed to create sub request", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/app", http.StatusSeeOther)
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
