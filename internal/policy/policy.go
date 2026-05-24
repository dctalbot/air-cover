package policy

import (
	"air-cover/internal/domain"
)

func CanDeleteSubRequest(viewer domain.CurrentUser, sr *domain.SubRequest) bool {
	return sr.CanBeDeletedBy(viewer)
}

func CanTakeSubRequest(viewer domain.CurrentUser, sr *domain.SubRequest) bool {
	return sr.CanBeTakenBy(viewer)
}

func CanUntakeSubRequest(viewer domain.CurrentUser, sr *domain.SubRequest) bool {
	return sr.CanBeUntakenBy(viewer)
}

func CanDeactivateUser(viewer domain.CurrentUser, targetUserID int) bool {
	return viewer.ID != targetUserID
}
