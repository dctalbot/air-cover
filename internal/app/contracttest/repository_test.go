package contracttest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

type memoryRepository struct {
	users          []*domain.User
	magicLinks     map[string]*domain.MagicLink
	sessions       map[string]*domain.Session
	subRequests    []*domain.SubRequest
	nextUserID     int
	nextRequestID  int
	takenRequestID int
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		magicLinks:    make(map[string]*domain.MagicLink),
		sessions:      make(map[string]*domain.Session),
		nextUserID:    1,
		nextRequestID: 1,
	}
}

func (r *memoryRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	for _, user := range r.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, apperrors.ErrNotFound
}

func (r *memoryRepository) GetUserByID(ctx context.Context, id int) (*domain.User, error) {
	for _, user := range r.users {
		if user.ID == id {
			return user, nil
		}
	}
	return nil, apperrors.ErrNotFound
}

func (r *memoryRepository) CreateUser(ctx context.Context, email string, role string) (*domain.User, error) {
	user := &domain.User{ID: r.nextUserID, Email: email, Role: role, IsEnabled: true}
	r.nextUserID++
	r.users = append(r.users, user)
	return user, nil
}

func (r *memoryRepository) UpdateUser(ctx context.Context, id int, role *string, isEnabled *bool) error {
	user, err := r.GetUserByID(ctx, id)
	if err != nil {
		return err
	}
	if role != nil {
		user.Role = *role
	}
	if isEnabled != nil {
		user.IsEnabled = *isEnabled
	}
	return nil
}

func (r *memoryRepository) ImportUsers(ctx context.Context, emails []string) error {
	for _, email := range emails {
		r.users = append(r.users, &domain.User{ID: r.nextUserID, Email: email, Role: "member", IsEnabled: true})
		r.nextUserID++
	}
	return nil
}

func (r *memoryRepository) ListUsers(ctx context.Context) ([]*domain.User, error) {
	return r.users, nil
}

func (r *memoryRepository) CreateMagicLink(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error {
	r.magicLinks[tokenHash] = &domain.MagicLink{UserID: userID, TokenHash: tokenHash, ExpiresAt: expiresAt}
	return nil
}

func (r *memoryRepository) UseMagicLink(ctx context.Context, tokenHash string) (*domain.MagicLink, error) {
	link, ok := r.magicLinks[tokenHash]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	delete(r.magicLinks, tokenHash)
	return link, nil
}

func (r *memoryRepository) CreateSession(ctx context.Context, sessionID, sessionToken string, userID int, expiresAt time.Time) error {
	r.sessions[sessionToken] = &domain.Session{ID: sessionID, SessionToken: sessionToken, UserID: userID, ExpiresAt: expiresAt}
	return nil
}

func (r *memoryRepository) GetSessionByToken(ctx context.Context, sessionToken string) (*domain.Session, error) {
	session, ok := r.sessions[sessionToken]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	return session, nil
}

func (r *memoryRepository) DeleteSessionsByUserID(ctx context.Context, userID int) error {
	for token, session := range r.sessions {
		if session.UserID == userID {
			delete(r.sessions, token)
		}
	}
	return nil
}

func (r *memoryRepository) ListSubRequests(ctx context.Context) ([]*domain.SubRequest, error) {
	return r.subRequests, nil
}

func (r *memoryRepository) CreateSubRequest(ctx context.Context, request *domain.SubRequest) error {
	request.ID = r.nextRequestID
	r.nextRequestID++
	r.subRequests = append(r.subRequests, request)
	return nil
}

func (r *memoryRepository) GetSubRequestByID(ctx context.Context, id int) (*domain.SubRequest, error) {
	for _, request := range r.subRequests {
		if request.ID == id {
			return request, nil
		}
	}
	return nil, apperrors.ErrNotFound
}

func (r *memoryRepository) DeleteSubRequest(ctx context.Context, id int) error {
	for i, request := range r.subRequests {
		if request.ID == id {
			r.subRequests = append(r.subRequests[:i], r.subRequests[i+1:]...)
			return nil
		}
	}
	return apperrors.ErrNotFound
}

func (r *memoryRepository) TakeSubRequest(ctx context.Context, id int, userID int) error {
	request, err := r.GetSubRequestByID(ctx, id)
	if err != nil {
		return err
	}
	if request.TakenByUserID != nil {
		return apperrors.ErrConflict
	}
	request.TakenByUserID = &userID
	r.takenRequestID = id
	return nil
}

func (r *memoryRepository) UntakeSubRequest(ctx context.Context, id int) error {
	request, err := r.GetSubRequestByID(ctx, id)
	if err != nil {
		return err
	}
	request.TakenByUserID = nil
	return nil
}

func TestCheckRepository(t *testing.T) {
	var ctx context.Context
	if err := CheckRepository(ctx, newMemoryRepository()); err != nil {
		t.Fatalf("CheckRepository returned error: %v", err)
	}
}

func TestCheckRepositoryReportsContractViolation(t *testing.T) {
	repo := newMemoryRepository()
	repo.nextUserID = 0

	err := CheckRepository(t.Context(), repo)
	if err == nil {
		t.Fatal("expected contract violation")
	}
	if !strings.Contains(err.Error(), "unexpected created user") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMustNoErrPanics(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected panic")
		}
	}()

	mustNoErr(errors.New("boom"), "operation failed")
}
