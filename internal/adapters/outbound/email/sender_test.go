package email

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/domain"
	"github.com/resend/resend-go/v3"
)

func TestConsoleSender(t *testing.T) {
	t.Parallel()
	s := &ConsoleSender{}
	if err := s.SendMagicLink("test@example.com", "http://example.com/verify?token=abc"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if err := s.SendSubRequestCreated([]string{"test@example.com"}, testSubRequestMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if err := s.SendSubRequestTaken("requester@example.com", []string{"taker@example.com"}, testSubRequestTakenMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if err := s.SendSubRequestUntaken("requester@example.com", []string{"untaker@example.com"}, testSubRequestUntakenMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestResendSender_Success(t *testing.T) {
	t.Parallel()
	emails := &fakeResendEmails{}
	s := &ResendSender{FromEmail: "noreply@example.com", Emails: emails}

	magicLink := "http://example.com/verify?token=abc&next=/dashboard"
	if err := s.SendMagicLink("test@example.com", magicLink); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if emails.request == nil {
		t.Fatal("expected email request")
	}
	if emails.request.From != "noreply@example.com" {
		t.Fatalf("expected from noreply@example.com, got %s", emails.request.From)
	}
	if len(emails.request.To) != 1 || emails.request.To[0] != "test@example.com" {
		t.Fatalf("expected recipient test@example.com, got %v", emails.request.To)
	}
	if emails.request.Subject != "Log in to Air Cover" {
		t.Fatalf("unexpected subject: %s", emails.request.Subject)
	}
	if emails.request.Text == "" || emails.request.Html == "" {
		t.Fatal("expected text and HTML content")
	}
	if !strings.Contains(emails.request.Text, "Use the secure link below to log in to Air Cover.") {
		t.Fatalf("expected refined copy in text content: %s", emails.request.Text)
	}
	if !strings.Contains(emails.request.Text, magicLink) {
		t.Fatalf("expected raw magic link in text content: %s", emails.request.Text)
	}
	if !strings.Contains(emails.request.Html, "Log in to Air Cover") {
		t.Fatalf("expected CTA copy in HTML content: %s", emails.request.Html)
	}
	if !strings.Contains(emails.request.Html, "http://example.com/verify?token=abc&amp;next=/dashboard") {
		t.Fatalf("expected escaped magic link in HTML content: %s", emails.request.Html)
	}
}

func TestResendSender_Error(t *testing.T) {
	t.Parallel()
	s := &ResendSender{
		FromEmail: "noreply@example.com",
		Emails:    &fakeResendEmails{err: errors.New("resend down")},
	}

	if err := s.SendMagicLink("test@example.com", "http://example.com/verify?token=abc"); err == nil {
		t.Error("expected error for resend failure")
	}
}

func TestResendSender_SubRequestCreated(t *testing.T) {
	t.Parallel()
	emails := &fakeResendEmails{}
	s := &ResendSender{FromEmail: "noreply@example.com", Emails: emails}

	recipients := []string{"one@example.com", "two@example.com"}
	if err := s.SendSubRequestCreated(recipients, testSubRequestMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if emails.request == nil {
		t.Fatal("expected email request")
	}
	if len(emails.request.To) != 1 || emails.request.To[0] != "noreply@example.com" {
		t.Fatalf("expected visible recipient noreply@example.com, got %v", emails.request.To)
	}
	if !sameStrings(emails.request.Bcc, recipients) {
		t.Fatalf("expected bcc recipients %v, got %v", recipients, emails.request.Bcc)
	}
	if emails.request.Subject != "New sub request: Test Show" {
		t.Fatalf("unexpected subject: %s", emails.request.Subject)
	}
	if !strings.Contains(emails.request.Text, "requester@example.com posted a new sub request for Test Show.") {
		t.Fatalf("expected summary in text content: %s", emails.request.Text)
	}
	if !strings.Contains(emails.request.Text, "View sub request: https://aircover.example.com/sub-requests/42") {
		t.Fatalf("expected details link in text content: %s", emails.request.Text)
	}
	if !strings.Contains(emails.request.Html, "Bring headphones") {
		t.Fatalf("expected notes in HTML content: %s", emails.request.Html)
	}
}

func TestResendSender_SubRequestCreatedError(t *testing.T) {
	t.Parallel()
	s := &ResendSender{
		FromEmail: "noreply@example.com",
		Emails:    &fakeResendEmails{err: errors.New("resend down")},
	}
	if err := s.SendSubRequestCreated([]string{"active@example.com"}, testSubRequestMessage()); err == nil {
		t.Fatal("expected resend error")
	}
}

func TestResendSender_SubRequestCreatedNoRecipients(t *testing.T) {
	t.Parallel()
	emails := &fakeResendEmails{}
	s := &ResendSender{FromEmail: "noreply@example.com", Emails: emails}
	if err := s.SendSubRequestCreated(nil, testSubRequestMessage()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if emails.request != nil {
		t.Fatal("expected no email request")
	}
}

func TestResendSender_SubRequestTaken(t *testing.T) {
	t.Parallel()
	emails := &fakeResendEmails{}
	s := &ResendSender{FromEmail: "noreply@example.com", Emails: emails}

	if err := s.SendSubRequestTaken("requester@example.com", []string{"taker@example.com"}, testSubRequestTakenMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if emails.request == nil {
		t.Fatal("expected email request")
	}
	if !sameStrings(emails.request.To, []string{"requester@example.com"}) {
		t.Fatalf("expected requester recipient, got %v", emails.request.To)
	}
	if !sameStrings(emails.request.Cc, []string{"taker@example.com"}) {
		t.Fatalf("expected taker cc, got %v", emails.request.Cc)
	}
	if emails.request.Subject != "Sub request taken: Test Show" {
		t.Fatalf("unexpected subject: %s", emails.request.Subject)
	}
	if !strings.Contains(emails.request.Text, "taker@example.com took your sub request for Test Show.") {
		t.Fatalf("expected summary in text content: %s", emails.request.Text)
	}
	if !strings.Contains(emails.request.Text, "View sub request: https://aircover.example.com/sub-requests/42") {
		t.Fatalf("expected details link in text content: %s", emails.request.Text)
	}
}

func TestResendSender_SubRequestTakenError(t *testing.T) {
	t.Parallel()
	s := &ResendSender{
		FromEmail: "noreply@example.com",
		Emails:    &fakeResendEmails{err: errors.New("resend down")},
	}
	if err := s.SendSubRequestTaken("requester@example.com", []string{"taker@example.com"}, testSubRequestTakenMessage()); err == nil {
		t.Fatal("expected resend error")
	}
}

func TestResendSender_SubRequestTakenNoRequester(t *testing.T) {
	t.Parallel()
	emails := &fakeResendEmails{}
	s := &ResendSender{FromEmail: "noreply@example.com", Emails: emails}
	if err := s.SendSubRequestTaken("", []string{"taker@example.com"}, testSubRequestTakenMessage()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if emails.request != nil {
		t.Fatal("expected no email request")
	}
}

func TestResendSender_SubRequestUntaken(t *testing.T) {
	t.Parallel()
	emails := &fakeResendEmails{}
	s := &ResendSender{FromEmail: "noreply@example.com", Emails: emails}

	if err := s.SendSubRequestUntaken("requester@example.com", []string{"untaker@example.com"}, testSubRequestUntakenMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if emails.request == nil {
		t.Fatal("expected email request")
	}
	if !sameStrings(emails.request.To, []string{"requester@example.com"}) {
		t.Fatalf("expected requester recipient, got %v", emails.request.To)
	}
	if !sameStrings(emails.request.Cc, []string{"untaker@example.com"}) {
		t.Fatalf("expected untaker cc, got %v", emails.request.Cc)
	}
	if emails.request.Subject != "Sub request no longer covered: Test Show" {
		t.Fatalf("unexpected subject: %s", emails.request.Subject)
	}
	if !strings.Contains(emails.request.Text, "untaker@example.com is no longer covering your sub request for Test Show.") {
		t.Fatalf("expected summary in text content: %s", emails.request.Text)
	}
	if !strings.Contains(emails.request.Text, "View sub request: https://aircover.example.com/sub-requests/42") {
		t.Fatalf("expected details link in text content: %s", emails.request.Text)
	}
}

func TestResendSender_SubRequestUntakenError(t *testing.T) {
	t.Parallel()
	s := &ResendSender{
		FromEmail: "noreply@example.com",
		Emails:    &fakeResendEmails{err: errors.New("resend down")},
	}
	if err := s.SendSubRequestUntaken("requester@example.com", []string{"untaker@example.com"}, testSubRequestUntakenMessage()); err == nil {
		t.Fatal("expected resend error")
	}
}

func TestResendSender_SubRequestUntakenNoRequester(t *testing.T) {
	t.Parallel()
	emails := &fakeResendEmails{}
	s := &ResendSender{FromEmail: "noreply@example.com", Emails: emails}
	if err := s.SendSubRequestUntaken("", []string{"untaker@example.com"}, testSubRequestUntakenMessage()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if emails.request != nil {
		t.Fatal("expected no email request")
	}
}

func TestSendGridSender_Success(t *testing.T) {
	t.Parallel()
	var payload sendGridPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	s := &SendGridSender{
		APIKey:    "test-key",
		FromEmail: "noreply@example.com",
		HTTPClient: &http.Client{
			Transport: &proxyTransport{target: srv.URL},
		},
	}

	if err := s.SendMagicLink("test@example.com", "http://example.com/verify?token=abc"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if payload.Subject != "Log in to Air Cover" {
		t.Fatalf("unexpected subject: %s", payload.Subject)
	}
	if len(payload.Content) != 2 {
		t.Fatalf("expected text and HTML content, got %d entries", len(payload.Content))
	}
	if payload.Content[0].Type != "text/plain" {
		t.Fatalf("expected first content type text/plain, got %s", payload.Content[0].Type)
	}
	if !strings.Contains(payload.Content[0].Value, "Use the secure link below to log in to Air Cover.") {
		t.Fatalf("expected refined copy in text content: %s", payload.Content[0].Value)
	}
	if payload.Content[1].Type != "text/html" {
		t.Fatalf("expected second content type text/html, got %s", payload.Content[1].Type)
	}
	if !strings.Contains(payload.Content[1].Value, "<!doctype html>") {
		t.Fatalf("expected rendered HTML content: %s", payload.Content[1].Value)
	}
}

func TestSendGridSender_SubRequestCreated(t *testing.T) {
	t.Parallel()
	var payload sendGridPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	s := &SendGridSender{
		APIKey:    "test-key",
		FromEmail: "noreply@example.com",
		HTTPClient: &http.Client{
			Transport: &proxyTransport{target: srv.URL},
		},
	}

	recipients := []string{"one@example.com", "two@example.com"}
	if err := s.SendSubRequestCreated(recipients, testSubRequestMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(payload.Personalizations) != 1 {
		t.Fatalf("expected one personalization, got %d", len(payload.Personalizations))
	}
	if !sameAddresses(payload.Personalizations[0].To, []string{"noreply@example.com"}) {
		t.Fatalf("expected visible recipient noreply@example.com, got %v", payload.Personalizations[0].To)
	}
	if !sameAddresses(payload.Personalizations[0].BCC, recipients) {
		t.Fatalf("expected bcc recipients %v, got %v", recipients, payload.Personalizations[0].BCC)
	}
	if payload.Subject != "New sub request: Test Show" {
		t.Fatalf("unexpected subject: %s", payload.Subject)
	}
	if !strings.Contains(payload.Content[0].Value, "Notes: Bring headphones") {
		t.Fatalf("expected notes in text content: %s", payload.Content[0].Value)
	}
	if !strings.Contains(payload.Content[1].Value, "https://aircover.example.com/sub-requests/42") {
		t.Fatalf("expected detail link in HTML content: %s", payload.Content[1].Value)
	}
}

func TestSendGridSender_SubRequestTaken(t *testing.T) {
	t.Parallel()
	var payload sendGridPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	s := &SendGridSender{
		APIKey:    "test-key",
		FromEmail: "noreply@example.com",
		HTTPClient: &http.Client{
			Transport: &proxyTransport{target: srv.URL},
		},
	}

	if err := s.SendSubRequestTaken("requester@example.com", []string{"taker@example.com"}, testSubRequestTakenMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(payload.Personalizations) != 1 {
		t.Fatalf("expected one personalization, got %d", len(payload.Personalizations))
	}
	if !sameAddresses(payload.Personalizations[0].To, []string{"requester@example.com"}) {
		t.Fatalf("expected requester recipient, got %v", payload.Personalizations[0].To)
	}
	if !sameAddresses(payload.Personalizations[0].CC, []string{"taker@example.com"}) {
		t.Fatalf("expected taker cc, got %v", payload.Personalizations[0].CC)
	}
	if payload.Subject != "Sub request taken: Test Show" {
		t.Fatalf("unexpected subject: %s", payload.Subject)
	}
	if !strings.Contains(payload.Content[0].Value, "taker@example.com took your sub request for Test Show.") {
		t.Fatalf("expected summary in text content: %s", payload.Content[0].Value)
	}
	if !strings.Contains(payload.Content[1].Value, "https://aircover.example.com/sub-requests/42") {
		t.Fatalf("expected detail link in HTML content: %s", payload.Content[1].Value)
	}
}

func TestSendGridSender_SubRequestTakenNoCC(t *testing.T) {
	t.Parallel()
	var payload sendGridPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	s := &SendGridSender{
		APIKey:    "test-key",
		FromEmail: "noreply@example.com",
		HTTPClient: &http.Client{
			Transport: &proxyTransport{target: srv.URL},
		},
	}

	if err := s.SendSubRequestTaken("requester@example.com", nil, testSubRequestTakenMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(payload.Personalizations) != 1 {
		t.Fatalf("expected one personalization, got %d", len(payload.Personalizations))
	}
	if len(payload.Personalizations[0].CC) != 0 {
		t.Fatalf("expected no cc recipients, got %v", payload.Personalizations[0].CC)
	}
}

func TestSendGridSender_SubRequestUntaken(t *testing.T) {
	t.Parallel()
	var payload sendGridPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	s := &SendGridSender{
		APIKey:    "test-key",
		FromEmail: "noreply@example.com",
		HTTPClient: &http.Client{
			Transport: &proxyTransport{target: srv.URL},
		},
	}

	if err := s.SendSubRequestUntaken("requester@example.com", []string{"untaker@example.com"}, testSubRequestUntakenMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(payload.Personalizations) != 1 {
		t.Fatalf("expected one personalization, got %d", len(payload.Personalizations))
	}
	if !sameAddresses(payload.Personalizations[0].To, []string{"requester@example.com"}) {
		t.Fatalf("expected requester recipient, got %v", payload.Personalizations[0].To)
	}
	if !sameAddresses(payload.Personalizations[0].CC, []string{"untaker@example.com"}) {
		t.Fatalf("expected untaker cc, got %v", payload.Personalizations[0].CC)
	}
	if payload.Subject != "Sub request no longer covered: Test Show" {
		t.Fatalf("unexpected subject: %s", payload.Subject)
	}
	if !strings.Contains(payload.Content[0].Value, "untaker@example.com is no longer covering your sub request for Test Show.") {
		t.Fatalf("expected summary in text content: %s", payload.Content[0].Value)
	}
	if !strings.Contains(payload.Content[1].Value, "https://aircover.example.com/sub-requests/42") {
		t.Fatalf("expected detail link in HTML content: %s", payload.Content[1].Value)
	}
}

func TestSendGridSender_SubRequestUntakenNoCC(t *testing.T) {
	t.Parallel()
	var payload sendGridPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	s := &SendGridSender{
		APIKey:    "test-key",
		FromEmail: "noreply@example.com",
		HTTPClient: &http.Client{
			Transport: &proxyTransport{target: srv.URL},
		},
	}

	if err := s.SendSubRequestUntaken("requester@example.com", nil, testSubRequestUntakenMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(payload.Personalizations) != 1 {
		t.Fatalf("expected one personalization, got %d", len(payload.Personalizations))
	}
	if len(payload.Personalizations[0].CC) != 0 {
		t.Fatalf("expected no cc recipients, got %v", payload.Personalizations[0].CC)
	}
}

func TestSendGridSender_SubRequestUntakenError(t *testing.T) {
	t.Parallel()
	s := &SendGridSender{
		APIKey: "test-key",
		HTTPClient: &http.Client{
			Transport: &errorTransport{},
		},
	}
	if err := s.SendSubRequestUntaken("requester@example.com", []string{"untaker@example.com"}, testSubRequestUntakenMessage()); err == nil {
		t.Fatal("expected sendgrid error")
	}
}

func TestSendGridSender_SubRequestUntakenHTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := &SendGridSender{
		APIKey: "test-key",
		HTTPClient: &http.Client{
			Transport: &proxyTransport{target: srv.URL},
		},
	}

	if err := s.SendSubRequestUntaken("requester@example.com", []string{"untaker@example.com"}, testSubRequestUntakenMessage()); err == nil {
		t.Fatal("expected sendgrid HTTP error")
	}
}

func TestSendGridSender_SubRequestUntakenNoRequester(t *testing.T) {
	t.Parallel()
	s := &SendGridSender{APIKey: "test-key", HTTPClient: &http.Client{}}
	if err := s.SendSubRequestUntaken("", []string{"untaker@example.com"}, testSubRequestUntakenMessage()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestSendGridSender_SubRequestTakenError(t *testing.T) {
	t.Parallel()
	s := &SendGridSender{
		APIKey: "test-key",
		HTTPClient: &http.Client{
			Transport: &errorTransport{},
		},
	}
	if err := s.SendSubRequestTaken("requester@example.com", []string{"taker@example.com"}, testSubRequestTakenMessage()); err == nil {
		t.Fatal("expected sendgrid error")
	}
}

func TestSendGridSender_SubRequestTakenHTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := &SendGridSender{
		APIKey: "test-key",
		HTTPClient: &http.Client{
			Transport: &proxyTransport{target: srv.URL},
		},
	}

	if err := s.SendSubRequestTaken("requester@example.com", []string{"taker@example.com"}, testSubRequestTakenMessage()); err == nil {
		t.Fatal("expected sendgrid HTTP error")
	}
}

func TestSendGridSender_SubRequestTakenNoRequester(t *testing.T) {
	t.Parallel()
	s := &SendGridSender{APIKey: "test-key", HTTPClient: &http.Client{}}
	if err := s.SendSubRequestTaken("", []string{"taker@example.com"}, testSubRequestTakenMessage()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestSendGridSender_SubRequestCreatedError(t *testing.T) {
	t.Parallel()
	s := &SendGridSender{
		APIKey: "test-key",
		HTTPClient: &http.Client{
			Transport: &errorTransport{},
		},
	}
	if err := s.SendSubRequestCreated([]string{"active@example.com"}, testSubRequestMessage()); err == nil {
		t.Fatal("expected sendgrid error")
	}
}

func TestSendGridSender_SubRequestCreatedHTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := &SendGridSender{
		APIKey: "test-key",
		HTTPClient: &http.Client{
			Transport: &proxyTransport{target: srv.URL},
		},
	}

	if err := s.SendSubRequestCreated([]string{"active@example.com"}, testSubRequestMessage()); err == nil {
		t.Fatal("expected sendgrid HTTP error")
	}
}

func TestSendGridSender_SubRequestCreatedNoRecipients(t *testing.T) {
	t.Parallel()
	s := &SendGridSender{APIKey: "test-key", HTTPClient: &http.Client{}}
	if err := s.SendSubRequestCreated(nil, testSubRequestMessage()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestSendGridSender_HTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := &SendGridSender{
		APIKey: "test-key",
		HTTPClient: &http.Client{
			Transport: &proxyTransport{target: srv.URL},
		},
	}

	if err := s.SendMagicLink("test@example.com", "http://example.com/verify?token=abc"); err == nil {
		t.Error("expected error for HTTP error status")
	}
}

func TestNewSender(t *testing.T) {
	t.Parallel()
	s := NewSender("resend-key", "sendgrid-key", "noreply@example.com")
	resendSender, ok := s.(*ResendSender)
	if !ok {
		t.Fatal("expected ResendSender when resend key is present")
	}
	if resendSender.FromEmail != "noreply@example.com" {
		t.Fatalf("expected noreply@example.com, got %s", resendSender.FromEmail)
	}
	if resendSender.Emails == nil {
		t.Fatal("expected Resend emails client to be initialized")
	}

	s2 := NewSender("", "sendgrid-key", "noreply@example.com")
	sg, ok := s2.(*SendGridSender)
	if !ok {
		t.Fatal("expected SendGridSender")
	}
	if sg.APIKey != "sendgrid-key" {
		t.Fatalf("expected sendgrid-key, got %s", sg.APIKey)
	}
	if sg.FromEmail != "noreply@example.com" {
		t.Fatalf("expected noreply@example.com, got %s", sg.FromEmail)
	}
	if sg.HTTPClient == nil {
		t.Fatal("expected HTTPClient to be initialized")
	}

	s3 := NewSender("", "", "noreply@example.com")
	if _, ok := s3.(*ConsoleSender); !ok {
		t.Fatal("expected ConsoleSender when no provider keys are present")
	}
}

// proxyTransport rewrites requests to send to a test server host instead.
type proxyTransport struct {
	target string
	base   http.RoundTripper
}

func (t *proxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Parse the test server URL to get its host
	testReq, _ := http.NewRequest("GET", t.target, nil) // nolint:gosec

	// Clone the request so we don't mutate the original
	newReq := req.Clone(req.Context())
	newReq.URL.Scheme = testReq.URL.Scheme
	newReq.URL.Host = testReq.URL.Host

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(newReq)
}

func TestSendGridSender_NetworkError(t *testing.T) {
	t.Parallel()
	s := &SendGridSender{
		APIKey: "test-key",
		HTTPClient: &http.Client{
			Transport: &errorTransport{},
		},
	}

	if err := s.SendMagicLink("test@example.com", "http://example.com/verify?token=abc"); err == nil {
		t.Error("expected error for network failure")
	}
}

type errorTransport struct{}

func (t *errorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, http.ErrHandlerTimeout
}

type fakeResendEmails struct {
	request *resend.SendEmailRequest
	err     error
}

type sendGridPayload struct {
	Personalizations []struct {
		To  []sendGridAddress `json:"to"`
		CC  []sendGridAddress `json:"cc"`
		BCC []sendGridAddress `json:"bcc"`
	} `json:"personalizations"`
	Subject string            `json:"subject"`
	Content []sendGridContent `json:"content"`
}

type sendGridAddress struct {
	Email string `json:"email"`
}

type sendGridContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func sameStrings(got, want []string) bool {
	return slices.Equal(got, want)
}

func sameAddresses(got []sendGridAddress, want []string) bool {
	emails := make([]string, 0, len(got))
	for _, address := range got {
		emails = append(emails, address.Email)
	}
	return slices.Equal(emails, want)
}

func (f *fakeResendEmails) Send(params *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
	f.request = params
	if f.err != nil {
		return nil, f.err
	}
	return &resend.SendEmailResponse{Id: "email-id"}, nil
}

func TestSendGridSender_MarshalAndRequestErrors(t *testing.T) {
	t.Run("marshal error", func(t *testing.T) {
		originalJSONMarshal := jsonMarshal
		t.Cleanup(func() { jsonMarshal = originalJSONMarshal })
		jsonMarshal = func(v any) ([]byte, error) {
			return nil, errors.New("marshal failed")
		}

		s := &SendGridSender{APIKey: "test-key", HTTPClient: &http.Client{}}
		if err := s.SendMagicLink("test@example.com", "http://example.com"); err == nil {
			t.Fatal("expected marshal error")
		}
	})

	t.Run("request creation error", func(t *testing.T) {
		originalNewHTTPRequest := newHTTPRequest
		t.Cleanup(func() { newHTTPRequest = originalNewHTTPRequest })
		newHTTPRequest = func(method, url string, body io.Reader) (*http.Request, error) {
			return nil, errors.New("request failed")
		}

		s := &SendGridSender{APIKey: "test-key", HTTPClient: &http.Client{}}
		if err := s.SendMagicLink("test@example.com", "http://example.com"); err == nil {
			t.Fatal("expected request creation error")
		}
	})
}

func TestSendGridSender_SubRequestCreatedMarshalAndRequestErrors(t *testing.T) {
	t.Run("marshal error", func(t *testing.T) {
		originalJSONMarshal := jsonMarshal
		t.Cleanup(func() { jsonMarshal = originalJSONMarshal })
		jsonMarshal = func(v any) ([]byte, error) {
			return nil, errors.New("marshal failed")
		}

		s := &SendGridSender{APIKey: "test-key", HTTPClient: &http.Client{}}
		if err := s.SendSubRequestCreated([]string{"active@example.com"}, testSubRequestMessage()); err == nil {
			t.Fatal("expected marshal error")
		}
	})

	t.Run("request creation error", func(t *testing.T) {
		originalNewHTTPRequest := newHTTPRequest
		t.Cleanup(func() { newHTTPRequest = originalNewHTTPRequest })
		newHTTPRequest = func(method, url string, body io.Reader) (*http.Request, error) {
			return nil, errors.New("request failed")
		}

		s := &SendGridSender{APIKey: "test-key", HTTPClient: &http.Client{}}
		if err := s.SendSubRequestCreated([]string{"active@example.com"}, testSubRequestMessage()); err == nil {
			t.Fatal("expected request creation error")
		}
	})
}

func TestSendGridSender_SubRequestTakenMarshalAndRequestErrors(t *testing.T) {
	t.Run("marshal error", func(t *testing.T) {
		originalJSONMarshal := jsonMarshal
		t.Cleanup(func() { jsonMarshal = originalJSONMarshal })
		jsonMarshal = func(v any) ([]byte, error) {
			return nil, errors.New("marshal failed")
		}

		s := &SendGridSender{APIKey: "test-key", HTTPClient: &http.Client{}}
		if err := s.SendSubRequestTaken("requester@example.com", []string{"taker@example.com"}, testSubRequestTakenMessage()); err == nil {
			t.Fatal("expected marshal error")
		}
	})

	t.Run("request creation error", func(t *testing.T) {
		originalNewHTTPRequest := newHTTPRequest
		t.Cleanup(func() { newHTTPRequest = originalNewHTTPRequest })
		newHTTPRequest = func(method, url string, body io.Reader) (*http.Request, error) {
			return nil, errors.New("request failed")
		}

		s := &SendGridSender{APIKey: "test-key", HTTPClient: &http.Client{}}
		if err := s.SendSubRequestTaken("requester@example.com", []string{"taker@example.com"}, testSubRequestTakenMessage()); err == nil {
			t.Fatal("expected request creation error")
		}
	})
}

func TestSendGridSender_SubRequestUntakenMarshalAndRequestErrors(t *testing.T) {
	t.Run("marshal error", func(t *testing.T) {
		originalJSONMarshal := jsonMarshal
		t.Cleanup(func() { jsonMarshal = originalJSONMarshal })
		jsonMarshal = func(v any) ([]byte, error) {
			return nil, errors.New("marshal failed")
		}

		s := &SendGridSender{APIKey: "test-key", HTTPClient: &http.Client{}}
		if err := s.SendSubRequestUntaken("requester@example.com", []string{"untaker@example.com"}, testSubRequestUntakenMessage()); err == nil {
			t.Fatal("expected marshal error")
		}
	})

	t.Run("request creation error", func(t *testing.T) {
		originalNewHTTPRequest := newHTTPRequest
		t.Cleanup(func() { newHTTPRequest = originalNewHTTPRequest })
		newHTTPRequest = func(method, url string, body io.Reader) (*http.Request, error) {
			return nil, errors.New("request failed")
		}

		s := &SendGridSender{APIKey: "test-key", HTTPClient: &http.Client{}}
		if err := s.SendSubRequestUntaken("requester@example.com", []string{"untaker@example.com"}, testSubRequestUntakenMessage()); err == nil {
			t.Fatal("expected request creation error")
		}
	})
}

func TestSubRequestNotifier(t *testing.T) {
	t.Parallel()
	users := &fakeActiveUsers{users: []*domain.User{
		{Email: "one@example.com", IsEnabled: true},
		{Email: "two@example.com", IsEnabled: true},
	}}
	sender := &fakeSubRequestSender{}
	notifier := &SubRequestNotifier{
		Users:   users,
		Sender:  sender,
		BaseURL: "https://aircover.example.com/",
	}

	err := notifier.SubRequestCreated(context.Background(), subrequestsapp.SubRequestCreatedEvent{
		Request:        &domain.SubRequest{ID: 42, StartTime: time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC), EndTime: time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC), Notes: "Bring headphones"},
		RequesterEmail: "requester@example.com",
		ShowTitle:      "Test Show",
		DetailPath:     "/sub-requests/42",
	})
	if err != nil {
		t.Fatalf("expected send failures to be logged but not returned, got %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("expected one send attempt, got %d", len(sender.messages))
	}
	if !sameStrings(sender.recipients[0], []string{"one@example.com", "two@example.com"}) {
		t.Fatalf("expected batched recipients, got %v", sender.recipients[0])
	}
	if sender.messages[0].DetailURL != "https://aircover.example.com/sub-requests/42" {
		t.Fatalf("detail URL = %q", sender.messages[0].DetailURL)
	}
}

func TestSubRequestNotifierSubRequestTaken(t *testing.T) {
	t.Parallel()
	sender := &fakeSubRequestSender{}
	notifier := &SubRequestNotifier{
		Sender:  sender,
		BaseURL: "https://aircover.example.com/",
	}

	err := notifier.SubRequestTaken(context.Background(), subrequestsapp.SubRequestTakenEvent{
		Request:        &domain.SubRequest{ID: 42, StartTime: time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC), EndTime: time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)},
		RequesterEmail: "requester@example.com",
		TakerEmail:     "taker@example.com",
		ShowTitle:      "Test Show",
		DetailPath:     "/sub-requests/42",
	})
	if err != nil {
		t.Fatalf("expected send failures to be logged but not returned, got %v", err)
	}
	if len(sender.takenMessages) != 1 {
		t.Fatalf("expected one send attempt, got %d", len(sender.takenMessages))
	}
	if sender.takenTo[0] != "requester@example.com" {
		t.Fatalf("expected requester recipient, got %q", sender.takenTo[0])
	}
	if !sameStrings(sender.takenCC[0], []string{"taker@example.com"}) {
		t.Fatalf("expected taker cc, got %v", sender.takenCC[0])
	}
	if sender.takenMessages[0].DetailURL != "https://aircover.example.com/sub-requests/42" {
		t.Fatalf("detail URL = %q", sender.takenMessages[0].DetailURL)
	}
}

func TestSubRequestNotifierSubRequestTakenNoRequester(t *testing.T) {
	t.Parallel()
	sender := &fakeSubRequestSender{}
	notifier := &SubRequestNotifier{Sender: sender}
	if err := notifier.SubRequestTaken(context.Background(), subrequestsapp.SubRequestTakenEvent{
		Request:    &domain.SubRequest{ID: 42},
		TakerEmail: "taker@example.com",
	}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(sender.takenMessages) != 0 {
		t.Fatalf("expected no sends, got %d", len(sender.takenMessages))
	}
}

func TestSubRequestNotifierSubRequestUntaken(t *testing.T) {
	t.Parallel()
	sender := &fakeSubRequestSender{}
	notifier := &SubRequestNotifier{
		Sender:  sender,
		BaseURL: "https://aircover.example.com/",
	}

	err := notifier.SubRequestUntaken(context.Background(), subrequestsapp.SubRequestUntakenEvent{
		Request:        &domain.SubRequest{ID: 42, StartTime: time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC), EndTime: time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)},
		RequesterEmail: "requester@example.com",
		UntakerEmail:   "untaker@example.com",
		ShowTitle:      "Test Show",
		DetailPath:     "/sub-requests/42",
	})
	if err != nil {
		t.Fatalf("expected send failures to be logged but not returned, got %v", err)
	}
	if len(sender.untakenMessages) != 1 {
		t.Fatalf("expected one send attempt, got %d", len(sender.untakenMessages))
	}
	if sender.untakenTo[0] != "requester@example.com" {
		t.Fatalf("expected requester recipient, got %q", sender.untakenTo[0])
	}
	if !sameStrings(sender.untakenCC[0], []string{"untaker@example.com"}) {
		t.Fatalf("expected untaker cc, got %v", sender.untakenCC[0])
	}
	if sender.untakenMessages[0].DetailURL != "https://aircover.example.com/sub-requests/42" {
		t.Fatalf("detail URL = %q", sender.untakenMessages[0].DetailURL)
	}
}

func TestSubRequestNotifierSubRequestUntakenNoRequester(t *testing.T) {
	t.Parallel()
	sender := &fakeSubRequestSender{}
	notifier := &SubRequestNotifier{Sender: sender}
	if err := notifier.SubRequestUntaken(context.Background(), subrequestsapp.SubRequestUntakenEvent{
		Request:      &domain.SubRequest{ID: 42},
		UntakerEmail: "untaker@example.com",
	}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(sender.untakenMessages) != 0 {
		t.Fatalf("expected no sends, got %d", len(sender.untakenMessages))
	}
}

func TestSubRequestNotifierSubRequestUntakenLogsSendError(t *testing.T) {
	t.Parallel()
	notifier := &SubRequestNotifier{
		Sender: &fakeSubRequestSender{untakenErr: errors.New("send failed")},
	}
	if err := notifier.SubRequestUntaken(context.Background(), subrequestsapp.SubRequestUntakenEvent{
		Request:        &domain.SubRequest{ID: 42},
		RequesterEmail: "requester@example.com",
	}); err != nil {
		t.Fatalf("expected send failures to be logged but not returned, got %v", err)
	}
}

func TestSubRequestNotifierSubRequestTakenLogsSendError(t *testing.T) {
	t.Parallel()
	notifier := &SubRequestNotifier{
		Sender: &fakeSubRequestSender{takenErr: errors.New("send failed")},
	}
	if err := notifier.SubRequestTaken(context.Background(), subrequestsapp.SubRequestTakenEvent{
		Request:        &domain.SubRequest{ID: 42},
		RequesterEmail: "requester@example.com",
	}); err != nil {
		t.Fatalf("expected send failures to be logged but not returned, got %v", err)
	}
}

func TestSubRequestNotifierRelativeDetailURL(t *testing.T) {
	t.Parallel()
	sender := &fakeSubRequestSender{}
	notifier := &SubRequestNotifier{
		Users:  &fakeActiveUsers{users: []*domain.User{{Email: "one@example.com", IsEnabled: true}}},
		Sender: sender,
	}
	if err := notifier.SubRequestCreated(context.Background(), subrequestsapp.SubRequestCreatedEvent{
		Request:    &domain.SubRequest{ID: 42},
		DetailPath: "/sub-requests/42",
	}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sender.messages[0].DetailURL != "/sub-requests/42" {
		t.Fatalf("expected relative detail URL, got %q", sender.messages[0].DetailURL)
	}
}

func TestSubRequestNotifierNoRecipients(t *testing.T) {
	t.Parallel()
	sender := &fakeSubRequestSender{}
	notifier := &SubRequestNotifier{
		Users:  &fakeActiveUsers{},
		Sender: sender,
	}
	if err := notifier.SubRequestCreated(context.Background(), subrequestsapp.SubRequestCreatedEvent{
		Request: &domain.SubRequest{ID: 42},
	}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("expected no sends, got %d", len(sender.messages))
	}
}

func TestSubRequestNotifierLogsSendError(t *testing.T) {
	t.Parallel()
	notifier := &SubRequestNotifier{
		Users:  &fakeActiveUsers{users: []*domain.User{{Email: "one@example.com", IsEnabled: true}}},
		Sender: &fakeSubRequestSender{err: errors.New("send failed")},
	}
	if err := notifier.SubRequestCreated(context.Background(), subrequestsapp.SubRequestCreatedEvent{
		Request: &domain.SubRequest{ID: 42},
	}); err != nil {
		t.Fatalf("expected send failures to be logged but not returned, got %v", err)
	}
}

func TestSubRequestNotifierConfigurationErrors(t *testing.T) {
	t.Parallel()
	notifier := &SubRequestNotifier{}
	if err := notifier.SubRequestCreated(context.Background(), subrequestsapp.SubRequestCreatedEvent{}); err == nil {
		t.Fatal("expected missing user lister error")
	}

	notifier.Users = &fakeActiveUsers{}
	if err := notifier.SubRequestCreated(context.Background(), subrequestsapp.SubRequestCreatedEvent{}); err == nil {
		t.Fatal("expected missing sender error")
	}

	notifier.Sender = &fakeSubRequestSender{}
	notifier.Users = &fakeActiveUsers{err: errors.New("db down")}
	if err := notifier.SubRequestCreated(context.Background(), subrequestsapp.SubRequestCreatedEvent{}); err == nil {
		t.Fatal("expected list users error")
	}

	notifier = &SubRequestNotifier{}
	if err := notifier.SubRequestTaken(context.Background(), subrequestsapp.SubRequestTakenEvent{}); err == nil {
		t.Fatal("expected missing sender error")
	}

	if err := notifier.SubRequestUntaken(context.Background(), subrequestsapp.SubRequestUntakenEvent{}); err == nil {
		t.Fatal("expected missing sender error")
	}
}

func TestAsyncNotifier(t *testing.T) {
	t.Parallel()
	next := &fakeNotifier{done: make(chan struct{}, 1)}
	notifier := NewAsyncNotifier(next, 1)
	notifier.Start()
	defer notifier.Stop()

	event := subrequestsapp.SubRequestCreatedEvent{Request: &domain.SubRequest{ID: 42}}
	if err := notifier.SubRequestCreated(context.Background(), event); err != nil {
		t.Fatalf("expected enqueue success, got %v", err)
	}
	select {
	case <-next.done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async notification")
	}
	if next.event.Request.ID != 42 {
		t.Fatalf("unexpected event: %+v", next.event)
	}
}

func TestAsyncNotifierSubRequestTaken(t *testing.T) {
	t.Parallel()
	next := &fakeNotifier{done: make(chan struct{}, 1)}
	notifier := NewAsyncNotifier(next, 1)
	notifier.Start()
	defer notifier.Stop()

	event := subrequestsapp.SubRequestTakenEvent{Request: &domain.SubRequest{ID: 42}, RequesterEmail: "requester@example.com"}
	if err := notifier.SubRequestTaken(context.Background(), event); err != nil {
		t.Fatalf("expected enqueue success, got %v", err)
	}
	select {
	case <-next.done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async notification")
	}
	if next.takenEvent.Request.ID != 42 {
		t.Fatalf("unexpected event: %+v", next.takenEvent)
	}
}

func TestAsyncNotifierSubRequestUntaken(t *testing.T) {
	t.Parallel()
	next := &fakeNotifier{done: make(chan struct{}, 1)}
	notifier := NewAsyncNotifier(next, 1)
	notifier.Start()
	defer notifier.Stop()

	event := subrequestsapp.SubRequestUntakenEvent{Request: &domain.SubRequest{ID: 42}, RequesterEmail: "requester@example.com"}
	if err := notifier.SubRequestUntaken(context.Background(), event); err != nil {
		t.Fatalf("expected enqueue success, got %v", err)
	}
	select {
	case <-next.done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async notification")
	}
	if next.untakenEvent.Request.ID != 42 {
		t.Fatalf("unexpected event: %+v", next.untakenEvent)
	}
}

func TestAsyncNotifierDefaultBufferAndErrors(t *testing.T) {
	t.Parallel()
	next := &fakeNotifier{err: errors.New("send failed"), done: make(chan struct{}, 1)}
	notifier := NewAsyncNotifier(next, 0)
	if cap(notifier.jobs) != defaultAsyncNotifierBuffer {
		t.Fatalf("expected default buffer %d, got %d", defaultAsyncNotifierBuffer, cap(notifier.jobs))
	}
	notifier.Start()
	if err := notifier.SubRequestCreated(context.Background(), subrequestsapp.SubRequestCreatedEvent{Request: &domain.SubRequest{ID: 7}}); err != nil {
		t.Fatalf("expected enqueue success, got %v", err)
	}
	select {
	case <-next.done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async notification")
	}
	notifier.Stop()
	select {
	case <-notifier.Done():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async notifier to stop")
	}
}

func TestAsyncNotifierWithoutNext(t *testing.T) {
	t.Parallel()
	notifier := NewAsyncNotifier(nil, 1)
	notifier.Start()
	if err := notifier.SubRequestCreated(context.Background(), subrequestsapp.SubRequestCreatedEvent{}); err != nil {
		t.Fatalf("expected no-op without next notifier, got %v", err)
	}
	if err := notifier.SubRequestTaken(context.Background(), subrequestsapp.SubRequestTakenEvent{}); err != nil {
		t.Fatalf("expected no-op without next notifier, got %v", err)
	}
	if err := notifier.SubRequestUntaken(context.Background(), subrequestsapp.SubRequestUntakenEvent{}); err != nil {
		t.Fatalf("expected no-op without next notifier, got %v", err)
	}
	if notifier.Done() == nil {
		t.Fatal("expected done channel")
	}
}

func TestAsyncNotifierQueueFull(t *testing.T) {
	t.Parallel()
	notifier := NewAsyncNotifier(&fakeNotifier{}, 1)
	event := subrequestsapp.SubRequestCreatedEvent{Request: &domain.SubRequest{ID: 42}}
	if err := notifier.SubRequestCreated(context.Background(), event); err != nil {
		t.Fatalf("expected first enqueue success, got %v", err)
	}
	if err := notifier.SubRequestCreated(context.Background(), event); err == nil {
		t.Fatal("expected queue full error")
	}
	notifier.Stop()
}

func TestAsyncNotifierSubRequestTakenQueueFull(t *testing.T) {
	t.Parallel()
	notifier := NewAsyncNotifier(&fakeNotifier{}, 1)
	event := subrequestsapp.SubRequestTakenEvent{Request: &domain.SubRequest{ID: 42}}
	if err := notifier.SubRequestTaken(context.Background(), event); err != nil {
		t.Fatalf("expected first enqueue success, got %v", err)
	}
	if err := notifier.SubRequestTaken(context.Background(), event); err == nil {
		t.Fatal("expected queue full error")
	}
	notifier.Stop()
}

func TestAsyncNotifierSubRequestUntakenQueueFull(t *testing.T) {
	t.Parallel()
	notifier := NewAsyncNotifier(&fakeNotifier{}, 1)
	event := subrequestsapp.SubRequestUntakenEvent{Request: &domain.SubRequest{ID: 42}}
	if err := notifier.SubRequestUntaken(context.Background(), event); err != nil {
		t.Fatalf("expected first enqueue success, got %v", err)
	}
	if err := notifier.SubRequestUntaken(context.Background(), event); err == nil {
		t.Fatal("expected queue full error")
	}
	notifier.Stop()
}

func TestAsyncNotificationHelpers(t *testing.T) {
	t.Parallel()
	notifier := &AsyncNotifier{}
	if err := notifier.process(asyncNotification{kind: asyncNotificationKind("unknown")}); err != nil {
		t.Fatalf("expected unknown notification kind to no-op, got %v", err)
	}

	tests := []struct {
		name string
		job  asyncNotification
		want int
	}{
		{name: "created", job: asyncNotification{kind: asyncNotificationCreated, createdEvent: subrequestsapp.SubRequestCreatedEvent{Request: &domain.SubRequest{ID: 1}}}, want: 1},
		{name: "created nil request", job: asyncNotification{kind: asyncNotificationCreated}, want: 0},
		{name: "taken", job: asyncNotification{kind: asyncNotificationTaken, takenEvent: subrequestsapp.SubRequestTakenEvent{Request: &domain.SubRequest{ID: 2}}}, want: 2},
		{name: "taken nil request", job: asyncNotification{kind: asyncNotificationTaken}, want: 0},
		{name: "untaken", job: asyncNotification{kind: asyncNotificationUntaken, untakenEvent: subrequestsapp.SubRequestUntakenEvent{Request: &domain.SubRequest{ID: 3}}}, want: 3},
		{name: "untaken nil request", job: asyncNotification{kind: asyncNotificationUntaken}, want: 0},
		{name: "unknown", job: asyncNotification{kind: asyncNotificationKind("unknown")}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.job.subRequestID(); got != tt.want {
				t.Fatalf("subRequestID() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAsyncNotifierNil(t *testing.T) {
	t.Parallel()
	if err := (*AsyncNotifier)(nil).SubRequestCreated(context.Background(), subrequestsapp.SubRequestCreatedEvent{}); err != nil {
		t.Fatalf("expected nil notifier no-op, got %v", err)
	}
	if err := (*AsyncNotifier)(nil).SubRequestTaken(context.Background(), subrequestsapp.SubRequestTakenEvent{}); err != nil {
		t.Fatalf("expected nil notifier no-op, got %v", err)
	}
	if err := (*AsyncNotifier)(nil).SubRequestUntaken(context.Background(), subrequestsapp.SubRequestUntakenEvent{}); err != nil {
		t.Fatalf("expected nil notifier no-op, got %v", err)
	}
	(*AsyncNotifier)(nil).Start()
	(*AsyncNotifier)(nil).Stop()
	select {
	case <-(*AsyncNotifier)(nil).Done():
	default:
		t.Fatal("expected nil Done channel to be closed")
	}
}

func testSubRequestMessage() SubRequestCreatedMessage {
	return SubRequestCreatedMessage{
		ShowTitle:      "Test Show",
		RequesterEmail: "requester@example.com",
		StartTime:      time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC),
		EndTime:        time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC),
		Notes:          "Bring headphones",
		DetailURL:      "https://aircover.example.com/sub-requests/42",
	}
}

func testSubRequestTakenMessage() SubRequestTakenMessage {
	return SubRequestTakenMessage{
		ShowTitle:  "Test Show",
		TakerEmail: "taker@example.com",
		StartTime:  time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC),
		EndTime:    time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC),
		DetailURL:  "https://aircover.example.com/sub-requests/42",
	}
}

func testSubRequestUntakenMessage() SubRequestUntakenMessage {
	return SubRequestUntakenMessage{
		ShowTitle:    "Test Show",
		UntakerEmail: "untaker@example.com",
		StartTime:    time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC),
		EndTime:      time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC),
		DetailURL:    "https://aircover.example.com/sub-requests/42",
	}
}

type fakeActiveUsers struct {
	users []*domain.User
	err   error
}

func (f *fakeActiveUsers) ListActiveUsers(ctx context.Context) ([]*domain.User, error) {
	return f.users, f.err
}

type fakeSubRequestSender struct {
	recipients      [][]string
	messages        []SubRequestCreatedMessage
	takenTo         []string
	takenCC         [][]string
	takenMessages   []SubRequestTakenMessage
	untakenTo       []string
	untakenCC       [][]string
	untakenMessages []SubRequestUntakenMessage
	err             error
	takenErr        error
	untakenErr      error
}

func (f *fakeSubRequestSender) SendMagicLink(toEmail, magicLink string) error {
	return nil
}

func (f *fakeSubRequestSender) SendSubRequestCreated(bccEmails []string, message SubRequestCreatedMessage) error {
	f.recipients = append(f.recipients, append([]string(nil), bccEmails...))
	f.messages = append(f.messages, message)
	return f.err
}

func (f *fakeSubRequestSender) SendSubRequestTaken(toEmail string, ccEmails []string, message SubRequestTakenMessage) error {
	f.takenTo = append(f.takenTo, toEmail)
	f.takenCC = append(f.takenCC, append([]string(nil), ccEmails...))
	f.takenMessages = append(f.takenMessages, message)
	return f.takenErr
}

func (f *fakeSubRequestSender) SendSubRequestUntaken(toEmail string, ccEmails []string, message SubRequestUntakenMessage) error {
	f.untakenTo = append(f.untakenTo, toEmail)
	f.untakenCC = append(f.untakenCC, append([]string(nil), ccEmails...))
	f.untakenMessages = append(f.untakenMessages, message)
	return f.untakenErr
}

type fakeNotifier struct {
	event        subrequestsapp.SubRequestCreatedEvent
	takenEvent   subrequestsapp.SubRequestTakenEvent
	untakenEvent subrequestsapp.SubRequestUntakenEvent
	err          error
	done         chan struct{}
}

func (f *fakeNotifier) SubRequestCreated(ctx context.Context, event subrequestsapp.SubRequestCreatedEvent) error {
	f.event = event
	if f.done != nil {
		f.done <- struct{}{}
	}
	return f.err
}

func (f *fakeNotifier) SubRequestTaken(ctx context.Context, event subrequestsapp.SubRequestTakenEvent) error {
	f.takenEvent = event
	if f.done != nil {
		f.done <- struct{}{}
	}
	return f.err
}

func (f *fakeNotifier) SubRequestUntaken(ctx context.Context, event subrequestsapp.SubRequestUntakenEvent) error {
	f.untakenEvent = event
	if f.done != nil {
		f.done <- struct{}{}
	}
	return f.err
}
