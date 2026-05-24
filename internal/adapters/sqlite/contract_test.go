package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"air-cover/internal/app"
	"air-cover/internal/apperrors"
	"air-cover/internal/models"
)

func TestRepositoryContracts(t *testing.T) {
	dbConn, err := InitDB("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()

	repo := NewRepository(dbConn)
	assertRepositoryContract(t, repo)
}

func assertRepositoryContract(t *testing.T, repo app.Repository) {
	t.Helper()
	ctx := context.Background()

	user, err := repo.CreateUser(ctx, "contract@example.com", "member")
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	if user.ID == 0 || user.Email != "contract@example.com" || !user.IsEnabled {
		t.Fatalf("unexpected created user: %+v", user)
	}

	if _, err := repo.GetUserByEmail(ctx, user.Email); err != nil {
		t.Fatalf("GetUserByEmail returned error: %v", err)
	}
	if _, err := repo.GetUserByID(ctx, user.ID); err != nil {
		t.Fatalf("GetUserByID returned error: %v", err)
	}
	if _, err := repo.GetUserByEmail(ctx, "missing@example.com"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("missing user error = %v, want not found", err)
	}

	rawHash := "contract-hash"
	if err := repo.CreateMagicLink(ctx, user.ID, rawHash, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateMagicLink returned error: %v", err)
	}
	if link, err := repo.UseMagicLink(ctx, rawHash); err != nil || link.UserID != user.ID {
		t.Fatalf("UseMagicLink = %+v, %v; want user %d", link, err, user.ID)
	}
	if _, err := repo.UseMagicLink(ctx, rawHash); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reused magic link error = %v, want not found", err)
	}

	if err := repo.CreateSession(ctx, "contract-session", "contract-token", user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if session, err := repo.GetSessionByToken(ctx, "contract-token"); err != nil || session.UserID != user.ID {
		t.Fatalf("GetSessionByToken = %+v, %v; want user %d", session, err, user.ID)
	}
	if err := repo.DeleteSessionsByUserID(ctx, user.ID); err != nil {
		t.Fatalf("DeleteSessionsByUserID returned error: %v", err)
	}
	if _, err := repo.GetSessionByToken(ctx, "contract-token"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("deleted session error = %v, want not found", err)
	}

	request := &models.SubRequest{
		ShowID:         7,
		PostedByUserID: user.ID,
		StartTime:      time.Now().Add(time.Hour),
		EndTime:        time.Now().Add(2 * time.Hour),
		Notes:          "contract",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := repo.CreateSubRequest(ctx, request); err != nil {
		t.Fatalf("CreateSubRequest returned error: %v", err)
	}
	if request.ID == 0 {
		t.Fatal("expected CreateSubRequest to assign ID")
	}
	if _, err := repo.GetSubRequestByID(ctx, request.ID); err != nil {
		t.Fatalf("GetSubRequestByID returned error: %v", err)
	}
	if requests, err := repo.ListSubRequests(ctx); err != nil || len(requests) == 0 {
		t.Fatalf("ListSubRequests = %d items, %v; want at least one", len(requests), err)
	}
	if err := repo.TakeSubRequest(ctx, request.ID, user.ID); err != nil {
		t.Fatalf("TakeSubRequest returned error: %v", err)
	}
	if err := repo.TakeSubRequest(ctx, request.ID, user.ID); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("second take error = %v, want conflict", err)
	}
	if err := repo.UntakeSubRequest(ctx, request.ID); err != nil {
		t.Fatalf("UntakeSubRequest returned error: %v", err)
	}
	if err := repo.DeleteSubRequest(ctx, request.ID); err != nil {
		t.Fatalf("DeleteSubRequest returned error: %v", err)
	}
	if err := repo.DeleteSubRequest(ctx, request.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("second delete error = %v, want not found", err)
	}

	role := "admin"
	enabled := false
	if err := repo.UpdateUser(ctx, user.ID, &role, &enabled); err != nil {
		t.Fatalf("UpdateUser returned error: %v", err)
	}
	if err := repo.ImportUsers(ctx, []string{"imported@example.com"}); err != nil {
		t.Fatalf("ImportUsers returned error: %v", err)
	}
	if users, err := repo.ListUsers(ctx); err != nil || len(users) < 2 {
		t.Fatalf("ListUsers = %d users, %v; want at least two", len(users), err)
	}
}
