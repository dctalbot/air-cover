package ui

type UserView struct {
	ID            int
	Email         string
	Role          string
	CreatedAt     string
	IsEnabled     bool
	CanDeactivate bool
}

type SubRequestView struct {
	ID             int
	ShowTitle      string
	RequesterEmail string
	StartTime      string
	EndTime        string
	Notes          string
	Status         string
	CanDelete      bool
}
