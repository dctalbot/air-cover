package api

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

type testServerRepository interface {
	authapp.Repository
	adminapp.Repository
	subrequestsapp.Repository
}

type testCatalog interface {
	adminapp.Catalog
	subrequestsapp.Catalog
}

func newTestServer(repo testServerRepository, auth *AuthHandler, catalog testCatalog) *Server {
	var subRequests subRequestService
	var admin adminService
	if auth == nil && repo != nil {
		auth = NewAuthHandler(authapp.NewService(repo, nil))
	}
	if repo != nil {
		subRequests = subrequestsapp.NewService(repo, catalog)
		admin = adminapp.NewService(repo, catalog)
	}
	return NewServer(auth, subRequests, admin)
}

func setupTestDB(t *testing.T) *testRepository {
	t.Helper()

	return newTestRepository()
}

var errTestRepositoryClosed = errors.New("test repository is closed")

type testRepository struct {
	nextUserID       int
	nextMagicLinkID  int
	nextSubRequestID int
	users            map[int]*domain.User
	magicLinks       map[string]*domain.MagicLink
	sessions         map[string]*domain.Session
	subRequests      map[int]*domain.SubRequest
	closed           bool
	failMagicLinks   bool
	failSessions     bool
	failUsers        bool
}

func newTestRepository() *testRepository {
	return &testRepository{
		nextUserID:       1,
		nextMagicLinkID:  1,
		nextSubRequestID: 1,
		users:            make(map[int]*domain.User),
		magicLinks:       make(map[string]*domain.MagicLink),
		sessions:         make(map[string]*domain.Session),
		subRequests:      make(map[int]*domain.SubRequest),
	}
}

func (r *testRepository) DB() *testDB {
	return &testDB{repo: r}
}

func (r *testRepository) checkAvailable() error {
	if r.closed {
		return errTestRepositoryClosed
	}
	return nil
}

func (r *testRepository) ListUsers(ctx context.Context) ([]*domain.User, error) {
	if err := r.checkAvailable(); err != nil {
		return nil, err
	}
	if r.failUsers {
		return nil, errors.New("users table unavailable")
	}
	users := make([]*domain.User, 0, len(r.users))
	for _, user := range r.users {
		users = append(users, cloneUser(user))
	}
	return users, nil
}

func (r *testRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	if err := r.checkAvailable(); err != nil {
		return nil, err
	}
	if r.failUsers {
		return nil, errors.New("users table unavailable")
	}
	for _, user := range r.users {
		if user.Email == email {
			return cloneUser(user), nil
		}
	}
	return nil, apperrors.ErrNotFound
}

func (r *testRepository) GetUserByID(ctx context.Context, id int) (*domain.User, error) {
	if err := r.checkAvailable(); err != nil {
		return nil, err
	}
	if r.failUsers {
		return nil, errors.New("users table unavailable")
	}
	user, ok := r.users[id]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	return cloneUser(user), nil
}

func (r *testRepository) CreateUser(ctx context.Context, email string, role string) (*domain.User, error) {
	if err := r.checkAvailable(); err != nil {
		return nil, err
	}
	if r.failUsers {
		return nil, errors.New("users table unavailable")
	}
	if _, err := r.GetUserByEmail(ctx, email); err == nil {
		return nil, errors.New("duplicate user")
	}
	user := &domain.User{
		ID:        r.nextUserID,
		Email:     email,
		Role:      domain.Role(role),
		IsEnabled: true,
		CreatedAt: time.Now(),
	}
	r.nextUserID++
	r.users[user.ID] = cloneUser(user)
	return cloneUser(user), nil
}

