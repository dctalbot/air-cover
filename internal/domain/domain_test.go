package domain

import "testing"

func TestRoleValid(t *testing.T) {
	if !RoleAdmin.Valid() || !RoleMember.Valid() {
		t.Fatal("expected known roles to be valid")
	}
	if Role("owner").Valid() {
		t.Fatal("expected unknown role to be invalid")
	}
}

func TestUserIsAdmin(t *testing.T) {
	admin := User{Role: RoleAdmin}
	member := User{Role: RoleMember}
	if !admin.IsAdmin() {
		t.Fatal("expected admin user")
	}
	if member.IsAdmin() {
		t.Fatal("expected member user")
	}
}

func TestCurrentUserIsAdmin(t *testing.T) {
	admin := CurrentUser{Role: RoleAdmin}
	member := CurrentUser{Role: RoleMember}
	if !admin.IsAdmin() {
		t.Fatal("expected admin current user")
	}
	if member.IsAdmin() {
		t.Fatal("expected member current user")
	}
}

func TestSubRequestStatusAndPolicy(t *testing.T) {
	viewer := CurrentUser{ID: 1, Role: RoleMember}
	admin := CurrentUser{ID: 2, Role: RoleAdmin}
	request := &SubRequest{PostedByUserID: 1}

	if request.GetStatus() != string(SubRequestStatusOpen) {
		t.Fatalf("expected open status, got %q", request.GetStatus())
	}
	if !request.CanBeDeletedBy(viewer) || !request.CanBeDeletedBy(admin) {
		t.Fatal("expected requester and admin to delete")
	}
	if request.CanBeTakenBy(viewer) {
		t.Fatal("expected requester member not to take own request")
	}
	if !request.CanBeTakenBy(admin) {
		t.Fatal("expected admin to take own request")
	}

	takerID := 3
	request.TakenByUserID = &takerID
	if request.GetStatus() != string(SubRequestStatusFilled) {
		t.Fatalf("expected filled status, got %q", request.GetStatus())
	}
	if !request.CanBeUntakenBy(CurrentUser{ID: takerID}) {
		t.Fatal("expected taker to untake")
	}
	if request.CanBeUntakenBy(viewer) {
		t.Fatal("expected non-taker not to untake")
	}
}

func TestCurrentUserCanDeactivateUser(t *testing.T) {
	viewer := CurrentUser{ID: 1}
	if viewer.CanDeactivateUser(1) {
		t.Fatal("expected current user not to deactivate themselves")
	}
	if !viewer.CanDeactivateUser(2) {
		t.Fatal("expected current user to deactivate another user")
	}
}
