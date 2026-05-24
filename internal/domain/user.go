package domain

import "time"

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

func (r Role) Valid() bool {
	return r == RoleAdmin || r == RoleMember
}

type User struct {
	ID        int       `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	IsEnabled bool      `json:"is_enabled"`
	CreatedAt time.Time `json:"created_at"`
}

func (u User) IsAdmin() bool {
	return Role(u.Role) == RoleAdmin
}

type CurrentUser struct {
	ID    int
	Email string
	Role  string
}

func (u CurrentUser) IsAdmin() bool {
	return Role(u.Role) == RoleAdmin
}
