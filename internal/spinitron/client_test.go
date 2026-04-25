package spinitron

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNewClient(t *testing.T) {
	apiKey := "test-api-key"
	client := NewClient(apiKey, "https://proxy.example.test/spinitron")

	if client.APIKey != apiKey {
		t.Errorf("expected API key %s, got %s", apiKey, client.APIKey)
	}

	if client.BaseURL != "https://proxy.example.test/spinitron" {
		t.Errorf("expected configured base URL, got %s", client.BaseURL)
	}

	if client.HTTPClient == nil {
		t.Error("expected HTTP client to be initialized")
	}
}

func TestNewClient_DefaultBaseURL(t *testing.T) {
	client := NewClient("k", "")
	if client.BaseURL != defaultBaseURL {
		t.Errorf("expected default base URL %s, got %s", defaultBaseURL, client.BaseURL)
	}
}

func TestGetShowsPage(t *testing.T) {
	client := NewClient("token-123", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/shows" {
				t.Fatalf("expected /api/shows route, got %s", r.URL.Path)
			}
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Fatalf("expected page=1, got %s", got)
			}
			if got := r.URL.Query().Get("count"); got != "200" {
				t.Fatalf("expected count=200, got %s", got)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
				t.Fatalf("expected bearer auth, got %q", got)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
			"items":[{"id":1,"title":"Morning Show"},{"id":"2","name":"Drive Time"}],
			"_links":{"next":{"href":"https://proxy.example.test/shows?page=2"}}
		}`)),
			}, nil
		}),
	}

	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(page.Items) != 2 {
		t.Fatalf("expected 2 shows, got %d", len(page.Items))
	}
	if page.Items[0].ID != "1" || page.Items[0].Title != "Morning Show" {
		t.Fatalf("unexpected first show: %+v", page.Items[0])
	}
	if page.Items[1].ID != "2" || page.Items[1].Title != "Drive Time" {
		t.Fatalf("unexpected second show: %+v", page.Items[1])
	}
	if page.NextPage == nil || *page.NextPage != 2 {
		t.Fatalf("expected next page 2, got %+v", page.NextPage)
	}
}

func TestGetShowsPage_LinkHeaderFallback(t *testing.T) {
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			headers := make(http.Header)
			headers.Set("Link", `<https://proxy.example.test/shows?page=3>; rel="next"`)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     headers,
				Body:       io.NopCloser(strings.NewReader(`{"items":[{"id":7,"title":"Late Night"}]}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 2)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if page.NextPage == nil || *page.NextPage != 3 {
		t.Fatalf("expected next page 3, got %+v", page.NextPage)
	}
}

func TestGetShowsPage_MetaCurrentPageDoesNotImplyNextPage(t *testing.T) {
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
					"items":[{"id":7,"title":"Late Night"}],
					"meta":{"page":1}
				}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if page.NextPage != nil {
		t.Fatalf("expected no next page, got %+v", page.NextPage)
	}
}

func TestGetShowsPage_ErrorStatus(t *testing.T) {
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("upstream failed")),
			}, nil
		}),
	}
	_, err := client.GetShowsPage(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for non-2xx upstream status")
	}
	if !strings.Contains(err.Error(), "unexpected status code 502") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}
