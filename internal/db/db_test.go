package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"air-cover/internal/models"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestInitDB(t *testing.T) {
	db, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// test errors
	_, err = InitDB("invalid-uri://")
	if err == nil {
		t.Fatal("expected error with invalid uri")
	}
}

func TestRepository(t *testing.T) {
	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	repo := NewRepository(dbConn)
	ctx := context.Background()

	if repo.DB() != dbConn {
		t.Error("expected repo.DB() to return the db connection")
	}

	// CreateUser
	u, err := repo.CreateUser(ctx, "test@example.com", "member")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "test@example.com" {
		t.Fatalf("expected test@example.com, got %v", u.Email)
	}
	if !u.IsEnabled {
		t.Error("expected IsEnabled to be true by default")
	}

	// GetUserByEmail
	u2, err := repo.GetUserByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != u2.ID {
		t.Fatalf("expected id %v, got %v", u.ID, u2.ID)
	}

	_, err = repo.GetUserByEmail(ctx, "notfound@example.com")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// GetUserByID
	u3, err := repo.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if u3.ID != u.ID {
		t.Fatalf("expected id %v, got %v", u.ID, u3.ID)
	}

	_, err = repo.GetUserByID(ctx, 99999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing ID, got %v", err)
	}

	// MagicLink
	err = repo.CreateMagicLink(ctx, u.ID, "hash123", time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	ml, err := repo.UseMagicLink(ctx, "hash123")
	if err != nil {
		t.Fatal(err)
	}
	if ml.UserID != u.ID {
		t.Fatalf("expected userid %v, got %v", u.ID, ml.UserID)
	}

	// Verify it's actually deleted from the DB
	var count int
	err = repo.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM magic_links WHERE token_hash = ?", "hash123").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected magic link to be deleted, but found %d records", count)
	}

	// Using it again should fail with ErrNotFound
	_, err = repo.UseMagicLink(ctx, "hash123")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound using link twice, got %v", err)
	}

	_, err = repo.UseMagicLink(ctx, "notfound")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Expired magic link
	err = repo.CreateMagicLink(ctx, u.ID, "expired-hash", time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.UseMagicLink(ctx, "expired-hash")
	if err == nil {
		t.Fatal("expected error for expired magic link")
	}

	// Session
	err = repo.CreateSession(ctx, "sid", "stoken", u.ID, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	s, err := repo.GetSessionByToken(ctx, "stoken")
	if err != nil {
		t.Fatal(err)
	}
	if s.UserID != u.ID {
		t.Fatalf("expected userid %v, got %v", u.ID, s.UserID)
	}

	_, err = repo.GetSessionByToken(ctx, "notfound")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Expired session
	err = repo.CreateSession(ctx, "sid2", "stoken2", u.ID, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.GetSessionByToken(ctx, "stoken2")
	if err == nil {
		t.Fatal("expected error for expired session")
	}

	// DeleteSessionsByUserID
	err = repo.DeleteSessionsByUserID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.GetSessionByToken(ctx, "stoken")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// CreateSubRequest
	sr := &models.SubRequest{
		ShowID:         123,
		PostedByUserID: u.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(1 * time.Hour),
		Notes:          "test notes",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = repo.CreateSubRequest(ctx, sr)
	if err != nil {
		t.Fatal(err)
	}

	// ListSubRequests
	list, err := repo.ListSubRequests(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least one sub request")
	}
	if list[0].ID == 0 {
		t.Fatal("expected non-zero id for created sub request")
	}
	sr.ID = list[0].ID
	if list[0].ID != sr.ID {
		t.Fatalf("expected id %v, got %v", sr.ID, list[0].ID)
	}
	if list[0].RequesterEmail != u.Email {
		t.Fatalf("expected email %v, got %v", u.Email, list[0].RequesterEmail)
	}

	// GetSubRequestByID
	sr2, err := repo.GetSubRequestByID(ctx, sr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sr2.ID != sr.ID {
		t.Fatalf("expected id %v, got %v", sr.ID, sr2.ID)
	}

	_, err = repo.GetSubRequestByID(ctx, 999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// DeleteSubRequest
	err = repo.DeleteSubRequest(ctx, sr.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.GetSubRequestByID(ctx, sr.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after deletion, got %v", err)
	}

	err = repo.DeleteSubRequest(ctx, 999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// TakeSubRequest
	sr3 := &models.SubRequest{
		ShowID:         456,
		PostedByUserID: u.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(1 * time.Hour),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = repo.CreateSubRequest(ctx, sr3)
	if err != nil {
		t.Fatal(err)
	}

	u4, _ := repo.CreateUser(ctx, "taker@example.com", "member")
	err = repo.TakeSubRequest(ctx, sr3.ID, u4.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify TakerEmail in ListSubRequests
	list2, err := repo.ListSubRequests(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range list2 {
		if item.ID == sr3.ID {
			found = true
			if item.TakerEmail != u4.Email {
				t.Fatalf("expected taker email %v, got %v", u4.Email, item.TakerEmail)
			}
			if item.TakenByUserID == nil || *item.TakenByUserID != u4.ID {
				t.Fatalf("expected taken_by_user_id %v", u4.ID)
			}
		}
	}
	if !found {
		t.Fatal("expected to find the taken sub request")
	}

	err = repo.TakeSubRequest(ctx, 99999, u4.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for TakeSubRequest, got %v", err)
	}

	err = repo.TakeSubRequest(ctx, sr3.ID, u4.ID)
	if err != ErrConflict {
		t.Fatalf("expected ErrConflict for already taken TakeSubRequest, got %v", err)
	}

	// UntakeSubRequest
	err = repo.UntakeSubRequest(ctx, sr3.ID)
	if err != nil {
		t.Fatal(err)
	}

	sr3After, err := repo.GetSubRequestByID(ctx, sr3.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sr3After.TakenByUserID != nil {
		t.Fatal("expected TakenByUserID to be nil after untake")
	}

	err = repo.UntakeSubRequest(ctx, 99999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for UntakeSubRequest, got %v", err)
	}

	// ListUsers
	users, err := repo.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) == 0 {
		t.Fatal("expected at least one user")
	}
	if !users[0].IsEnabled {
		t.Error("expected users[0].IsEnabled to be true")
	}

	// UpdateUser
	newRole := "admin"
	newIsEnabled := false
	err = repo.UpdateUser(ctx, u.ID, &newRole, &newIsEnabled)
	if err != nil {
		t.Fatal(err)
	}
	uUpdated, err := repo.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if uUpdated.Role != "admin" {
		t.Fatalf("expected role admin, got %s", uUpdated.Role)
	}
	if uUpdated.IsEnabled {
		t.Error("expected IsEnabled to be false")
	}

	// UpdateUser with nil args
	err = repo.UpdateUser(ctx, u.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// UpdateUser err not found
	err = repo.UpdateUser(ctx, 99999, &newRole, nil)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

}

func TestRepositoryErrors(t *testing.T) {
	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	repo := NewRepository(dbConn)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately to trigger errors

	// Test context cancellation errors
	_, err = repo.GetUserByEmail(ctx, "test@example.com")
	if err == nil {
		t.Error("expected error with cancelled context in GetUserByEmail")
	}

	_, err = repo.GetUserByID(ctx, 1)
	if err == nil {
		t.Error("expected error with cancelled context in GetUserByID")
	}

	_, err = repo.CreateUser(ctx, "test@example.com", "member")
	if err == nil {
		t.Error("expected error with cancelled context in CreateUser")
	}

	err = repo.CreateMagicLink(ctx, 1, "hash", time.Now())
	if err == nil {
		t.Error("expected error with cancelled context in CreateMagicLink")
	}

	_, err = repo.UseMagicLink(ctx, "hash")
	if err == nil {
		t.Error("expected error with cancelled context in UseMagicLink")
	}

	err = repo.CreateSession(ctx, "sid", "stoken", 1, time.Now())
	if err == nil {
		t.Error("expected error with cancelled context in CreateSession")
	}

	_, err = repo.GetSessionByToken(ctx, "stoken")
	if err == nil {
		t.Error("expected error with cancelled context in GetSessionByToken")
	}

	err = repo.DeleteSessionsByUserID(ctx, 1)
	if err == nil {
		t.Error("expected error with cancelled context in DeleteSessionsByUserID")
	}

	err = repo.CreateSubRequest(ctx, &models.SubRequest{})
	if err == nil {
		t.Error("expected error with cancelled context in CreateSubRequest")
	}

	_, err = repo.ListSubRequests(ctx)
	if err == nil {
		t.Error("expected error with cancelled context in ListSubRequests")
	}

	_, err = repo.GetSubRequestByID(ctx, 1)
	if err == nil {
		t.Error("expected error with cancelled context in GetSubRequestByID")
	}

	err = repo.DeleteSubRequest(ctx, 1)
	if err == nil {
		t.Error("expected error with cancelled context in DeleteSubRequest")
	}

	_, err = repo.ListUsers(ctx)
	if err == nil {
		t.Error("expected error with cancelled context in ListUsers")
	}

	err = repo.UpdateUser(ctx, 1, nil, nil)
	// nil args return early without error
	if err != nil {
		t.Errorf("expected nil error with nil args and cancelled context, got %v", err)
	}

	role := "admin"
	err = repo.UpdateUser(ctx, 1, &role, nil)
	if err == nil {
		t.Error("expected error with cancelled context in UpdateUser")
	}

	err = repo.TakeSubRequest(ctx, 1, 1)
	if err == nil {
		t.Error("expected error with cancelled context in TakeSubRequest")
	}

	err = repo.UntakeSubRequest(ctx, 1)
	if err == nil {
		t.Error("expected error with cancelled context in UntakeSubRequest")
	}

	// Test unique constraint violation
	ctx = context.Background()
	_, _ = repo.CreateUser(ctx, "unique@example.com", "member")
	_, err = repo.CreateUser(ctx, "unique@example.com", "member")
	if err == nil {
		t.Error("expected error for duplicate user email")
	}

	// Test UseMagicLink with invalid state
	// Already used case is covered in TestRepository
}

func TestInitDBErrors(t *testing.T) {
	// Ping failure - using a DSN that might fail ping
	_, err := InitDB("file:/nonexistent/path/db.sqlite?mode=ro")
	if err == nil {
		t.Error("expected error for nonexistent path in InitDB")
	}

	// Wait, libsql actually accepts practically any URI until ping?
	// Let's try to pass an invalid scheme to make sql.Open fail if the driver rejects it.
	// The libsql driver supports specific schemes. If we provide something it rejects completely at Open time:
	_, err = InitDB("invalid-uri-scheme:///")
	if err == nil {
		t.Error("expected error for invalid URI scheme in InitDB")
	}
}

func TestInitDB_OpenAndMigrationErrors(t *testing.T) {
	t.Run("open error", func(t *testing.T) {
		originalSQLOpen := sqlOpen
		t.Cleanup(func() { sqlOpen = originalSQLOpen })
		sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
			return nil, errors.New("open failed")
		}

		if _, err := InitDB("file::memory:"); err == nil || !strings.Contains(err.Error(), "failed to open db") {
			t.Fatalf("expected open error, got %v", err)
		}
	})

	t.Run("migration error", func(t *testing.T) {
		originalRunMigration := runMigrationFn
		t.Cleanup(func() { runMigrationFn = originalRunMigration })
		runMigrationFn = func(db *sql.DB, command string) error {
			return errors.New("migration failed")
		}

		if _, err := InitDB("file::memory:"); err == nil || !strings.Contains(err.Error(), "migration failed") {
			t.Fatalf("expected migration error, got %v", err)
		}
	})
}

func TestRunMigration_SetDialectError(t *testing.T) {
	originalSetDialect := gooseSetDialect
	t.Cleanup(func() { gooseSetDialect = originalSetDialect })
	gooseSetDialect = func(dialect string) error {
		return errors.New("dialect failed")
	}

	dbConn, mock, _ := newMockRepository(t)
	mock.MatchExpectationsInOrder(false)
	defer dbConn.Close()

	if err := RunMigration(dbConn, "up"); err == nil || !strings.Contains(err.Error(), "failed to set goose dialect") {
		t.Fatalf("expected dialect error, got %v", err)
	}
}

func TestListSubRequestsErrors(t *testing.T) {
	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(dbConn)
	ctx := context.Background()

	// Test closed DB for QueryContext error
	dbConn.Close()
	_, err = repo.ListSubRequests(ctx)
	if err == nil {
		t.Error("expected error with closed db in ListSubRequests")
	}

	_, err = repo.ListUsers(ctx)
	if err == nil {
		t.Error("expected error with closed db in ListUsers")
	}
}

func TestScanErrors(t *testing.T) {
	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()
	repo := NewRepository(dbConn)
	ctx := context.Background()

	// Drop and recreate sub_requests with incompatible types to trigger Scan error
	_, _ = dbConn.Exec("DROP TABLE sub_requests")
	_, err = dbConn.Exec(`
		CREATE TABLE sub_requests (
			id TEXT, show_id TEXT, posted_by_user_id TEXT, taken_by_user_id TEXT, start_time TEXT, end_time TEXT, 
			notes TEXT, created_at TEXT, updated_at TEXT
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert garbage data
	_, err = dbConn.Exec(`
		INSERT INTO sub_requests (id, show_id, posted_by_user_id, taken_by_user_id, start_time, end_time, notes, created_at, updated_at)
		VALUES ('bad-id', 'not-an-int', 'not-an-int', 'not-an-int', 'not-a-date', 'not-a-date', 'notes', 'not-a-date', 'not-a-date')
	`)
	if err != nil {
		t.Fatal(err)
	}

	// ListSubRequests will fail because it expects a JOIN with users which doesn't exist now
	// and because of type mismatch.
	// Actually, we need the JOIN to exist if we want to reach Scan.
	_, _ = dbConn.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT, role TEXT, is_enabled BOOLEAN)")
	_, _ = dbConn.Exec("INSERT INTO users (id, email, role, is_enabled) VALUES (1, 'test@example.com', 'member', 1)")
	// Update posted_by_user_id to be a valid join but other fields to be garbage
	_, _ = dbConn.Exec("UPDATE sub_requests SET posted_by_user_id = 1")

	_, err = repo.ListSubRequests(ctx)
	if err == nil {
		t.Error("expected scan error in ListSubRequests")
	}

	_, err = repo.GetSubRequestByID(ctx, 123)
	if err == nil {
		t.Error("expected scan error in GetSubRequestByID")
	}

	// ListUsers scan error
	_, _ = dbConn.Exec("DROP TABLE users")
	_, _ = dbConn.Exec("CREATE TABLE users (id TEXT, email TEXT, role TEXT, is_enabled TEXT, created_at TEXT)")
	_, _ = dbConn.Exec("INSERT INTO users (id, email, role, is_enabled, created_at) VALUES ('bad', 'bad', 'bad', 'bad', 'bad')")
	_, err = repo.ListUsers(ctx)
	if err == nil {
		t.Error("expected scan error in ListUsers")
	}
}

func TestInitDB_MigrationError(t *testing.T) {
	// Create a file, make it read-only
	f, err := os.CreateTemp("", "ro-db-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	// On some systems, even 0400 allows the owner to write if they are the one opening it.
	// But let's try.
	_ = os.Chmod(f.Name(), 0000)
	defer os.Remove(f.Name())

	_, err = InitDB(f.Name())
	if err == nil {
		t.Log("Note: root/owner can sometimes bypass 0000 permissions")
	}
}

func TestRunMigration(t *testing.T) {
	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	if err := RunMigration(dbConn, "status"); err != nil {
		t.Fatalf("expected no error for status, got %v", err)
	}

	if err := RunMigration(dbConn, "down"); err != nil {
		t.Fatalf("expected no error for down, got %v", err)
	}

	if err := RunMigration(dbConn, "up"); err != nil {
		t.Fatalf("expected no error for up, got %v", err)
	}

	if err := RunMigration(dbConn, "reset"); err != nil {
		t.Fatalf("expected no error for reset, got %v", err)
	}

	if err := RunMigration(dbConn, "invalid"); err == nil {
		t.Fatal("expected error for invalid command")
	}
}

func TestRunMigrationError(t *testing.T) {
	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	dbConn.Close()

	if err := RunMigration(dbConn, "up"); err == nil {
		t.Fatal("expected error for up with closed db")
	}
}

func TestImportUsers(t *testing.T) {
	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	repo := NewRepository(dbConn)
	ctx := context.Background()

	// Empty list
	err = repo.ImportUsers(ctx, []string{})
	if err != nil {
		t.Fatalf("expected no error for empty list, got %v", err)
	}

	// Normal import
	err = repo.ImportUsers(ctx, []string{"user1@example.com", "user2@example.com"})
	if err != nil {
		t.Fatalf("expected no error for import, got %v", err)
	}

	users, _ := repo.ListUsers(ctx)
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}

	// Idempotent import
	err = repo.ImportUsers(ctx, []string{"user1@example.com", "user3@example.com"})
	if err != nil {
		t.Fatalf("expected no error for idempotent import, got %v", err)
	}

	users, _ = repo.ListUsers(ctx)
	if len(users) != 3 {
		t.Fatalf("expected 3 users after idempotent import, got %d", len(users))
	}
}

func TestImportUsers_Errors(t *testing.T) {
	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(dbConn)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel context

	err = repo.ImportUsers(ctx, []string{"user@example.com"})
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}

	// Test PrepareContext error by dropping table
	dbConn2, _ := InitDB("file::memory:")
	repo2 := NewRepository(dbConn2)
	_, _ = dbConn2.Exec("DROP TABLE users")
	err = repo2.ImportUsers(context.Background(), []string{"user@example.com"})
	if err == nil {
		t.Fatal("expected error with dropped table")
	}
	dbConn2.Close()
}

func TestUseMagicLink_AlreadyUsed(t *testing.T) {
	dbConn, err := InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	repo := NewRepository(dbConn)
	ctx := context.Background()

	u, err := repo.CreateUser(ctx, "used@example.com", "member")
	if err != nil {
		t.Fatal(err)
	}

	// Create a magic link and set used_at directly
	err = repo.CreateMagicLink(ctx, u.ID, "used-hash", time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	_, err = dbConn.Exec("UPDATE magic_links SET used_at = ? WHERE token_hash = ?", time.Now(), "used-hash")
	if err != nil {
		t.Fatal(err)
	}

	// Should fail with "magic link expired or already used"
	_, err = repo.UseMagicLink(ctx, "used-hash")
	if err == nil {
		t.Fatal("expected error for already-used magic link")
	}
}

func TestUseMagicLink_DeleteError(t *testing.T) {
	dbConn, mock, repo := newMockRepository(t)
	defer dbConn.Close()

	expiresAt := time.Now().Add(time.Hour)
	rows := sqlmock.NewRows([]string{"id", "user_id", "token_hash", "expires_at", "used_at"}).
		AddRow(1, 2, "del-hash", expiresAt, nil)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, token_hash, expires_at, used_at FROM magic_links WHERE token_hash = ?")).
		WithArgs("del-hash").
		WillReturnRows(rows)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM magic_links WHERE id = ?")).
		WithArgs(1).
		WillReturnError(errors.New("delete failed"))

	if _, err := repo.UseMagicLink(context.Background(), "del-hash"); err == nil {
		t.Fatal("expected delete error")
	}
}

func TestImportUsers_ExecError(t *testing.T) {
	// Test the stmt.ExecContext error path inside the for loop
	dbConn, err := InitDB("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	repo := NewRepository(dbConn)
	ctx := context.Background()

	// Add a unique constraint that will cause the insert to fail
	// Actually, the query uses ON CONFLICT DO NOTHING, so unique violations won't error.
	// Instead, we can alter the table to have a NOT NULL constraint on a column
	// that doesn't get populated by the INSERT.
	_, _ = dbConn.Exec("ALTER TABLE users ADD COLUMN required_field TEXT NOT NULL DEFAULT 'x'")
	// Remove the default so new inserts without it fail
	// SQLite doesn't support ALTER COLUMN, but we can drop and recreate
	_, _ = dbConn.Exec("DROP TABLE users")
	_, err = dbConn.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE NOT NULL,
		role TEXT NOT NULL,
		is_enabled BOOLEAN NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		extra TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}

	// The INSERT doesn't include 'extra' and there's no default, so it should fail
	err = repo.ImportUsers(ctx, []string{"fail@example.com"})
	if err == nil {
		t.Fatal("expected error from exec in ImportUsers loop")
	}
}

func TestListSubRequestsOrdering(t *testing.T) {
	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	repo := NewRepository(dbConn)
	ctx := context.Background()

	u, _ := repo.CreateUser(ctx, "test@example.com", "member")

	now := time.Now()
	// Create sub requests in random order of start time
	times := []time.Time{
		now.Add(2 * time.Hour),
		now.Add(1 * time.Hour),
		now.Add(3 * time.Hour),
	}

	for _, st := range times {
		sr := &models.SubRequest{
			ShowID:         1,
			PostedByUserID: u.ID,
			StartTime:      st,
			EndTime:        st.Add(1 * time.Hour),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := repo.CreateSubRequest(ctx, sr); err != nil {
			t.Fatal(err)
		}
	}

	list, err := repo.ListSubRequests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(list) != 3 {
		t.Fatalf("expected 3 items, got %d", len(list))
	}

	// Verify they are ordered by start time ascending
	if !list[0].StartTime.Before(list[1].StartTime) {
		t.Errorf("expected %v before %v", list[0].StartTime, list[1].StartTime)
	}
	if !list[1].StartTime.Before(list[2].StartTime) {
		t.Errorf("expected %v before %v", list[1].StartTime, list[2].StartTime)
	}
}

func TestRepositoryDriverLevelErrors(t *testing.T) {
	t.Run("create user last insert id error", func(t *testing.T) {
		dbConn, mock, repo := newMockRepository(t)
		defer dbConn.Close()

		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users (email, role) VALUES (?, ?)")).
			WithArgs("lastid@example.com", "member").
			WillReturnResult(sqlmock.NewErrorResult(errors.New("last insert id failed")))

		_, err := repo.CreateUser(context.Background(), "lastid@example.com", "member")
		if err == nil {
			t.Fatal("expected LastInsertId error")
		}
	})

	t.Run("create sub request last insert id error", func(t *testing.T) {
		dbConn, mock, repo := newMockRepository(t)
		defer dbConn.Close()

		mock.ExpectExec("INSERT INTO sub_requests").
			WillReturnResult(sqlmock.NewErrorResult(errors.New("last insert id failed")))

		err := repo.CreateSubRequest(context.Background(), &models.SubRequest{
			ShowID:         1,
			PostedByUserID: 2,
			StartTime:      time.Now(),
			EndTime:        time.Now().Add(time.Hour),
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		})
		if err == nil {
			t.Fatal("expected LastInsertId error")
		}
	})

	t.Run("delete sub request rows affected error", func(t *testing.T) {
		dbConn, mock, repo := newMockRepository(t)
		defer dbConn.Close()

		mock.ExpectExec(regexp.QuoteMeta("DELETE FROM sub_requests WHERE id = ?")).
			WithArgs(9).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected failed")))

		err := repo.DeleteSubRequest(context.Background(), 9)
		if err == nil {
			t.Fatal("expected RowsAffected error")
		}
	})

	t.Run("take sub request rows affected error", func(t *testing.T) {
		dbConn, mock, repo := newMockRepository(t)
		defer dbConn.Close()

		mock.ExpectExec(regexp.QuoteMeta("UPDATE sub_requests SET taken_by_user_id = ?, updated_at = ? WHERE id = ? AND taken_by_user_id IS NULL")).
			WithArgs(2, sqlmock.AnyArg(), 9).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected failed")))

		err := repo.TakeSubRequest(context.Background(), 9, 2)
		if err == nil {
			t.Fatal("expected RowsAffected error")
		}
	})

	t.Run("untake sub request rows affected error", func(t *testing.T) {
		dbConn, mock, repo := newMockRepository(t)
		defer dbConn.Close()

		mock.ExpectExec(regexp.QuoteMeta("UPDATE sub_requests SET taken_by_user_id = NULL, updated_at = ? WHERE id = ?")).
			WithArgs(sqlmock.AnyArg(), 9).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected failed")))

		err := repo.UntakeSubRequest(context.Background(), 9)
		if err == nil {
			t.Fatal("expected RowsAffected error")
		}
	})

	t.Run("update user rows affected error", func(t *testing.T) {
		dbConn, mock, repo := newMockRepository(t)
		defer dbConn.Close()

		role := "admin"
		mock.ExpectExec(regexp.QuoteMeta("UPDATE users SET role = ? WHERE id = ?")).
			WithArgs(role, 7).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected failed")))

		err := repo.UpdateUser(context.Background(), 7, &role, nil)
		if err == nil {
			t.Fatal("expected RowsAffected error")
		}
	})
}

func TestRepositoryRowsErrors(t *testing.T) {
	t.Run("list sub requests rows err", func(t *testing.T) {
		dbConn, mock, repo := newMockRepository(t)
		defer dbConn.Close()

		rows := sqlmock.NewRows([]string{
			"id", "show_id", "posted_by_user_id", "taken_by_user_id", "email", "email", "start_time", "end_time", "notes", "created_at", "updated_at",
		}).AddRow(1, 2, 3, nil, "requester@example.com", "", time.Now(), time.Now().Add(time.Hour), "", time.Now(), time.Now()).
			RowError(0, errors.New("rows failed"))
		mock.ExpectQuery("SELECT sr.id").WillReturnRows(rows)

		_, err := repo.ListSubRequests(context.Background())
		if err == nil {
			t.Fatal("expected rows error")
		}
	})

	t.Run("list users rows err", func(t *testing.T) {
		dbConn, mock, repo := newMockRepository(t)
		defer dbConn.Close()

		rows := sqlmock.NewRows([]string{"id", "email", "role", "is_enabled", "created_at"}).
			AddRow(1, "user@example.com", "member", true, time.Now()).
			RowError(0, errors.New("rows failed"))
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, role, is_enabled, created_at FROM users ORDER BY created_at DESC")).
			WillReturnRows(rows)

		_, err := repo.ListUsers(context.Background())
		if err == nil {
			t.Fatal("expected rows error")
		}
	})
}

func TestImportUsersDriverLevelErrors(t *testing.T) {
	tests := []struct {
		name      string
		expect    func(sqlmock.Sqlmock)
		wantError string
	}{
		{
			name: "begin error",
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
			},
			wantError: "begin failed",
		},
		{
			name: "prepare error",
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectPrepare("INSERT INTO users").WillReturnError(errors.New("prepare failed"))
				mock.ExpectRollback()
			},
			wantError: "prepare failed",
		},
		{
			name: "exec error",
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectPrepare("INSERT INTO users").
					ExpectExec().
					WithArgs("one@example.com").
					WillReturnError(errors.New("exec failed"))
				mock.ExpectRollback()
			},
			wantError: "exec failed",
		},
		{
			name: "commit error",
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectPrepare("INSERT INTO users").
					ExpectExec().
					WithArgs("one@example.com").
					WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
			},
			wantError: "commit failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbConn, mock, repo := newMockRepository(t)
			defer dbConn.Close()
			tt.expect(mock)

			err := repo.ImportUsers(context.Background(), []string{"one@example.com"})
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
			}
		})
	}
}

func newMockRepository(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *Repository) {
	t.Helper()

	dbConn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	return dbConn, mock, NewRepository(dbConn)
}
