package sqlite

import (
	"context"

	"air-cover/internal/domain"
)

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
