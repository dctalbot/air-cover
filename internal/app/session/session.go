package session

type CurrentUser struct {
	ID    int
	Email string
	Role  string
}

func (u CurrentUser) IsAdmin() bool {
	return u.Role == "admin"
}
