package subrequests

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appcatalog "air-cover/internal/app/catalog"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

type fakeRepository struct {
	subRequests []DashboardReadModel
	detail      DetailReadModel
	subRequest  *domain.SubRequest
	listErr     error
	detailErr   error
	createErr   error
	getErr      error
	deleteErr   error
	takeErr     error
	untakeErr   error
	created     *domain.SubRequest
	deletedID   int
	takenID     int
	takenUserID int
	takenAt     time.Time
	untakenID   int
	untakenAt   time.Time
}

func (f *fakeRepository) ListDashboardSubRequests(ctx context.Context) ([]DashboardReadModel, error) {
	return f.subRequests, f.listErr
}

func (f *fakeRepository) GetSubRequestDetailByID(ctx context.Context, id int) (DetailReadModel, error) {
	if f.detailErr != nil {
		return DetailReadModel{}, f.detailErr
	}
	return f.detail, nil
}

func (f *fakeRepository) CreateSubRequest(ctx context.Context, sr *domain.SubRequest) error {
	sr.ID = 42
	f.created = sr
	return f.createErr
}

func (f *fakeRepository) GetSubRequestByID(ctx context.Context, id int) (*domain.SubRequest, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.subRequest, nil
}

func (f *fakeRepository) DeleteSubRequest(ctx context.Context, id int) error {
	f.deletedID = id
	return f.deleteErr
}

func (f *fakeRepository) TakeSubRequest(ctx context.Context, id int, userID int, updatedAt time.Time) error {
	f.takenID = id
	f.takenUserID = userID
	f.takenAt = updatedAt
	return f.takeErr
}

func (f *fakeRepository) UntakeSubRequest(ctx context.Context, id int, updatedAt time.Time) error {
	f.untakenID = id
	f.untakenAt = updatedAt
	return f.untakeErr
}

type fakeCatalog struct {
	shows     []appcatalog.Show
	err       error
	calls     int
	errOnCall int
}

func (f *fakeCatalog) ListShows(ctx context.Context) ([]appcatalog.Show, error) {
	f.calls++
	if f.err != nil && (f.errOnCall == 0 || f.calls == f.errOnCall) {
		return nil, f.err
	}
	return f.shows, nil
}

type fakeNotifier struct {
	event        SubRequestCreatedEvent
	takenEvent   SubRequestTakenEvent
	untakenEvent SubRequestUntakenEvent
	err          error
	calls        int
	takenCalls   int
	untakenCalls int
}

func (f *fakeNotifier) SubRequestCreated(ctx context.Context, event SubRequestCreatedEvent) error {
	f.calls++
	f.event = event
	return f.err
}

func (f *fakeNotifier) SubRequestTaken(ctx context.Context, event SubRequestTakenEvent) error {
	f.takenCalls++
	f.takenEvent = event
	return f.err
}

func (f *fakeNotifier) SubRequestUntaken(ctx context.Context, event SubRequestUntakenEvent) error {
	f.untakenCalls++
	f.untakenEvent = event
	return f.err
}

func TestListDashboard(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	takerID := 2
	repo := &fakeRepository{subRequests: []DashboardReadModel{
		{Request: &domain.SubRequest{ID: 1, ShowID: 2, PostedByUserID: 1, StartTime: now.Add(2 * time.Hour), EndTime: now.Add(3 * time.Hour)}},
		{Request: &domain.SubRequest{ID: 2, ShowID: 999, PostedByUserID: 1, TakenByUserID: &takerID, StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-1 * time.Hour)}},
		{Request: &domain.SubRequest{ID: 3, ShowID: 1, PostedByUserID: 3, StartTime: now.Add(-4 * time.Hour), EndTime: now.Add(-3 * time.Hour)}},
	}}
	svc := NewService(repo, &fakeCatalog{shows: []appcatalog.Show{
		{ID: "2", Title: "Beta"},
		{ID: "bad", Title: "Bad ID"},
		{ID: "1", Title: "Alpha"},
	}})
	svc.nowFunc = func() time.Time { return now }

	dashboard, err := svc.ListDashboard(context.Background(), domain.CurrentUser{ID: takerID, Role: domain.RoleMember})
	if err != nil {
		t.Fatalf("ListDashboard returned error: %v", err)
	}
	if dashboard.Shows[0].Title != "Alpha" || dashboard.Shows[1].Title != "Bad ID" || dashboard.Shows[2].Title != "Beta" {
		t.Fatalf("shows not sorted by title: %+v", dashboard.Shows)
	}
	if len(dashboard.Upcoming) != 1 || dashboard.Upcoming[0].ShowTitle != "Beta" {
		t.Fatalf("unexpected upcoming dashboard: %+v", dashboard.Upcoming)
	}
	if len(dashboard.Past) != 2 || dashboard.Past[1].Request.ID != 2 || dashboard.Past[1].ShowTitle != "Unknown Show" {
		t.Fatalf("unexpected past dashboard: %+v", dashboard.Past)
	}
	if !dashboard.Past[1].CanUntake {
		t.Error("expected taker to be able to untake their request")
	}
}

