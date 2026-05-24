package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

var (
	ErrNotFound = apperrors.ErrNotFound
	ErrConflict = apperrors.ErrConflict
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) DB() *sql.DB {
	return r.db
}

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

func (r *Repository) CreateUser(ctx context.Context, email string, role string) (*domain.User, error) {
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

func (r *Repository) UseMagicLink(ctx context.Context, tokenHash string) (*domain.MagicLink, error) {
	var ml domain.MagicLink
	err := r.db.QueryRowContext(ctx, `
		DELETE FROM magic_links
		WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?
		RETURNING id, user_id, token_hash, expires_at
	`, tokenHash, time.Now()).
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

func (r *Repository) GetSessionByToken(ctx context.Context, sessionToken string) (*domain.Session, error) {
	var s domain.Session
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

func (r *Repository) CreateSubRequest(ctx context.Context, sr *domain.SubRequest) error {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO sub_requests (show_id, posted_by_user_id, taken_by_user_id, start_time, end_time, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, sr.ShowID, sr.PostedByUserID, sr.TakenByUserID, sr.StartTime, sr.EndTime, sr.Notes, sr.CreatedAt, sr.UpdatedAt)
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
func (r *Repository) ListSubRequests(ctx context.Context) ([]*domain.SubRequest, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT sr.id, sr.show_id, sr.posted_by_user_id, sr.taken_by_user_id, u.email, COALESCE(u2.email, ''), sr.start_time, sr.end_time, sr.notes, sr.created_at, sr.updated_at
		FROM sub_requests sr
		JOIN users u ON sr.posted_by_user_id = u.id
		LEFT JOIN users u2 ON sr.taken_by_user_id = u2.id
		ORDER BY sr.start_time ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subRequests []*domain.SubRequest
	for rows.Next() {
		var sr domain.SubRequest
		err := rows.Scan(
			&sr.ID, &sr.ShowID, &sr.PostedByUserID, &sr.TakenByUserID, &sr.RequesterEmail,
			&sr.TakerEmail, &sr.StartTime, &sr.EndTime, &sr.Notes, &sr.CreatedAt, &sr.UpdatedAt,
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

func (r *Repository) GetSubRequestByID(ctx context.Context, id int) (*domain.SubRequest, error) {
	var sr domain.SubRequest
	err := r.db.QueryRowContext(ctx, `
		SELECT id, show_id, posted_by_user_id, taken_by_user_id, start_time, end_time, notes, created_at, updated_at
		FROM sub_requests
		WHERE id = ?
	`, id).Scan(
		&sr.ID, &sr.ShowID, &sr.PostedByUserID, &sr.TakenByUserID, &sr.StartTime, &sr.EndTime, &sr.Notes, &sr.CreatedAt, &sr.UpdatedAt,
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

func (r *Repository) TakeSubRequest(ctx context.Context, id int, userID int) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE sub_requests SET taken_by_user_id = ?, updated_at = ? WHERE id = ? AND taken_by_user_id IS NULL",
		userID, time.Now(), id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		if _, err := r.GetSubRequestByID(ctx, id); err != nil {
			return err
		}
		return ErrConflict
	}
	return nil
}

func (r *Repository) UntakeSubRequest(ctx context.Context, id int) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE sub_requests SET taken_by_user_id = NULL, updated_at = ? WHERE id = ?",
		time.Now(), id)
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

func (r *Repository) ListUsers(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, email, role, is_enabled, created_at FROM users ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.Email, &user.Role, &user.IsEnabled, &user.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, &user)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func (r *Repository) UpdateUser(ctx context.Context, id int, role *string, isEnabled *bool) error {
	query := "UPDATE users SET "
	var args []any
	if role != nil {
		query += "role = ?, "
		args = append(args, *role)
	}
	if isEnabled != nil {
		query += "is_enabled = ?, "
		args = append(args, *isEnabled)
	}

	if len(args) == 0 {
		return nil
	}

	// Remove trailing comma and space
	query = query[:len(query)-2]
	query += " WHERE id = ?"
	args = append(args, id)

	res, err := r.db.ExecContext(ctx, query, args...)
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

func (r *Repository) ImportUsers(ctx context.Context, emails []string) error {
	if len(emails) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.PrepareContext(ctx, "INSERT INTO users (email, role) VALUES (?, 'member') ON CONFLICT (email) DO NOTHING")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, email := range emails {
		_, err := stmt.ExecContext(ctx, email)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
