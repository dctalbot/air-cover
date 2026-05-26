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
	UntakeSubRequest(ctx context.Context, id int, userID int, updatedAt time.Time) error
}

type DashboardQuery interface {
	ListDashboardSubRequests(ctx context.Context) ([]DashboardReadModel, error)
}

type DetailQuery interface {
	GetSubRequestDetailByID(ctx context.Context, id int) (DetailReadModel, error)
}

type RequestReader interface {
	GetSubRequestByID(ctx context.Context, id int) (*domain.SubRequest, error)
}

type Notifier interface {
	SubRequestCreated(ctx context.Context, event SubRequestCreatedEvent) error
	SubRequestTaken(ctx context.Context, event SubRequestTakenEvent) error
	SubRequestUntaken(ctx context.Context, event SubRequestUntakenEvent) error
}

type ActiveUserLister interface {
	ListActiveUsers(ctx context.Context) ([]*domain.User, error)
}

type Repository interface {
	CommandRepository
	DashboardQuery
	DetailQuery
}

type Catalog interface {
	ListShows(ctx context.Context) ([]appcatalog.Show, error)
}