func TestListDashboardErrors(t *testing.T) {
	if _, err := NewService(&fakeRepository{}, nil).
		ListDashboard(context.Background(), domain.CurrentUser{}); !errors.Is(err, ErrCatalog) {
		t.Errorf("nil catalog error = %v, want ErrCatalog", err)
	}
	if _, err := NewService(&fakeRepository{}, &fakeCatalog{err: errors.New("down")}).
		ListDashboard(context.Background(), domain.CurrentUser{}); !errors.Is(err, ErrCatalog) {
		t.Errorf("catalog error = %v, want ErrCatalog", err)
	}

	wantErr := errors.New("list failed")
	if _, err := NewService(&fakeRepository{listErr: wantErr}, &fakeCatalog{}).
		ListDashboard(context.Background(), domain.CurrentUser{}); !errors.Is(err, wantErr) {
		t.Errorf("list error = %v, want %v", err, wantErr)
	}
}

func TestGet(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	takerID := 2
	repo := &fakeRepository{detail: DetailReadModel{
		Request: &domain.SubRequest{
			ID:             7,
			ShowID:         2,
			PostedByUserID: 1,
			TakenByUserID:  &takerID,
			StartTime:      now.Add(2 * time.Hour),
			EndTime:        now.Add(4 * time.Hour),
			Notes:          "Bring headphones",
		},
		RequesterEmail: "requester@example.com",
		TakerEmail:     "taker@example.com",
	}}
	svc := NewService(repo, &fakeCatalog{shows: []appcatalog.Show{{ID: "2", Title: "Detail Show"}}})
	svc.nowFunc = func() time.Time { return now }

	detail, err := svc.Get(context.Background(), domain.CurrentUser{ID: takerID, Role: domain.RoleMember}, 7)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if detail.ShowTitle != "Detail Show" {
		t.Fatalf("show title = %q, want Detail Show", detail.ShowTitle)
	}
	if detail.RequesterEmail != "requester@example.com" || detail.TakerEmail != "taker@example.com" {
		t.Fatalf("unexpected emails: %+v", detail)
	}
	if !detail.CanUntake || detail.CanTake || detail.IsPast {
		t.Fatalf("unexpected permissions/past flag: %+v", detail)
	}
}

func TestGetErrors(t *testing.T) {
	if _, err := NewService(&fakeRepository{detailErr: apperrors.ErrNotFound}, nil).
		Get(context.Background(), domain.CurrentUser{}, 99); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("not found error = %v, want app not found", err)
	}

	svc := NewService(&fakeRepository{detail: DetailReadModel{Request: &domain.SubRequest{ShowID: 1}}}, &fakeCatalog{err: errors.New("down")})
	if _, err := svc.Get(context.Background(), domain.CurrentUser{}, 1); !errors.Is(err, ErrCatalog) {
		t.Errorf("catalog error = %v, want ErrCatalog", err)
	}

	svc = NewService(&fakeRepository{detail: DetailReadModel{Request: &domain.SubRequest{ShowID: 999}}}, &fakeCatalog{shows: []appcatalog.Show{{ID: "bad", Title: "Bad ID"}}})
	detail, err := svc.Get(context.Background(), domain.CurrentUser{}, 1)
	if err != nil {
		t.Fatalf("Get unknown show returned error: %v", err)
	}
	if detail.ShowTitle != "Unknown Show" {
		t.Fatalf("show title = %q, want Unknown Show", detail.ShowTitle)
	}

	detail, err = NewService(&fakeRepository{detail: DetailReadModel{Request: &domain.SubRequest{ShowID: 1}}}, nil).
		Get(context.Background(), domain.CurrentUser{}, 1)
	if err != nil {
		t.Fatalf("Get with nil catalog returned error: %v", err)
	}
	if detail.ShowTitle != "Unknown Show" {
		t.Fatalf("nil catalog show title = %q, want Unknown Show", detail.ShowTitle)
	}
}

