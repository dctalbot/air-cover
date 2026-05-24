package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

type fakeRepository struct {
	userByEmail        *domain.User
	userByID           *domain.User
	magicLink          *domain.MagicLink
	session            *domain.Session
	err                error
	getUserIDErr       error
	createMagicLinkErr error

	createdMagicLinkHash string
	createdSessionHash   string
	readSessionHash      string
	deletedUserID        int
	usedMagicLinkAt      time.Time
	readSessionAt        time.Time
}

func (f *fakeRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.userByEmail, nil
}

func (f *fakeRepository) GetUserByID(ctx context.Context, id int) (*domain.User, error) {
	if f.getUserIDErr != nil {
		return nil, f.getUserIDErr
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.userByID, nil
}

func (f *fakeRepository) CreateMagicLink(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error {
	f.createdMagicLinkHash = tokenHash
	if f.createMagicLinkErr != nil {
		return f.createMagicLinkErr
	}
	return f.err
}

func (f *fakeRepository) UseMagicLink(ctx context.Context, tokenHash string, now time.Time) (*domain.MagicLink, error) {
	f.usedMagicLinkAt = now
	if f.err != nil {
		return nil, f.err
	}
	return f.magicLink, nil
}

func (f *fakeRepository) CreateSession(ctx context.Context, sessionID, sessionTokenHash string, userID int, expiresAt time.Time) error {
	f.createdSessionHash = sessionTokenHash
	return f.err
}

func (f *fakeRepository) GetSessionByToken(ctx context.Context, sessionTokenHash string, now time.Time) (*domain.Session, error) {
	f.readSessionHash = sessionTokenHash
	f.readSessionAt = now
	if f.err != nil {
		return nil, f.err
	}
	return f.session, nil
}

func (f *fakeRepository) DeleteSessionsByUserID(ctx context.Context, userID int) error {
	f.deletedUserID = userID
	return f.err
}

type fakeSender struct {
	toEmail   string
	magicLink string
	err       error
}

func (f *fakeSender) SendMagicLink(toEmail, magicLink string) error {
	f.toEmail = toEmail
	f.magicLink = magicLink
	return f.err
}

func TestRequestLogin(t *testing.T) {
	repo := &fakeRepository{userByEmail: &domain.User{ID: 1, Email: "user@example.com", IsEnabled: true}}
	sender := &fakeSender{}
	svc := NewService(repo, sender).WithTokenGenerator(func(n int) (string, error) { return "raw-token", nil })

	result, err := svc.RequestLogin(context.Background(), LoginInput{
		Email:        "user@example.com",
		MagicLinkURL: func(rawToken string) string { return "https://example.test?token=" + rawToken },
	})
	if err != nil {
		t.Fatalf("RequestLogin returned error: %v", err)
	}
	if !result.Sent || repo.createdMagicLinkHash != HashToken("raw-token") {
		t.Fatalf("unexpected login result: result=%+v repo=%+v", result, repo)
	}
	if sender.toEmail != "user@example.com" || !strings.Contains(sender.magicLink, "raw-token") {
		t.Fatalf("unexpected sent link: %+v", sender)
	}
}

func TestRequestLoginBranches(t *testing.T) {
	disabled := NewService(&fakeRepository{userByEmail: &domain.User{IsEnabled: false}}, nil)
	if result, err := disabled.RequestLogin(context.Background(), LoginInput{}); err != nil || result.Sent {
		t.Fatalf("disabled login = %+v, %v; want no send and no error", result, err)
	}

	wantErr := errors.New("lookup failed")
	if _, err := NewService(&fakeRepository{err: wantErr}, nil).RequestLogin(context.Background(), LoginInput{}); !errors.Is(err, wantErr) {
		t.Fatalf("lookup error = %v, want %v", err, wantErr)
	}

	tokenErr := errors.New("token failed")
	tokenSvc := NewService(&fakeRepository{userByEmail: &domain.User{IsEnabled: true}}, nil).
		WithTokenGenerator(func(n int) (string, error) { return "", tokenErr })
	if _, err := tokenSvc.RequestLogin(context.Background(), LoginInput{}); !errors.Is(err, tokenErr) {
		t.Fatalf("token error = %v, want %v", err, tokenErr)
	}

	if _, err := NewService(&fakeRepository{userByEmail: &domain.User{IsEnabled: true}}, &fakeSender{}).
		WithTokenGenerator(func(n int) (string, error) { return "token", nil }).
		RequestLogin(context.Background(), LoginInput{}); err == nil {
		t.Fatal("expected missing URL builder error")
	}

	sendErr := errors.New("send failed")
	if _, err := NewService(&fakeRepository{userByEmail: &domain.User{Email: "u", IsEnabled: true}}, &fakeSender{err: sendErr}).
		WithTokenGenerator(func(n int) (string, error) { return "token", nil }).
		RequestLogin(context.Background(), LoginInput{MagicLinkURL: func(rawToken string) string { return rawToken }}); !errors.Is(err, sendErr) {
		t.Fatalf("send error = %v, want %v", err, sendErr)
	}

	createErr := errors.New("create magic link failed")
	if _, err := NewService(&fakeRepository{userByEmail: &domain.User{IsEnabled: true}, createMagicLinkErr: createErr}, nil).
		WithTokenGenerator(func(n int) (string, error) { return "token", nil }).
		RequestLogin(context.Background(), LoginInput{}); !errors.Is(err, createErr) {
		t.Fatalf("create magic link error = %v, want %v", err, createErr)
	}

	noSenderSvc := NewService(&fakeRepository{userByEmail: &domain.User{IsEnabled: true}}, nil).
		WithTokenGenerator(func(n int) (string, error) { return "token", nil })
	if result, err := noSenderSvc.RequestLogin(context.Background(), LoginInput{}); err != nil || !result.Sent {
		t.Fatalf("no sender login = %+v, %v; want sent with no error", result, err)
	}
}

func TestVerifyMagicLink(t *testing.T) {
	repo := &fakeRepository{magicLink: &domain.MagicLink{UserID: 7}}
	svc := NewService(repo, nil).WithTokenGenerator(sequenceTokens("session-id", "session-token"))

	session, err := svc.VerifyMagicLink(context.Background(), "raw-token")
	if err != nil {
		t.Fatalf("VerifyMagicLink returned error: %v", err)
	}
	if session.Token != "session-token" || repo.createdSessionHash != HashToken("session-token") {
		t.Fatalf("unexpected session: %+v repo=%+v", session, repo)
	}
}

func TestVerifyMagicLinkErrors(t *testing.T) {
	if _, err := NewService(&fakeRepository{err: apperrors.ErrNotFound}, nil).VerifyMagicLink(context.Background(), "raw"); !errors.Is(err, ErrInvalidMagicLink) {
		t.Fatalf("use link error = %v", err)
	}

	tokenErr := errors.New("token failed")
	if _, err := NewService(&fakeRepository{magicLink: &domain.MagicLink{UserID: 1}}, nil).
		WithTokenGenerator(func(n int) (string, error) { return "", tokenErr }).
		VerifyMagicLink(context.Background(), "raw"); !errors.Is(err, tokenErr) {
		t.Fatalf("token error = %v, want %v", err, tokenErr)
	}

	sessionTokenErr := errors.New("session token failed")
	if _, err := NewService(&fakeRepository{magicLink: &domain.MagicLink{UserID: 1}}, nil).
		WithTokenGenerator(func() TokenGenerator {
			calls := 0
			return func(n int) (string, error) {
				calls++
				if calls == 2 {
					return "", sessionTokenErr
				}
				return "session-id", nil
			}
		}()).
		VerifyMagicLink(context.Background(), "raw"); !errors.Is(err, sessionTokenErr) {
		t.Fatalf("session token error = %v, want %v", err, sessionTokenErr)
	}

	createErr := errors.New("create session failed")
	repo := &fakeRepository{magicLink: &domain.MagicLink{UserID: 1}}
	svc := NewService(repo, nil).WithTokenGenerator(func() TokenGenerator {
		calls := 0
		return func(n int) (string, error) {
			calls++
			if calls == 2 {
				repo.err = createErr
			}
			return "token", nil
		}
	}())
	if _, err := svc.VerifyMagicLink(context.Background(), "raw"); !errors.Is(err, createErr) {
		t.Fatalf("create session error = %v, want %v", err, createErr)
	}
}

func TestLogoutAndAuthenticateSession(t *testing.T) {
	repo := &fakeRepository{
		session:  &domain.Session{UserID: 5},
		userByID: &domain.User{ID: 5, Email: "u@example.com", Role: domain.RoleAdmin, IsEnabled: true},
	}
	svc := NewService(repo, nil)
	if err := svc.Logout(context.Background(), 5); err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
	if repo.deletedUserID != 5 {
		t.Fatalf("deleted user = %d, want 5", repo.deletedUserID)
	}
	user, err := svc.AuthenticateSession(context.Background(), "token")
	if err != nil {
		t.Fatalf("AuthenticateSession returned error: %v", err)
	}
	if !user.IsAdmin() || user.Email != "u@example.com" {
		t.Fatalf("unexpected current user: %+v", user)
	}
	if repo.readSessionHash != HashToken("token") {
		t.Fatalf("session lookup hash = %q, want %q", repo.readSessionHash, HashToken("token"))
	}
}

func TestAuthenticateSessionErrors(t *testing.T) {
	wantErr := errors.New("session failed")
	if _, err := NewService(&fakeRepository{err: wantErr}, nil).AuthenticateSession(context.Background(), "token"); !errors.Is(err, wantErr) {
		t.Fatalf("session error = %v, want %v", err, wantErr)
	}

	if _, err := NewService(&fakeRepository{
		session:      &domain.Session{UserID: 1},
		getUserIDErr: wantErr,
	}, nil).AuthenticateSession(context.Background(), "token"); !errors.Is(err, wantErr) {
		t.Fatalf("user error = %v, want %v", err, wantErr)
	}

	if _, err := NewService(&fakeRepository{
		session:  &domain.Session{UserID: 1},
		userByID: &domain.User{ID: 1, IsEnabled: false},
	}, nil).AuthenticateSession(context.Background(), "token"); err == nil {
		t.Fatal("expected disabled user error")
	}
}

func TestNowFallback(t *testing.T) {
	svc := &Service{}
	if svc.now().IsZero() {
		t.Fatal("expected fallback time")
	}
}

func TestTokenHelpers(t *testing.T) {
	if got := EncodeRandomToken([]byte("abc")); got == "" {
		t.Fatal("expected encoded token")
	}
	originalRandomRead := randomRead
	t.Cleanup(func() { randomRead = originalRandomRead })
	if token, err := GenerateRandomToken(4); err != nil || token == "" {
		t.Fatalf("GenerateRandomToken = %q, %v", token, err)
	}
	randomRead = func(b []byte) (int, error) {
		return 0, errors.New("random failed")
	}
	if _, err := GenerateRandomToken(4); err == nil {
		t.Fatal("expected random read error")
	}
	firstHash := HashToken("token")
	secondHash := HashToken("token")
	if firstHash != secondHash {
		t.Fatal("expected deterministic hash")
	}
}

func sequenceTokens(tokens ...string) TokenGenerator {
	i := 0
	return func(n int) (string, error) {
		token := tokens[i]
		i++
		return token, nil
	}
}
