package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"air-cover/internal/app/authorization"
	appcatalog "air-cover/internal/app/catalog"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

type fakeRepository struct {
	users         []*domain.User
	userByEmail   *domain.User
	getByEmailErr error
	createEmail   string
	createRole    string
	createCalled  bool
	createErr     error
	updateID      int
	updateRole    *string
	updateEnabled *bool
	updateErr     error
	importEmails  []string
	importErr     error
	listErr       error
}

func (f *fakeRepository) ListUsers(ctx context.Context) ([]*domain.User, error) {
	return f.users, f.listErr
}

func (f *fakeRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	if f.getByEmailErr != nil {
		return nil, f.getByEmailErr
	}
	if f.userByEmail != nil {
		return f.userByEmail, nil
	}
	return nil, apperrors.ErrNotFound
}

func (f *fakeRepository) CreateUser(ctx context.Context, email string, role string) (*domain.User, error) {
	f.createCalled = true
	f.createEmail = email
	f.createRole = role
	return &domain.User{Email: email, Role: domain.Role(role)}, f.createErr
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
	personas []appcatalog.Persona
	err      error
}

func (f *fakeCatalog) ListPersonas(ctx context.Context) ([]appcatalog.Persona, error) {
	return f.personas, f.err
}

type allowAllAuthorizer struct{}

func (allowAllAuthorizer) Authorize(ctx context.Context, subject authorization.Subject, action authorization.Action, resource authorization.Resource) error {
	return nil
}

func TestListUsersSortsForAdminView(t *testing.T) {
	created := time.Now()
	users := []*domain.User{
		{ID: 1, Email: "z@example.com", Role: domain.RoleMember, IsEnabled: false, CreatedAt: created},
		{ID: 2, Email: "b@example.com", Role: domain.RoleMember, IsEnabled: true, CreatedAt: created},
		{ID: 3, Email: "a@example.com", Role: domain.RoleAdmin, IsEnabled: true, CreatedAt: created},
		{ID: 4, Email: "a-disabled@example.com", Role: domain.RoleAdmin, IsEnabled: false, CreatedAt: created},
		{ID: 5, Email: "c@example.com", Role: domain.RoleMember, IsEnabled: true, CreatedAt: created},
	}
	svc := NewService(&fakeRepository{users: users}, nil)

	got, err := svc.ListUsers(context.Background(), domain.CurrentUser{ID: 99, Role: domain.RoleAdmin})
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
	if _, err := svc.ListUsers(context.Background(), domain.CurrentUser{ID: 99, Role: domain.RoleAdmin}); !errors.Is(err, wantErr) {
		t.Fatalf("ListUsers error = %v, want %v", err, wantErr)
	}
}

func TestCreateUser(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo, nil)
	viewer := domain.CurrentUser{ID: 99, Role: domain.RoleAdmin}
	if err := svc.CreateUser(context.Background(), viewer, CreateUserInput{Email: "user@example.com"}); err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	if !repo.createCalled {
		t.Fatal("expected repository CreateUser to be called")
	}
	if repo.createRole != "member" {
		t.Errorf("default role = %q, want member", repo.createRole)
	}

	if err := svc.CreateUser(context.Background(), viewer, CreateUserInput{Email: "", Role: "member"}); !errors.Is(err, apperrors.ErrInvalid) {
		t.Errorf("empty email error = %v, want invalid", err)
	}
	if err := svc.CreateUser(context.Background(), viewer, CreateUserInput{Email: "user@example.com", Role: "owner"}); !errors.Is(err, apperrors.ErrInvalid) {
		t.Errorf("invalid role error = %v, want invalid", err)
	}
}

func TestCreateUserAlreadyExists(t *testing.T) {
	repo := &fakeRepository{
		userByEmail: &domain.User{ID: 1, Email: "user@example.com", Role: domain.RoleMember, IsEnabled: true},
	}
	svc := NewService(repo, nil)

	err := svc.CreateUser(context.Background(), domain.CurrentUser{ID: 99, Role: domain.RoleAdmin}, CreateUserInput{Email: "user@example.com"})

	if !errors.Is(err, ErrUserAlreadyExists) {
		t.Fatalf("CreateUser error = %v, want ErrUserAlreadyExists", err)
	}
	if repo.createCalled {
		t.Fatal("expected duplicate user to skip repository CreateUser")
	}
}