func TestCreate(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{}
	svc := NewService(repo, &fakeCatalog{shows: []appcatalog.Show{{ID: "1", Title: "Test Show"}}})
	notifier := &fakeNotifier{}
	svc.SetNotifier(notifier)
	svc.nowFunc = func() time.Time { return now }
	input := CreateInput{
		ShowID:    1,
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(2 * time.Hour),
		Notes:     "please help",
	}

	if err := svc.Create(context.Background(), domain.CurrentUser{ID: 7, Email: "requester@example.com"}, input); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if repo.created.PostedByUserID != 7 || repo.created.CreatedAt != now {
		t.Errorf("unexpected created request: %+v", repo.created)
	}
	if notifier.calls != 1 {
		t.Fatalf("expected notifier to be called once, got %d", notifier.calls)
	}
	if notifier.event.ShowTitle != "Test Show" || notifier.event.RequesterEmail != "requester@example.com" {
		t.Fatalf("unexpected notification event: %+v", notifier.event)
	}
	if notifier.event.DetailPath != "/sub-requests/42" {
		t.Fatalf("detail path = %q, want /sub-requests/42", notifier.event.DetailPath)
	}

	catalogErrSvc := NewService(&fakeRepository{}, &fakeCatalog{err: errors.New("catalog down")})
	catalogErrSvc.nowFunc = func() time.Time { return now }
	if err := catalogErrSvc.Create(context.Background(), domain.CurrentUser{ID: 7}, input); !errors.Is(err, ErrCatalog) {
		t.Errorf("catalog error = %v, want ErrCatalog", err)
	}

	wantErr := errors.New("create failed")
	createErrSvc := NewService(&fakeRepository{createErr: wantErr}, nil)
	createErrSvc.nowFunc = func() time.Time { return now }
	if err := createErrSvc.Create(context.Background(), domain.CurrentUser{ID: 7}, input); !errors.Is(err, wantErr) {
		t.Errorf("create error = %v, want %v", err, wantErr)
	}
}

func TestCreateNotificationErrorsDoNotFailCreate(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	input := CreateInput{
		ShowID:    1,
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(2 * time.Hour),
	}

	notifyErrSvc := NewService(&fakeRepository{}, &fakeCatalog{shows: []appcatalog.Show{{ID: "1", Title: "Test Show"}}})
	notifyErrSvc.SetNotifier(&fakeNotifier{err: errors.New("queue failed")})
	notifyErrSvc.nowFunc = func() time.Time { return now }
	if err := notifyErrSvc.Create(context.Background(), domain.CurrentUser{ID: 7, Email: "requester@example.com"}, input); err != nil {
		t.Fatalf("Create with notification error returned error: %v", err)
	}

	postPersistCatalogErrSvc := NewService(&fakeRepository{}, &fakeCatalog{
		shows:     []appcatalog.Show{{ID: "1", Title: "Test Show"}},
		err:       errors.New("catalog down"),
		errOnCall: 2,
	})
	postPersistCatalogErrSvc.SetNotifier(&fakeNotifier{})
	postPersistCatalogErrSvc.nowFunc = func() time.Time { return now }
	if err := postPersistCatalogErrSvc.Create(context.Background(), domain.CurrentUser{ID: 7}, input); err != nil {
		t.Fatalf("Create with post-persist catalog fallback returned error: %v", err)
	}
}

