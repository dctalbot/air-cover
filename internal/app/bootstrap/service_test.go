package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

type fakeRepository struct {
	user          *domain.User
	getUserErr    error
	createUserErr error
	createdEmail  string
	createdRole   string
}

func (f *fakeRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	if f.getUserErr != nil {
		return nil, f.getUserErr
	}
	return f.user, nil
}

func (f *fakeRepository) CreateUser(ctx context.Context, email string, role string) (*domain.User, error) {
	f.createdEmail = email
	f.createdRole = role
	if f.createUserErr != nil {
		return nil, f.createUserErr
	}
	return &domain.User{ID: 1, Email: email, Role: domain.Role(role), IsEnabled: true}, nil
}

func TestEnsureMasterUser(t *testing.T) {
	tests := []struct {
		name       string
		email      string
		repo       *fakeRepository
		wantCreate bool
		wantErr    string
	}{
		{
			name:  "empty email",
			email: "",
			repo:  &fakeRepository{},
		},
		{
			name:  "existing user",
			email: "admin@example.com",
			repo:  &fakeRepository{user: &domain.User{ID: 1, Email: "admin@example.com", Role: domain.RoleAdmin}},
		},
		{
			name:       "missing user",
			email:      "admin@example.com",
			repo:       &fakeRepository{getUserErr: apperrors.ErrNotFound},
			wantCreate: true,
		},
		{
			name:    "check error",
			email:   "admin@example.com",
			repo:    &fakeRepository{getUserErr: errors.New("check failed")},
			wantErr: "failed to check master user",
		},
		{
			name:    "create error",
			email:   "admin@example.com",
			repo:    &fakeRepository{getUserErr: apperrors.ErrNotFound, createUserErr: errors.New("create failed")},
			wantErr: "failed to create master user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewService(tt.repo).EnsureMasterUser(context.Background(), tt.email)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.wantCreate {
				if tt.repo.createdEmail != tt.email || tt.repo.createdRole != string(domain.RoleAdmin) {
					t.Fatalf("created user = %q/%q, want %q/%q", tt.repo.createdEmail, tt.repo.createdRole, tt.email, domain.RoleAdmin)
				}
				return
			}
			if tt.repo.createdEmail != "" {
				t.Fatalf("unexpected user creation for %q", tt.repo.createdEmail)
			}
		})
	}
}
