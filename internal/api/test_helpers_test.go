package api

import (
	coreapp "air-cover/internal/app"
	adminapp "air-cover/internal/app/admin"
	subrequestsapp "air-cover/internal/app/subrequests"
)

type testServerRepository interface {
	authRepository
	adminapp.Repository
	subrequestsapp.Repository
}

func newTestServer(repo testServerRepository, auth *AuthHandler, catalog coreapp.Catalog) *Server {
	var subRequests *subrequestsapp.Service
	var admin *adminapp.Service
	if auth == nil && repo != nil {
		auth = NewAuthHandler(repo, nil)
	}
	if repo != nil {
		subRequests = subrequestsapp.NewService(repo, catalog)
		admin = adminapp.NewService(repo, catalog)
	}
	return NewServer(auth, subRequests, admin)
}
