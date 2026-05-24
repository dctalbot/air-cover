package app

import (
	"context"

	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/spinitron"
)

type Repository interface {
	authapp.Repository
	adminapp.Repository
	subrequestsapp.Repository
}

type ShowCatalog interface {
	adminapp.Catalog
	subrequestsapp.Catalog
	ListPersonas(ctx context.Context) ([]spinitron.Persona, error)
}
