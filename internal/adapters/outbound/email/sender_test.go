package email

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	subrequestsapp "air-cover/internal/app/subrequests"
	"air-cover/internal/domain"
	"github.com/resend/resend-go/v3"
)

func TestConsoleSender(t *testing.T) {
	s := &ConsoleSender{}
	if err := s.SendMagicLink("test@example.com", "http://example.com/verify?token=abc"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if err := s.SendSubRequestCreated("test@example.com", testSubRequestMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestResendSender_Success(t *testing.T) {
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
	s := &ResendSender{
		FromEmail: "noreply@example.com",
		Emails:    &fakeResendEmails{err: errors.New("resend down")},
	}

	if err := s.SendMagicLink("test@example.com", "http://example.com/verify?token=abc"); err == nil {
		t.Error("expected error for resend failure")
	}
}

func TestResendSender_SubRequestCreated(t *testing.T) {
	emails := &fakeResendEmails{}
	s := &ResendSender{FromEmail: "noreply@example.com", Emails: emails}

	if err := s.SendSubRequestCreated("active@example.com", testSubRequestMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if emails.request == nil {
		t.Fatal("expected email request")
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
	s := &ResendSender{
		FromEmail: "noreply@example.com",
		Emails:    &fakeResendEmails{err: errors.New("resend down")},
	}
	if err := s.SendSubRequestCreated("active@example.com", testSubRequestMessage()); err == nil {
		t.Fatal("expected resend error")
	}
}

func TestSendGridSender_Success(t *testing.T) {
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

	if err := s.SendSubRequestCreated("active@example.com", testSubRequestMessage()); err != nil {
		t.Errorf("expected no error, got %v", err)
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

func TestSendGridSender_SubRequestCreatedError(t *testing.T) {
	s := &SendGridSender{
		APIKey: "test-key",
		HTTPClient: &http.Client{
			Transport: &errorTransport{},
		},
	}
	if err := s.SendSubRequestCreated("active@example.com", testSubRequestMessage()); err == nil {
		t.Fatal("expected sendgrid error")
	}
}

func TestSendGridSender_HTTPError(t *testing.T) {
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
	Subject string `json:"subject"`
	Content []struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"content"`
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

func TestSubRequestNotifier(t *testing.T) {
	users := &fakeActiveUsers{users: []*domain.User{
		{Email: "one@example.com", IsEnabled: true},
		{Email: "two@example.com", IsEnabled: true},
	}}
	sender := &fakeSubRequestSender{errFor: "two@example.com"}
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
	if len(sender.messages) != 2 {
		t.Fatalf("expected two send attempts, got %d", len(sender.messages))
	}
	if sender.messages[0].DetailURL != "https://aircover.example.com/sub-requests/42" {
		t.Fatalf("detail URL = %q", sender.messages[0].DetailURL)
	}
}

func TestSubRequestNotifierRelativeDetailURL(t *testing.T) {
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

func TestSubRequestNotifierConfigurationErrors(t *testing.T) {
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
}

func TestAsyncNotifier(t *testing.T) {
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

func TestAsyncNotifierDefaultBufferAndErrors(t *testing.T) {
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
	notifier := NewAsyncNotifier(nil, 1)
	notifier.Start()
	if err := notifier.SubRequestCreated(context.Background(), subrequestsapp.SubRequestCreatedEvent{}); err != nil {
		t.Fatalf("expected no-op without next notifier, got %v", err)
	}
	if notifier.Done() == nil {
		t.Fatal("expected done channel")
	}
}

func TestAsyncNotifierQueueFull(t *testing.T) {
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

func TestAsyncNotifierNil(t *testing.T) {
	if err := (*AsyncNotifier)(nil).SubRequestCreated(context.Background(), subrequestsapp.SubRequestCreatedEvent{}); err != nil {
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

type fakeActiveUsers struct {
	users []*domain.User
	err   error
}

func (f *fakeActiveUsers) ListActiveUsers(ctx context.Context) ([]*domain.User, error) {
	return f.users, f.err
}

type fakeSubRequestSender struct {
	messages []SubRequestCreatedMessage
	errFor   string
}

func (f *fakeSubRequestSender) SendMagicLink(toEmail, magicLink string) error {
	return nil
}

func (f *fakeSubRequestSender) SendSubRequestCreated(toEmail string, message SubRequestCreatedMessage) error {
	f.messages = append(f.messages, message)
	if toEmail == f.errFor {
		return errors.New("send failed")
	}
	return nil
}

type fakeNotifier struct {
	event subrequestsapp.SubRequestCreatedEvent
	err   error
	done  chan struct{}
}

func (f *fakeNotifier) SubRequestCreated(ctx context.Context, event subrequestsapp.SubRequestCreatedEvent) error {
	f.event = event
	if f.done != nil {
		f.done <- struct{}{}
	}
	return f.err
}
