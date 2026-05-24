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
	subrequestsapp.CommandRepository
	subrequestsapp.DashboardQuery
}

type SubRequestCommandRepository interface {
	adminapp.Repository
	subrequestsapp.CommandRepository
}

type SubRequestDashboardQuery interface {
	adminapp.Repository
	subrequestsapp.CommandRepository
	subrequestsapp.DashboardQuery
}

func CheckAuthRepository(ctx context.Context, repo Repository) (err error) {
	return check(ctx, func(ctx context.Context) {
		checkAuthRepository(ctx, repo)
	})
}

func CheckAdminRepository(ctx context.Context, repo Repository) (err error) {
	return check(ctx, func(ctx context.Context) {
		checkAdminRepository(ctx, repo)
	})
}

func CheckSubRequestRepository(ctx context.Context, repo Repository) (err error) {
	return check(ctx, func(ctx context.Context) {
		checkSubRequestRepository(ctx, repo)
	})
}

func CheckSubRequestCommandRepository(ctx context.Context, repo SubRequestCommandRepository) (err error) {
	return check(ctx, func(ctx context.Context) {
		checkSubRequestCommandRepository(ctx, repo)
	})
}

func CheckSubRequestDashboardQuery(ctx context.Context, repo SubRequestDashboardQuery) (err error) {
	return check(ctx, func(ctx context.Context) {
		checkSubRequestDashboardQuery(ctx, repo)
	})
}

func CheckRepository(ctx context.Context, repo Repository) (err error) {
	return check(ctx, func(ctx context.Context) {
		checkRepository(ctx, repo)
	})
}

func check(ctx context.Context, run func(context.Context)) (err error) {
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

	run(ctx)
	return nil
}

func checkRepository(ctx context.Context, repo Repository) {
	checkAuthRepository(ctx, repo)
	checkAdminRepository(ctx, repo)
	checkSubRequestRepository(ctx, repo)
}

func checkAuthRepository(ctx context.Context, repo Repository) {
	user, err := repo.CreateUser(ctx, "contract-auth@example.com", "member")
	mustNoErr(err, "CreateUser returned error")
	must(user.ID != 0 && user.Email == "contract-auth@example.com" && user.IsEnabled, "unexpected created user: %+v", user)

	byEmail, err := repo.GetUserByEmail(ctx, user.Email)
	must(err == nil && byEmail.ID == user.ID, "GetUserByEmail = %+v, %v; want user %d", byEmail, err, user.ID)
	byID, err := repo.GetUserByID(ctx, user.ID)
	must(err == nil && byID.Email == user.Email, "GetUserByID = %+v, %v; want email %s", byID, err, user.Email)
	_, err = repo.GetUserByEmail(ctx, "missing@example.com")
	must(errors.Is(err, apperrors.ErrNotFound), "missing user error = %v, want not found", err)
	_, err = repo.GetUserByID(ctx, -1)
	must(errors.Is(err, apperrors.ErrNotFound), "missing user by ID error = %v, want not found", err)

	rawHash := "contract-auth-hash"
	now := time.Now().Truncate(time.Second)
	err = repo.CreateMagicLink(ctx, user.ID, rawHash, now.Add(time.Hour))
	mustNoErr(err, "CreateMagicLink returned error")
	link, err := repo.UseMagicLink(ctx, rawHash, now)
	must(err == nil && link.UserID == user.ID, "UseMagicLink = %+v, %v; want user %d", link, err, user.ID)
	_, err = repo.UseMagicLink(ctx, rawHash, now)
	must(errors.Is(err, apperrors.ErrNotFound), "reused magic link error = %v, want not found", err)

	expiredHash := "contract-auth-expired-hash"
	err = repo.CreateMagicLink(ctx, user.ID, expiredHash, now.Add(-time.Minute))
	mustNoErr(err, "CreateMagicLink expired returned error")
	_, err = repo.UseMagicLink(ctx, expiredHash, now)
	must(errors.Is(err, apperrors.ErrNotFound), "expired magic link error = %v, want not found", err)

	err = repo.CreateSession(ctx, "contract-auth-session", "contract-auth-token", user.ID, now.Add(time.Hour))
	mustNoErr(err, "CreateSession returned error")
	session, err := repo.GetSessionByToken(ctx, "contract-auth-token", now)
	must(err == nil && session.UserID == user.ID, "GetSessionByToken = %+v, %v; want user %d", session, err, user.ID)
	_, err = repo.GetSessionByToken(ctx, "contract-auth-missing-token", now)
	must(errors.Is(err, apperrors.ErrNotFound), "missing session error = %v, want not found", err)
	err = repo.CreateSession(ctx, "contract-auth-expired-session", "contract-auth-expired-token", user.ID, now.Add(-time.Minute))
	mustNoErr(err, "CreateSession expired returned error")
	_, err = repo.GetSessionByToken(ctx, "contract-auth-expired-token", now)
	must(err != nil, "expired session unexpectedly authenticated")
	err = repo.DeleteSessionsByUserID(ctx, user.ID)
	mustNoErr(err, "DeleteSessionsByUserID returned error")
	_, err = repo.GetSessionByToken(ctx, "contract-auth-token", now)
	must(errors.Is(err, apperrors.ErrNotFound), "deleted session error = %v, want not found", err)

	enabled := false
	err = repo.UpdateUser(ctx, user.ID, nil, &enabled)
	mustNoErr(err, "UpdateUser disable returned error")
	disabled, err := repo.GetUserByID(ctx, user.ID)
	must(err == nil && !disabled.IsEnabled, "disabled user = %+v, %v; want disabled", disabled, err)
}

