package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"air-cover/internal/adapters/inbound/presenter"
	"air-cover/internal/adapters/inbound/ui"
	adminapp "air-cover/internal/app/admin"
	"air-cover/internal/apperrors"
)

// Admin dashboard
// (GET /admin)
func (s *Server) GetAdmin(w http.ResponseWriter, r *http.Request) {
	viewer := currentUser(r)
	users, err := s.admin.ListUsers(r.Context(), viewer)
	if err != nil {
		if writeAppError(w, r, err) {
			return
		}
		slog.Error("Failed to load users", "error", err)
		http.Error(w, "Unable to load users", http.StatusInternalServerError)
		return
	}

	views := presenter.AdminUsers(users)

	if err := ui.Admin(views, viewer.Email).Render(r.Context(), w); err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}

// Create a new user
// (POST /users)
func (s *Server) PostUsers(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBodyBytes)

	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse form", "error", err)
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	input := adminapp.CreateUserInput{
		Email: r.FormValue("email"),
		Role:  r.FormValue("role"),
	}
	viewer := currentUser(r)
	if err := s.admin.CreateUser(r.Context(), viewer, input); err != nil {
		if errors.Is(err, adminapp.ErrUserAlreadyExists) {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		if errors.Is(err, apperrors.ErrInvalid) {
			http.Error(w, "Invalid user", http.StatusBadRequest)
			return
		}
		if writeAppError(w, r, err) {
			return
		}
		slog.Error("Failed to create user", "email", strconv.Quote(input.Email), "role", strconv.Quote(input.Role), "error", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
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
		if writeAppError(w, r, err) {
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

// Import users from Spinitron
// (POST /users/import/spinitron)
func (s *Server) PostUsersImportSpinitron(w http.ResponseWriter, r *http.Request) {
	viewer := currentUser(r)
	if err := s.admin.ImportCatalogUsers(r.Context(), viewer); err != nil {
		if writeAppError(w, r, err) {
			return
		}
		slog.Error("Failed to import users from spinitron", "error", err)
		http.Error(w, "Failed to import users", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
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
