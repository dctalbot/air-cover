package app

import (
	"context"

	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	appcatalog "air-cover/internal/app/catalog"
	subrequestsapp "air-cover/internal/app/subrequests"
)

type Repository interface {
	authapp.Repository
	adminapp.Repository
	subrequestsapp.Repository
}

type Catalog interface {
	adminapp.Catalog
	subrequestsapp.Catalog
	ListPersonas(ctx context.Context) ([]appcatalog.Persona, error)
}