func checkAdminRepository(ctx context.Context, repo Repository) {
	user, err := repo.CreateUser(ctx, "contract-admin@example.com", "member")
	mustNoErr(err, "CreateUser returned error")
	must(user.ID != 0 && user.Email == "contract-admin@example.com" && user.Role == domain.RoleMember && user.IsEnabled, "unexpected created admin test user: %+v", user)

	err = repo.UpdateUser(ctx, -1, nil, boolPtr(true))
	must(errors.Is(err, apperrors.ErrNotFound), "missing user update error = %v, want not found", err)
	err = repo.UpdateUser(ctx, user.ID, nil, nil)
	mustNoErr(err, "UpdateUser nil update returned error")
	unchanged, err := repo.GetUserByID(ctx, user.ID)
	must(err == nil && unchanged.Role == domain.RoleMember && unchanged.IsEnabled, "nil update changed user: %+v, %v", unchanged, err)

	role := "admin"
	enabled := false
	err = repo.UpdateUser(ctx, user.ID, &role, &enabled)
	mustNoErr(err, "UpdateUser returned error")
	updated, err := repo.GetUserByID(ctx, user.ID)
	must(err == nil && updated.Role == domain.RoleAdmin && !updated.IsEnabled, "full update user = %+v, %v; want admin disabled", updated, err)

	member := "member"
	err = repo.UpdateUser(ctx, user.ID, &member, nil)
	mustNoErr(err, "UpdateUser role-only returned error")
	updated, err = repo.GetUserByID(ctx, user.ID)
	must(err == nil && updated.Role == domain.RoleMember && !updated.IsEnabled, "role-only update user = %+v, %v; want member disabled", updated, err)

	err = repo.UpdateUser(ctx, user.ID, nil, boolPtr(true))
	mustNoErr(err, "UpdateUser enabled-only returned error")
	updated, err = repo.GetUserByID(ctx, user.ID)
	must(err == nil && updated.Role == domain.RoleMember && updated.IsEnabled, "enabled-only update user = %+v, %v; want member enabled", updated, err)

	err = repo.ImportUsers(ctx, []string{"contract-imported@example.com", "contract-imported@example.com"})
	mustNoErr(err, "ImportUsers returned error")
	users, err := repo.ListUsers(ctx)
	mustNoErr(err, "ListUsers returned error")
	importedCount := 0
	for _, listed := range users {
		if listed.Email == "contract-imported@example.com" {
			importedCount++
			must(listed.Role == domain.RoleMember && listed.IsEnabled, "imported user = %+v; want enabled member", listed)
		}
	}
	must(importedCount == 1, "imported user count = %d, want 1", importedCount)
}

func checkSubRequestRepository(ctx context.Context, repo Repository) {
	checkSubRequestCommandRepository(ctx, repo)
	checkSubRequestDashboardQuery(ctx, repo)
}

