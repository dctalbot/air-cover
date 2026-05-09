package ui

import (
	"bytes"
	"context"
	"testing"

	"air-cover/internal/spinitron"
)

func TestUnauthenticated(t *testing.T) {
	buf := new(bytes.Buffer)
	component := Unauthenticated(true)
	err := component.Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("If an account exists, an email has been sent.")) {
		t.Error("expected success message not found in rendered output")
	}
}

func TestAdmin(t *testing.T) {
	users := []UserView{
		{ID: 1, Email: "admin@example.com", Role: "admin", IsEnabled: true},
	}
	buf := new(bytes.Buffer)
	component := Admin(users, "admin@example.com")
	err := component.Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("admin@example.com")) {
		t.Error("expected user email not found in rendered output")
	}
}

func TestAuthenticated(t *testing.T) {
	shows := []spinitron.Show{
		{ID: "1", Title: "Test Show"},
	}
	subRequests := []SubRequestView{
		{ID: 1, ShowTitle: "Test Show", RequesterEmail: "user@example.com", Status: "open"},
	}
	buf := new(bytes.Buffer)
	component := Authenticated(shows, "user@example.com", subRequests, false)
	err := component.Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("Test Show")) {
		t.Error("expected show title not found in rendered output")
	}
}
