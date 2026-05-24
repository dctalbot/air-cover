package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

type Repository interface {
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	CreateUser(ctx context.Context, email string, role string) (*domain.User, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) EnsureMasterUser(ctx context.Context, email string) error {
	if email == "" {
		return nil
	}
	_, err := s.repo.GetUserByEmail(ctx, email)
	if errors.Is(err, apperrors.ErrNotFound) {
		slog.Info("Creating master admin user", "email", email)
		if _, err := s.repo.CreateUser(ctx, email, string(domain.RoleAdmin)); err != nil {
			return fmt.Errorf("failed to create master user: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to check master user: %w", err)
	}
	return nil
}
