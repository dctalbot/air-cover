package httpadapter

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"air-cover/internal/apperrors"
)

func writeAppError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		writeHTTPError(w, r, http.StatusNotFound, "Not found")
	case errors.Is(err, apperrors.ErrConflict):
		writeHTTPError(w, r, http.StatusConflict, "Conflict")
	case errors.Is(err, apperrors.ErrForbidden):
		writeHTTPError(w, r, http.StatusForbidden, "Forbidden")
	case errors.Is(err, apperrors.ErrInvalid):
		writeHTTPError(w, r, http.StatusBadRequest, "Invalid request")
	default:
		return false
	}
	return true
}

func writeHTTPError(w http.ResponseWriter, r *http.Request, status int, message string) {
	if wantsJSONError(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}

	http.Error(w, message, status)
}

func wantsJSONError(r *http.Request) bool {
	return headerContainsMediaType(r.Header.Get("Accept"), "application/json") ||
		headerContainsMediaType(r.Header.Get("Content-Type"), "application/json")
}

func headerContainsMediaType(header, mediaType string) bool {
	for _, part := range strings.Split(header, ",") {
		value := strings.TrimSpace(part)
		if semicolon := strings.Index(value, ";"); semicolon >= 0 {
			value = strings.TrimSpace(value[:semicolon])
		}
		if strings.EqualFold(value, mediaType) {
			return true
		}
	}
	return false
}
