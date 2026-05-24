package domain

import "time"

type SubRequestStatus string

const (
	SubRequestStatusOpen   SubRequestStatus = "open"
	SubRequestStatusFilled SubRequestStatus = "filled"
)

// SubRequest is the core request entity.
type SubRequest struct {
	ID             int
	ShowID         int
	PostedByUserID int
	TakenByUserID  *int
	SubstituteID   *int
	StartTime      time.Time
	EndTime        time.Time
	Notes          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (s *SubRequest) GetStatus() string {
	if s.TakenByUserID == nil {
		return string(SubRequestStatusOpen)
	}
	return string(SubRequestStatusFilled)
}

func (s *SubRequest) CanBeDeletedBy(viewer CurrentUser) bool {
	return s.PostedByUserID == viewer.ID || viewer.IsAdmin()
}

func (s *SubRequest) CanBeTakenBy(viewer CurrentUser) bool {
	return s.TakenByUserID == nil && (s.PostedByUserID != viewer.ID || viewer.IsAdmin())
}

func (s *SubRequest) CanBeUntakenBy(viewer CurrentUser) bool {
	return s.TakenByUserID != nil && *s.TakenByUserID == viewer.ID
}
