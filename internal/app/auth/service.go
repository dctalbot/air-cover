package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"air-cover/internal/domain"
)

const (
	loginTokenBytes   = 32
	sessionTokenBytes = 32
	magicLinkTTL      = 15 * time.Minute
	sessionTTL        = 24 * time.Hour
)

var ErrInvalidMagicLink = errors.New("invalid magic link")

var randomRead = rand.Read

type TokenGenerator func(int) (string, error)

type Repository interface {
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUserByID(ctx context.Context, id int) (*domain.User, error)
	CreateMagicLink(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error
	UseMagicLink(ctx context.Context, tokenHash string, now time.Time) (*domain.MagicLink, error)
	CreateSession(ctx context.Context, sessionID, sessionToken string, userID int, expiresAt time.Time) error
	GetSessionByToken(ctx context.Context, sessionToken string, now time.Time) (*domain.Session, error)
	DeleteSessionsByUserID(ctx context.Context, userID int) error
}

type Sender interface {
	SendMagicLink(toEmail, magicLink string) error
}

type LoginInput struct {
	Email        string
	MagicLinkURL func(rawToken string) string
}

type LoginResult struct {
	Sent bool
}

type VerifiedSession struct {
	Token     string
	ExpiresAt time.Time
}

type Service struct {
	repo           Repository
	sender         Sender
	tokenGenerator TokenGenerator
	nowFunc        func() time.Time
}

func NewService(repo Repository, sender Sender) *Service {
	return &Service{
		repo:           repo,
		sender:         sender,
		tokenGenerator: GenerateRandomToken,
		nowFunc:        time.Now,
	}
}

func (s *Service) WithTokenGenerator(generator TokenGenerator) *Service {
	if generator != nil {
		s.tokenGenerator = generator
	}
	return s
}

func GenerateRandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := randomRead(b); err != nil {
		return "", err
	}
	return EncodeRandomToken(b), nil
}

func EncodeRandomToken(b []byte) string {
	return base64.URLEncoding.EncodeToString(b)
}

func HashToken(token string) string {
	h := sha256.New()
	h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Service) RequestLogin(ctx context.Context, input LoginInput) (LoginResult, error) {
	user, err := s.repo.GetUserByEmail(ctx, input.Email)
	if err != nil {
		return LoginResult{}, err
	}
	if !user.IsEnabled {
		return LoginResult{}, nil
	}

	rawToken, err := s.tokenGenerator(loginTokenBytes)
	if err != nil {
		return LoginResult{}, err
	}

	if err := s.repo.CreateMagicLink(ctx, user.ID, HashToken(rawToken), s.now().Add(magicLinkTTL)); err != nil {
		return LoginResult{}, err
	}
	if s.sender == nil {
		return LoginResult{Sent: true}, nil
	}
	if input.MagicLinkURL == nil {
		return LoginResult{}, fmt.Errorf("magic link URL builder is required")
	}
	if err := s.sender.SendMagicLink(user.Email, input.MagicLinkURL(rawToken)); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Sent: true}, nil
}

func (s *Service) VerifyMagicLink(ctx context.Context, rawToken string) (VerifiedSession, error) {
	now := s.now()
	ml, err := s.repo.UseMagicLink(ctx, HashToken(rawToken), now)
	if err != nil {
		return VerifiedSession{}, fmt.Errorf("%w: %v", ErrInvalidMagicLink, err)
	}

	sessionID, err := s.tokenGenerator(sessionTokenBytes)
	if err != nil {
		return VerifiedSession{}, err
	}
	sessionToken, err := s.tokenGenerator(sessionTokenBytes)
	if err != nil {
		return VerifiedSession{}, err
	}

	expiresAt := now.Add(sessionTTL)
	if err := s.repo.CreateSession(ctx, sessionID, sessionToken, ml.UserID, expiresAt); err != nil {
		return VerifiedSession{}, err
	}
	return VerifiedSession{Token: sessionToken, ExpiresAt: expiresAt}, nil
}

func (s *Service) Logout(ctx context.Context, userID int) error {
	return s.repo.DeleteSessionsByUserID(ctx, userID)
}

func (s *Service) AuthenticateSession(ctx context.Context, sessionToken string) (domain.CurrentUser, error) {
	sess, err := s.repo.GetSessionByToken(ctx, sessionToken, s.now())
	if err != nil {
		return domain.CurrentUser{}, err
	}
	user, err := s.repo.GetUserByID(ctx, sess.UserID)
	if err != nil {
		return domain.CurrentUser{}, err
	}
	if !user.IsEnabled {
		return domain.CurrentUser{}, fmt.Errorf("user is disabled")
	}
	return domain.CurrentUser{ID: user.ID, Email: user.Email, Role: user.Role}, nil
}

func (s *Service) now() time.Time {
	if s.nowFunc != nil {
		return s.nowFunc()
	}
	return time.Now()
}