func (r *testRepository) UpdateUser(ctx context.Context, id int, role *string, isEnabled *bool) error {
	if err := r.checkAvailable(); err != nil {
		return err
	}
	if r.failUsers {
		return errors.New("users table unavailable")
	}
	user, ok := r.users[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	if role != nil {
		user.Role = domain.Role(*role)
	}
	if isEnabled != nil {
		user.IsEnabled = *isEnabled
	}
	return nil
}

func (r *testRepository) ImportUsers(ctx context.Context, emails []string) error {
	if err := r.checkAvailable(); err != nil {
		return err
	}
	if r.failUsers {
		return errors.New("users table unavailable")
	}
	for _, email := range emails {
		if _, err := r.GetUserByEmail(ctx, email); errors.Is(err, apperrors.ErrNotFound) {
			if _, err := r.CreateUser(ctx, email, string(domain.RoleMember)); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

func (r *testRepository) CreateMagicLink(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error {
	if err := r.checkAvailable(); err != nil {
		return err
	}
	if r.failMagicLinks {
		return errors.New("magic links table unavailable")
	}
	r.magicLinks[tokenHash] = &domain.MagicLink{
		ID:        r.nextMagicLinkID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	}
	r.nextMagicLinkID++
	return nil
}

func (r *testRepository) UseMagicLink(ctx context.Context, tokenHash string, now time.Time) (*domain.MagicLink, error) {
	if err := r.checkAvailable(); err != nil {
		return nil, err
	}
	if r.failMagicLinks {
		return nil, errors.New("magic links table unavailable")
	}
	link, ok := r.magicLinks[tokenHash]
	if !ok || !link.ExpiresAt.After(now) {
		return nil, apperrors.ErrNotFound
	}
	delete(r.magicLinks, tokenHash)
	return cloneMagicLink(link), nil
}

func (r *testRepository) CreateSession(ctx context.Context, sessionID, sessionTokenHash string, userID int, expiresAt time.Time) error {
	if err := r.checkAvailable(); err != nil {
		return err
	}
	if r.failSessions {
		return errors.New("sessions table unavailable")
	}
	r.sessions[sessionTokenHash] = &domain.Session{
		ID:        sessionID,
		UserID:    userID,
		TokenHash: sessionTokenHash,
		ExpiresAt: expiresAt,
	}
	return nil
}

func (r *testRepository) GetSessionByToken(ctx context.Context, sessionTokenHash string, now time.Time) (*domain.Session, error) {
	if err := r.checkAvailable(); err != nil {
		return nil, err
	}
	if r.failSessions {
		return nil, errors.New("sessions table unavailable")
	}
	session, ok := r.sessions[sessionTokenHash]
	if !ok || session.ExpiresAt.Before(now) {
		return nil, apperrors.ErrNotFound
	}
	return cloneSession(session), nil
}

func (r *testRepository) DeleteSessionsByUserID(ctx context.Context, userID int) error {
	if err := r.checkAvailable(); err != nil {
		return err
	}
	if r.failSessions {
		return errors.New("sessions table unavailable")
	}
	for hash, session := range r.sessions {
		if session.UserID == userID {
			delete(r.sessions, hash)
		}
	}
	return nil
}

func (r *testRepository) CreateSubRequest(ctx context.Context, sr *domain.SubRequest) error {
	if err := r.checkAvailable(); err != nil {
		return err
	}
	clone := cloneSubRequest(sr)
	clone.ID = r.nextSubRequestID
	r.nextSubRequestID++
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = time.Now()
	}
	if clone.UpdatedAt.IsZero() {
		clone.UpdatedAt = clone.CreatedAt
	}
	r.subRequests[clone.ID] = clone
	sr.ID = clone.ID
	return nil
}

func (r *testRepository) ListDashboardSubRequests(ctx context.Context) ([]subrequestsapp.DashboardReadModel, error) {
	if err := r.checkAvailable(); err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(r.subRequests))
	for id := range r.subRequests {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	models := make([]subrequestsapp.DashboardReadModel, 0, len(ids))
	for _, id := range ids {
		sr := r.subRequests[id]
		models = append(models, subrequestsapp.DashboardReadModel{
			Request:        cloneSubRequest(sr),
			RequesterEmail: r.emailForUser(sr.PostedByUserID),
			TakerEmail:     r.emailForOptionalUser(sr.TakenByUserID),
		})
	}
	return models, nil
}

func (r *testRepository) GetSubRequestDetailByID(ctx context.Context, id int) (subrequestsapp.DetailReadModel, error) {
	if err := r.checkAvailable(); err != nil {
		return subrequestsapp.DetailReadModel{}, err
	}
	sr, ok := r.subRequests[id]
	if !ok {
		return subrequestsapp.DetailReadModel{}, apperrors.ErrNotFound
	}
	return subrequestsapp.DetailReadModel{
		Request:        cloneSubRequest(sr),
		RequesterEmail: r.emailForUser(sr.PostedByUserID),
		TakerEmail:     r.emailForOptionalUser(sr.TakenByUserID),
	}, nil
}

func (r *testRepository) GetSubRequestByID(ctx context.Context, id int) (*domain.SubRequest, error) {
	if err := r.checkAvailable(); err != nil {
		return nil, err
	}
	sr, ok := r.subRequests[id]
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	return cloneSubRequest(sr), nil
}

func (r *testRepository) DeleteSubRequest(ctx context.Context, id int) error {
	if err := r.checkAvailable(); err != nil {
		return err
	}
	if _, ok := r.subRequests[id]; !ok {
		return apperrors.ErrNotFound
	}
	delete(r.subRequests, id)
	return nil
}

func (r *testRepository) TakeSubRequest(ctx context.Context, id int, userID int, updatedAt time.Time) error {
	if err := r.checkAvailable(); err != nil {
		return err
	}
	sr, ok := r.subRequests[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	if sr.TakenByUserID != nil {
		return apperrors.ErrConflict
	}
	takerID := userID
	sr.TakenByUserID = &takerID
	sr.UpdatedAt = updatedAt
	return nil
}

func (r *testRepository) UntakeSubRequest(ctx context.Context, id int, userID int, updatedAt time.Time) error {
	if err := r.checkAvailable(); err != nil {
		return err
	}
	sr, ok := r.subRequests[id]
	if !ok {
		return apperrors.ErrNotFound
	}
	if sr.TakenByUserID == nil || *sr.TakenByUserID != userID {
		return apperrors.ErrForbidden
	}
	sr.TakenByUserID = nil
	sr.UpdatedAt = updatedAt
	return nil
}

func (r *testRepository) emailForUser(id int) string {
	if user, ok := r.users[id]; ok {
		return user.Email
	}
	return ""
}

func (r *testRepository) emailForOptionalUser(id *int) string {
	if id == nil {
		return ""
	}
	return r.emailForUser(*id)
}

type testDB struct {
	repo *testRepository
}

func (db *testDB) Close() error {
	db.repo.closed = true
	return nil
}

func (db *testDB) Exec(query string, args ...any) (testResult, error) {
	switch query {
	case "UPDATE users SET is_enabled = 0 WHERE id = ?":
		id, ok := args[0].(int)
		if !ok {
			return testResult{}, nil
		}
		enabled := false
		return testResult{}, db.repo.UpdateUser(context.Background(), id, nil, &enabled)
	case "DELETE FROM users WHERE id = ?":
		id, ok := args[0].(int)
		if ok {
			delete(db.repo.users, id)
		}
	case "DROP TABLE magic_links":
		db.repo.failMagicLinks = true
	case "DROP TABLE sessions":
		db.repo.failSessions = true
	case "DROP TABLE users":
		db.repo.failUsers = true
	}
	return testResult{}, nil
}

type testResult struct{}

func cloneUser(user *domain.User) *domain.User {
	if user == nil {
		return nil
	}
	clone := *user
	return &clone
}

func cloneMagicLink(link *domain.MagicLink) *domain.MagicLink {
	if link == nil {
		return nil
	}
	clone := *link
	return &clone
}

func cloneSession(session *domain.Session) *domain.Session {
	if session == nil {
		return nil
	}
	clone := *session
	return &clone
}

func cloneSubRequest(sr *domain.SubRequest) *domain.SubRequest {
	if sr == nil {
		return nil
	}
	clone := *sr
	if sr.TakenByUserID != nil {
		takerID := *sr.TakenByUserID
		clone.TakenByUserID = &takerID
	}
	if sr.SubstituteID != nil {
		substituteID := *sr.SubstituteID
		clone.SubstituteID = &substituteID
	}
	return &clone
}
