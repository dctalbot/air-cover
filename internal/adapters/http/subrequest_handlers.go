package httpadapter

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"air-cover/internal/adapters/http/presenter"
	"air-cover/internal/adapters/http/ui"
	subrequestsapp "air-cover/internal/app/subrequests"
)

// Authenticated application page
// (GET /app)
func (s *Server) GetApp(w http.ResponseWriter, r *http.Request) {
	viewer := currentUser(r)
	dashboard, err := s.subRequests.ListDashboard(r.Context(), viewer)
	if err != nil {
		slog.Error("Failed to load app dashboard", "error", err)
		if writeAppError(w, r, err) {
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
		if writeAppError(w, r, err) {
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
		if writeAppError(w, r, err) {
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
		if writeAppError(w, r, err) {
			return
		}
		slog.Error("Failed to update sub request", "id", id, "action", req.Action, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
