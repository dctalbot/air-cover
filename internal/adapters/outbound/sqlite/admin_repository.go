package sqlite

import (
	"context"
	"database/sql"

	"air-cover/internal/adapters/outbound/sqlite/dbgen"
	"air-cover/internal/domain"
)

func (r *Repository) CreateUser(ctx context.Context, email string, role string) (*domain.User, error) {
	res, err := r.queries.CreateUser(ctx, dbgen.CreateUserParams{
		Email: email,
		Role:  sqlNullString(role),
	})
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetUserByID(ctx, int(id))
}

func (r *Repository) ListUsers(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.queries.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	users := make([]*domain.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, userFromSQL(row))
	}
	return users, nil
}

func (r *Repository) UpdateUser(ctx context.Context, id int, role *string, isEnabled *bool) error {
	var (
		res sql.Result
		err error
	)
	switch {
	case role != nil && isEnabled != nil:
		res, err = r.queries.UpdateUserRoleAndEnabled(ctx, dbgen.UpdateUserRoleAndEnabledParams{
			Role:      sqlNullString(*role),
			IsEnabled: *isEnabled,
			ID:        int64(id),
		})
	case role != nil:
		res, err = r.queries.UpdateUserRole(ctx, dbgen.UpdateUserRoleParams{
			Role: sqlNullString(*role),
			ID:   int64(id),
		})
	case isEnabled != nil:
		res, err = r.queries.UpdateUserEnabled(ctx, dbgen.UpdateUserEnabledParams{
			IsEnabled: *isEnabled,
			ID:        int64(id),
		})
	default:
		return nil
	}
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

	queries := r.queries.WithTx(tx)

	for _, email := range emails {
		if err := queries.ImportUser(ctx, email); err != nil {
			return err
		}
	}

	return tx.Commit()
}
