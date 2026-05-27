package authorization

import (
	"context"
	"errors"
	"testing"

	"air-cover/internal/apperrors"
	"air-cover/internal/domain"
)

func TestParityAuthorizer(t *testing.T) {
	t.Parallel()
	authorizer := NewParityAuthorizer()
	admin := SubjectFromCurrentUser(domain.CurrentUser{ID: 1, Role: domain.RoleAdmin})
	member := SubjectFromCurrentUser(domain.CurrentUser{ID: 2, Role: domain.RoleMember})
	takerID := 3

	tests := []struct {
		name      string
		subject   Subject
		action    Action
		resource  Resource
		wantError error
	}{
		{"admin access", admin, ActionAdminAccess, AdminResource(), nil},
		{"member admin denied", member, ActionAdminAccess, AdminResource(), apperrors.ErrForbidden},
		{"admin self deactivate denied", admin, ActionUserUpdate, UserResource(1, true), apperrors.ErrForbidden},
		{"admin own role change allowed", admin, ActionUserUpdate, UserResource(1, false), nil},
		{"owner delete allowed", member, ActionSubRequestDelete, SubRequestResource(&domain.SubRequest{PostedByUserID: 2}), nil},
		{"non owner delete denied", member, ActionSubRequestDelete, SubRequestResource(&domain.SubRequest{PostedByUserID: 1}), apperrors.ErrForbidden},
		{"take open request allowed", member, ActionSubRequestTake, SubRequestResource(&domain.SubRequest{PostedByUserID: 1}), nil},
		{"take own request denied", member, ActionSubRequestTake, SubRequestResource(&domain.SubRequest{PostedByUserID: 2}), apperrors.ErrForbidden},
		{"admin take own request denied", admin, ActionSubRequestTake, SubRequestResource(&domain.SubRequest{PostedByUserID: 1}), apperrors.ErrForbidden},
		{"untake by taker allowed", SubjectFromCurrentUser(domain.CurrentUser{ID: takerID}), ActionSubRequestUntake, SubRequestResource(&domain.SubRequest{TakenByUserID: &takerID}), nil},
		{"unknown action denied", admin, Action("unknown"), AdminResource(), apperrors.ErrForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := authorizer.Authorize(context.Background(), tt.subject, tt.action, tt.resource)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Authorize error = %v, want %v", err, tt.wantError)
			}
		})
	}
}

func TestSubRequestResourceHandlesNil(t *testing.T) {
	t.Parallel()
	resource := SubRequestResource(nil)
	if resource.Type != ResourceSubRequest || resource.SubRequestOpen {
		t.Fatalf("unexpected nil subrequest resource: %+v", resource)
	}
}
