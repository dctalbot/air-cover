package session

import "testing"

func TestCurrentUserIsAdmin(t *testing.T) {
	if !(CurrentUser{Role: "admin"}).IsAdmin() {
		t.Error("expected admin role to be admin")
	}
	if (CurrentUser{Role: "member"}).IsAdmin() {
		t.Error("expected member role not to be admin")
	}
}
