package authorization

import (
	"context"

	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

type Action string

const (
	ActionAdminAccess      Action = "admin:access"
	ActionUserList         Action = "user:list"
	ActionUserCreate       Action = "user:create"
	ActionUserUpdate       Action = "user:update"
	ActionSubRequestDelete Action = "subrequest:delete"
	ActionSubRequestTake   Action = "subrequest:take"
	ActionSubRequestUntake Action = "subrequest:untake"
)

type ResourceType string

const (
	ResourceAdmin      ResourceType = "admin"
	ResourceUser       ResourceType = "user"
	ResourceSubRequest ResourceType = "subrequest"
)

type Subject struct {
	User domain.CurrentUser
}

type Resource struct {
	Type           ResourceType
	OwnerUserID    int
	TakerUserID    int
	HasTaker       bool
	TargetUserID   int
	DisableTarget  bool
	SubRequestOpen bool
}

type Authorizer interface {
	Authorize(ctx context.Context, subject Subject, action Action, resource Resource) error
}

func SubjectFromCurrentUser(user domain.CurrentUser) Subject {
	return Subject{User: user}
}

func AdminResource() Resource {
	return Resource{Type: ResourceAdmin}
}

func UserResource(targetUserID int, disableTarget bool) Resource {
	return Resource{
		Type:          ResourceUser,
		TargetUserID:  targetUserID,
		DisableTarget: disableTarget,
	}
}

func SubRequestResource(sr *domain.SubRequest) Resource {
	resource := Resource{Type: ResourceSubRequest}
	if sr == nil {
		return resource
	}
	resource.OwnerUserID = sr.PostedByUserID
	if sr.TakenByUserID != nil {
		resource.TakerUserID = *sr.TakenByUserID
		resource.HasTaker = true
	}
	resource.SubRequestOpen = sr.TakenByUserID == nil
	return resource
}

type ParityAuthorizer struct{}

func NewParityAuthorizer() ParityAuthorizer {
	return ParityAuthorizer{}
}

func (ParityAuthorizer) Authorize(ctx context.Context, subject Subject, action Action, resource Resource) error {
	if allowed(subject, action, resource) {
		return nil
	}
	return apperrors.ErrForbidden
}

func allowed(subject Subject, action Action, resource Resource) bool {
	user := subject.User
	switch action {
	case ActionAdminAccess, ActionUserList, ActionUserCreate:
		return user.IsAdmin()
	case ActionUserUpdate:
		return user.IsAdmin() && (!resource.DisableTarget || user.ID != resource.TargetUserID)
	case ActionSubRequestDelete:
		return resource.OwnerUserID == user.ID || user.IsAdmin()
	case ActionSubRequestTake:
		return resource.SubRequestOpen && (resource.OwnerUserID != user.ID || user.IsAdmin())
	case ActionSubRequestUntake:
		return resource.HasTaker && resource.TakerUserID == user.ID
	default:
		return false
	}
}
