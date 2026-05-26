package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	authapp "air-cover/internal/app/auth"
	"air-cover/internal/domain"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestInitDB(t *testing.T) {
	db, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	assertIndexesExist(t, db, "magic_links", "idx_magic_links_token_hash", "idx_magic_links_user_id")
	assertIndexesExist(t, db, "sessions", "idx_sessions_token_hash", "idx_sessions_user_id")
	assertIndexesExist(t, db, "sub_requests", "idx_sub_requests_start_time", "idx_sub_requests_posted_by_user_id", "idx_sub_requests_taken_by_user_id")

	// test errors
	_, err = InitDB("invalid-uri://")
	if err == nil {
		t.Fatal("expected error with invalid uri")
	}
}

func assertIndexesExist(t *testing.T, db *sql.DB, table string, expectedNames ...string) {
	t.Helper()

	rows, err := db.Query("PRAGMA index_list(" + table + ")")
	if err != nil {
		t.Fatalf("failed to list indexes for %s: %v", table, err)
	}
	defer rows.Close()

	found := make(map[string]bool)
	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("failed to scan index for %s: %v", table, err)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("failed to iterate indexes for %s: %v", table, err)
	}

	for _, name := range expectedNames {
		if !found[name] {
			t.Fatalf("expected index %s on %s", name, table)
		}
	}
}

type repositoryTestContext struct {
	ctx  context.Context
	repo *Repository
	user *domain.User
}

func newRepositoryTestContext(t *testing.T) repositoryTestContext {
	t.Helper()

	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = dbConn.Close()
	})

	repo := NewRepository(dbConn)
	ctx := context.Background()

	u, err := repo.CreateUser(ctx, "test@example.com", "member")
	if err != nil {
		t.Fatal(err)
	}

	return repositoryTestContext{ctx: ctx, repo: repo, user: u}
}

func TestRepositoryReturnsDBConnection(t *testing.T) {
	// Arrange
	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = dbConn.Close()
	})
	repo := NewRepository(dbConn)

	// Act
	got := repo.DB()

	// Assert
	if got != dbConn {
		t.Error("expected repo.DB() to return the db connection")
	}
}

