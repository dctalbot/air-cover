package app

import (
	"context"

	appcatalog "air-cover/internal/app/catalog"
)

type Catalog interface {
	ListShows(ctx context.Context) ([]appcatalog.Show, error)
	ListPersonas(ctx context.Context) ([]appcatalog.Persona, error)
}
