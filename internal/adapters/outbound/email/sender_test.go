package email

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/resend/resend-go/v3"
)

func TestConsoleSender(t *testing.T) {
	s := &ConsoleSender{}
	if err := s.SendMagicLink("test@example.com", "http://example.com/verify?token=abc"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestResendSender_Success(t *testing.T) {
	emails := &fakeResendEmails{}
	s := &ResendSender{FromEmail: "noreply@example.com", Emails: emails}

	if err := s.SendMagicLink("test@example.com", "http://example.com/verify?token=abc"); err != nil {
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
	if emails.request.Subject != "Your Air Cover Login Link" {
		t.Fatalf("unexpected subject: %s", emails.request.Subject)
	}
	if emails.request.Text == "" || emails.request.Html == "" {
		t.Fatal("expected text and HTML content")
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

func TestSendGridSender_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
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
