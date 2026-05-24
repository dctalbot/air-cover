package sqlite

import (
	"testing"

	"air-cover/internal/app/contracttest"
)

func TestRepositoryContracts(t *testing.T) {
	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	repo := NewRepository(dbConn)
	if err := contracttest.CheckRepository(t.Context(), repo); err != nil {
		t.Fatal(err)
	}
}
