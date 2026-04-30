package models

import (
	"testing"
	"time"
)

func TestModels(t *testing.T) {
	u := User{ID: 1, Email: "test@example.com", CreatedAt: time.Now()}
	if u.ID != 1 {
		t.Errorf("expected ID 1, got %d", u.ID)
	}

	ml := MagicLink{ID: 1, UserID: 1, TokenHash: "hash", ExpiresAt: time.Now()}
	if ml.UserID != 1 {
		t.Errorf("expected UserID 1, got %d", ml.UserID)
	}

	s := Session{ID: "sid", UserID: 1, SessionToken: "tok", ExpiresAt: time.Now()}
	if s.SessionToken != "tok" {
		t.Errorf("expected token tok, got %s", s.SessionToken)
	}

	sr := SubRequest{
		ID:             1,
		ShowID:         42,
		PostedByUserID: 1,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(time.Hour),
		Notes:          "test",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if sr.ShowID != 42 {
		t.Errorf("expected ShowID 42, got %d", sr.ShowID)
	}

	if sr.GetStatus() != "open" {
		t.Errorf("expected status open, got %s", sr.GetStatus())
	}

	uid := 1
	sr.TakenByUserID = &uid
	if sr.GetStatus() != "filled" {
		t.Errorf("expected status filled, got %s", sr.GetStatus())
	}

	p := Persona{ID: 1, Name: "DJ Test", Email: "dj@example.com"}
	if p.Name != "DJ Test" {
		t.Errorf("expected DJ Test, got %s", p.Name)
	}
}
