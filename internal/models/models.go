package models

import "air-cover/internal/domain"

// Persona represents a DJ on Spinitron.
type Persona struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// SubRequest represents a request for a substitute DJ for a specific show.
type SubRequest = domain.SubRequest
