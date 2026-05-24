package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"air-cover/internal/adapters/outbound/sqlite/dbgen"
	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/domain"
)

func (r *Repository) CreateSubRequest(ctx context.Context, sr *domain.SubRequest) error {
	res, err := r.queries.CreateSubRequest(ctx, dbgen.CreateSubRequestParams{
		ShowID:         int64(sr.ShowID),
		PostedByUserID: int64(sr.PostedByUserID),
		TakenByUserID:  sqlNullIntFromPtr(sr.TakenByUserID),
		StartTime:      sr.StartTime,
		EndTime:        sr.EndTime,
		Notes:          sqlNullString(sr.Notes),
		CreatedAt:      sqlNullTime(sr.CreatedAt),
		UpdatedAt:      sqlNullTime(sr.UpdatedAt),
	})
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

func (r *Repository) ListDashboardSubRequests(ctx context.Context) ([]subrequestsapp.DashboardReadModel, error) {
	rows, err := r.queries.ListSubRequests(ctx)
	if err != nil {
		return nil, err
	}

	subRequests := make([]subrequestsapp.DashboardReadModel, 0, len(rows))
	for _, row := range rows {
		subRequests = append(subRequests, subRequestSummaryFromSQL(row))
	}
	return subRequests, nil
}

func (r *Repository) GetSubRequestByID(ctx context.Context, id int) (*domain.SubRequest, error) {
	sr, err := r.queries.GetSubRequestByID(ctx, int64(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return subRequestFromSQL(sr), nil
}

func (r *Repository) DeleteSubRequest(ctx context.Context, id int) error {
	res, err := r.queries.DeleteSubRequest(ctx, int64(id))
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
	res, err := r.queries.TakeSubRequest(ctx, dbgen.TakeSubRequestParams{
		TakenByUserID: sql.NullInt64{Int64: int64(userID), Valid: true},
		UpdatedAt:     sqlNullTime(updatedAt),
		ID:            int64(id),
	})
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
	res, err := r.queries.UntakeSubRequest(ctx, dbgen.UntakeSubRequestParams{
		UpdatedAt: sqlNullTime(updatedAt),
		ID:        int64(id),
	})
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
