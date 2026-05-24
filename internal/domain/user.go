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
	ID        int
	Email     string
	Role      Role
	IsEnabled bool
	CreatedAt time.Time
}

func (u User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

type CurrentUser struct {
	ID    int
	Email string
	Role  Role
}

func (u CurrentUser) IsAdmin() bool {
	return u.Role == RoleAdmin
}

func (u CurrentUser) CanDeactivateUser(targetUserID int) bool {
	return u.ID != targetUserID
}
