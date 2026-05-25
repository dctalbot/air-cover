package sqlite

import (
	"database/sql"
	"time"

	"air-cover/internal/adapters/outbound/sqlite/dbgen"
	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

var (
	ErrNotFound = apperrors.ErrNotFound
	ErrConflict = apperrors.ErrConflict
)

type Repository struct {
	db      *sql.DB
	queries *dbgen.Queries
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{
		db:      db,
		queries: dbgen.New(db),
	}
}

func (r *Repository) DB() *sql.DB {
	return r.db
}

func sqlNullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func sqlNullTime(value time.Time) sql.NullTime {
	return sql.NullTime{Time: value, Valid: true}
}

func sqlNullIntFromPtr(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func ptrFromSQLNullInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	id := int(value.Int64)
	return &id
}

func ptrFromSQLNullTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func stringFromSQLNull(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func timeFromSQLNull(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func userFromSQL(row dbgen.User) *domain.User {
	return &domain.User{
		ID:        int(row.ID),
		Email:     row.Email,
		Role:      domain.Role(stringFromSQLNull(row.Role)),
		IsEnabled: row.IsEnabled,
		CreatedAt: timeFromSQLNull(row.CreatedAt),
	}
}

func magicLinkFromSQL(row dbgen.MagicLink) *domain.MagicLink {
	return &domain.MagicLink{
		ID:        int(row.ID),
		UserID:    int(row.UserID),
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
		UsedAt:    ptrFromSQLNullTime(row.UsedAt),
	}
}

func sessionFromSQL(row dbgen.Session) *domain.Session {
	return &domain.Session{
		ID:        row.ID,
		UserID:    int(row.UserID),
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
	}
}

func subRequestFromSQL(row dbgen.SubRequest) *domain.SubRequest {
	return &domain.SubRequest{
		ID:             int(row.ID),
		ShowID:         int(row.ShowID),
		PostedByUserID: int(row.PostedByUserID),
		TakenByUserID:  ptrFromSQLNullInt(row.TakenByUserID),
		StartTime:      row.StartTime,
		EndTime:        row.EndTime,
		Notes:          stringFromSQLNull(row.Notes),
		CreatedAt:      timeFromSQLNull(row.CreatedAt),
		UpdatedAt:      timeFromSQLNull(row.UpdatedAt),
	}
}

func subRequestSummaryFromSQL(row dbgen.ListSubRequestsRow) subrequestsapp.DashboardReadModel {
	return subrequestsapp.DashboardReadModel{
		Request: &domain.SubRequest{
			ID:             int(row.ID),
			ShowID:         int(row.ShowID),
			PostedByUserID: int(row.PostedByUserID),
			TakenByUserID:  ptrFromSQLNullInt(row.TakenByUserID),
			StartTime:      row.StartTime,
			EndTime:        row.EndTime,
			Notes:          stringFromSQLNull(row.Notes),
			CreatedAt:      timeFromSQLNull(row.CreatedAt),
			UpdatedAt:      timeFromSQLNull(row.UpdatedAt),
		},
		RequesterEmail: row.Email,
		TakerEmail:     row.TakerEmail,
	}
}

func subRequestDetailFromSQL(row dbgen.GetSubRequestDetailByIDRow) subrequestsapp.DetailReadModel {
	return subrequestsapp.DetailReadModel{
		Request: &domain.SubRequest{
			ID:             int(row.ID),
			ShowID:         int(row.ShowID),
			PostedByUserID: int(row.PostedByUserID),
			TakenByUserID:  ptrFromSQLNullInt(row.TakenByUserID),
			StartTime:      row.StartTime,
			EndTime:        row.EndTime,
			Notes:          stringFromSQLNull(row.Notes),
			CreatedAt:      timeFromSQLNull(row.CreatedAt),
			UpdatedAt:      timeFromSQLNull(row.UpdatedAt),
		},
		RequesterEmail: row.RequesterEmail,
		TakerEmail:     row.TakerEmail,
	}
}
