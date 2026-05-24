package domain

import "time"

type MagicLink struct {
	ID        int
	UserID    int
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

type Session struct {
	ID        string
	UserID    int
	TokenHash string
	ExpiresAt time.Time
}