func TestCreateWithoutNotifier(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	svc := NewService(&fakeRepository{}, &fakeCatalog{shows: []appcatalog.Show{{ID: "1", Title: "Test Show"}}})
	svc.nowFunc = func() time.Time { return now }

	err := svc.Create(context.Background(), domain.CurrentUser{ID: 7}, CreateInput{
		ShowID:    1,
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Create without notifier returned error: %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	valid := CreateInput{
		ShowID:    1,
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(2 * time.Hour),
		Notes:     "please help",
	}

	tests := []struct {
		name  string
		input CreateInput
	}{
		{name: "missing show id", input: CreateInput{ShowID: 0, StartTime: valid.StartTime, EndTime: valid.EndTime}},
		{name: "past start time", input: CreateInput{ShowID: 1, StartTime: now.Add(-time.Minute), EndTime: valid.EndTime}},
		{name: "start time equal now", input: CreateInput{ShowID: 1, StartTime: now, EndTime: valid.EndTime}},
		{name: "end time equal start time", input: CreateInput{ShowID: 1, StartTime: valid.StartTime, EndTime: valid.StartTime}},
		{name: "end time before start time", input: CreateInput{ShowID: 1, StartTime: valid.StartTime, EndTime: valid.StartTime.Add(-time.Minute)}},
		{name: "notes too long", input: CreateInput{ShowID: 1, StartTime: valid.StartTime, EndTime: valid.EndTime, Notes: strings.Repeat("a", maxCreateNotesLength+1)}},
		{name: "unknown catalog show", input: CreateInput{ShowID: 2, StartTime: valid.StartTime, EndTime: valid.EndTime}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{}
			svc := NewService(repo, &fakeCatalog{shows: []appcatalog.Show{{ID: "1", Title: "Test Show"}, {ID: "bad", Title: "Bad ID"}}})
			svc.nowFunc = func() time.Time { return now }

			if err := svc.Create(context.Background(), domain.CurrentUser{ID: 7}, tt.input); !errors.Is(err, apperrors.ErrInvalid) {
				t.Fatalf("Create error = %v, want ErrInvalid", err)
			}
			if repo.created != nil {
				t.Errorf("invalid input should not create request: %+v", repo.created)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	repo := &fakeRepository{subRequest: &domain.SubRequest{ID: 1, PostedByUserID: 1}}
	svc := NewService(repo, nil)
	viewer := domain.CurrentUser{ID: 1, Role: domain.RoleMember}
	if err := svc.Delete(context.Background(), viewer, 1); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if repo.deletedID != 1 {
		t.Errorf("deleted ID = %d, want 1", repo.deletedID)
	}

	repo.subRequest = &domain.SubRequest{ID: 2, PostedByUserID: 2}
	if err := svc.Delete(context.Background(), viewer, 2); !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("forbidden delete error = %v, want forbidden", err)
	}

	repo.getErr = apperrors.ErrNotFound
	if err := svc.Delete(context.Background(), viewer, 3); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("not found delete error = %v, want app not found", err)
	}
	repo.getErr = nil
	repo.subRequest = &domain.SubRequest{ID: 1, PostedByUserID: 1}
	repo.deleteErr = apperrors.ErrConflict
	if err := svc.Delete(context.Background(), viewer, 1); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("delete conflict error = %v, want conflict", err)
	}
	repo.deleteErr = errors.New("delete failed")
	if err := svc.Delete(context.Background(), viewer, 1); err == nil {
		t.Error("expected raw delete error")
	}
}

func TestApplyAction(t *testing.T) {
	takerID := 2
	repo := &fakeRepository{subRequest: &domain.SubRequest{ID: 1, PostedByUserID: 1}}
	svc := NewService(repo, nil)
	viewer := domain.CurrentUser{ID: takerID, Role: domain.RoleMember}

	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionTake); err != nil {
		t.Fatalf("take returned error: %v", err)
	}
	if repo.takenID != 1 || repo.takenUserID != takerID || repo.takenAt.IsZero() {
		t.Errorf("unexpected take call: %+v", repo)
	}

	repo.subRequest = &domain.SubRequest{ID: 1, PostedByUserID: 1, TakenByUserID: &takerID}
	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionUntake); err != nil {
		t.Fatalf("untake returned error: %v", err)
	}
	if repo.untakenID != 1 || repo.untakenAt.IsZero() {
		t.Errorf("untaken ID = %d, want 1", repo.untakenID)
	}

	if err := svc.ApplyAction(context.Background(), viewer, 1, Action("bad")); !errors.Is(err, apperrors.ErrInvalid) {
		t.Errorf("invalid action error = %v, want invalid", err)
	}

	repo.subRequest = &domain.SubRequest{ID: 1, PostedByUserID: 1}
	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionUntake); !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("untake forbidden error = %v, want forbidden", err)
	}

	repo.takeErr = apperrors.ErrConflict
	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionTake); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("take conflict error = %v, want conflict", err)
	}
	repo.takeErr = nil
	repo.getErr = apperrors.ErrNotFound
	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionTake); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("take lookup error = %v, want not found", err)
	}
	repo.getErr = nil
	repo.untakeErr = apperrors.ErrNotFound
	repo.subRequest = &domain.SubRequest{ID: 1, PostedByUserID: 1, TakenByUserID: &takerID}
	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionUntake); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("untake error = %v, want not found", err)
	}
}

