package admin

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"

	"air-cover/internal/app/authorization"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

type Service struct {
	repo       Repository
	catalog    Catalog
	authorizer authorization.Authorizer
}

var ErrUserAlreadyExists = errors.New("user already exists")

func NewService(repo Repository, catalog Catalog) *Service {
	return &Service{repo: repo, catalog: catalog, authorizer: authorization.NewParityAuthorizer()}
}

func (s *Service) SetAuthorizer(authorizer authorization.Authorizer) {
	if authorizer != nil {
		s.authorizer = authorizer
	}
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

type UserReadModel struct {
	User          *domain.User
	CanDeactivate bool
}

func (s *Service) ListUsers(ctx context.Context, viewer domain.CurrentUser) ([]UserReadModel, error) {
	if err := s.authorize(ctx, viewer, authorization.ActionUserList, authorization.AdminResource()); err != nil {
		return nil, err
	}
	users, err := s.repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	sortUsers(users)
	readModels := make([]UserReadModel, 0, len(users))
	for _, user := range users {
		canDeactivate, err := s.canDeactivate(ctx, viewer, user)
		if err != nil {
			return nil, err
		}
		readModels = append(readModels, UserReadModel{
			User:          user,
			CanDeactivate: canDeactivate,
		})
	}
	return readModels, nil
}

func (s *Service) CreateUser(ctx context.Context, viewer domain.CurrentUser, input CreateUserInput) error {
	if err := s.authorize(ctx, viewer, authorization.ActionUserCreate, authorization.AdminResource()); err != nil {
		return err
	}
	role := input.Role
	if role == "" {
		role = "member"
	}
	if input.Email == "" || !validRole(role) {
		return apperrors.ErrInvalid
	}
	if _, err := s.repo.GetUserByEmail(ctx, input.Email); err == nil {
		return ErrUserAlreadyExists
	} else if !errors.Is(err, apperrors.ErrNotFound) {
		return err
	}

	_, err := s.repo.CreateUser(ctx, input.Email, role)
	return err
}

func (s *Service) UpdateUser(ctx context.Context, viewer domain.CurrentUser, input UpdateUserInput) error {
	if input.Role != nil && !domain.Role(*input.Role).Valid() {
		return apperrors.ErrInvalid
	}
	disableTarget := input.IsEnabled != nil && !*input.IsEnabled
	if disableTarget && input.ID == viewer.ID {
		return apperrors.ErrForbidden
	}
	if err := s.authorize(ctx, viewer, authorization.ActionUserUpdate, authorization.UserResource(input.ID, disableTarget)); err != nil {
		return err
	}
	if err := s.repo.UpdateUser(ctx, input.ID, input.Role, input.IsEnabled); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Service) authorize(ctx context.Context, viewer domain.CurrentUser, action authorization.Action, resource authorization.Resource) error {
	return s.authorizer.Authorize(ctx, authorization.SubjectFromCurrentUser(viewer), action, resource)
}

func (s *Service) canDeactivate(ctx context.Context, viewer domain.CurrentUser, user *domain.User) (bool, error) {
	disableTarget := user.IsEnabled
	if disableTarget && user.ID == viewer.ID {
		return false, nil
	}
	err := s.authorize(ctx, viewer, authorization.ActionUserUpdate, authorization.UserResource(user.ID, disableTarget))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, apperrors.ErrForbidden) {
		return false, nil
	}
	return false, err
}

func (s *Service) ImportCatalogUsers(ctx context.Context, viewer domain.CurrentUser) error {
	if err := s.authorize(ctx, viewer, authorization.ActionUserCreate, authorization.AdminResource()); err != nil {
		return err
	}
	if s.catalog == nil {
		return nil
	}
	personas, err := s.catalog.ListPersonas(ctx)
	if err != nil {
		slog.Error("Failed to load personas from catalog", "error", err)
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

func sortUsers(users []*domain.User) {
	sort.Slice(users, func(i, j int) bool {
		if users[i].IsEnabled != users[j].IsEnabled {
			return users[i].IsEnabled
		}
		if !users[i].IsEnabled {
			return strings.ToLower(users[i].Email) < strings.ToLower(users[j].Email)
		}
		if users[i].Role != users[j].Role {
			return users[i].Role == domain.RoleAdmin
		}
		return strings.ToLower(users[i].Email) < strings.ToLower(users[j].Email)
	})
}

func validRole(role string) bool {
	return domain.Role(role).Valid()
}
