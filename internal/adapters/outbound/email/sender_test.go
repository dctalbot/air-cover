package email

import (
	"errors"
	"io"
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
	s := NewSender("fake-key", "noreply@example.com")
	sg, ok := s.(*SendGridSender)
	if !ok {
		t.Fatal("expected SendGridSender")
	}
	if sg.APIKey != "fake-key" {
		t.Fatalf("expected fake-key, got %s", sg.APIKey)
	}
	if sg.FromEmail != "noreply@example.com" {
		t.Fatalf("expected noreply@example.com, got %s", sg.FromEmail)
	}
	if sg.HTTPClient == nil {
		t.Fatal("expected HTTPClient to be initialized")
	}

	s2 := NewSender("", "noreply@example.com")
	if _, ok := s2.(*ConsoleSender); !ok {
		t.Fatal("expected ConsoleSender for empty key")
	}

	s3 := NewSender("any-key", "noreply@example.com")
	if _, ok := s3.(*SendGridSender); !ok {
		t.Fatal("expected SendGridSender for any non-empty key")
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
