package contracttest

import (
	"context"
	"errors"
	"fmt"
	"time"

	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

type Repository interface {
	authapp.Repository
	adminapp.Repository
	subrequestsapp.Repository
}

func CheckRepository(ctx context.Context, repo Repository) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		err = recovered.(error)
	}()

	checkRepository(ctx, repo)
	return nil
}

func checkRepository(ctx context.Context, repo Repository) {
	user, err := repo.CreateUser(ctx, "contract@example.com", "member")
	mustNoErr(err, "CreateUser returned error")
	must(user.ID != 0 && user.Email == "contract@example.com" && user.IsEnabled, "unexpected created user: %+v", user)

	_, err = repo.GetUserByEmail(ctx, user.Email)
	mustNoErr(err, "GetUserByEmail returned error")
	_, err = repo.GetUserByID(ctx, user.ID)
	mustNoErr(err, "GetUserByID returned error")
	_, err = repo.GetUserByEmail(ctx, "missing@example.com")
	must(errors.Is(err, apperrors.ErrNotFound), "missing user error = %v, want not found", err)

	rawHash := "contract-hash"
	err = repo.CreateMagicLink(ctx, user.ID, rawHash, time.Now().Add(time.Hour))
	mustNoErr(err, "CreateMagicLink returned error")
	now := time.Now()
	link, err := repo.UseMagicLink(ctx, rawHash, now)
	must(err == nil && link.UserID == user.ID, "UseMagicLink = %+v, %v; want user %d", link, err, user.ID)
	_, err = repo.UseMagicLink(ctx, rawHash, now)
	must(errors.Is(err, apperrors.ErrNotFound), "reused magic link error = %v, want not found", err)

	err = repo.CreateSession(ctx, "contract-session", "contract-token", user.ID, time.Now().Add(time.Hour))
	mustNoErr(err, "CreateSession returned error")
	session, err := repo.GetSessionByToken(ctx, "contract-token", now)
	must(err == nil && session.UserID == user.ID, "GetSessionByToken = %+v, %v; want user %d", session, err, user.ID)
	err = repo.DeleteSessionsByUserID(ctx, user.ID)
	mustNoErr(err, "DeleteSessionsByUserID returned error")
	_, err = repo.GetSessionByToken(ctx, "contract-token", now)
	must(errors.Is(err, apperrors.ErrNotFound), "deleted session error = %v, want not found", err)

	request := &domain.SubRequest{
		ShowID:         7,
		PostedByUserID: user.ID,
		StartTime:      time.Now().Add(time.Hour),
		EndTime:        time.Now().Add(2 * time.Hour),
		Notes:          "contract",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = repo.CreateSubRequest(ctx, request)
	mustNoErr(err, "CreateSubRequest returned error")
	must(request.ID != 0, "expected CreateSubRequest to assign ID")
	_, err = repo.GetSubRequestByID(ctx, request.ID)
	mustNoErr(err, "GetSubRequestByID returned error")
	requests, err := repo.ListSubRequests(ctx)
	must(err == nil && len(requests) != 0, "ListSubRequests = %d items, %v; want at least one", len(requests), err)
	err = repo.TakeSubRequest(ctx, request.ID, user.ID, now)
	mustNoErr(err, "TakeSubRequest returned error")
	err = repo.TakeSubRequest(ctx, request.ID, user.ID, now)
	must(errors.Is(err, apperrors.ErrConflict), "second take error = %v, want conflict", err)
	err = repo.UntakeSubRequest(ctx, request.ID, now)
	mustNoErr(err, "UntakeSubRequest returned error")
	err = repo.DeleteSubRequest(ctx, request.ID)
	mustNoErr(err, "DeleteSubRequest returned error")
	err = repo.DeleteSubRequest(ctx, request.ID)
	must(errors.Is(err, apperrors.ErrNotFound), "second delete error = %v, want not found", err)

	role := "admin"
	enabled := false
	err = repo.UpdateUser(ctx, user.ID, &role, &enabled)
	mustNoErr(err, "UpdateUser returned error")
	err = repo.ImportUsers(ctx, []string{"imported@example.com"})
	mustNoErr(err, "ImportUsers returned error")
	users, err := repo.ListUsers(ctx)
	must(err == nil && len(users) >= 2, "ListUsers = %d users, %v; want at least two", len(users), err)
}

func mustNoErr(err error, message string) {
	if err != nil {
		panic(fmt.Errorf("%s: %w", message, err))
	}
}

func must(ok bool, format string, args ...any) {
	if !ok {
		panic(fmt.Errorf(format, args...))
	}
}
