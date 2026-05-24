package sqlite

import (
	"database/sql"

	"air-cover/internal/apperrors"
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
