package presenter

import (
	"fmt"
	"strings"
	"time"

	"air-cover/internal/app/subrequests"
	"air-cover/internal/domain"
)

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
	TakerEmail     string
	StartTime      string
	EndTime        string
	Duration       string
	Notes          string
	Status         string
	CanDelete      bool
	CanTake        bool
	CanUntake      bool
	IsPast         bool
}

func SubRequestDashboard(dashboard subrequests.Dashboard) ([]SubRequestView, []SubRequestView) {
	return subRequestViews(dashboard.Upcoming), subRequestViews(dashboard.Past)
}

func AdminUsers(users []*domain.User, currentUserID int) []UserView {
	views := make([]UserView, 0, len(users))
	for _, u := range users {
		views = append(views, UserView{
			ID:            u.ID,
			Email:         u.Email,
			Role:          string(u.Role),
			CreatedAt:     u.CreatedAt.Format("Jan 02, 2006 at 3:04 PM"),
			IsEnabled:     u.IsEnabled,
			CanDeactivate: u.ID != currentUserID,
		})
	}
	return views
}

func subRequestViews(items []subrequests.DashboardSubRequest) []SubRequestView {
	views := make([]SubRequestView, 0, len(items))
	for _, item := range items {
		sr := item.Request
		views = append(views, SubRequestView{
			ID:             sr.ID,
			ShowTitle:      item.ShowTitle,
			RequesterEmail: item.RequesterEmail,
			TakerEmail:     item.TakerEmail,
			StartTime:      sr.StartTime.Format("Jan 2, 3:04pm"),
			EndTime:        sr.EndTime.Format(time.RFC3339),
			Duration:       FormatDuration(sr.EndTime.Sub(sr.StartTime)),
			Notes:          sr.Notes,
			Status:         sr.GetStatus(),
			CanDelete:      item.CanDelete,
			CanTake:        item.CanTake,
			CanUntake:      item.CanUntake,
			IsPast:         item.IsPast,
		})
	}
	return views
}

func FormatDuration(duration time.Duration) string {
	if duration < time.Hour {
		return fmt.Sprintf("%d min", int(duration.Minutes()))
	}
	hours := duration.Hours()
	durationStr := fmt.Sprintf("%.2f", hours)
	durationStr = strings.TrimSuffix(durationStr, "0")
	durationStr = strings.TrimSuffix(durationStr, "0")
	durationStr = strings.TrimSuffix(durationStr, ".")
	return durationStr + " hours"
}
