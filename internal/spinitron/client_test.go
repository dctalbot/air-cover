package spinitron

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
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

func TestNewClient_WhitespaceBaseURL(t *testing.T) {
	client := NewClient("k", "   ")
	if client.BaseURL != defaultBaseURL {
		t.Errorf("expected default base URL for whitespace-only input, got %s", client.BaseURL)
	}
}

func TestNewClient_TrailingSlash(t *testing.T) {
	client := NewClient("k", "https://example.com/api/")
	if strings.HasSuffix(client.BaseURL, "/") {
		t.Errorf("expected trailing slash to be trimmed, got %s", client.BaseURL)
	}
}

func TestGetShowsPage(t *testing.T) {
	client := NewClient("token-123", "https://proxy.example.test/api")
	now := time.Date(2026, time.January, 2, 15, 4, 5, 0, time.UTC)
	client.nowFunc = func() time.Time { return now }
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
			if got := r.URL.Query().Get("start"); got != "2026-01-02T15:04:05Z" {
				t.Fatalf("expected start query parameter to match current time, got %s", got)
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

func TestGetShowsPage_PageZeroNormalized(t *testing.T) {
	// page < 1 should be normalized to 1
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Fatalf("expected page=1 for input 0, got %s", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"items":[]}`)),
			}, nil
		}),
	}
	_, err := client.GetShowsPage(context.Background(), 0)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestGetShowsPage_NoAPIKey(t *testing.T) {
	// No API key → no Authorization header
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("expected no auth header, got %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"items":[]}`)),
			}, nil
		}),
	}
	_, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestGetShowsPage_RequestError(t *testing.T) {
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("connection refused")
		}),
	}
	_, err := client.GetShowsPage(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for connection failure")
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

func TestGetShowsPage_MetaNextPage(t *testing.T) {
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
					"items":[{"id":7,"title":"Late Night"}],
					"meta":{"next_page":2}
				}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if page.NextPage == nil || *page.NextPage != 2 {
		t.Fatalf("expected next page 2, got %+v", page.NextPage)
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

func TestGetShowsPage_InvalidJSON(t *testing.T) {
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`not-json`)),
			}, nil
		}),
	}
	_, err := client.GetShowsPage(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetShowsPage_InvalidItemsJSON(t *testing.T) {
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"items":"not-an-array"}`)),
			}, nil
		}),
	}
	_, err := client.GetShowsPage(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for invalid items JSON")
	}
}

func TestGetShowsPage_DataKey(t *testing.T) {
	// API using "data" key instead of "items"
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":5,"title":"Data Show"}]}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "Data Show" {
		t.Fatalf("unexpected items: %+v", page.Items)
	}
}

func TestGetShowsPage_MissingItemsKey(t *testing.T) {
	// No "items" or "data" key → empty page
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"other":"stuff"}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected empty page, got %+v", page.Items)
	}
}

func TestGetShowsPage_ShowWithNoIDSkipped(t *testing.T) {
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				// null id → normalizeID returns "" → skipped
				Body: io.NopCloser(strings.NewReader(`{"items":[{"id":null,"title":"Ghost Show"},{"id":1,"title":"Real Show"}]}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "Real Show" {
		t.Fatalf("expected only Real Show, got %+v", page.Items)
	}
}

func TestGetShowsPage_DisplayNameTitle(t *testing.T) {
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"items":[{"id":1,"display_name":"Display Show"}]}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "Display Show" {
		t.Fatalf("expected Display Show, got %+v", page.Items)
	}
}

func TestGetShowsPage_ShowFallbackToID(t *testing.T) {
	// No title/name/display_name → fallback to ID string
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"items":[{"id":42}]}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "42" {
		t.Fatalf("expected ID as title fallback, got %+v", page.Items)
	}
}

