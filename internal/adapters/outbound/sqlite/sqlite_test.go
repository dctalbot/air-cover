package sqlite

import (
	"database/sql"
	"testing"
	"time"
)

func TestNewRepository(t *testing.T) {
	database := &sql.DB{}
	repo := NewRepository(database)
	if repo == nil || repo.DB() != database {
		t.Fatal("expected repository to wrap database")
	}
}

func TestRepositoryNullMapperHelpers(t *testing.T) {
	if got := sqlNullIntFromPtr(nil); got.Valid {
		t.Fatal("expected nil int pointer to map to invalid sql null int")
	}
	id := 7
	if got := sqlNullIntFromPtr(&id); !got.Valid || got.Int64 != 7 {
		t.Fatalf("expected int pointer to map to valid sql null int, got %#v", got)
	}
	if got := ptrFromSQLNullTime(sql.NullTime{}); got != nil {
		t.Fatal("expected invalid sql null time to map to nil pointer")
	}
	now := time.Now()
	if got := ptrFromSQLNullTime(sql.NullTime{Time: now, Valid: true}); got == nil || !got.Equal(now) {
		t.Fatalf("expected valid sql null time to map to time pointer, got %v", got)
	}
	if got := stringFromSQLNull(sql.NullString{}); got != "" {
		t.Fatalf("expected invalid sql null string to map to empty string, got %q", got)
	}
	if got := timeFromSQLNull(sql.NullTime{}); !got.IsZero() {
		t.Fatalf("expected invalid sql null time to map to zero time, got %v", got)
	}
}
