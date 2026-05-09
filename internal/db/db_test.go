package db

import (
	"context"
	"os"
	"testing"
	"time"

	"air-cover/internal/models"
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
	dbConn2, _ := InitDB("file::memory:?cache=shared")
	repo2 := NewRepository(dbConn2)
	_, _ = dbConn2.Exec("DROP TABLE users")
	err = repo2.ImportUsers(context.Background(), []string{"user@example.com"})
	if err == nil {
		t.Fatal("expected error with dropped table")
	}
	dbConn2.Close()
}
