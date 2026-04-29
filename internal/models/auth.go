package models

import "time"

type User struct {
	ID        int       `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

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
