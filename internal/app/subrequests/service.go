package subrequests

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"air-cover/internal/app/session"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
	"air-cover/internal/policy"
	"air-cover/internal/spinitron"
)

var ErrCatalog = errors.New("show catalog error")

const maxCreateNotesLength = 1000

type Repository interface {
	ListSubRequests(ctx context.Context) ([]*domain.SubRequest, error)
	CreateSubRequest(ctx context.Context, sr *domain.SubRequest) error
	GetSubRequestByID(ctx context.Context, id int) (*domain.SubRequest, error)
	DeleteSubRequest(ctx context.Context, id int) error
	TakeSubRequest(ctx context.Context, id int, userID int) error
	UntakeSubRequest(ctx context.Context, id int) error
}

type Catalog interface {
	ListShows(ctx context.Context) ([]spinitron.Show, error)
}

type Service struct {
	repo    Repository
	catalog Catalog
	nowFunc func() time.Time
}

func NewService(repo Repository, catalog Catalog) *Service {
	return &Service{
		repo:    repo,
		catalog: catalog,
		nowFunc: time.Now,
	}
}

type Dashboard struct {
	Shows    []spinitron.Show
	Upcoming []SubRequest
	Past     []SubRequest
}

type SubRequest struct {
	Request   *domain.SubRequest
	ShowTitle string
	CanDelete bool
	CanTake   bool
	CanUntake bool
	IsPast    bool
}

type CreateInput struct {
	ShowID    int
	StartTime time.Time
	EndTime   time.Time
	Notes     string
}

type Action string

const (
	ActionTake   Action = "take"
	ActionUntake Action = "untake"
)

func (a Action) Valid() bool {
	return a == ActionTake || a == ActionUntake
}

func (s *Service) ListDashboard(ctx context.Context, viewer session.CurrentUser) (Dashboard, error) {
	if s.catalog == nil {
		return Dashboard{}, fmt.Errorf("%w: show catalog is not configured", ErrCatalog)
	}
	shows, err := s.catalog.ListShows(ctx)
	if err != nil {
		return Dashboard{}, fmt.Errorf("%w: %v", ErrCatalog, err)
	}

	sort.Slice(shows, func(i, j int) bool {
		return strings.ToLower(shows[i].Title) < strings.ToLower(shows[j].Title)
	})

	subRequests, err := s.repo.ListSubRequests(ctx)
	if err != nil {
		return Dashboard{}, err
	}

	showMap := showTitlesByID(shows)
	now := s.now()
	var upcoming []SubRequest
	var past []SubRequest

	for _, sr := range subRequests {
		item := SubRequest{
			Request:   sr,
			ShowTitle: showTitle(showMap, sr.ShowID),
			CanDelete: policy.CanDeleteSubRequest(viewer, sr),
			CanTake:   policy.CanTakeSubRequest(viewer, sr),
			CanUntake: policy.CanUntakeSubRequest(viewer, sr),
			IsPast:    sr.StartTime.Before(now),
		}
		if item.IsPast {
			past = append(past, item)
		} else {
			upcoming = append(upcoming, item)
		}
	}

	for i, j := 0, len(past)-1; i < j; i, j = i+1, j-1 {
		past[i], past[j] = past[j], past[i]
	}

	return Dashboard{
		Shows:    shows,
		Upcoming: upcoming,
		Past:     past,
	}, nil
}

func (s *Service) Create(ctx context.Context, viewer session.CurrentUser, input CreateInput) error {
	if err := s.validateCreateInput(ctx, input); err != nil {
		return err
	}

	now := s.now()
	if err := s.repo.CreateSubRequest(ctx, &domain.SubRequest{
		ShowID:         input.ShowID,
		PostedByUserID: viewer.ID,
		StartTime:      input.StartTime,
		EndTime:        input.EndTime,
		Notes:          input.Notes,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		return mapRepositoryError(err)
	}
	return nil
}

func (s *Service) validateCreateInput(ctx context.Context, input CreateInput) error {
	now := s.now()
	switch {
	case input.ShowID <= 0:
		return apperrors.ErrInvalid
	case !input.StartTime.After(now):
		return apperrors.ErrInvalid
	case !input.EndTime.After(input.StartTime):
		return apperrors.ErrInvalid
	case len(input.Notes) > maxCreateNotesLength:
		return apperrors.ErrInvalid
	}

	if s.catalog == nil {
		return nil
	}
	shows, err := s.catalog.ListShows(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCatalog, err)
	}
	if !showExists(shows, input.ShowID) {
		return apperrors.ErrInvalid
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, viewer session.CurrentUser, id int) error {
	sr, err := s.repo.GetSubRequestByID(ctx, id)
	if err != nil {
		return mapRepositoryError(err)
	}
	if !policy.CanDeleteSubRequest(viewer, sr) {
		return apperrors.ErrForbidden
	}
	if err := s.repo.DeleteSubRequest(ctx, id); err != nil {
		return mapRepositoryError(err)
	}
	return nil
}

func (s *Service) ApplyAction(ctx context.Context, viewer session.CurrentUser, id int, action Action) error {
	if !action.Valid() {
		return apperrors.ErrInvalid
	}

	sr, err := s.repo.GetSubRequestByID(ctx, id)
	if err != nil {
		return mapRepositoryError(err)
	}

	switch action {
	case ActionTake:
		if err := s.repo.TakeSubRequest(ctx, id, viewer.ID); err != nil {
			return mapRepositoryError(err)
		}
	case ActionUntake:
		if !policy.CanUntakeSubRequest(viewer, sr) {
			return apperrors.ErrForbidden
		}
		if err := s.repo.UntakeSubRequest(ctx, id); err != nil {
			return mapRepositoryError(err)
		}
	}
	return nil
}

func mapRepositoryError(err error) error {
	if errors.Is(err, apperrors.ErrNotFound) {
		return apperrors.ErrNotFound
	}
	if errors.Is(err, apperrors.ErrConflict) {
		return apperrors.ErrConflict
	}
	return err
}

func showTitlesByID(shows []spinitron.Show) map[int]string {
	showMap := make(map[int]string)
	for _, show := range shows {
		id, err := strconv.Atoi(show.ID)
		if err != nil {
			continue
		}
		showMap[id] = show.Title
	}
	return showMap
}

func showTitle(showMap map[int]string, showID int) string {
	if title := showMap[showID]; title != "" {
		return title
	}
	return "Unknown Show"
}

func showExists(shows []spinitron.Show, showID int) bool {
	for _, show := range shows {
		id, err := strconv.Atoi(show.ID)
		if err != nil {
			continue
		}
		if id == showID {
			return true
		}
	}
	return false
}

func (s *Service) now() time.Time {
	if s.nowFunc != nil {
		return s.nowFunc()
	}
	return time.Now()
}
