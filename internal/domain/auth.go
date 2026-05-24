package domain

import "time"

type MagicLink struct {
	ID        int        `json:"id"`
	UserID    int        `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
}

type Session struct {
	ID           string    `json:"id"`
	UserID       int       `json:"user_id"`
	SessionToken string    `json:"-"`
	ExpiresAt    time.Time `json:"expires_at"`
}
