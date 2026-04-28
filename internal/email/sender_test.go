package email

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConsoleSender(t *testing.T) {
	s := &ConsoleSender{}
	if err := s.SendMagicLink("test@example.com", "http://example.com/verify?token=abc"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestSendGridSender_EmptyAPIKey(t *testing.T) {
	// Empty API key falls back to ConsoleSender
	s := &SendGridSender{APIKey: ""}
	if err := s.SendMagicLink("test@example.com", "http://example.com/verify?token=abc"); err != nil {
		t.Errorf("expected no error from console fallback, got %v", err)
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

	s := &SendGridSender{APIKey: "test-key"}

	// Use a custom http.Client that redirects to the test server
	origClient := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: &proxyTransport{target: srv.URL},
	}
	defer func() { http.DefaultClient = origClient }()

	if err := s.SendMagicLink("test@example.com", "http://example.com/verify?token=abc"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestSendGridSender_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := &SendGridSender{APIKey: "test-key"}

	origClient := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: &proxyTransport{target: srv.URL},
	}
	defer func() { http.DefaultClient = origClient }()

	if err := s.SendMagicLink("test@example.com", "http://example.com/verify?token=abc"); err == nil {
		t.Error("expected error for HTTP error status")
	}
}

func TestNewSender(t *testing.T) {
	// production + non-empty key → SendGridSender
	s := NewSender("fake-key", "production")
	sg, ok := s.(*SendGridSender)
	if !ok {
		t.Fatal("expected SendGridSender")
	}
	if sg.APIKey != "fake-key" {
		t.Fatalf("expected fake-key, got %s", sg.APIKey)
	}

	// production + empty key → ConsoleSender (not SendGridSender because key is empty)
	s2 := NewSender("", "production")
	if _, ok := s2.(*ConsoleSender); !ok {
		t.Fatal("expected ConsoleSender for empty key in production")
	}

	// non-production → ConsoleSender
	s3 := NewSender("any-key", "development")
	if _, ok := s3.(*ConsoleSender); !ok {
		t.Fatal("expected ConsoleSender for non-production env")
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
