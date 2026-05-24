package sqlite

import (
	"testing"

	"air-cover/internal/app/contracttest"
)

func TestRepositoryContracts(t *testing.T) {
	t.Run("all repositories", func(t *testing.T) {
		repo := newContractRepository(t)
		if err := contracttest.CheckRepository(t.Context(), repo); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("subrequest commands", func(t *testing.T) {
		repo := newContractRepository(t)
		if err := contracttest.CheckSubRequestCommandRepository(t.Context(), repo); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("subrequest dashboard query", func(t *testing.T) {
		repo := newContractRepository(t)
		if err := contracttest.CheckSubRequestDashboardQuery(t.Context(), repo); err != nil {
			t.Fatal(err)
		}
	})
}

func newContractRepository(t *testing.T) *Repository {
	t.Helper()

	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = dbConn.Close()
	})
	return NewRepository(dbConn)
}
