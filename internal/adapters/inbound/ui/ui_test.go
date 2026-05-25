package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"air-cover/internal/adapters/inbound/presenter"
	appcatalog "air-cover/internal/app/catalog"
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
	users := []presenter.UserView{
		{ID: 1, Email: "admin@example.com", Role: "admin", IsEnabled: true},
		{ID: 2, Email: "member@example.com", Role: "member", IsEnabled: true, CanDeactivate: true},
		{ID: 3, Email: "disabled@example.com", Role: "member", IsEnabled: false, CanDeactivate: true},
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
	output := buf.String()
	for _, want := range []string{
		`form action="/users/import/spinitron" method="POST"`,
		`form action="/users" method="POST"`,
		`input type="email" id="email" name="email" required`,
		`form action="/users/2" method="POST"`,
		`name="is_enabled" value="false"`,
		`form action="/users/3" method="POST"`,
		`name="is_enabled" value="true"`,
		`role-badge role-admin`,
		`role-badge role-member`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected admin output to contain %q", want)
		}
	}
}

func TestAdmin_Empty(t *testing.T) {
	buf := new(bytes.Buffer)
	component := Admin(nil, "admin@example.com")
	if err := component.Render(context.Background(), buf); err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	if !strings.Contains(buf.String(), "No users found.") {
		t.Error("expected empty users message")
	}
}

func TestAuthenticated(t *testing.T) {
	shows := []appcatalog.Show{
		{ID: "1", Title: "Test Show"},
	}
	upcoming := []presenter.SubRequestView{
		{ID: 2, ShowTitle: "Another Show", RequesterEmail: "other@example.com", Status: "open", CanTake: true},
	}
	past := []presenter.SubRequestView{
		{ID: 1, ShowTitle: "Test Show", RequesterEmail: "user@example.com", TakerEmail: "taker@example.com", Status: "filled", CanDelete: true, CanUntake: true, IsPast: true},
	}
	buf := new(bytes.Buffer)
	component := Authenticated(shows, "user@example.com", upcoming, past, false)
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
	if !strings.Contains(buf.String(), `form action="/sub-requests" method="POST"`) {
		t.Error("expected sub-request form action not found")
	}
	if !strings.Contains(buf.String(), `name="show" required`) {
		t.Error("expected show selector name and required attribute")
	}
	if !strings.Contains(buf.String(), `name="start_time" required`) {
		t.Error("expected start_time input name and required attribute")
	}
	if !strings.Contains(buf.String(), `name="end_time" required`) {
		t.Error("expected end_time input name and required attribute")
	}
	if !strings.Contains(buf.String(), `body: JSON.stringify({ action: "take" })`) {
		t.Error("expected take action script payload")
	}
	if !strings.Contains(buf.String(), `body: JSON.stringify({ action: "untake" })`) {
		t.Error("expected untake action script payload")
	}
}

func TestAuthenticated_AdminLinkAndFallbackShowTitle(t *testing.T) {
	shows := []appcatalog.Show{{ID: "99"}}
	buf := new(bytes.Buffer)
	component := Authenticated(shows, "admin@example.com", nil, nil, true)
	if err := component.Render(context.Background(), buf); err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `<a href="/admin">Admin</a>`) {
		t.Error("expected admin navigation link")
	}
	if !strings.Contains(output, `<option value="99">99</option>`) {
		t.Error("expected show ID fallback when title is empty")
	}
}

func TestSubRequestTable_ActionVisibility(t *testing.T) {
	requests := []presenter.SubRequestView{
		{ID: 1, ShowTitle: "Open Show", RequesterEmail: "requester@example.com", Status: "open", CanTake: true, CanDelete: true},
		{ID: 2, ShowTitle: "Taken Show", RequesterEmail: "requester@example.com", TakerEmail: "taker@example.com", Status: "filled", CanUntake: true},
		{ID: 3, ShowTitle: "Past Show", RequesterEmail: "requester@example.com", TakerEmail: "taker@example.com", Status: "filled", IsPast: true},
	}
	buf := new(bytes.Buffer)
	component := SubRequestTable(requests)
	if err := component.Render(context.Background(), buf); err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		`id="sub-request-row-1"`,
		`takeRequest('1')`,
		`deleteRequest('1')`,
		`id="sub-request-row-2"`,
		`untakeRequest('2')`,
		`Past Show`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected rendered table to contain %q", want)
		}
	}
}

func TestSubRequestDetails(t *testing.T) {
	req := presenter.SubRequestView{
		ID:             5,
		ShowTitle:      "Detail Show",
		RequesterEmail: "requester@example.com",
		TakerEmail:     "taker@example.com",
		StartTime:      "May 23, 3:04pm",
		EndTime:        "May 23, 5:04pm",
		Duration:       "2 hours",
		Notes:          "Bring records",
		Status:         "filled",
		CanDelete:      true,
		CanUntake:      true,
	}
	buf := new(bytes.Buffer)
	if err := SubRequestDetails("admin@example.com", req, true).Render(context.Background(), buf); err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		"Sub Request Details",
		"Detail Show",
		"requester@example.com",
		"taker@example.com",
		"Bring records",
		`<a href="/admin">Admin</a>`,
		`deleteRequest('5')`,
		`untakeRequest('5')`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected detail output to contain %q", want)
		}
	}
}

func TestSubRequestDetails_EmptyOptionalFields(t *testing.T) {
	req := presenter.SubRequestView{
		ID:             6,
		ShowTitle:      "Open Show",
		RequesterEmail: "requester@example.com",
		Status:         "open",
		CanTake:        true,
	}
	buf := new(bytes.Buffer)
	if err := SubRequestDetails("user@example.com", req, false).Render(context.Background(), buf); err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	output := buf.String()
	for _, want := range []string{"None yet", "No notes provided.", `takeRequest('6')`} {
		if !strings.Contains(output, want) {
			t.Errorf("expected detail output to contain %q", want)
		}
	}
	if strings.Contains(output, `<a href="/admin">Admin</a>`) {
		t.Error("admin link should be hidden for non-admin users")
	}
}

func TestAuthenticated_Empty(t *testing.T) {
	buf := new(bytes.Buffer)
	component := Authenticated(nil, "user@example.com", nil, nil, false)
	err := component.Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No upcoming sub requests yet.") {
		t.Error("expected empty message not found")
	}
	if strings.Contains(output, "Recent history") {
		t.Error("Recent history header should not be present when past is empty")
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
