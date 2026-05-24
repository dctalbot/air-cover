package cmd

import (
	"log/slog"
	"time"

	httpadapter "air-cover/internal/adapters/http"
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
		r.Get("/app", wrapper.GetApp)
		r.Post("/auth/logout", wrapper.PostAuthLogout)
		r.Post("/sub-requests", wrapper.PostSubRequests)
		r.Delete("/sub-requests/{id}", wrapper.DeleteSubRequestsId)
		r.Patch("/sub-requests/{id}", wrapper.PatchSubRequestsId)
	})

	r.Group(func(r chi.Router) {
		r.Use(authHandler.AuthMiddleware)
		r.Use(authHandler.RequireAdmin)
		r.Get("/admin", wrapper.GetAdmin)
		r.Post("/users", wrapper.PostUsers)
		r.Post("/users/import/spinitron", wrapper.PostUsersImportSpinitron)
		r.Post("/users/{id}", wrapper.PostUsersId)
	})

	return r
}
