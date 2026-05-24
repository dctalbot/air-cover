package api

import (
	adminapp "air-cover/internal/app/admin"
	subrequestsapp "air-cover/internal/app/subrequests"
)

type testServerRepository interface {
	authRepository
	adminapp.Repository
	subrequestsapp.Repository
}

type testCatalog interface {
	adminapp.Catalog
	subrequestsapp.Catalog
}

func newTestServer(repo testServerRepository, auth *AuthHandler, catalog testCatalog) *Server {
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