func TestCreateUserReturnsLookupError(t *testing.T) {
	wantErr := errors.New("lookup failed")
	repo := &fakeRepository{getByEmailErr: wantErr}
	svc := NewService(repo, nil)

	err := svc.CreateUser(context.Background(), domain.CurrentUser{ID: 99, Role: domain.RoleAdmin}, CreateUserInput{Email: "user@example.com"})

	if !errors.Is(err, wantErr) {
		t.Fatalf("CreateUser error = %v, want %v", err, wantErr)
	}
	if repo.createCalled {
		t.Fatal("expected lookup error to skip repository CreateUser")
	}
}

func TestAdminAuthorization(t *testing.T) {
	member := domain.CurrentUser{ID: 1, Role: domain.RoleMember}
	svc := NewService(&fakeRepository{}, nil)

	if _, err := svc.ListUsers(context.Background(), member); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("member ListUsers error = %v, want forbidden", err)
	}
	if err := svc.CreateUser(context.Background(), member, CreateUserInput{Email: "user@example.com"}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("member CreateUser error = %v, want forbidden", err)
	}
	if err := svc.ImportCatalogUsers(context.Background(), member); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("member ImportCatalogUsers error = %v, want forbidden", err)
	}
	if err := svc.UpdateUser(context.Background(), member, UpdateUserInput{ID: 2}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("member UpdateUser error = %v, want forbidden", err)
	}
}

func TestSetAuthorizer(t *testing.T) {
	svc := NewService(&fakeRepository{}, nil)
	member := domain.CurrentUser{ID: 1, Role: domain.RoleMember}

	svc.SetAuthorizer(nil)
	if _, err := svc.ListUsers(context.Background(), member); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("nil SetAuthorizer should keep parity authorizer, got %v", err)
	}

	svc.SetAuthorizer(allowAllAuthorizer{})
	if _, err := svc.ListUsers(context.Background(), member); err != nil {
		t.Fatalf("custom authorizer error = %v", err)
	}
}

func TestUpdateUser(t *testing.T) {
	disabled := false
	role := "admin"
	viewer := domain.CurrentUser{ID: 1, Role: domain.RoleAdmin}
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

func TestImportCatalogUsers(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo, &fakeCatalog{personas: []appcatalog.Persona{
		{Email: "one@example.com"},
		{Email: " "},
		{Email: "two@example.com"},
	}})
	if err := svc.ImportCatalogUsers(context.Background(), domain.CurrentUser{ID: 99, Role: domain.RoleAdmin}); err != nil {
		t.Fatalf("ImportCatalogUsers returned error: %v", err)
	}
	if len(repo.importEmails) != 2 {
		t.Fatalf("imported %d emails, want 2", len(repo.importEmails))
	}

	if err := NewService(repo, &fakeCatalog{err: errors.New("catalog down")}).
		ImportCatalogUsers(context.Background(), domain.CurrentUser{ID: 99, Role: domain.RoleAdmin}); err != nil {
		t.Errorf("catalog errors should be swallowed, got %v", err)
	}
	if err := NewService(repo, nil).ImportCatalogUsers(context.Background(), domain.CurrentUser{ID: 99, Role: domain.RoleAdmin}); err != nil {
		t.Errorf("nil catalog should be ignored, got %v", err)
	}
	if err := NewService(repo, &fakeCatalog{personas: []appcatalog.Persona{{Email: " "}}}).
		ImportCatalogUsers(context.Background(), domain.CurrentUser{ID: 99, Role: domain.RoleAdmin}); err != nil {
		t.Errorf("empty import should be ignored, got %v", err)
	}

	importErr := errors.New("import failed")
	repo.importErr = importErr
	if err := svc.ImportCatalogUsers(context.Background(), domain.CurrentUser{ID: 99, Role: domain.RoleAdmin}); !errors.Is(err, importErr) {
		t.Errorf("import error = %v, want %v", err, importErr)
	}
}

func stringPtr(v string) *string {
	return &v
}
