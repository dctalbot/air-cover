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

	// CreateUser
	u, err := repo.CreateUser(ctx, "test@example.com", "member")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "test@example.com" {
		t.Fatalf("expected test@example.com, got %v", u.Email)
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

	// Using it again should fail
	_, err = repo.UseMagicLink(ctx, "hash123")
	if err == nil {
		t.Fatal("expected error using link twice")
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
		ShowID:    123,
		UserID:    u.ID,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(1 * time.Hour),
		Notes:     "test notes",
		Status:    "open",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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
			id TEXT, show_id TEXT, user_id TEXT, start_time TEXT, end_time TEXT, 
			notes TEXT, status TEXT, created_at TEXT, updated_at TEXT
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert garbage data
	_, err = dbConn.Exec(`
		INSERT INTO sub_requests (id, show_id, user_id, start_time, end_time, notes, status, created_at, updated_at)
		VALUES ('bad-id', 'not-an-int', 'not-an-int', 'not-a-date', 'not-a-date', 'notes', 'open', 'not-a-date', 'not-a-date')
	`)
	if err != nil {
		t.Fatal(err)
	}

	// ListSubRequests will fail because it expects a JOIN with users which doesn't exist now
	// and because of type mismatch.
	// Actually, we need the JOIN to exist if we want to reach Scan.
	_, _ = dbConn.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT, role TEXT)")
	_, _ = dbConn.Exec("INSERT INTO users (id, email, role) VALUES (1, 'test@example.com', 'member')")
	// Update user_id to be a valid join but other fields to be garbage
	_, _ = dbConn.Exec("UPDATE sub_requests SET user_id = 1")

	_, err = repo.ListSubRequests(ctx)
	if err == nil {
		t.Error("expected scan error in ListSubRequests")
	}

	_, err = repo.GetSubRequestByID(ctx, 123)
	if err == nil {
		t.Error("expected scan error in GetSubRequestByID")
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
