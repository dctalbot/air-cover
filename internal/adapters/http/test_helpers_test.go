package api

import (
	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	subrequestsapp "air-cover/internal/app/subrequests"
)

type testServerRepository interface {
	authapp.Repository
	adminapp.Repository
	subrequestsapp.Repository
}

type testCatalog interface {
	adminapp.Catalog
	subrequestsapp.Catalog
}

func newTestServer(repo testServerRepository, auth *AuthHandler, catalog testCatalog) *Server {
	var subRequests subRequestService
	var admin adminService
	if auth == nil && repo != nil {
		auth = NewAuthHandler(authapp.NewService(repo, nil))
	}
	if repo != nil {
		subRequests = subrequestsapp.NewService(repo, catalog)
		admin = adminapp.NewService(repo, catalog)
	}
	return NewServer(auth, subRequests, admin)
}
