package policy

import (
	"testing"

	"air-cover/internal/app/session"
	"air-cover/internal/models"
)

func TestSubRequestPermissions(t *testing.T) {
	takerID := 2
	tests := []struct {
		name      string
		viewer    session.CurrentUser
		request   *models.SubRequest
		canDelete bool
		canTake   bool
		canUntake bool
	}{
		{
			name:      "poster can delete but cannot take own request",
			viewer:    session.CurrentUser{ID: 1, Role: "member"},
			request:   &models.SubRequest{PostedByUserID: 1},
			canDelete: true,
		},
		{
			name:    "member can take someone else's open request",
			viewer:  session.CurrentUser{ID: 2, Role: "member"},
			request: &models.SubRequest{PostedByUserID: 1},
			canTake: true,
		},
		{
			name:      "admin can delete and take own open request",
			viewer:    session.CurrentUser{ID: 1, Role: "admin"},
			request:   &models.SubRequest{PostedByUserID: 1},
			canDelete: true,
			canTake:   true,
		},
		{
			name:      "taker can untake filled request",
			viewer:    session.CurrentUser{ID: takerID, Role: "member"},
			request:   &models.SubRequest{PostedByUserID: 1, TakenByUserID: &takerID},
			canUntake: true,
		},
		{
			name:    "other member cannot modify filled request",
			viewer:  session.CurrentUser{ID: 3, Role: "member"},
			request: &models.SubRequest{PostedByUserID: 1, TakenByUserID: &takerID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanDeleteSubRequest(tt.viewer, tt.request); got != tt.canDelete {
				t.Errorf("CanDeleteSubRequest() = %v, want %v", got, tt.canDelete)
			}
			if got := CanTakeSubRequest(tt.viewer, tt.request); got != tt.canTake {
				t.Errorf("CanTakeSubRequest() = %v, want %v", got, tt.canTake)
			}
			if got := CanUntakeSubRequest(tt.viewer, tt.request); got != tt.canUntake {
				t.Errorf("CanUntakeSubRequest() = %v, want %v", got, tt.canUntake)
			}
		})
	}
}

func TestCanDeactivateUser(t *testing.T) {
	viewer := session.CurrentUser{ID: 1}
	if CanDeactivateUser(viewer, 1) {
		t.Error("expected users not to be able to deactivate themselves")
	}
	if !CanDeactivateUser(viewer, 2) {
		t.Error("expected users to be able to deactivate other users")
	}
}
