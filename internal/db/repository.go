package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"air-cover/internal/models"
)

var ErrNotFound = errors.New("record not found")

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.db.QueryRowContext(ctx, "SELECT id, email, created_at FROM users WHERE email = ?", email).
		Scan(&user.ID, &user.Email, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id int) (*models.User, error) {
	var user models.User
	err := r.db.QueryRowContext(ctx, "SELECT id, email, created_at FROM users WHERE id = ?", id).
		Scan(&user.ID, &user.Email, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *Repository) CreateUser(ctx context.Context, email string) (*models.User, error) {
	res, err := r.db.ExecContext(ctx, "INSERT INTO users (email) VALUES (?)", email)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetUserByID(ctx, int(id))
}

func (r *Repository) CreateMagicLink(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO magic_links (user_id, token_hash, expires_at) VALUES (?, ?, ?)",
		userID, tokenHash, expiresAt)
	return err
}

func (r *Repository) UseMagicLink(ctx context.Context, tokenHash string) (*models.MagicLink, error) {
	var ml models.MagicLink
	var usedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, "SELECT id, user_id, token_hash, expires_at, used_at FROM magic_links WHERE token_hash = ?", tokenHash).
		Scan(&ml.ID, &ml.UserID, &ml.TokenHash, &ml.ExpiresAt, &usedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if usedAt.Valid {
		ml.UsedAt = &usedAt.Time
	}

	if ml.UsedAt != nil || ml.ExpiresAt.Before(time.Now()) {
		return &ml, errors.New("magic link expired or already used")
	}

	_, err = r.db.ExecContext(ctx, "UPDATE magic_links SET used_at = ? WHERE id = ?", time.Now(), ml.ID)
	if err != nil {
		return nil, err
	}

	return &ml, nil
}

func (r *Repository) CreateSession(ctx context.Context, sessionID, sessionToken string, userID int, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO sessions (id, user_id, session_token, expires_at) VALUES (?, ?, ?, ?)",
		sessionID, userID, sessionToken, expiresAt)
	return err
}

func (r *Repository) GetSessionByToken(ctx context.Context, sessionToken string) (*models.Session, error) {
	var s models.Session
	err := r.db.QueryRowContext(ctx, "SELECT id, user_id, session_token, expires_at FROM sessions WHERE session_token = ?", sessionToken).
		Scan(&s.ID, &s.UserID, &s.SessionToken, &s.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if s.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("session expired")
	}
	return &s, nil
}
