package admin

import (
	"context"

	appcatalog "air-cover/internal/app/catalog"
	"air-cover/internal/domain"
)

type Repository interface {
	ListUsers(ctx context.Context) ([]*domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	CreateUser(ctx context.Context, email string, role string) (*domain.User, error)
	UpdateUser(ctx context.Context, id int, role *string, isEnabled *bool) error
	ImportUsers(ctx context.Context, emails []string) error
}

type Catalog interface {
	ListPersonas(ctx context.Context) ([]appcatalog.Persona, error)
}
