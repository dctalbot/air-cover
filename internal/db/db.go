package db

import (
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

func InitDB(uri string) (*sql.DB, error) {
	db, err := sql.Open("libsql", uri)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to db: %w", err)
	}

	if err := RunMigration(db, "up"); err != nil {
		return nil, err
	}

	return db, nil
}

func RunMigration(db *sql.DB, command string) error {
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	var err error
	switch command {
	case "up":
		err = goose.Up(db, "migrations")
	case "down":
		err = goose.Down(db, "migrations")
	case "reset":
		err = goose.Reset(db, "migrations")
	case "status":
		err = goose.Status(db, "migrations")
	default:
		return fmt.Errorf("unknown migration command: %s", command)
	}

	if err != nil {
		return fmt.Errorf("failed to run migration %s: %w", command, err)
	}

	return nil
}