func TestApplyActionTakeNotifiesRequesterAndTaker(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	takerID := 2
	repo := &fakeRepository{
		subRequest: &domain.SubRequest{ID: 1, PostedByUserID: 1},
		detail: DetailReadModel{
			Request: &domain.SubRequest{
				ID:             1,
				ShowID:         2,
				PostedByUserID: 1,
				TakenByUserID:  &takerID,
				StartTime:      now.Add(time.Hour),
				EndTime:        now.Add(2 * time.Hour),
			},
			RequesterEmail: "requester@example.com",
			TakerEmail:     "taker@example.com",
		},
	}
	svc := NewService(repo, &fakeCatalog{shows: []appcatalog.Show{{ID: "2", Title: "Test Show"}}})
	notifier := &fakeNotifier{}
	svc.SetNotifier(notifier)
	svc.nowFunc = func() time.Time { return now }

	if err := svc.ApplyAction(context.Background(), domain.CurrentUser{ID: takerID, Role: domain.RoleMember}, 1, ActionTake); err != nil {
		t.Fatalf("take returned error: %v", err)
	}
	if notifier.takenCalls != 1 {
		t.Fatalf("expected taken notifier to be called once, got %d", notifier.takenCalls)
	}
	if notifier.takenEvent.RequesterEmail != "requester@example.com" || notifier.takenEvent.TakerEmail != "taker@example.com" {
		t.Fatalf("unexpected taken notification emails: %+v", notifier.takenEvent)
	}
	if notifier.takenEvent.ShowTitle != "Test Show" {
		t.Fatalf("show title = %q, want Test Show", notifier.takenEvent.ShowTitle)
	}
	if notifier.takenEvent.DetailPath != "/sub-requests/1" {
		t.Fatalf("detail path = %q, want /sub-requests/1", notifier.takenEvent.DetailPath)
	}
}

func TestApplyActionUntakeNotifiesRequesterAndUntaker(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	takerID := 2
	repo := &fakeRepository{
		subRequest: &domain.SubRequest{ID: 1, PostedByUserID: 1, TakenByUserID: &takerID},
		detail: DetailReadModel{
			Request: &domain.SubRequest{
				ID:             1,
				ShowID:         2,
				PostedByUserID: 1,
				TakenByUserID:  &takerID,
				StartTime:      now.Add(time.Hour),
				EndTime:        now.Add(2 * time.Hour),
			},
			RequesterEmail: "requester@example.com",
			TakerEmail:     "stale-taker@example.com",
		},
	}
	svc := NewService(repo, &fakeCatalog{shows: []appcatalog.Show{{ID: "2", Title: "Test Show"}}})
	notifier := &fakeNotifier{}
	svc.SetNotifier(notifier)
	svc.nowFunc = func() time.Time { return now }

	if err := svc.ApplyAction(context.Background(), domain.CurrentUser{ID: takerID, Email: "untaker@example.com", Role: domain.RoleMember}, 1, ActionUntake); err != nil {
		t.Fatalf("untake returned error: %v", err)
	}
	if notifier.untakenCalls != 1 {
		t.Fatalf("expected untaken notifier to be called once, got %d", notifier.untakenCalls)
	}
	if notifier.untakenEvent.RequesterEmail != "requester@example.com" || notifier.untakenEvent.UntakerEmail != "untaker@example.com" {
		t.Fatalf("unexpected untaken notification emails: %+v", notifier.untakenEvent)
	}
	if notifier.untakenEvent.ShowTitle != "Test Show" {
		t.Fatalf("show title = %q, want Test Show", notifier.untakenEvent.ShowTitle)
	}
	if notifier.untakenEvent.DetailPath != "/sub-requests/1" {
		t.Fatalf("detail path = %q, want /sub-requests/1", notifier.untakenEvent.DetailPath)
	}
}

