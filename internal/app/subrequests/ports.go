package subrequests

import (
	"context"
	"time"

	appcatalog "air-cover/internal/app/catalog"
	"air-cover/internal/domain"
)

type CommandRepository interface {
	CreateSubRequest(ctx context.Context, sr *domain.SubRequest) error
	GetSubRequestByID(ctx context.Context, id int) (*domain.SubRequest, error)
	DeleteSubRequest(ctx context.Context, id int) error
	TakeSubRequest(ctx context.Context, id int, userID int, updatedAt time.Time) error
	UntakeSubRequest(ctx context.Context, id int, updatedAt time.Time) error
}

type DashboardQuery interface {
	ListDashboardSubRequests(ctx context.Context) ([]DashboardReadModel, error)
}

type RequestReader interface {
	GetSubRequestByID(ctx context.Context, id int) (*domain.SubRequest, error)
}

type Repository interface {
	CommandRepository
	DashboardQuery
}

type Catalog interface {
	ListShows(ctx context.Context) ([]appcatalog.Show, error)
}
