package sqlite

import (
	"database/sql"

	"air-cover/internal/db"
)

type Repository = db.Repository

func InitDB(uri string) (*sql.DB, error) {
	return db.InitDB(uri)
}

func NewRepository(database *sql.DB) *Repository {
	return db.NewRepository(database)
}

func RunMigration(database *sql.DB, command string) error {
	return db.RunMigration(database, command)
}
