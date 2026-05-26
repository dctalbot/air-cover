package domain

import (
	"testing"
	"time"
)

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

func TestSubRequestStatus(t *testing.T) {
	request := &SubRequest{}

	if request.GetStatus() != string(SubRequestStatusOpen) {
		t.Fatalf("expected open status, got %q", request.GetStatus())
	}
	if request.Status() != SubRequestStatusOpen {
		t.Fatalf("expected open status value, got %q", request.Status())
	}

	takerID := 3
	request.TakenByUserID = &takerID
	if request.GetStatus() != string(SubRequestStatusFilled) {
		t.Fatalf("expected filled status, got %q", request.GetStatus())
	}
	if request.Status() != SubRequestStatusFilled {
		t.Fatalf("expected filled status value, got %q", request.Status())
	}
}

func TestSubRequestHasValidTimeRange(t *testing.T) {
	start := time.Now()
	if !(&SubRequest{StartTime: start, EndTime: start.Add(time.Hour)}).HasValidTimeRange() {
		t.Fatal("expected end after start to be valid")
	}
	if (&SubRequest{StartTime: start, EndTime: start}).HasValidTimeRange() {
		t.Fatal("expected equal start and end to be invalid")
	}
	if (&SubRequest{StartTime: start, EndTime: start.Add(-time.Minute)}).HasValidTimeRange() {
		t.Fatal("expected end before start to be invalid")
	}
}
