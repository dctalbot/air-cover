package cmd

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	httpadapter "air-cover/internal/adapters/inbound/http"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	nethttp_middleware "github.com/oapi-codegen/nethttp-middleware"
)

func newRouter(apiServer *httpadapter.Server, authHandler *httpadapter.AuthHandler) chi.Router {
	swagger, err := getSwagger()
	if err != nil {
		slog.Error("Failed to load swagger spec", "error", err)
		osExit(1)
	}

	// Disable server name validation so the validator doesn't
	// reject requests based on the Host header.
	swagger.Servers = nil

	wrapper := httpadapter.NewWrapper(apiServer)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(nethttp_middleware.OapiRequestValidator(swagger))

	r.Get("/", wrapper.Get)
	r.Get("/health", wrapper.GetHealth)

	r.Group(func(r chi.Router) {
		r.Use(httprate.LimitByIP(5, time.Minute))
		r.Post("/auth/login", wrapper.PostAuthLogin)
		r.Get("/auth/verify", wrapper.GetAuthVerify)
	})

	r.Group(func(r chi.Router) {
		r.Use(authHandler.AuthMiddleware)
		r.Use(requireSameOriginMutation)
		r.Get("/app", wrapper.GetApp)
		r.Post("/auth/logout", wrapper.PostAuthLogout)
		r.Post("/sub-requests", wrapper.PostSubRequests)
		r.Delete("/sub-requests/{id}", wrapper.DeleteSubRequestsId)
		r.Patch("/sub-requests/{id}", wrapper.PatchSubRequestsId)
	})

	r.Group(func(r chi.Router) {
		r.Use(authHandler.AuthMiddleware)
		r.Use(authHandler.RequireAdmin)
		r.Use(requireSameOriginMutation)
		r.Get("/admin", wrapper.GetAdmin)
		r.Post("/users", wrapper.PostUsers)
		r.Post("/users/import/spinitron", wrapper.PostUsersImportSpinitron)
		r.Post("/users/{id}", wrapper.PostUsersId)
	})

	return r
}

func requireSameOriginMutation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutation(r.Method) || isSameOriginRequest(r) {
			next.ServeHTTP(w, r)
			return
		}

		http.Error(w, "Forbidden", http.StatusForbidden)
	})
}

func isMutation(method string) bool {
	return method == http.MethodPost || method == http.MethodPatch || method == http.MethodDelete
}

func isSameOriginRequest(r *http.Request) bool {
	switch strings.ToLower(r.Header.Get("Sec-Fetch-Site")) {
	case "same-origin", "same-site", "none":
		return true
	case "cross-site":
		return false
	}

	if origin := r.Header.Get("Origin"); origin != "" {
		return requestOriginMatches(r, origin)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		return requestOriginMatches(r, referer)
	}
	return false
}

func requestOriginMatches(r *http.Request, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Scheme, requestScheme(r)) && strings.EqualFold(u.Host, r.Host)
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		return "https"
	}
	return "http"
}
