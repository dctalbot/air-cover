package db

import (
	"context"
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
	u, err := repo.CreateUser(ctx, "test@example.com")
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
		ID:        "sr1",
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
}
