package policy

import (
	"air-cover/internal/app/session"
	"air-cover/internal/domain"
)

func CanDeleteSubRequest(viewer session.CurrentUser, sr *domain.SubRequest) bool {
	return sr.CanBeDeletedBy(viewer)
}

func CanTakeSubRequest(viewer session.CurrentUser, sr *domain.SubRequest) bool {
	return sr.CanBeTakenBy(viewer)
}

func CanUntakeSubRequest(viewer session.CurrentUser, sr *domain.SubRequest) bool {
	return sr.CanBeUntakenBy(viewer)
}

func CanDeactivateUser(viewer session.CurrentUser, targetUserID int) bool {
	return viewer.ID != targetUserID
}
