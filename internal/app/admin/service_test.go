package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"air-cover/internal/app/session"
	"air-cover/internal/apperrors"
	"air-cover/internal/models"
	"air-cover/internal/spinitron"
)

type fakeRepository struct {
	users         []*models.User
	createEmail   string
	createRole    string
	createErr     error
	updateID      int
	updateRole    *string
	updateEnabled *bool
	updateErr     error
	importEmails  []string
	importErr     error
	listErr       error
}

func (f *fakeRepository) ListUsers(ctx context.Context) ([]*models.User, error) {
	return f.users, f.listErr
}

func (f *fakeRepository) CreateUser(ctx context.Context, email string, role string) (*models.User, error) {
	f.createEmail = email
	f.createRole = role
	return &models.User{Email: email, Role: role}, f.createErr
}

func (f *fakeRepository) UpdateUser(ctx context.Context, id int, role *string, isEnabled *bool) error {
	f.updateID = id
	f.updateRole = role
	f.updateEnabled = isEnabled
	return f.updateErr
}

func (f *fakeRepository) ImportUsers(ctx context.Context, emails []string) error {
	f.importEmails = emails
	return f.importErr
}

type fakeCatalog struct {
	personas []spinitron.Persona
	err      error
}

func (f *fakeCatalog) ListPersonas(ctx context.Context) ([]spinitron.Persona, error) {
	return f.personas, f.err
}

func TestListUsersSortsForAdminView(t *testing.T) {
	created := time.Now()
	users := []*models.User{
		{ID: 1, Email: "z@example.com", Role: "member", IsEnabled: false, CreatedAt: created},
		{ID: 2, Email: "b@example.com", Role: "member", IsEnabled: true, CreatedAt: created},
		{ID: 3, Email: "a@example.com", Role: "admin", IsEnabled: true, CreatedAt: created},
		{ID: 4, Email: "a-disabled@example.com", Role: "admin", IsEnabled: false, CreatedAt: created},
		{ID: 5, Email: "c@example.com", Role: "member", IsEnabled: true, CreatedAt: created},
	}
	svc := NewService(&fakeRepository{users: users}, nil)

	got, err := svc.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers returned error: %v", err)
	}
	wantEmails := []string{"a@example.com", "b@example.com", "c@example.com", "a-disabled@example.com", "z@example.com"}
	for i, want := range wantEmails {
		if got[i].Email != want {
			t.Fatalf("user %d = %q, want %q", i, got[i].Email, want)
		}
	}
}

func TestListUsersReturnsRepositoryError(t *testing.T) {
	wantErr := errors.New("list failed")
	svc := NewService(&fakeRepository{listErr: wantErr}, nil)
	if _, err := svc.ListUsers(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("ListUsers error = %v, want %v", err, wantErr)
	}
}

func TestCreateUser(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo, nil)
	if err := svc.CreateUser(context.Background(), CreateUserInput{Email: "user@example.com"}); err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	if repo.createRole != "member" {
		t.Errorf("default role = %q, want member", repo.createRole)
	}

	if err := svc.CreateUser(context.Background(), CreateUserInput{Email: "", Role: "member"}); !errors.Is(err, apperrors.ErrInvalid) {
		t.Errorf("empty email error = %v, want invalid", err)
	}
	if err := svc.CreateUser(context.Background(), CreateUserInput{Email: "user@example.com", Role: "owner"}); !errors.Is(err, apperrors.ErrInvalid) {
		t.Errorf("invalid role error = %v, want invalid", err)
	}
}

func TestUpdateUser(t *testing.T) {
	disabled := false
	role := "admin"
	viewer := session.CurrentUser{ID: 1}
	repo := &fakeRepository{}
	svc := NewService(repo, nil)

	if err := svc.UpdateUser(context.Background(), viewer, UpdateUserInput{ID: 2, Role: &role, IsEnabled: &disabled}); err != nil {
		t.Fatalf("UpdateUser returned error: %v", err)
	}
	if repo.updateID != 2 || repo.updateRole == nil || *repo.updateRole != "admin" {
		t.Errorf("unexpected update call: %+v", repo)
	}
	if err := svc.UpdateUser(context.Background(), viewer, UpdateUserInput{ID: 2, Role: stringPtr("owner")}); !errors.Is(err, apperrors.ErrInvalid) {
		t.Errorf("invalid role error = %v, want invalid", err)
	}
	if err := svc.UpdateUser(context.Background(), viewer, UpdateUserInput{ID: 1, IsEnabled: &disabled}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("self deactivation error = %v, want forbidden", err)
	}

	repo.updateErr = apperrors.ErrNotFound
	if err := svc.UpdateUser(context.Background(), viewer, UpdateUserInput{ID: 99}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("not found error = %v, want app not found", err)
	}

	updateErr := errors.New("update failed")
	repo.updateErr = updateErr
	if err := svc.UpdateUser(context.Background(), viewer, UpdateUserInput{ID: 99}); !errors.Is(err, updateErr) {
		t.Errorf("update error = %v, want %v", err, updateErr)
	}
}

func TestImportSpinitronUsers(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo, &fakeCatalog{personas: []spinitron.Persona{
		{Email: "one@example.com"},
		{Email: " "},
		{Email: "two@example.com"},
	}})
	if err := svc.ImportSpinitronUsers(context.Background()); err != nil {
		t.Fatalf("ImportSpinitronUsers returned error: %v", err)
	}
	if len(repo.importEmails) != 2 {
		t.Fatalf("imported %d emails, want 2", len(repo.importEmails))
	}

	if err := NewService(repo, &fakeCatalog{err: errors.New("spinitron down")}).ImportSpinitronUsers(context.Background()); err != nil {
		t.Errorf("spinitron errors should be swallowed, got %v", err)
	}
	if err := NewService(repo, nil).ImportSpinitronUsers(context.Background()); err != nil {
		t.Errorf("nil catalog should be ignored, got %v", err)
	}
	if err := NewService(repo, &fakeCatalog{personas: []spinitron.Persona{{Email: " "}}}).
		ImportSpinitronUsers(context.Background()); err != nil {
		t.Errorf("empty import should be ignored, got %v", err)
	}

	importErr := errors.New("import failed")
	repo.importErr = importErr
	if err := svc.ImportSpinitronUsers(context.Background()); !errors.Is(err, importErr) {
		t.Errorf("import error = %v, want %v", err, importErr)
	}
}

func stringPtr(v string) *string {
	return &v
}
