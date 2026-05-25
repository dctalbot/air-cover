package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"air-cover/internal/adapters/outbound/sqlite/dbgen"
	"air-cover/internal/domain"
)

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	user, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return userFromSQL(user), nil
}

func (r *Repository) GetUserByID(ctx context.Context, id int) (*domain.User, error) {
	user, err := r.queries.GetUserByID(ctx, int64(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return userFromSQL(user), nil
}

func (r *Repository) ListActiveUsers(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.queries.ListActiveUsers(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]*domain.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, userFromSQL(row))
	}
	return users, nil
}

func (r *Repository) CreateMagicLink(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error {
	return r.queries.CreateMagicLink(ctx, dbgen.CreateMagicLinkParams{
		UserID:    int64(userID),
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	})
}

func (r *Repository) UseMagicLink(ctx context.Context, tokenHash string, now time.Time) (*domain.MagicLink, error) {
	ml, err := r.queries.UseMagicLink(ctx, dbgen.UseMagicLinkParams{
		TokenHash: tokenHash,
		ExpiresAt: now,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return magicLinkFromSQL(ml), nil
}

func (r *Repository) CreateSession(ctx context.Context, sessionID, sessionTokenHash string, userID int, expiresAt time.Time) error {
	return r.queries.CreateSession(ctx, dbgen.CreateSessionParams{
		ID:        sessionID,
		UserID:    int64(userID),
		TokenHash: sessionTokenHash,
		ExpiresAt: expiresAt,
	})
}

func (r *Repository) GetSessionByToken(ctx context.Context, sessionTokenHash string, now time.Time) (*domain.Session, error) {
	row, err := r.queries.GetSessionByToken(ctx, sessionTokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	s := sessionFromSQL(row)
	if s.ExpiresAt.Before(now) {
		return nil, errors.New("session expired")
	}
	return s, nil
}

func (r *Repository) DeleteSessionsByUserID(ctx context.Context, userID int) error {
	return r.queries.DeleteSessionsByUserID(ctx, int64(userID))
}
