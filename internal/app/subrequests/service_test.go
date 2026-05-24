package subrequests

import (
	"context"
	"errors"
	"testing"
	"time"

	"air-cover/internal/app/session"
	"air-cover/internal/apperrors"
	"air-cover/internal/db"
	"air-cover/internal/models"
	"air-cover/internal/spinitron"
)

type fakeRepository struct {
	subRequests []*models.SubRequest
	subRequest  *models.SubRequest
	listErr     error
	createErr   error
	getErr      error
	deleteErr   error
	takeErr     error
	untakeErr   error
	created     *models.SubRequest
	deletedID   int
	takenID     int
	takenUserID int
	untakenID   int
}

func (f *fakeRepository) ListSubRequests(ctx context.Context) ([]*models.SubRequest, error) {
	return f.subRequests, f.listErr
}

func (f *fakeRepository) CreateSubRequest(ctx context.Context, sr *models.SubRequest) error {
	f.created = sr
	return f.createErr
}

func (f *fakeRepository) GetSubRequestByID(ctx context.Context, id int) (*models.SubRequest, error) {
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
	shows []spinitron.Show
	err   error
}

func (f *fakeCatalog) ListShows(ctx context.Context) ([]spinitron.Show, error) {
	return f.shows, f.err
}

func TestListDashboard(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	takerID := 2
	repo := &fakeRepository{subRequests: []*models.SubRequest{
		{ID: 1, ShowID: 2, PostedByUserID: 1, StartTime: now.Add(2 * time.Hour), EndTime: now.Add(3 * time.Hour)},
		{ID: 2, ShowID: 999, PostedByUserID: 1, TakenByUserID: &takerID, StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-1 * time.Hour)},
		{ID: 3, ShowID: 1, PostedByUserID: 3, StartTime: now.Add(-4 * time.Hour), EndTime: now.Add(-3 * time.Hour)},
	}}
	svc := NewService(repo, &fakeCatalog{shows: []spinitron.Show{
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
	svc := NewService(repo, nil)
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

	wantErr := errors.New("create failed")
	if err := NewService(&fakeRepository{createErr: wantErr}, nil).
		Create(context.Background(), session.CurrentUser{ID: 7}, input); !errors.Is(err, wantErr) {
		t.Errorf("create error = %v, want %v", err, wantErr)
	}
}

func TestDelete(t *testing.T) {
	repo := &fakeRepository{subRequest: &models.SubRequest{ID: 1, PostedByUserID: 1}}
	svc := NewService(repo, nil)
	viewer := session.CurrentUser{ID: 1, Role: "member"}
	if err := svc.Delete(context.Background(), viewer, 1); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if repo.deletedID != 1 {
		t.Errorf("deleted ID = %d, want 1", repo.deletedID)
	}

	repo.subRequest = &models.SubRequest{ID: 2, PostedByUserID: 2}
	if err := svc.Delete(context.Background(), viewer, 2); !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("forbidden delete error = %v, want forbidden", err)
	}

	repo.getErr = db.ErrNotFound
	if err := svc.Delete(context.Background(), viewer, 3); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("not found delete error = %v, want app not found", err)
	}
	repo.getErr = nil
	repo.subRequest = &models.SubRequest{ID: 1, PostedByUserID: 1}
	repo.deleteErr = db.ErrConflict
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
	repo := &fakeRepository{subRequest: &models.SubRequest{ID: 1, PostedByUserID: 1}}
	svc := NewService(repo, nil)
	viewer := session.CurrentUser{ID: takerID, Role: "member"}

	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionTake); err != nil {
		t.Fatalf("take returned error: %v", err)
	}
	if repo.takenID != 1 || repo.takenUserID != takerID {
		t.Errorf("unexpected take call: %+v", repo)
	}

	repo.subRequest = &models.SubRequest{ID: 1, PostedByUserID: 1, TakenByUserID: &takerID}
	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionUntake); err != nil {
		t.Fatalf("untake returned error: %v", err)
	}
	if repo.untakenID != 1 {
		t.Errorf("untaken ID = %d, want 1", repo.untakenID)
	}

	if err := svc.ApplyAction(context.Background(), viewer, 1, Action("bad")); !errors.Is(err, apperrors.ErrInvalid) {
		t.Errorf("invalid action error = %v, want invalid", err)
	}

	repo.subRequest = &models.SubRequest{ID: 1, PostedByUserID: 1}
	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionUntake); !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("untake forbidden error = %v, want forbidden", err)
	}

	repo.takeErr = db.ErrConflict
	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionTake); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("take conflict error = %v, want conflict", err)
	}
	repo.takeErr = nil
	repo.getErr = db.ErrNotFound
	if err := svc.ApplyAction(context.Background(), viewer, 1, ActionTake); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("take lookup error = %v, want not found", err)
	}
	repo.getErr = nil
	repo.untakeErr = db.ErrNotFound
	repo.subRequest = &models.SubRequest{ID: 1, PostedByUserID: 1, TakenByUserID: &takerID}
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
