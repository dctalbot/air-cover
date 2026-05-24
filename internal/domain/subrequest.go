package domain

import "time"

type SubRequestStatus string

const (
	SubRequestStatusOpen   SubRequestStatus = "open"
	SubRequestStatusFilled SubRequestStatus = "filled"
)

// SubRequest is the core request entity. Email fields are read-model
// enrichment for dashboards until those query models are split out fully.
type SubRequest struct {
	ID             int       `json:"id"`
	ShowID         int       `json:"show_id"`
	PostedByUserID int       `json:"posted_by_user_id"`
	TakenByUserID  *int      `json:"taken_by_user_id"`
	RequesterEmail string    `json:"requester_email,omitempty"`
	TakerEmail     string    `json:"taker_email,omitempty"`
	SubstituteID   *int      `json:"substitute_id"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	Notes          string    `json:"notes"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
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