func TestApplyActionTakeNotificationErrorsDoNotFailTake(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	takerID := 2
	repo := &fakeRepository{
		subRequest: &domain.SubRequest{ID: 1, PostedByUserID: 1},
		detail: DetailReadModel{
			Request: &domain.SubRequest{
				ID:             1,
				ShowID:         2,
				PostedByUserID: 1,
				TakenByUserID:  &takerID,
				StartTime:      now.Add(time.Hour),
				EndTime:        now.Add(2 * time.Hour),
			},
			RequesterEmail: "requester@example.com",
			TakerEmail:     "taker@example.com",
		},
	}
	svc := NewService(repo, &fakeCatalog{err: errors.New("catalog down")})
	notifier := &fakeNotifier{err: errors.New("queue failed")}
	svc.SetNotifier(notifier)
	svc.nowFunc = func() time.Time { return now }

	if err := svc.ApplyAction(context.Background(), domain.CurrentUser{ID: takerID, Role: domain.RoleMember}, 1, ActionTake); err != nil {
		t.Fatalf("take with notification errors returned error: %v", err)
	}
	if notifier.takenCalls != 1 {
		t.Fatalf("expected taken notifier to be called once, got %d", notifier.takenCalls)
	}
	if notifier.takenEvent.ShowTitle != "Unknown Show" {
		t.Fatalf("show title = %q, want Unknown Show", notifier.takenEvent.ShowTitle)
	}

	repo.detailErr = errors.New("detail reload failed")
	if err := svc.ApplyAction(context.Background(), domain.CurrentUser{ID: takerID, Role: domain.RoleMember}, 1, ActionTake); err != nil {
		t.Fatalf("take with detail reload error returned error: %v", err)
	}
}

func TestApplyActionUntakeNotificationErrorsDoNotFailUntake(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	takerID := 2
	repo := &fakeRepository{
		subRequest: &domain.SubRequest{ID: 1, PostedByUserID: 1, TakenByUserID: &takerID},
		detail: DetailReadModel{
			Request: &domain.SubRequest{
				ID:             1,
				ShowID:         2,
				PostedByUserID: 1,
				TakenByUserID:  &takerID,
				StartTime:      now.Add(time.Hour),
				EndTime:        now.Add(2 * time.Hour),
			},
			RequesterEmail: "requester@example.com",
			TakerEmail:     "taker@example.com",
		},
	}
	svc := NewService(repo, &fakeCatalog{err: errors.New("catalog down")})
	notifier := &fakeNotifier{err: errors.New("queue failed")}
	svc.SetNotifier(notifier)
	svc.nowFunc = func() time.Time { return now }

	if err := svc.ApplyAction(context.Background(), domain.CurrentUser{ID: takerID, Email: "untaker@example.com", Role: domain.RoleMember}, 1, ActionUntake); err != nil {
		t.Fatalf("untake with notification errors returned error: %v", err)
	}
	if notifier.untakenCalls != 1 {
		t.Fatalf("expected untaken notifier to be called once, got %d", notifier.untakenCalls)
	}
	if notifier.untakenEvent.ShowTitle != "Unknown Show" {
		t.Fatalf("show title = %q, want Unknown Show", notifier.untakenEvent.ShowTitle)
	}

	repo.detailErr = errors.New("detail reload failed")
	if err := svc.ApplyAction(context.Background(), domain.CurrentUser{ID: takerID, Email: "untaker@example.com", Role: domain.RoleMember}, 1, ActionUntake); err != nil {
		t.Fatalf("untake with detail reload error returned error: %v", err)
	}
}

func TestApplyActionDoesNotNotifyWhenTakeFailsOrUntakeFails(t *testing.T) {
	takerID := 2
	repo := &fakeRepository{subRequest: &domain.SubRequest{ID: 1, PostedByUserID: 1}}
	svc := NewService(repo, nil)
	notifier := &fakeNotifier{}
	svc.SetNotifier(notifier)
	viewer := domain.CurrentUser{ID: takerID, Role: domain.RoleMember}

	repo.takeErr = apperrors.ErrConflict
	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionTake); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("take conflict error = %v, want conflict", err)
	}
	if notifier.takenCalls != 0 {
		t.Fatalf("expected no notification for failed take, got %d", notifier.takenCalls)
	}

	repo.takeErr = nil
	repo.untakeErr = apperrors.ErrConflict
	repo.subRequest = &domain.SubRequest{ID: 1, PostedByUserID: 1, TakenByUserID: &takerID}
	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionUntake); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("untake conflict error = %v, want conflict", err)
	}
	if notifier.untakenCalls != 0 {
		t.Fatalf("expected no notification for failed untake, got %d", notifier.untakenCalls)
	}
}

func TestNowFallsBackToTimeNow(t *testing.T) {
	svc := &Service{}
	if svc.now().IsZero() {
		t.Error("expected fallback time to be non-zero")
	}
}

func TestActionValid(t *testing.T) {
	if !ActionTake.Valid() || !ActionUntake.Valid() {
		t.Error("expected known actions to be valid")
	}
	if Action("invalid").Valid() {
		t.Error("expected unknown action to be invalid")
	}
}
