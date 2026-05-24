package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	adminapp "air-cover/internal/app/admin"
	"air-cover/internal/app/session"
	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
	"air-cover/internal/presenter"
	"air-cover/internal/ui"
)

const maxFormBodyBytes = 1024 * 1024

type Server struct {
	auth        *AuthHandler
	subRequests subRequestService
	admin       adminService
}

type subRequestService interface {
	ListDashboard(context.Context, session.CurrentUser) (subrequestsapp.Dashboard, error)
	Create(context.Context, session.CurrentUser, subrequestsapp.CreateInput) error
	Delete(context.Context, session.CurrentUser, int) error
	ApplyAction(context.Context, session.CurrentUser, int, subrequestsapp.Action) error
}

type adminService interface {
	ListUsers(context.Context) ([]*domain.User, error)
	CreateUser(context.Context, adminapp.CreateUserInput) error
	UpdateUser(context.Context, session.CurrentUser, adminapp.UpdateUserInput) error
	ImportCatalogUsers(context.Context) error
}

func NewServer(auth *AuthHandler, subRequests subRequestService, admin adminService) *Server {
	return &Server{
		auth:        auth,
		subRequests: subRequests,
		admin:       admin,
	}
}

// Home page or login page
// (GET /)
func (s *Server) Get(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session_id"); err == nil && cookie.Value != "" {
		if s.auth != nil {
			if _, err := s.auth.auth.AuthenticateSession(r.Context(), cookie.Value); err == nil {
				http.Redirect(w, r, "/app", http.StatusFound)
				return
			}
		} else if s.auth == nil && s.subRequests == nil && s.admin == nil {
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

	if err := ui.Unauthenticated(r.URL.Query().Get("submitted") == "true").Render(r.Context(), w); err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}

// Authenticated application page
// (GET /app)
func (s *Server) GetApp(w http.ResponseWriter, r *http.Request) {
	viewer := currentUser(r)
	dashboard, err := s.subRequests.ListDashboard(r.Context(), viewer)
	if err != nil {
		slog.Error("Failed to load app dashboard", "error", err)
		if writeAppError(w, err) {
			return
		}
		if errors.Is(err, subrequestsapp.ErrCatalog) {
			http.Error(w, "Unable to load shows", http.StatusBadGateway)
		} else {
			http.Error(w, "Unable to load app", http.StatusInternalServerError)
		}
		return
	}

	upcoming, past := presenter.SubRequestDashboard(dashboard)
	if err := ui.Authenticated(dashboard.Shows, viewer.Email, upcoming, past, viewer.IsAdmin()).Render(r.Context(), w); err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}

// Admin dashboard
// (GET /admin)
func (s *Server) GetAdmin(w http.ResponseWriter, r *http.Request) {
	users, err := s.admin.ListUsers(r.Context())
	if err != nil {
		slog.Error("Failed to load users", "error", err)
		http.Error(w, "Unable to load users", http.StatusInternalServerError)
		return
	}

	viewer := currentUser(r)
	views := presenter.AdminUsers(users, viewer.ID)

	if err := ui.Admin(views, viewer.Email).Render(r.Context(), w); err != nil {
		slog.Error("Failed to write response", "error", err)
	}
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

	input := adminapp.CreateUserInput{
		Email: r.FormValue("email"),
		Role:  r.FormValue("role"),
	}
	if input.Email == "" || (input.Role != "" && !domain.Role(input.Role).Valid()) {
		http.Error(w, "Invalid user", http.StatusBadRequest)
		return
	}
	if err := s.admin.CreateUser(r.Context(), input); err != nil {
		if errors.Is(err, apperrors.ErrInvalid) {
			http.Error(w, "Invalid user", http.StatusBadRequest)
			return
		}
		slog.Error("Failed to create user", "email", strconv.Quote(input.Email), "role", strconv.Quote(input.Role), "error", err)
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
	input, ok := parseCreateSubRequestInput(w, r)
	if !ok {
		return
	}

	viewer, ok := requireCurrentUser(w, r)
	if !ok {
		slog.Error("User ID not found in context")
		return
	}

	if err := s.subRequests.Create(r.Context(), viewer, input); err != nil {
		if writeAppError(w, err) {
			return
		}
		if errors.Is(err, subrequestsapp.ErrCatalog) {
			slog.Error("Failed to validate sub request against catalog", "error", err)
			http.Error(w, "Unable to validate show", http.StatusBadGateway)
			return
		}
		slog.Error("Failed to create sub request", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

// Delete a sub request
// (DELETE /sub-requests/{id})
func (s *Server) DeleteSubRequestsId(w http.ResponseWriter, r *http.Request, id int) {
	viewer, ok := requireCurrentUser(w, r)
	if !ok {
		return
	}
	if err := s.subRequests.Delete(r.Context(), viewer, id); err != nil {
		if writeAppError(w, err) {
			return
		}
		slog.Error("Failed to delete sub request", "id", id, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Take or untake a sub request
// (PATCH /sub-requests/{id})
func (s *Server) PatchSubRequestsId(w http.ResponseWriter, r *http.Request, id int) {
	var req struct {
		Action string `json:"action"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	viewer, ok := requireCurrentUser(w, r)
	if !ok {
		return
	}

	if err := s.subRequests.ApplyAction(r.Context(), viewer, id, subrequestsapp.Action(req.Action)); err != nil {
		if writeAppError(w, err) {
			return
		}
		slog.Error("Failed to update sub request", "id", id, "action", req.Action, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Update a user's status
// (POST /users/{id})
func (s *Server) PostUsersId(w http.ResponseWriter, r *http.Request, id int) {
	input, isForm, ok := parseUpdateUserInput(w, r, id)
	if !ok {
		return
	}

	viewer, ok := requireCurrentUser(w, r)
	if !ok {
		return
	}

	if err := s.admin.UpdateUser(r.Context(), viewer, input); err != nil {
		if writeAppError(w, err) {
			return
		}
		slog.Error("Failed to update user", "id", id, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if isForm {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
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

// Import users from Spinitron
// (POST /users/import/spinitron)
func (s *Server) PostUsersImportSpinitron(w http.ResponseWriter, r *http.Request) {
	if err := s.admin.ImportCatalogUsers(r.Context()); err != nil {
		slog.Error("Failed to import users from spinitron", "error", err)
		http.Error(w, "Failed to import users", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func currentUser(r *http.Request) session.CurrentUser {
	viewer, _ := userFromContext(r)
	return viewer
}

func requireCurrentUser(w http.ResponseWriter, r *http.Request) (session.CurrentUser, bool) {
	viewer, ok := userFromContext(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return session.CurrentUser{}, false
	}
	return viewer, true
}

func userFromContext(r *http.Request) (session.CurrentUser, bool) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		return session.CurrentUser{}, false
	}
	email, _ := r.Context().Value(UserEmailKey).(string)
	role, _ := r.Context().Value(UserRoleKey).(string)
	return session.CurrentUser{
		ID:    userID,
		Email: email,
		Role:  role,
	}, true
}

func parseCreateSubRequestInput(w http.ResponseWriter, r *http.Request) (subrequestsapp.CreateInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBodyBytes)

	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse form", "error", err)
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return subrequestsapp.CreateInput{}, false
	}

	showIDStr := r.FormValue("show")
	showID, err := strconv.Atoi(showIDStr)
	if err != nil {
		slog.Error("Invalid show ID", "show", strconv.Quote(showIDStr), "error", err)
		http.Error(w, "Invalid show ID", http.StatusBadRequest)
		return subrequestsapp.CreateInput{}, false
	}

	startTimeStr := r.FormValue("start_time")
	startTime, err := time.Parse("2006-01-02T15:04", startTimeStr)
	if err != nil {
		slog.Error("Invalid start time", "start_time", strconv.Quote(startTimeStr), "error", err)
		http.Error(w, "Invalid start time", http.StatusBadRequest)
		return subrequestsapp.CreateInput{}, false
	}

	endTimeStr := r.FormValue("end_time")
	endTime, err := time.Parse("2006-01-02T15:04", endTimeStr)
	if err != nil {
		slog.Error("Invalid end time", "end_time", strconv.Quote(endTimeStr), "error", err)
		http.Error(w, "Invalid end time", http.StatusBadRequest)
		return subrequestsapp.CreateInput{}, false
	}

	return subrequestsapp.CreateInput{
		ShowID:    showID,
		StartTime: startTime,
		EndTime:   endTime,
		Notes:     r.FormValue("notes"),
	}, true
}

func parseUpdateUserInput(w http.ResponseWriter, r *http.Request, id int) (adminapp.UpdateUserInput, bool, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBodyBytes)

	contentType := r.Header.Get("Content-Type")
	isForm := strings.HasPrefix(contentType, "application/x-www-form-urlencoded")

	var input adminapp.UpdateUserInput
	input.ID = id

	if isForm {
		if err := r.ParseForm(); err != nil {
			slog.Error("Failed to parse form", "error", err)
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return adminapp.UpdateUserInput{}, isForm, false
		}
		if val := r.FormValue("is_enabled"); val != "" {
			b := val == "true"
			input.IsEnabled = &b
		}
		if val := r.FormValue("role"); val != "" {
			input.Role = &val
		}
		return input, isForm, true
	}

	var req struct {
		IsEnabled *bool   `json:"is_enabled"`
		Role      *string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return adminapp.UpdateUserInput{}, isForm, false
	}
	input.IsEnabled = req.IsEnabled
	input.Role = req.Role
	return input, isForm, true
}

func writeAppError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		http.Error(w, "Not found", http.StatusNotFound)
	case errors.Is(err, apperrors.ErrConflict):
		http.Error(w, "Conflict", http.StatusConflict)
	case errors.Is(err, apperrors.ErrForbidden):
		http.Error(w, "Forbidden", http.StatusForbidden)
	case errors.Is(err, apperrors.ErrInvalid):
		http.Error(w, "Invalid request", http.StatusBadRequest)
	default:
		return false
	}
	return true
}
