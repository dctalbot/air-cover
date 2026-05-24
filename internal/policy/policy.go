package policy

import (
	"air-cover/internal/app/session"
	"air-cover/internal/models"
)

func CanDeleteSubRequest(viewer session.CurrentUser, sr *models.SubRequest) bool {
	return sr.PostedByUserID == viewer.ID || viewer.IsAdmin()
}

func CanTakeSubRequest(viewer session.CurrentUser, sr *models.SubRequest) bool {
	return sr.TakenByUserID == nil && (sr.PostedByUserID != viewer.ID || viewer.IsAdmin())
}

func CanUntakeSubRequest(viewer session.CurrentUser, sr *models.SubRequest) bool {
	return sr.TakenByUserID != nil && *sr.TakenByUserID == viewer.ID
}

func CanDeactivateUser(viewer session.CurrentUser, targetUserID int) bool {
	return viewer.ID != targetUserID
}
