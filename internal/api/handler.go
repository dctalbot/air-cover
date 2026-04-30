package api

import (
	"context"
	"encoding/json"
	"errors"
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

	message := r.URL.Query().Get("message")
	ui.RenderUnauthenticated(w, map[string]any{
		"Message": message,
	})
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

	subRequests, err := s.repo.ListSubRequests(r.Context())
	if err != nil {
		slog.Error("Failed to load sub requests", "error", err)
		http.Error(w, "Unable to load sub requests", http.StatusInternalServerError)
		return
	}

	showMap := make(map[int]string)
	for _, show := range allShows {
		id, err := strconv.Atoi(show.ID)
		if err != nil {
			slog.Warn("Failed to parse show ID", "id", show.ID, "error", err)
			continue
		}
		showMap[id] = show.Title
	}

	type subRequestView struct {
		ID             int
		ShowTitle      string
		RequesterEmail string
		StartTime      string
		EndTime        string
		Notes          string
		Status         string
		CanDelete      bool
	}

	userID, _ := r.Context().Value(UserIDKey).(int)
	var views []subRequestView
	for _, sr := range subRequests {
		title := showMap[sr.ShowID]
		if title == "" {
			title = "Unknown Show"
		}
		views = append(views, subRequestView{
			ID:             sr.ID,
			ShowTitle:      title,
			RequesterEmail: sr.RequesterEmail,
			StartTime:      sr.StartTime.Format("Mon, Jan 02 at 3:04 PM"),
			EndTime:        sr.EndTime.Format("Mon, Jan 02 at 3:04 PM"),
			Notes:          sr.Notes,
			Status:         sr.GetStatus(),
			CanDelete:      sr.PostedByUserID == userID,
		})
	}

	role, _ := r.Context().Value(UserRoleKey).(string)

	ui.RenderAuthenticated(w, map[string]any{
		"Shows":       allShows,
		"Email":       email,
		"SubRequests": views,
		"IsAdmin":     role == "admin",
	})
}

// Admin dashboard
// (GET /admin)
func (s *Server) GetAdmin(w http.ResponseWriter, r *http.Request) {
	users, err := s.repo.ListUsers(r.Context())
	if err != nil {
		slog.Error("Failed to load users", "error", err)
		http.Error(w, "Unable to load users", http.StatusInternalServerError)
		return
	}

	type userView struct {
		ID        int
		Email     string
		Role      string
		CreatedAt string
		IsEnabled bool
		CanBan    bool
	}

	currentUserID, _ := r.Context().Value(UserIDKey).(int)
	var views []userView
	for _, u := range users {
		views = append(views, userView{
			ID:        u.ID,
			Email:     u.Email,
			Role:      u.Role,
			CreatedAt: u.CreatedAt.Format("Jan 02, 2006 at 3:04 PM"),
			IsEnabled: u.IsEnabled,
			CanBan:    u.ID != currentUserID,
		})
	}

	email, _ := r.Context().Value(UserEmailKey).(string)

	ui.RenderAdmin(w, map[string]any{
		"Users": views,
		"Email": email,
	})
}

// Create a new user
// (POST /users)
func (s *Server) PostUsers(w http.ResponseWriter, r *http.Request) {
	// Limit request body size to 1MB
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse form", "error", err)
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	if email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	role := r.FormValue("role")
	if role == "" {
		http.Error(w, "Role is required", http.StatusBadRequest)
		return
	}

	if role != "admin" && role != "member" {
		http.Error(w, "Invalid role", http.StatusBadRequest)
		return
	}

	_, err := s.repo.CreateUser(r.Context(), email, role)
	if err != nil {
		slog.Error("Failed to create user", "email", strconv.Quote(email), "role", strconv.Quote(role), "error", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
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

// Create a new sub request
// (POST /sub-requests)
func (s *Server) PostSubRequests(w http.ResponseWriter, r *http.Request) {
	// Limit request body size to 1MB to prevent memory exhaustion (G120)
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse form", "error", err)
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	showIDStr := r.FormValue("show")
	showID, err := strconv.Atoi(showIDStr)
	if err != nil {
		slog.Error("Invalid show ID", "show", strconv.Quote(showIDStr), "error", err)
		http.Error(w, "Invalid show ID", http.StatusBadRequest)
		return
	}

	startTimeStr := r.FormValue("start_time")
	startTime, err := time.Parse("2006-01-02T15:04", startTimeStr)
	if err != nil {
		slog.Error("Invalid start time", "start_time", strconv.Quote(startTimeStr), "error", err)
		http.Error(w, "Invalid start time", http.StatusBadRequest)
		return
	}

	endTimeStr := r.FormValue("end_time")
	endTime, err := time.Parse("2006-01-02T15:04", endTimeStr)
	if err != nil {
		slog.Error("Invalid end time", "end_time", strconv.Quote(endTimeStr), "error", err)
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

	sr := &models.SubRequest{
		ShowID:         showID,
		PostedByUserID: userID,
		StartTime:      startTime,
		EndTime:        endTime,
		Notes:          notes,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := s.repo.CreateSubRequest(r.Context(), sr); err != nil {
		slog.Error("Failed to create sub request", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

// Delete a sub request
// (DELETE /sub-requests/{id})
func (s *Server) DeleteSubRequestsId(w http.ResponseWriter, r *http.Request, id int) {
	sr, err := s.repo.GetSubRequestByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "Sub request not found", http.StatusNotFound)
			return
		}
		slog.Error("Failed to get sub request", "id", id, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if sr.PostedByUserID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := s.repo.DeleteSubRequest(r.Context(), id); err != nil {
		slog.Error("Failed to delete sub request", "id", id, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Update a user's status
// (PATCH /users/{id})
func (s *Server) PatchUsersId(w http.ResponseWriter, r *http.Request, id int) {
	var req struct {
		IsEnabled *bool   `json:"is_enabled"`
		Role      *string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Role != nil {
		role := *req.Role
		if role != "admin" && role != "member" {
			http.Error(w, "Invalid role", http.StatusBadRequest)
			return
		}
	}

	currentUserID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Only check self-ban if is_enabled is provided and false
	if req.IsEnabled != nil && !*req.IsEnabled && currentUserID == id {
		http.Error(w, "Cannot ban your own account", http.StatusForbidden)
		return
	}

	if err := s.repo.UpdateUser(r.Context(), id, req.Role, req.IsEnabled); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		slog.Error("Failed to update user", "id", id, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
