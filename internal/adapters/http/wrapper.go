package api

import "net/http"

// NewWrapper creates a ServerInterfaceWrapper with default error handling.
// This enables using the generated parameter-binding wrapper methods
// (e.g., GetAuthVerify, DeleteSubRequestsId) as chi route handlers,
// while keeping manual control over middleware groups.
func NewWrapper(si ServerInterface) *ServerInterfaceWrapper {
	return &ServerInterfaceWrapper{
		Handler: si,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		},
	}
}
