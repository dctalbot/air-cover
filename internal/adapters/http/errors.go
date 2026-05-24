package httpadapter

import (
	"errors"
	"net/http"

	"air-cover/internal/apperrors"
)

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
