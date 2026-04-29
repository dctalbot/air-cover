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

func (r *Repository) DB() *sql.DB {
	return r.db
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.db.QueryRowContext(ctx, "SELECT id, email, role, created_at FROM users WHERE email = ?", email).
		Scan(&user.ID, &user.Email, &user.Role, &user.CreatedAt)
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
	err := r.db.QueryRowContext(ctx, "SELECT id, email, role, created_at FROM users WHERE id = ?", id).
		Scan(&user.ID, &user.Email, &user.Role, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *Repository) CreateUser(ctx context.Context, email string, role string) (*models.User, error) {
	res, err := r.db.ExecContext(ctx, "INSERT INTO users (email, role) VALUES (?, ?)", email, role)
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

func (r *Repository) DeleteSessionsByUserID(ctx context.Context, userID int) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

func (r *Repository) CreateSubRequest(ctx context.Context, sr *models.SubRequest) error {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO sub_requests (show_id, user_id, start_time, end_time, notes, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, sr.ShowID, sr.UserID, sr.StartTime, sr.EndTime, sr.Notes, sr.Status, sr.CreatedAt, sr.UpdatedAt)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	sr.ID = int(id)
	return nil
}
func (r *Repository) ListSubRequests(ctx context.Context) ([]*models.SubRequest, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT sr.id, sr.show_id, sr.user_id, u.email, sr.start_time, sr.end_time, sr.notes, sr.status, sr.created_at, sr.updated_at
		FROM sub_requests sr
		JOIN users u ON sr.user_id = u.id
		ORDER BY sr.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subRequests []*models.SubRequest
	for rows.Next() {
		var sr models.SubRequest
		err := rows.Scan(
			&sr.ID, &sr.ShowID, &sr.UserID, &sr.RequesterEmail,
			&sr.StartTime, &sr.EndTime, &sr.Notes, &sr.Status, &sr.CreatedAt, &sr.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		subRequests = append(subRequests, &sr)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return subRequests, nil
}

func (r *Repository) GetSubRequestByID(ctx context.Context, id int) (*models.SubRequest, error) {
	var sr models.SubRequest
	err := r.db.QueryRowContext(ctx, `
		SELECT id, show_id, user_id, start_time, end_time, notes, status, created_at, updated_at
		FROM sub_requests
		WHERE id = ?
	`, id).Scan(
		&sr.ID, &sr.ShowID, &sr.UserID, &sr.StartTime, &sr.EndTime, &sr.Notes, &sr.Status, &sr.CreatedAt, &sr.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sr, nil
}

func (r *Repository) DeleteSubRequest(ctx context.Context, id int) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM sub_requests WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ListUsers(ctx context.Context) ([]*models.User, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, email, role, created_at FROM users ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Email, &user.Role, &user.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, &user)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func (r *Repository) DeleteUser(ctx context.Context, id int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Delete associated records
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM magic_links WHERE user_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sub_requests WHERE user_id = ?", id); err != nil {
		return err
	}

	// Delete the user
	res, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}

	return tx.Commit()
}
