package subrequests

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	repo     Repository
	catalog  Catalog
	notifier Notifier
	nowFunc  func() time.Time
}

func NewService(repo Repository, catalog Catalog) *Service {
	return &Service{
		repo:    repo,
		catalog: catalog,
		nowFunc: time.Now,
	}
}

func (s *Service) SetNotifier(notifier Notifier) {
	s.notifier = notifier
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

type DetailReadModel struct {
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

type Detail struct {
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

type SubRequestCreatedEvent struct {
	Request        *domain.SubRequest
	RequesterEmail string
	ShowTitle      string
	DetailPath     string
}

type SubRequestTakenEvent struct {
	Request        *domain.SubRequest
	RequesterEmail string
	TakerEmail     string
	ShowTitle      string
	DetailPath     string
}

type SubRequestUntakenEvent struct {
	Request        *domain.SubRequest
	RequesterEmail string
	UntakerEmail   string
	ShowTitle      string
	DetailPath     string
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

func (s *Service) Get(ctx context.Context, viewer domain.CurrentUser, id int) (Detail, error) {
	record, err := s.repo.GetSubRequestDetailByID(ctx, id)
	if err != nil {
		return Detail{}, mapRepositoryError(err)
	}

	showTitleValue, err := s.showTitleFor(ctx, record.Request.ShowID)
	if err != nil {
		return Detail{}, err
	}

	sr := record.Request
	return Detail{
		Request:        sr,
		RequesterEmail: record.RequesterEmail,
		TakerEmail:     record.TakerEmail,
		ShowTitle:      showTitleValue,
		CanDelete:      sr.CanBeDeletedBy(viewer),
		CanTake:        sr.CanBeTakenBy(viewer),
		CanUntake:      sr.CanBeUntakenBy(viewer),
		IsPast:         sr.StartTime.Before(s.now()),
	}, nil
}

func (s *Service) Create(ctx context.Context, viewer domain.CurrentUser, input CreateInput) error {
	if err := s.validateCreateInput(ctx, input); err != nil {
		return err
	}

	now := s.now()
	sr := &domain.SubRequest{
		ShowID:         input.ShowID,
		PostedByUserID: viewer.ID,
		StartTime:      input.StartTime,
		EndTime:        input.EndTime,
		Notes:          input.Notes,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.CreateSubRequest(ctx, sr); err != nil {
		return mapRepositoryError(err)
	}
	s.notifySubRequestCreated(ctx, viewer, sr)
	return nil
}

func (s *Service) notifySubRequestCreated(ctx context.Context, viewer domain.CurrentUser, sr *domain.SubRequest) {
	if s.notifier == nil {
		return
	}
	showTitleValue, err := s.showTitleFor(ctx, sr.ShowID)
	if err != nil {
		slog.Error("Failed to load show title for sub request notification", "sub_request_id", sr.ID, "error", err)
		showTitleValue = "Unknown Show"
	}
	if err := s.notifier.SubRequestCreated(ctx, SubRequestCreatedEvent{
		Request:        sr,
		RequesterEmail: viewer.Email,
		ShowTitle:      showTitleValue,
		DetailPath:     fmt.Sprintf("/sub-requests/%d", sr.ID),
	}); err != nil {
		slog.Error("Failed to queue sub request notification", "sub_request_id", sr.ID, "error", err)
	}
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
		s.notifySubRequestTaken(ctx, id)
	case ActionUntake:
		if !sr.CanBeUntakenBy(viewer) {
			return apperrors.ErrForbidden
		}
		var event SubRequestUntakenEvent
		if s.notifier != nil {
			event = s.subRequestUntakenEvent(ctx, id, viewer.Email)
		}
		if err := s.repo.UntakeSubRequest(ctx, id, s.now()); err != nil {
			return mapRepositoryError(err)
		}
		s.notifySubRequestUntaken(ctx, event)
	}
	return nil
}

func (s *Service) notifySubRequestTaken(ctx context.Context, id int) {
	if s.notifier == nil {
		return
	}
	record, err := s.repo.GetSubRequestDetailByID(ctx, id)
	if err != nil {
		slog.Error("Failed to load sub request detail for taken notification", "sub_request_id", id, "error", err)
		return
	}
	showTitleValue, err := s.showTitleFor(ctx, record.Request.ShowID)
	if err != nil {
		slog.Error("Failed to load show title for sub request taken notification", "sub_request_id", id, "error", err)
		showTitleValue = "Unknown Show"
	}
	if err := s.notifier.SubRequestTaken(ctx, SubRequestTakenEvent{
		Request:        record.Request,
		RequesterEmail: record.RequesterEmail,
		TakerEmail:     record.TakerEmail,
		ShowTitle:      showTitleValue,
		DetailPath:     fmt.Sprintf("/sub-requests/%d", record.Request.ID),
	}); err != nil {
		slog.Error("Failed to queue sub request taken notification", "sub_request_id", record.Request.ID, "error", err)
	}
}

func (s *Service) subRequestUntakenEvent(ctx context.Context, id int, untakerEmail string) SubRequestUntakenEvent {
	record, err := s.repo.GetSubRequestDetailByID(ctx, id)
	if err != nil {
		slog.Error("Failed to load sub request detail for untaken notification", "sub_request_id", id, "error", err)
		return SubRequestUntakenEvent{}
	}
	if record.Request == nil {
		slog.Error("Sub request detail missing request for untaken notification", "sub_request_id", id)
		return SubRequestUntakenEvent{}
	}
	showTitleValue, err := s.showTitleFor(ctx, record.Request.ShowID)
	if err != nil {
		slog.Error("Failed to load show title for sub request untaken notification", "sub_request_id", id, "error", err)
		showTitleValue = "Unknown Show"
	}
	return SubRequestUntakenEvent{
		Request:        record.Request,
		RequesterEmail: record.RequesterEmail,
		UntakerEmail:   untakerEmail,
		ShowTitle:      showTitleValue,
		DetailPath:     fmt.Sprintf("/sub-requests/%d", record.Request.ID),
	}
}

func (s *Service) notifySubRequestUntaken(ctx context.Context, event SubRequestUntakenEvent) {
	if s.notifier == nil || event.Request == nil {
		return
	}
	if err := s.notifier.SubRequestUntaken(ctx, event); err != nil {
		slog.Error("Failed to queue sub request untaken notification", "sub_request_id", event.Request.ID, "error", err)
	}
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

func (s *Service) showTitleFor(ctx context.Context, showID int) (string, error) {
	if s.catalog == nil {
		return "Unknown Show", nil
	}
	shows, err := s.catalog.ListShows(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrCatalog, err)
	}
	return showTitle(showTitlesByID(shows), showID), nil
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
