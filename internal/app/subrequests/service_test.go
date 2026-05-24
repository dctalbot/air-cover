package subrequests

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appcatalog "air-cover/internal/app/catalog"
	"air-cover/internal/app/session"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

type fakeRepository struct {
	subRequests []*domain.SubRequest
	subRequest  *domain.SubRequest
	listErr     error
	createErr   error
	getErr      error
	deleteErr   error
	takeErr     error
	untakeErr   error
	created     *domain.SubRequest
	deletedID   int
	takenID     int
	takenUserID int
	untakenID   int
}

func (f *fakeRepository) ListSubRequests(ctx context.Context) ([]*domain.SubRequest, error) {
	return f.subRequests, f.listErr
}

func (f *fakeRepository) CreateSubRequest(ctx context.Context, sr *domain.SubRequest) error {
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

func (f *fakeRepository) TakeSubRequest(ctx context.Context, id int, userID int) error {
	f.takenID = id
	f.takenUserID = userID
	return f.takeErr
}

func (f *fakeRepository) UntakeSubRequest(ctx context.Context, id int) error {
	f.untakenID = id
	return f.untakeErr
}

type fakeCatalog struct {
	shows []appcatalog.Show
	err   error
}

func (f *fakeCatalog) ListShows(ctx context.Context) ([]appcatalog.Show, error) {
	return f.shows, f.err
}

func TestListDashboard(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	takerID := 2
	repo := &fakeRepository{subRequests: []*domain.SubRequest{
		{ID: 1, ShowID: 2, PostedByUserID: 1, StartTime: now.Add(2 * time.Hour), EndTime: now.Add(3 * time.Hour)},
		{ID: 2, ShowID: 999, PostedByUserID: 1, TakenByUserID: &takerID, StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-1 * time.Hour)},
		{ID: 3, ShowID: 1, PostedByUserID: 3, StartTime: now.Add(-4 * time.Hour), EndTime: now.Add(-3 * time.Hour)},
	}}
	svc := NewService(repo, &fakeCatalog{shows: []appcatalog.Show{
		{ID: "2", Title: "Beta"},
		{ID: "bad", Title: "Bad ID"},
		{ID: "1", Title: "Alpha"},
	}})
	svc.nowFunc = func() time.Time { return now }

	dashboard, err := svc.ListDashboard(context.Background(), session.CurrentUser{ID: takerID, Role: "member"})
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
		ListDashboard(context.Background(), session.CurrentUser{}); !errors.Is(err, ErrCatalog) {
		t.Errorf("nil catalog error = %v, want ErrCatalog", err)
	}
	if _, err := NewService(&fakeRepository{}, &fakeCatalog{err: errors.New("down")}).
		ListDashboard(context.Background(), session.CurrentUser{}); !errors.Is(err, ErrCatalog) {
		t.Errorf("catalog error = %v, want ErrCatalog", err)
	}

	wantErr := errors.New("list failed")
	if _, err := NewService(&fakeRepository{listErr: wantErr}, &fakeCatalog{}).
		ListDashboard(context.Background(), session.CurrentUser{}); !errors.Is(err, wantErr) {
		t.Errorf("list error = %v, want %v", err, wantErr)
	}
}

func TestCreate(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{}
	svc := NewService(repo, &fakeCatalog{shows: []appcatalog.Show{{ID: "1", Title: "Test Show"}}})
	svc.nowFunc = func() time.Time { return now }
	input := CreateInput{
		ShowID:    1,
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(2 * time.Hour),
		Notes:     "please help",
	}

	if err := svc.Create(context.Background(), session.CurrentUser{ID: 7}, input); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if repo.created.PostedByUserID != 7 || repo.created.CreatedAt != now {
		t.Errorf("unexpected created request: %+v", repo.created)
	}

	catalogErrSvc := NewService(&fakeRepository{}, &fakeCatalog{err: errors.New("catalog down")})
	catalogErrSvc.nowFunc = func() time.Time { return now }
	if err := catalogErrSvc.Create(context.Background(), session.CurrentUser{ID: 7}, input); !errors.Is(err, ErrCatalog) {
		t.Errorf("catalog error = %v, want ErrCatalog", err)
	}

	wantErr := errors.New("create failed")
	createErrSvc := NewService(&fakeRepository{createErr: wantErr}, nil)
	createErrSvc.nowFunc = func() time.Time { return now }
	if err := createErrSvc.Create(context.Background(), session.CurrentUser{ID: 7}, input); !errors.Is(err, wantErr) {
		t.Errorf("create error = %v, want %v", err, wantErr)
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

			if err := svc.Create(context.Background(), session.CurrentUser{ID: 7}, tt.input); !errors.Is(err, apperrors.ErrInvalid) {
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
	viewer := session.CurrentUser{ID: 1, Role: "member"}
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
	viewer := session.CurrentUser{ID: takerID, Role: "member"}

	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionTake); err != nil {
		t.Fatalf("take returned error: %v", err)
	}
	if repo.takenID != 1 || repo.takenUserID != takerID {
		t.Errorf("unexpected take call: %+v", repo)
	}

	repo.subRequest = &domain.SubRequest{ID: 1, PostedByUserID: 1, TakenByUserID: &takerID}
	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionUntake); err != nil {
		t.Fatalf("untake returned error: %v", err)
	}
	if repo.untakenID != 1 {
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