func checkSubRequestCommandRepository(ctx context.Context, repo SubRequestCommandRepository) {
	requester, err := repo.CreateUser(ctx, "contract-requester@example.com", "member")
	mustNoErr(err, "CreateUser requester returned error")
	taker, err := repo.CreateUser(ctx, "contract-taker@example.com", "member")
	mustNoErr(err, "CreateUser taker returned error")

	now := time.Now().Truncate(time.Second)
	late := &domain.SubRequest{
		ShowID:         7,
		PostedByUserID: requester.ID,
		StartTime:      now.Add(3 * time.Hour),
		EndTime:        now.Add(4 * time.Hour),
		Notes:          "late contract",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	early := &domain.SubRequest{
		ShowID:         8,
		PostedByUserID: requester.ID,
		StartTime:      now.Add(time.Hour),
		EndTime:        now.Add(2 * time.Hour),
		Notes:          "early contract",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	err = repo.CreateSubRequest(ctx, late)
	mustNoErr(err, "CreateSubRequest late returned error")
	err = repo.CreateSubRequest(ctx, early)
	mustNoErr(err, "CreateSubRequest early returned error")
	must(late.ID != 0 && early.ID != 0 && late.ID != early.ID, "expected assigned distinct IDs, got late=%d early=%d", late.ID, early.ID)

	fetched, err := repo.GetSubRequestByID(ctx, early.ID)
	must(err == nil && fetched.ID == early.ID && fetched.Notes == early.Notes, "GetSubRequestByID = %+v, %v; want early request", fetched, err)
	_, err = repo.GetSubRequestByID(ctx, -1)
	must(errors.Is(err, apperrors.ErrNotFound), "missing subrequest error = %v, want not found", err)

	takenAt := now.Add(30 * time.Minute)
	err = repo.TakeSubRequest(ctx, early.ID, taker.ID, takenAt)
	mustNoErr(err, "TakeSubRequest returned error")
	err = repo.TakeSubRequest(ctx, early.ID, taker.ID, takenAt)
	must(errors.Is(err, apperrors.ErrConflict), "second take error = %v, want conflict", err)
	err = repo.TakeSubRequest(ctx, -1, taker.ID, takenAt)
	must(errors.Is(err, apperrors.ErrNotFound), "missing take error = %v, want not found", err)

	taken, err := repo.GetSubRequestByID(ctx, early.ID)
	must(err == nil && taken.TakenByUserID != nil && *taken.TakenByUserID == taker.ID && taken.UpdatedAt.Equal(takenAt), "taken request = %+v, %v; want taker %d at %v", taken, err, taker.ID, takenAt)

	untakenAt := now.Add(45 * time.Minute)
	err = repo.UntakeSubRequest(ctx, early.ID, untakenAt)
	mustNoErr(err, "UntakeSubRequest returned error")
	err = repo.UntakeSubRequest(ctx, -1, untakenAt)
	must(errors.Is(err, apperrors.ErrNotFound), "missing untake error = %v, want not found", err)
	untaken, err := repo.GetSubRequestByID(ctx, early.ID)
	must(err == nil && untaken.TakenByUserID == nil && untaken.UpdatedAt.Equal(untakenAt), "untaken request = %+v, %v; want no taker at %v", untaken, err, untakenAt)

	err = repo.DeleteSubRequest(ctx, early.ID)
	mustNoErr(err, "DeleteSubRequest returned error")
	_, err = repo.GetSubRequestByID(ctx, early.ID)
	must(errors.Is(err, apperrors.ErrNotFound), "deleted subrequest error = %v, want not found", err)
	err = repo.DeleteSubRequest(ctx, early.ID)
	must(errors.Is(err, apperrors.ErrNotFound), "second delete error = %v, want not found", err)
}

func checkSubRequestDashboardQuery(ctx context.Context, repo SubRequestDashboardQuery) {
	requester, err := repo.CreateUser(ctx, "contract-dashboard-requester@example.com", "member")
	mustNoErr(err, "CreateUser dashboard requester returned error")
	taker, err := repo.CreateUser(ctx, "contract-dashboard-taker@example.com", "member")
	mustNoErr(err, "CreateUser dashboard taker returned error")

	now := time.Now().Truncate(time.Second)
	late := &domain.SubRequest{
		ShowID:         17,
		PostedByUserID: requester.ID,
		StartTime:      now.Add(3 * time.Hour),
		EndTime:        now.Add(4 * time.Hour),
		Notes:          "late dashboard contract",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	early := &domain.SubRequest{
		ShowID:         18,
		PostedByUserID: requester.ID,
		StartTime:      now.Add(time.Hour),
		EndTime:        now.Add(2 * time.Hour),
		Notes:          "early dashboard contract",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	mustNoErr(repo.CreateSubRequest(ctx, late), "CreateSubRequest dashboard late returned error")
	mustNoErr(repo.CreateSubRequest(ctx, early), "CreateSubRequest dashboard early returned error")

	requests, err := repo.ListDashboardSubRequests(ctx)
	mustNoErr(err, "ListDashboardSubRequests returned error")
	summaries := summariesByID(requests)
	must(summaries[early.ID] != nil && summaries[early.ID].RequesterEmail == requester.Email, "early summary = %+v; want requester email %s", summaries[early.ID], requester.Email)
	must(summaries[late.ID] != nil && summaries[late.ID].RequesterEmail == requester.Email, "late summary = %+v; want requester email %s", summaries[late.ID], requester.Email)
	must(indexOfSummary(requests, early.ID) < indexOfSummary(requests, late.ID), "ListDashboardSubRequests is not ordered by start time ascending")

	takenAt := now.Add(30 * time.Minute)
	mustNoErr(repo.TakeSubRequest(ctx, early.ID, taker.ID, takenAt), "TakeSubRequest dashboard returned error")
	requests, err = repo.ListDashboardSubRequests(ctx)
	mustNoErr(err, "ListDashboardSubRequests after take returned error")
	summaries = summariesByID(requests)
	must(summaries[early.ID] != nil && summaries[early.ID].TakerEmail == taker.Email, "taken summary = %+v; want taker email %s", summaries[early.ID], taker.Email)
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

func boolPtr(value bool) *bool {
	return &value
}

func summariesByID(summaries []subrequestsapp.DashboardRecord) map[int]*subrequestsapp.DashboardRecord {
	byID := make(map[int]*subrequestsapp.DashboardRecord, len(summaries))
	for i := range summaries {
		if summaries[i].Request != nil {
			byID[summaries[i].Request.ID] = &summaries[i]
		}
	}
	return byID
}

func indexOfSummary(summaries []subrequestsapp.DashboardRecord, id int) int {
	for index, summary := range summaries {
		if summary.Request != nil && summary.Request.ID == id {
			return index
		}
	}
	return len(summaries)
}
