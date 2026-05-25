package api

import (
	"log/slog"
	"net/http"
)

// NewWrapper creates a ServerInterfaceWrapper with default error handling.
// This enables using the generated parameter-binding wrapper methods
// (e.g., GetAuthVerify, DeleteSubRequestsId) as chi route handlers,
// while keeping manual control over middleware groups.
func NewWrapper(si ServerInterface) *ServerInterfaceWrapper {
	return &ServerInterfaceWrapper{
		Handler: si,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Info("Rejected malformed request", "error", err)
			writeHTTPError(w, r, http.StatusBadRequest, "Invalid request")
		},
	}
}
