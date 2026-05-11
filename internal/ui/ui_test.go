package ui

import (
	"bytes"
	"context"
	"testing"

	"air-cover/internal/spinitron"
)

func TestLayout(t *testing.T) {
	buf := new(bytes.Buffer)
	component := Layout("Test Title")
	err := component.Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("<title>Test Title</title>")) {
		t.Errorf("expected title tag not found in rendered output: %s", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("<!doctype html>")) {
		t.Errorf("expected doctype not found in rendered output: %s", output)
	}
}

func TestUserMenu(t *testing.T) {
	buf := new(bytes.Buffer)
	component := UserMenu("user@example.com")
	err := component.Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	output := buf.Bytes()
	if !bytes.Contains(output, []byte("user@example.com")) {
		t.Error("expected email not found in rendered output")
	}
	if !bytes.Contains(output, []byte("/auth/logout")) {
		t.Error("expected logout form action not found in rendered output")
	}
	if !bytes.Contains(output, []byte("Log Out")) {
		t.Error("expected logout button text not found in rendered output")
	}
}

func TestBaseStyles(t *testing.T) {
	buf := new(bytes.Buffer)
	component := BaseStyles()
	err := component.Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("system-ui")) {
		t.Error("expected body font-family not found in rendered output")
	}
}

func TestPageHeader(t *testing.T) {
	buf := new(bytes.Buffer)
	component := PageHeader("Test Header")
	err := component.Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("Test Header")) {
		t.Error("expected header title not found in rendered output")
	}
}

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
		{ID: 1, ShowTitle: "Test Show", RequesterEmail: "user@example.com", TakerEmail: "taker@example.com", Status: "filled", CanDelete: true, CanUntake: true},
		{ID: 2, ShowTitle: "Another Show", RequesterEmail: "other@example.com", Status: "open", CanTake: true},
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
	if !bytes.Contains(buf.Bytes(), []byte("row-taken")) {
		t.Error("expected row-taken class not found in rendered output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("row-available")) {
		t.Error("expected row-available class not found in rendered output")
	}
}

func TestTableStyles(t *testing.T) {
	buf := new(bytes.Buffer)
	component := TableStyles()
	err := component.Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("data-table")) {
		t.Error("expected data-table class not found in rendered output")
	}
	if !bytes.Contains(buf.Bytes(), []byte(".row-taken")) {
		t.Error("expected .row-taken style not found in rendered output")
	}
	if !bytes.Contains(buf.Bytes(), []byte(".row-available")) {
		t.Error("expected .row-available style not found in rendered output")
	}
}
