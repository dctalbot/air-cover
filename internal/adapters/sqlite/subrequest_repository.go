package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/domain"
)

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

func (r *Repository) ListSubRequests(ctx context.Context) ([]subrequestsapp.SubRequestRecord, error) {
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

	var subRequests []subrequestsapp.SubRequestRecord
	for rows.Next() {
		var sr domain.SubRequest
		var requesterEmail string
		var takerEmail string
		err := rows.Scan(
			&sr.ID, &sr.ShowID, &sr.PostedByUserID, &sr.TakenByUserID, &requesterEmail,
			&takerEmail, &sr.StartTime, &sr.EndTime, &sr.Notes, &sr.CreatedAt, &sr.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		subRequests = append(subRequests, subrequestsapp.SubRequestRecord{
			Request:        &sr,
			RequesterEmail: requesterEmail,
			TakerEmail:     takerEmail,
		})
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

func (r *Repository) TakeSubRequest(ctx context.Context, id int, userID int, updatedAt time.Time) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE sub_requests SET taken_by_user_id = ?, updated_at = ? WHERE id = ? AND taken_by_user_id IS NULL",
		userID, updatedAt, id)
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

func (r *Repository) UntakeSubRequest(ctx context.Context, id int, updatedAt time.Time) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE sub_requests SET taken_by_user_id = NULL, updated_at = ? WHERE id = ?",
		updatedAt, id)
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