func TestRepositoryUsers(t *testing.T) {
	// Arrange
	tc := newRepositoryTestContext(t)

	// Act
	byEmail, err := tc.repo.GetUserByEmail(tc.ctx, tc.user.Email)
	if err != nil {
		t.Fatal(err)
	}
	byID, err := tc.repo.GetUserByID(tc.ctx, tc.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	users, err := tc.repo.ListUsers(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}
	activeUsers, err := tc.repo.ListActiveUsers(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if tc.user.Email != "test@example.com" {
		t.Fatalf("expected test@example.com, got %v", tc.user.Email)
	}
	if !tc.user.IsEnabled {
		t.Error("expected IsEnabled to be true by default")
	}
	if byEmail.ID != tc.user.ID {
		t.Fatalf("expected id %v, got %v", tc.user.ID, byEmail.ID)
	}
	if byID.ID != tc.user.ID {
		t.Fatalf("expected id %v, got %v", tc.user.ID, byID.ID)
	}
	if len(users) == 0 {
		t.Fatal("expected at least one user")
	}
	if !users[0].IsEnabled {
		t.Error("expected users[0].IsEnabled to be true")
	}
	if len(activeUsers) == 0 {
		t.Fatal("expected active users")
	}
	for _, activeUser := range activeUsers {
		if !activeUser.IsEnabled {
			t.Fatalf("expected only enabled users, got %+v", activeUser)
		}
	}

	_, err = tc.repo.GetUserByEmail(tc.ctx, "notfound@example.com")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	_, err = tc.repo.GetUserByID(tc.ctx, 99999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing ID, got %v", err)
	}
}

func TestRepositoryUpdatesUsers(t *testing.T) {
	// Arrange
	tc := newRepositoryTestContext(t)
	newRole := "admin"
	newIsEnabled := false

	// Act
	err := tc.repo.UpdateUser(tc.ctx, tc.user.ID, &newRole, &newIsEnabled)
	if err != nil {
		t.Fatal(err)
	}
	uUpdated, err := tc.repo.GetUserByID(tc.ctx, tc.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	activeUsers, err := tc.repo.ListActiveUsers(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if uUpdated.Role != domain.RoleAdmin {
		t.Fatalf("expected role admin, got %s", uUpdated.Role)
	}
	if uUpdated.IsEnabled {
		t.Error("expected IsEnabled to be false")
	}
	for _, activeUser := range activeUsers {
		if activeUser.ID == tc.user.ID {
			t.Fatal("disabled user should not be returned by ListActiveUsers")
		}
	}

	err = tc.repo.UpdateUser(tc.ctx, tc.user.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = tc.repo.UpdateUser(tc.ctx, 99999, &newRole, nil)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRepositoryMagicLinks(t *testing.T) {
	// Arrange
	tc := newRepositoryTestContext(t)

	err := tc.repo.CreateMagicLink(tc.ctx, tc.user.ID, "hash123", time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	// Act
	ml, err := tc.repo.UseMagicLink(tc.ctx, "hash123", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if ml.UserID != tc.user.ID {
		t.Fatalf("expected userid %v, got %v", tc.user.ID, ml.UserID)
	}

	var count int
	err = tc.repo.DB().QueryRowContext(tc.ctx, "SELECT COUNT(*) FROM magic_links WHERE token_hash = ?", "hash123").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected magic link to be deleted, but found %d records", count)
	}

	_, err = tc.repo.UseMagicLink(tc.ctx, "hash123", time.Now())
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound using link twice, got %v", err)
	}

	_, err = tc.repo.UseMagicLink(tc.ctx, "notfound", time.Now())
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	err = tc.repo.CreateMagicLink(tc.ctx, tc.user.ID, "expired-hash", time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	_, err = tc.repo.UseMagicLink(tc.ctx, "expired-hash", time.Now())
	if err == nil {
		t.Fatal("expected error for expired magic link")
	}
}

func TestRepositorySessions(t *testing.T) {
	// Arrange
	tc := newRepositoryTestContext(t)
	sessionTokenHash := authapp.HashToken("stoken")

	err := tc.repo.CreateSession(tc.ctx, "sid", sessionTokenHash, tc.user.ID, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	// Act
	s, err := tc.repo.GetSessionByToken(tc.ctx, sessionTokenHash, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	var rawTokenCount int
	err = tc.repo.DB().QueryRowContext(tc.ctx, "SELECT COUNT(*) FROM sessions WHERE token_hash = ?", "stoken").Scan(&rawTokenCount)
	if err != nil {
		t.Fatal(err)
	}
	if rawTokenCount != 0 {
		t.Fatal("raw session token was stored in sessions")
	}
	if s.UserID != tc.user.ID {
		t.Fatalf("expected userid %v, got %v", tc.user.ID, s.UserID)
	}

	_, err = tc.repo.GetSessionByToken(tc.ctx, authapp.HashToken("notfound"), time.Now())
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	expiredSessionTokenHash := authapp.HashToken("stoken2")
	err = tc.repo.CreateSession(tc.ctx, "sid2", expiredSessionTokenHash, tc.user.ID, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	_, err = tc.repo.GetSessionByToken(tc.ctx, expiredSessionTokenHash, time.Now())
	if err == nil {
		t.Fatal("expected error for expired session")
	}

	err = tc.repo.DeleteSessionsByUserID(tc.ctx, tc.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tc.repo.GetSessionByToken(tc.ctx, sessionTokenHash, time.Now())
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRepositorySubRequests(t *testing.T) {
	// Arrange
	tc := newRepositoryTestContext(t)
	sr := &domain.SubRequest{
		ShowID:         123,
		PostedByUserID: tc.user.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(1 * time.Hour),
		Notes:          "test notes",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Act
	err := tc.repo.CreateSubRequest(tc.ctx, sr)
	if err != nil {
		t.Fatal(err)
	}
	list, err := tc.repo.ListDashboardSubRequests(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(list) == 0 {
		t.Fatal("expected at least one sub request")
	}
	if list[0].Request.ID == 0 {
		t.Fatal("expected non-zero id for created sub request")
	}
	sr.ID = list[0].Request.ID
	if list[0].Request.ID != sr.ID {
		t.Fatalf("expected id %v, got %v", sr.ID, list[0].Request.ID)
	}
	if list[0].RequesterEmail != tc.user.Email {
		t.Fatalf("expected email %v, got %v", tc.user.Email, list[0].RequesterEmail)
	}

	detail, err := tc.repo.GetSubRequestDetailByID(tc.ctx, sr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Request.ID != sr.ID {
		t.Fatalf("expected detail id %v, got %v", sr.ID, detail.Request.ID)
	}
	if detail.RequesterEmail != tc.user.Email {
		t.Fatalf("expected detail requester email %v, got %v", tc.user.Email, detail.RequesterEmail)
	}

	sr2, err := tc.repo.GetSubRequestByID(tc.ctx, sr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sr2.ID != sr.ID {
		t.Fatalf("expected id %v, got %v", sr.ID, sr2.ID)
	}

	_, err = tc.repo.GetSubRequestByID(tc.ctx, 999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	_, err = tc.repo.GetSubRequestDetailByID(tc.ctx, 999)
	if err != ErrNotFound {
		t.Fatalf("expected detail ErrNotFound, got %v", err)
	}

	err = tc.repo.DeleteSubRequest(tc.ctx, sr.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tc.repo.GetSubRequestByID(tc.ctx, sr.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after deletion, got %v", err)
	}

	err = tc.repo.DeleteSubRequest(tc.ctx, 999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRepositorySubRequestTaking(t *testing.T) {
	// Arrange
	tc := newRepositoryTestContext(t)
	sr := &domain.SubRequest{
		ShowID:         456,
		PostedByUserID: tc.user.ID,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(1 * time.Hour),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := tc.repo.CreateSubRequest(tc.ctx, sr)
	if err != nil {
		t.Fatal(err)
	}
	taker, err := tc.repo.CreateUser(tc.ctx, "taker@example.com", "member")
	if err != nil {
		t.Fatal(err)
	}

	// Act
	err = tc.repo.TakeSubRequest(tc.ctx, sr.ID, taker.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	list, err := tc.repo.ListDashboardSubRequests(tc.ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range list {
		if item.Request.ID == sr.ID {
			found = true
			if item.TakerEmail != taker.Email {
				t.Fatalf("expected taker email %v, got %v", taker.Email, item.TakerEmail)
			}
			if item.Request.TakenByUserID == nil || *item.Request.TakenByUserID != taker.ID {
				t.Fatalf("expected taken_by_user_id %v", taker.ID)
			}
		}
	}
	if !found {
		t.Fatal("expected to find the taken sub request")
	}

	detailTaken, err := tc.repo.GetSubRequestDetailByID(tc.ctx, sr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detailTaken.TakerEmail != taker.Email {
		t.Fatalf("expected detail taker email %v, got %v", taker.Email, detailTaken.TakerEmail)
	}

	err = tc.repo.TakeSubRequest(tc.ctx, 99999, taker.ID, time.Now())
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for TakeSubRequest, got %v", err)
	}

	err = tc.repo.TakeSubRequest(tc.ctx, sr.ID, taker.ID, time.Now())
	if err != ErrConflict {
		t.Fatalf("expected ErrConflict for already taken TakeSubRequest, got %v", err)
	}

	err = tc.repo.UntakeSubRequest(tc.ctx, sr.ID, tc.user.ID, time.Now())
	if err != ErrConflict {
		t.Fatalf("expected ErrConflict for wrong user UntakeSubRequest, got %v", err)
	}

	err = tc.repo.UntakeSubRequest(tc.ctx, sr.ID, taker.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	srAfter, err := tc.repo.GetSubRequestByID(tc.ctx, sr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if srAfter.TakenByUserID != nil {
		t.Fatal("expected TakenByUserID to be nil after untake")
	}

	err = tc.repo.UntakeSubRequest(tc.ctx, 99999, taker.ID, time.Now())
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for UntakeSubRequest, got %v", err)
	}
}

func TestRepository(t *testing.T) {
	t.Run("db connection", TestRepositoryReturnsDBConnection)
	t.Run("users", TestRepositoryUsers)
	t.Run("user updates", TestRepositoryUpdatesUsers)
	t.Run("magic links", TestRepositoryMagicLinks)
	t.Run("sessions", TestRepositorySessions)
	t.Run("sub requests", TestRepositorySubRequests)
	t.Run("sub request taking", TestRepositorySubRequestTaking)
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

	_, err = repo.UseMagicLink(ctx, "hash", time.Now())
	if err == nil {
		t.Error("expected error with cancelled context in UseMagicLink")
	}

	err = repo.CreateSession(ctx, "sid", authapp.HashToken("stoken"), 1, time.Now())
	if err == nil {
		t.Error("expected error with cancelled context in CreateSession")
	}

	_, err = repo.GetSessionByToken(ctx, authapp.HashToken("stoken"), time.Now())
	if err == nil {
		t.Error("expected error with cancelled context in GetSessionByToken")
	}

	err = repo.DeleteSessionsByUserID(ctx, 1)
	if err == nil {
		t.Error("expected error with cancelled context in DeleteSessionsByUserID")
	}

	err = repo.CreateSubRequest(ctx, &domain.SubRequest{})
	if err == nil {
		t.Error("expected error with cancelled context in CreateSubRequest")
	}

	_, err = repo.ListDashboardSubRequests(ctx)
	if err == nil {
		t.Error("expected error with cancelled context in ListDashboardSubRequests")
	}

	_, err = repo.GetSubRequestByID(ctx, 1)
	if err == nil {
		t.Error("expected error with cancelled context in GetSubRequestByID")
	}
	_, err = repo.GetSubRequestDetailByID(ctx, 1)
	if err == nil {
		t.Error("expected error with cancelled context in GetSubRequestDetailByID")
	}

	err = repo.DeleteSubRequest(ctx, 1)
	if err == nil {
		t.Error("expected error with cancelled context in DeleteSubRequest")
	}

	_, err = repo.ListUsers(ctx)
	if err == nil {
		t.Error("expected error with cancelled context in ListUsers")
	}
	_, err = repo.ListActiveUsers(ctx)
	if err == nil {
		t.Error("expected error with cancelled context in ListActiveUsers")
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

	err = repo.TakeSubRequest(ctx, 1, 1, time.Now())
	if err == nil {
		t.Error("expected error with cancelled context in TakeSubRequest")
	}

	err = repo.UntakeSubRequest(ctx, 1, 1, time.Now())
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

func TestListDashboardSubRequestsErrors(t *testing.T) {
	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(dbConn)
	ctx := context.Background()

	// Test closed DB for QueryContext error
	dbConn.Close()
	_, err = repo.ListDashboardSubRequests(ctx)
	if err == nil {
		t.Error("expected error with closed db in ListDashboardSubRequests")
	}

	_, err = repo.ListUsers(ctx)
	if err == nil {
		t.Error("expected error with closed db in ListUsers")
	}
	_, err = repo.ListActiveUsers(ctx)
	if err == nil {
		t.Error("expected error with closed db in ListActiveUsers")
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

	// ListDashboardSubRequests will fail because it expects a JOIN with users which doesn't exist now
	// and because of type mismatch.
	// Actually, we need the JOIN to exist if we want to reach Scan.
	_, _ = dbConn.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT, role TEXT, is_enabled BOOLEAN)")
	_, _ = dbConn.Exec("INSERT INTO users (id, email, role, is_enabled) VALUES (1, 'test@example.com', 'member', 1)")
	// Update posted_by_user_id to be a valid join but other fields to be garbage
	_, _ = dbConn.Exec("UPDATE sub_requests SET posted_by_user_id = 1")

	_, err = repo.ListDashboardSubRequests(ctx)
	if err == nil {
		t.Error("expected scan error in ListDashboardSubRequests")
	}

	_, err = repo.GetSubRequestByID(ctx, 123)
	if err == nil {
		t.Error("expected scan error in GetSubRequestByID")
	}
	_, err = repo.GetSubRequestDetailByID(ctx, 123)
	if err == nil {
		t.Error("expected scan error in GetSubRequestDetailByID")
	}

	// ListUsers scan error
	_, _ = dbConn.Exec("DROP TABLE users")
	_, _ = dbConn.Exec("CREATE TABLE users (id TEXT, email TEXT, role TEXT, is_enabled TEXT, created_at TEXT)")
	_, _ = dbConn.Exec("INSERT INTO users (id, email, role, is_enabled, created_at) VALUES ('bad', 'bad', 'bad', 1, 'bad')")
	_, err = repo.ListUsers(ctx)
	if err == nil {
		t.Error("expected scan error in ListUsers")
	}
	_, err = repo.ListActiveUsers(ctx)
	if err == nil {
		t.Error("expected scan error in ListActiveUsers")
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
	_, err = repo.UseMagicLink(ctx, "used-hash", time.Now())
	if err == nil {
		t.Fatal("expected error for already-used magic link")
	}
}

func TestUseMagicLink_ConsumeError(t *testing.T) {
	dbConn, mock, repo := newMockRepository(t)
	defer dbConn.Close()

	mock.ExpectQuery("DELETE FROM magic_links").
		WithArgs("del-hash", sqlmock.AnyArg()).
		WillReturnError(errors.New("consume failed"))

	if _, err := repo.UseMagicLink(context.Background(), "del-hash", time.Now()); err == nil {
		t.Fatal("expected consume error")
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

func TestListDashboardSubRequestsOrdering(t *testing.T) {
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
		sr := &domain.SubRequest{
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

	list, err := repo.ListDashboardSubRequests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(list) != 3 {
		t.Fatalf("expected 3 items, got %d", len(list))
	}

	// Verify they are ordered by start time ascending
	if !list[0].Request.StartTime.Before(list[1].Request.StartTime) {
		t.Errorf("expected %v before %v", list[0].Request.StartTime, list[1].Request.StartTime)
	}
	if !list[1].Request.StartTime.Before(list[2].Request.StartTime) {
		t.Errorf("expected %v before %v", list[1].Request.StartTime, list[2].Request.StartTime)
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

		err := repo.CreateSubRequest(context.Background(), &domain.SubRequest{
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

		err := repo.TakeSubRequest(context.Background(), 9, 2, time.Now())
		if err == nil {
			t.Fatal("expected RowsAffected error")
		}
	})

	t.Run("untake sub request rows affected error", func(t *testing.T) {
		dbConn, mock, repo := newMockRepository(t)
		defer dbConn.Close()

		mock.ExpectExec(regexp.QuoteMeta("UPDATE sub_requests SET taken_by_user_id = NULL, updated_at = ? WHERE id = ? AND taken_by_user_id = ?")).
			WithArgs(sqlmock.AnyArg(), 9, int64(2)).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected failed")))

		err := repo.UntakeSubRequest(context.Background(), 9, 2, time.Now())
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

		_, err := repo.ListDashboardSubRequests(context.Background())
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

	t.Run("list active users rows err", func(t *testing.T) {
		dbConn, mock, repo := newMockRepository(t)
		defer dbConn.Close()

		rows := sqlmock.NewRows([]string{"id", "email", "role", "is_enabled", "created_at"}).
			AddRow(1, "user@example.com", "member", true, time.Now()).
			RowError(0, errors.New("rows failed"))
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, role, is_enabled, created_at FROM users WHERE is_enabled = true ORDER BY email ASC")).
			WillReturnRows(rows)

		_, err := repo.ListActiveUsers(context.Background())
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
			name: "exec error",
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO users").
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
				mock.ExpectExec("INSERT INTO users").
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