func TestGetShowsPage_ShowFallbackToUnknown(t *testing.T) {
	// No title/name/display_name and no id → "Unknown show"
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				// id is present but valid so we need a different scenario:
				// We need normalizeTitle to return "Unknown show" which requires id to be ""
				// That's hard since normalizeID will skip it. Let's force it via string id = ""
				Body: io.NopCloser(strings.NewReader(`{"items":[{"id":"","title":""}]}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// id="" means normalizeID returns "" → item is skipped
	if len(page.Items) != 0 {
		t.Fatalf("expected empty page for blank id, got %+v", page.Items)
	}
}

func TestNormalizeTitle_UnknownShow(t *testing.T) {
	// Directly test normalizeTitle with no valid keys and no id
	item := map[string]interface{}{
		"id":    nil,
		"title": "",
	}
	title := normalizeTitle(item)
	if title != "Unknown show" {
		t.Errorf("expected 'Unknown show', got %q", title)
	}
}

func TestNormalizeID_Types(t *testing.T) {
	if v := normalizeID("hello"); v != "hello" {
		t.Errorf("expected 'hello', got %q", v)
	}
	if v := normalizeID(float64(42)); v != "42" {
		t.Errorf("expected '42', got %q", v)
	}
	if v := normalizeID(nil); v != "" {
		t.Errorf("expected '', got %q", v)
	}
}

func TestGetShowsPage_NextPageAsNumber(t *testing.T) {
	// "next_page": 2 at top level
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"items":[{"id":1,"title":"Show"}],"next_page":2}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if page.NextPage == nil || *page.NextPage != 2 {
		t.Fatalf("expected next page 2, got %+v", page.NextPage)
	}
}

func TestGetShowsPage_NextPageAsString(t *testing.T) {
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"items":[{"id":1,"title":"Show"}],"next":"3"}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if page.NextPage == nil || *page.NextPage != 3 {
		t.Fatalf("expected next page 3, got %+v", page.NextPage)
	}
}

func TestGetShowsPage_NextPageAsURL(t *testing.T) {
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"items":[{"id":1,"title":"Show"}],"next":"https://example.com/api/shows?page=4"}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if page.NextPage == nil || *page.NextPage != 4 {
		t.Fatalf("expected next page 4, got %+v", page.NextPage)
	}
}

func TestGetShowsPage_NextPageAsArray(t *testing.T) {
	// links key is an array containing a page number
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"items":[{"id":1,"title":"Show"}],"links":[5]}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if page.NextPage == nil || *page.NextPage != 5 {
		t.Fatalf("expected next page 5, got %+v", page.NextPage)
	}
}

func TestGetShowsPage_NextPageNegative(t *testing.T) {
	// negative page → no next page
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"items":[{"id":1,"title":"Show"}],"next_page":-1}`)),
			}, nil
		}),
	}
	page, err := client.GetShowsPage(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if page.NextPage != nil {
		t.Fatalf("expected no next page for negative, got %+v", page.NextPage)
	}
}

func TestParseNextPageFromLinkHeader(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		wantPage *int
	}{
		{"empty", "", nil},
		{"no next rel", `<https://example.com?page=2>; rel="prev"`, nil},
		{"malformed no brackets", `rel="next"`, nil},
		{"valid", `<https://example.com?page=5>; rel="next"`, intPtr(5)},
		{"multiple parts", `<https://example.com?page=1>; rel="prev", <https://example.com?page=3>; rel="next"`, intPtr(3)},
		{"no page param", `<https://example.com/path>; rel="next"`, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNextPageFromLinkHeader(tt.header)
			if tt.wantPage == nil {
				if got != nil {
					t.Errorf("expected nil, got %v", *got)
				}
			} else {
				if got == nil || *got != *tt.wantPage {
					t.Errorf("expected page %d, got %v", *tt.wantPage, got)
				}
			}
		})
	}
}

func TestParsePageString(t *testing.T) {
	tests := []struct {
		input    string
		wantPage int
		wantOK   bool
	}{
		{"", 0, false},
		{"   ", 0, false},
		{"0", 0, false},
		{"-1", 0, false},
		{"abc", 0, false},
		{"2", 2, true},
		{"https://example.com?page=3", 3, true},
		{"https://example.com/noparam", 0, false},
		{"https://example.com?page=0", 0, false},
	}

	for _, tt := range tests {
		page, ok := parsePageString(tt.input)
		if ok != tt.wantOK || page != tt.wantPage {
			t.Errorf("parsePageString(%q) = (%d, %v), want (%d, %v)", tt.input, page, ok, tt.wantPage, tt.wantOK)
		}
	}
}

func TestPageFromGeneric_MapWithNextPageKey(t *testing.T) {
	// map with "next_page" key
	v := map[string]interface{}{
		"next_page": float64(7),
	}
	page := pageFromGeneric(v)
	if page == nil || *page != 7 {
		t.Errorf("expected page 7, got %v", page)
	}
}

func TestPageFromGeneric_MapWithHrefKey(t *testing.T) {
	v := map[string]interface{}{
		"href": "https://example.com?page=8",
	}
	page := pageFromGeneric(v)
	if page == nil || *page != 8 {
		t.Errorf("expected page 8, got %v", page)
	}
}

