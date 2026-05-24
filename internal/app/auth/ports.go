package auth

import (
	"context"
	"time"

	"air-cover/internal/domain"
)

type UserReader interface {
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUserByID(ctx context.Context, id int) (*domain.User, error)
}

type MagicLinkStore interface {
	CreateMagicLink(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error
	UseMagicLink(ctx context.Context, tokenHash string, now time.Time) (*domain.MagicLink, error)
}

type SessionStore interface {
	CreateSession(ctx context.Context, sessionID, sessionTokenHash string, userID int, expiresAt time.Time) error
	GetSessionByToken(ctx context.Context, sessionTokenHash string, now time.Time) (*domain.Session, error)
	DeleteSessionsByUserID(ctx context.Context, userID int) error
}

type Repository interface {
	UserReader
	MagicLinkStore
	SessionStore
}

type Sender interface {
	SendMagicLink(toEmail, magicLink string) error
}
