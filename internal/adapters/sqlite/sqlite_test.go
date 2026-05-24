package sqlite

import (
	"database/sql"
	"testing"
)

func TestNewRepository(t *testing.T) {
	database := &sql.DB{}
	repo := NewRepository(database)
	if repo == nil || repo.DB() != database {
		t.Fatal("expected repository to wrap database")
	}
}