func TestPageFromGeneric_EmptyString(t *testing.T) {
	page := pageFromGeneric("")
	if page != nil {
		t.Errorf("expected nil for empty string, got %v", page)
	}
}

func TestParseNextPageFromMeta_InvalidJSON(t *testing.T) {
	// not-valid json bytes
	result := parseNextPageFromMeta([]byte("not-json"))
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %v", result)
	}
}

func TestPageFromGeneric_MapWithURLKey(t *testing.T) {
	// map with "url" key containing a page number in URL
	v := map[string]interface{}{
		"url": "https://example.com/api?page=9",
	}
	page := pageFromGeneric(v)
	if page == nil || *page != 9 {
		t.Errorf("expected page 9, got %v", page)
	}
}

func TestPageFromGeneric_MapWithPageKey(t *testing.T) {
	// map with "page" key (numeric)
	v := map[string]interface{}{
		"page": float64(6),
	}
	page := pageFromGeneric(v)
	if page == nil || *page != 6 {
		t.Errorf("expected page 6, got %v", page)
	}
}

func TestPageFromGeneric_MapWithNoUsefulKey(t *testing.T) {
	// map with no recognized keys → nil
	v := map[string]interface{}{
		"something": "else",
	}
	page := pageFromGeneric(v)
	if page != nil {
		t.Errorf("expected nil for map with no useful keys, got %v", page)
	}
}

func TestParseNextPageValue_InvalidJSON(t *testing.T) {
	// json.RawMessage with invalid content
	result := parseNextPageValue(json.RawMessage(`not-valid-json`))
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %v", result)
	}
}

func TestNormalizeID_JSONNumber(t *testing.T) {
	// json.Number type
	n := json.Number("42")
	if v := normalizeID(n); v != "42" {
		t.Errorf("expected '42' for json.Number, got %q", v)
	}
}

func TestPageFromGeneric_GetRequest_ReadBodyError(t *testing.T) {
	// Test the get() method when body reading fails
	client := NewClient("", "https://proxy.example.test/api")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       &errorReader{},
			}, nil
		}),
	}
	_, err := client.GetShowsPage(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for body read failure")
	}
}

// errorReader implements io.ReadCloser but always errors.
type errorReader struct{}

func (e *errorReader) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("read error")
}

func (e *errorReader) Close() error {
	return nil
}

func TestGetShowsPage_InvalidBaseURL(t *testing.T) {
	// An invalid base URL causes url.Parse to fail in get()
	client := &Client{
		BaseURL:    "://invalid-url",
		HTTPClient: &http.Client{},
	}
	_, err := client.GetShowsPage(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for invalid base URL")
	}
}

func TestPageFromGeneric_MapWithNextKey(t *testing.T) {
	// map with "next" key that is a string page URL
	v := map[string]interface{}{
		"next": "https://example.com/api?page=7",
	}
	page := pageFromGeneric(v)
	if page == nil || *page != 7 {
		t.Errorf("expected page 7, got %v", page)
	}
}

func TestPageFromGeneric_MapWithNextKeyThatIsNil(t *testing.T) {
	// map with "next" key that returns nil → falls through to other keys
	v := map[string]interface{}{
		"next":      nil,
		"next_page": float64(10),
	}
	page := pageFromGeneric(v)
	if page == nil || *page != 10 {
		t.Errorf("expected page 10 from fallback, got %v", page)
	}
}

func intPtr(i int) *int {
	return &i
}

func TestParsePageString_InvalidURL(t *testing.T) {
	// url.Parse error (usually requires non-ASCII or control chars)
	_, ok := parsePageString("https://example.com/%%")
	if ok {
		t.Error("expected ok=false for invalid URL")
	}
}

func TestParsePageString_NoPageParam(t *testing.T) {
	_, ok := parsePageString("https://example.com/api?notpage=1")
	if ok {
		t.Error("expected ok=false for no page param")
	}
}

func TestParsePageString_InvalidPageParam(t *testing.T) {
	_, ok := parsePageString("https://example.com/api?page=abc")
	if ok {
		t.Error("expected ok=false for invalid page param")
	}
}

func TestGet_RequestCreateError(t *testing.T) {
	// To trigger http.NewRequestWithContext error, we need an invalid method or URL.
	// But route is hardcoded and method is GET.
	// However, if we can pass a bad context? No.
}

func TestParsePageString_ControlChar(t *testing.T) {
	_, ok := parsePageString("http://example.com/\x01")
	if ok {
		t.Error("expected ok=false for control char in URL")
	}
}
