package admin

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"

	"air-cover/internal/app/session"
	"air-cover/internal/apperrors"
	"air-cover/internal/db"
	"air-cover/internal/models"
	"air-cover/internal/policy"
	"air-cover/internal/spinitron"
)

type Repository interface {
	ListUsers(ctx context.Context) ([]*models.User, error)
	CreateUser(ctx context.Context, email string, role string) (*models.User, error)
	UpdateUser(ctx context.Context, id int, role *string, isEnabled *bool) error
	ImportUsers(ctx context.Context, emails []string) error
}

type Catalog interface {
	ListPersonas(ctx context.Context) ([]spinitron.Persona, error)
}

type Service struct {
	repo    Repository
	catalog Catalog
}

func NewService(repo Repository, catalog Catalog) *Service {
	return &Service{repo: repo, catalog: catalog}
}

type CreateUserInput struct {
	Email string
	Role  string
}

type UpdateUserInput struct {
	ID        int
	Role      *string
	IsEnabled *bool
}

func (s *Service) ListUsers(ctx context.Context) ([]*models.User, error) {
	users, err := s.repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	sortUsers(users)
	return users, nil
}

func (s *Service) CreateUser(ctx context.Context, input CreateUserInput) error {
	role := input.Role
	if role == "" {
		role = "member"
	}
	if input.Email == "" || !validRole(role) {
		return apperrors.ErrInvalid
	}
	_, err := s.repo.CreateUser(ctx, input.Email, role)
	return err
}

func (s *Service) UpdateUser(ctx context.Context, viewer session.CurrentUser, input UpdateUserInput) error {
	if input.Role != nil && !validRole(*input.Role) {
		return apperrors.ErrInvalid
	}
	if input.IsEnabled != nil && !*input.IsEnabled && !policy.CanDeactivateUser(viewer, input.ID) {
		return apperrors.ErrForbidden
	}
	if err := s.repo.UpdateUser(ctx, input.ID, input.Role, input.IsEnabled); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return apperrors.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Service) ImportSpinitronUsers(ctx context.Context) error {
	if s.catalog == nil {
		return nil
	}
	personas, err := s.catalog.ListPersonas(ctx)
	if err != nil {
		slog.Error("Failed to load personas from spinitron", "error", err)
		return nil
	}

	var emails []string
	for _, p := range personas {
		if strings.TrimSpace(p.Email) != "" {
			emails = append(emails, p.Email)
		}
	}
	if len(emails) == 0 {
		return nil
	}
	return s.repo.ImportUsers(ctx, emails)
}

func sortUsers(users []*models.User) {
	sort.Slice(users, func(i, j int) bool {
		if users[i].IsEnabled != users[j].IsEnabled {
			return users[i].IsEnabled
		}
		if !users[i].IsEnabled {
			return strings.ToLower(users[i].Email) < strings.ToLower(users[j].Email)
		}
		if users[i].Role != users[j].Role {
			return users[i].Role == "admin"
		}
		return strings.ToLower(users[i].Email) < strings.ToLower(users[j].Email)
	})
}

func validRole(role string) bool {
	return role == "admin" || role == "member"
}
