package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"air-cover/internal/domain"
)

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	err := r.db.QueryRowContext(ctx, "SELECT id, email, role, is_enabled, created_at FROM users WHERE email = ?", email).
		Scan(&user.ID, &user.Email, &user.Role, &user.IsEnabled, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id int) (*domain.User, error) {
	var user domain.User
	err := r.db.QueryRowContext(ctx, "SELECT id, email, role, is_enabled, created_at FROM users WHERE id = ?", id).
		Scan(&user.ID, &user.Email, &user.Role, &user.IsEnabled, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *Repository) CreateMagicLink(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO magic_links (user_id, token_hash, expires_at) VALUES (?, ?, ?)",
		userID, tokenHash, expiresAt)
	return err
}

func (r *Repository) UseMagicLink(ctx context.Context, tokenHash string, now time.Time) (*domain.MagicLink, error) {
	var ml domain.MagicLink
	err := r.db.QueryRowContext(ctx, `
		DELETE FROM magic_links
		WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?
		RETURNING id, user_id, token_hash, expires_at
	`, tokenHash, now).
		Scan(&ml.ID, &ml.UserID, &ml.TokenHash, &ml.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &ml, nil
}

func (r *Repository) CreateSession(ctx context.Context, sessionID, sessionToken string, userID int, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO sessions (id, user_id, session_token, expires_at) VALUES (?, ?, ?, ?)",
		sessionID, userID, sessionToken, expiresAt)
	return err
}

func (r *Repository) GetSessionByToken(ctx context.Context, sessionToken string, now time.Time) (*domain.Session, error) {
	var s domain.Session
	err := r.db.QueryRowContext(ctx, "SELECT id, user_id, session_token, expires_at FROM sessions WHERE session_token = ?", sessionToken).
		Scan(&s.ID, &s.UserID, &s.SessionToken, &s.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if s.ExpiresAt.Before(now) {
		return nil, errors.New("session expired")
	}
	return &s, nil
}

func (r *Repository) DeleteSessionsByUserID(ctx context.Context, userID int) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}
