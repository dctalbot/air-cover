package models

import (
	"time"
)

// Persona represents a DJ on Spinitron.
type Persona struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// SubRequest represents a request for a substitute DJ for a specific show.
type SubRequest struct {
	ID             int       `json:"id"`
	ShowID         int       `json:"show_id"`
	PostedByUserID int       `json:"posted_by_user_id"`
	TakenByUserID  *int      `json:"taken_by_user_id"`
	RequesterEmail string    `json:"requester_email,omitempty"`
	SubstituteID   *int      `json:"substitute_id"` // Persona ID, nil if not picked up yet
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	Notes          string    `json:"notes"`
	Status         string    `json:"status"` // "open", "filled", "cancelled"
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
