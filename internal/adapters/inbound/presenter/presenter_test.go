package presenter

import (
	"testing"
	"time"

	adminapp "air-cover/internal/app/admin"
	"air-cover/internal/app/subrequests"
	"air-cover/internal/domain"
)

func TestSubRequestDashboard(t *testing.T) {
	start := time.Date(2026, 5, 23, 15, 4, 0, 0, time.UTC)
	dashboard := subrequests.Dashboard{
		Upcoming: []subrequests.DashboardSubRequest{{
			Request: &domain.SubRequest{
				ID:        1,
				StartTime: start,
				EndTime:   start.Add(90 * time.Minute),
			},
			RequesterEmail: "requester@example.com",
			ShowTitle:      "Example Show",
			CanDelete:      true,
			CanTake:        true,
		}},
		Past: []subrequests.DashboardSubRequest{{
			Request: &domain.SubRequest{
				ID:            2,
				TakenByUserID: intPtr(3),
				StartTime:     start.Add(-24 * time.Hour),
				EndTime:       start.Add(-23 * time.Hour),
			},
			TakerEmail: "taker@example.com",
			ShowTitle:  "Past Show",
			CanUntake:  true,
			IsPast:     true,
		}},
	}

	upcoming, past := SubRequestDashboard(dashboard)
	if len(upcoming) != 1 || len(past) != 1 {
		t.Fatalf("expected one upcoming and one past view, got %d and %d", len(upcoming), len(past))
	}
	if upcoming[0].ShowTitle != "Example Show" || upcoming[0].Duration != "1.5 hours" {
		t.Errorf("unexpected upcoming view: %+v", upcoming[0])
	}
	if !upcoming[0].CanDelete || !upcoming[0].CanTake {
		t.Errorf("expected upcoming permissions to be preserved: %+v", upcoming[0])
	}
	if past[0].Status != "filled" || !past[0].CanUntake || !past[0].IsPast {
		t.Errorf("unexpected past view: %+v", past[0])
	}
}

func TestSubRequestDetail(t *testing.T) {
	start := time.Date(2026, 5, 23, 15, 4, 0, 0, time.UTC)
	takerID := 3
	view := SubRequestDetail(subrequests.Detail{
		Request: &domain.SubRequest{
			ID:            4,
			TakenByUserID: &takerID,
			StartTime:     start,
			EndTime:       start.Add(2 * time.Hour),
			Notes:         "details",
		},
		RequesterEmail: "requester@example.com",
		TakerEmail:     "taker@example.com",
		ShowTitle:      "Detail Show",
		CanDelete:      true,
		CanUntake:      true,
	})

	if view.ID != 4 || view.ShowTitle != "Detail Show" || view.Status != "filled" {
		t.Fatalf("unexpected detail view: %+v", view)
	}
	if !view.CanDelete || !view.CanUntake || view.Duration != "2 hours" || view.Notes != "details" {
		t.Fatalf("unexpected detail fields: %+v", view)
	}
}

func TestAdminUsers(t *testing.T) {
	created := time.Date(2026, 5, 23, 15, 4, 0, 0, time.UTC)
	views := AdminUsers([]adminapp.UserReadModel{{
		User: &domain.User{
			ID:        1,
			Email:     "admin@example.com",
			Role:      "admin",
			CreatedAt: created,
			IsEnabled: true,
		},
		CanDeactivate: true,
	}})
	if len(views) != 1 {
		t.Fatalf("expected one view, got %d", len(views))
	}
	if views[0].CreatedAt != "May 23, 2026 at 3:04 PM" {
		t.Errorf("unexpected created-at format: %q", views[0].CreatedAt)
	}
	if !views[0].CanDeactivate {
		t.Error("expected deactivate capability to be preserved")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{45 * time.Minute, "45 min"},
		{time.Hour, "1 hours"},
		{90 * time.Minute, "1.5 hours"},
		{75 * time.Minute, "1.25 hours"},
	}

	for _, tt := range tests {
		if got := FormatDuration(tt.duration); got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
		}
	}
}

func intPtr(v int) *int {
	return &v
}
