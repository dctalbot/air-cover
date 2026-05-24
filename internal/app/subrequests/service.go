package subrequests

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	appcatalog "air-cover/internal/app/catalog"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

var ErrCatalog = errors.New("show catalog error")

const maxCreateNotesLength = 1000

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
	Shows    []appcatalog.Show
	Upcoming []DashboardSubRequest
	Past     []DashboardSubRequest
}

type DashboardReadModel struct {
	Request        *domain.SubRequest
	RequesterEmail string
	TakerEmail     string
}

type DashboardSubRequest struct {
	Request        *domain.SubRequest
	RequesterEmail string
	TakerEmail     string
	ShowTitle      string
	CanDelete      bool
	CanTake        bool
	CanUntake      bool
	IsPast         bool
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

func (s *Service) ListDashboard(ctx context.Context, viewer domain.CurrentUser) (Dashboard, error) {
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

	subRequests, err := s.repo.ListDashboardSubRequests(ctx)
	if err != nil {
		return Dashboard{}, err
	}

	showMap := showTitlesByID(shows)
	now := s.now()
	var upcoming []DashboardSubRequest
	var past []DashboardSubRequest

	for _, record := range subRequests {
		sr := record.Request
		item := DashboardSubRequest{
			Request:        sr,
			RequesterEmail: record.RequesterEmail,
			TakerEmail:     record.TakerEmail,
			ShowTitle:      showTitle(showMap, sr.ShowID),
			CanDelete:      sr.CanBeDeletedBy(viewer),
			CanTake:        sr.CanBeTakenBy(viewer),
			CanUntake:      sr.CanBeUntakenBy(viewer),
			IsPast:         sr.StartTime.Before(now),
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

func (s *Service) Create(ctx context.Context, viewer domain.CurrentUser, input CreateInput) error {
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
	case !(&domain.SubRequest{StartTime: input.StartTime, EndTime: input.EndTime}).HasValidTimeRange():
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

func (s *Service) Delete(ctx context.Context, viewer domain.CurrentUser, id int) error {
	sr, err := s.repo.GetSubRequestByID(ctx, id)
	if err != nil {
		return mapRepositoryError(err)
	}
	if !sr.CanBeDeletedBy(viewer) {
		return apperrors.ErrForbidden
	}
	if err := s.repo.DeleteSubRequest(ctx, id); err != nil {
		return mapRepositoryError(err)
	}
	return nil
}

func (s *Service) ApplyAction(ctx context.Context, viewer domain.CurrentUser, id int, action Action) error {
	if !action.Valid() {
		return apperrors.ErrInvalid
	}

	sr, err := s.repo.GetSubRequestByID(ctx, id)
	if err != nil {
		return mapRepositoryError(err)
	}

	switch action {
	case ActionTake:
		if err := s.repo.TakeSubRequest(ctx, id, viewer.ID, s.now()); err != nil {
			return mapRepositoryError(err)
		}
	case ActionUntake:
		if !sr.CanBeUntakenBy(viewer) {
			return apperrors.ErrForbidden
		}
		if err := s.repo.UntakeSubRequest(ctx, id, s.now()); err != nil {
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

func showTitlesByID(shows []appcatalog.Show) map[int]string {
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

func showExists(shows []appcatalog.Show, showID int) bool {
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
