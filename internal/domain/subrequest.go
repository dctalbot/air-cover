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
	return string(s.Status())
}

func (s *SubRequest) Status() SubRequestStatus {
	if s.TakenByUserID != nil {
		return SubRequestStatusFilled
	}
	return SubRequestStatusOpen
}

func (s *SubRequest) HasValidTimeRange() bool {
	return s.EndTime.After(s.StartTime)
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
