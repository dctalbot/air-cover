package spinitron

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client is used to interact with the Spinitron API.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	nowFunc    func() time.Time
}

const (
	defaultBaseURL       = "https://spinitron.com/api"
	defaultShowsPageSize = 200
)

type Show struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type ShowsPage struct {
	Items    []Show `json:"items"`
	NextPage *int   `json:"next_page,omitempty"`
}

type Persona struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type PersonasPage struct {
	Items    []Persona `json:"items"`
	NextPage *int      `json:"next_page,omitempty"`
}

// NewClient creates a new Spinitron API client.
func NewClient(apiKey, baseURL string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		HTTPClient: &http.Client{},
		nowFunc:    time.Now,
	}
}

func (c *Client) GetShowsPage(ctx context.Context, page int) (ShowsPage, error) {
	if page < 1 {
		page = 1
	}

	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("count", strconv.Itoa(defaultShowsPageSize))
	now := time.Now
	if c.nowFunc != nil {
		now = c.nowFunc
	}
	query.Set("start", now().UTC().Format(time.RFC3339))

	body, headers, err := c.get(ctx, "/shows", query)
	if err != nil {
		return ShowsPage{}, err
	}

	return parseShowsPage(body, headers)
}

func (c *Client) GetPersonasPage(ctx context.Context, page int) (PersonasPage, error) {
	if page < 1 {
		page = 1
	}

	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("count", strconv.Itoa(defaultShowsPageSize))

	body, headers, err := c.get(ctx, "/personas", query)
	if err != nil {
		return PersonasPage{}, err
	}

	return parsePersonasPage(body, headers)
}

func (c *Client) get(ctx context.Context, route string, query url.Values) ([]byte, http.Header, error) {
	endpoint := c.BaseURL + "/" + strings.TrimLeft(route, "/")
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse request URL: %w", err)
	}
	reqURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return body, resp.Header, nil
}

func parseShowsPage(body []byte, headers http.Header) (ShowsPage, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return ShowsPage{}, fmt.Errorf("failed to decode shows response: %w", err)
	}

	itemsRaw, ok := payload["items"]
	if !ok {
		itemsRaw = payload["data"]
	}
	if len(itemsRaw) == 0 {
		return ShowsPage{}, nil
	}

	var rawItems []map[string]interface{}
	if err := json.Unmarshal(itemsRaw, &rawItems); err != nil {
		return ShowsPage{}, fmt.Errorf("failed to decode show list: %w", err)
	}

	items := make([]Show, 0, len(rawItems))
	for _, item := range rawItems {
		id := normalizeID(item["id"])
		if id == "" {
			continue
		}

		title := normalizeTitle(item)
		items = append(items, Show{
			ID:    id,
			Title: title,
		})
	}

	return ShowsPage{
		Items:    items,
		NextPage: findNextPage(payload, headers),
	}, nil
}

func parsePersonasPage(body []byte, headers http.Header) (PersonasPage, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return PersonasPage{}, fmt.Errorf("failed to decode personas response: %w", err)
	}

	itemsRaw, ok := payload["items"]
	if !ok {
		itemsRaw = payload["data"]
	}
	if len(itemsRaw) == 0 {
		return PersonasPage{}, nil
	}

	var rawItems []map[string]interface{}
	if err := json.Unmarshal(itemsRaw, &rawItems); err != nil {
		return PersonasPage{}, fmt.Errorf("failed to decode persona list: %w", err)
	}

	items := make([]Persona, 0, len(rawItems))
	for _, item := range rawItems {
		idStr := normalizeID(item["id"])
		if idStr == "" {
			continue
		}
		id, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}

		name := normalizeTitle(item)

		var email string
		if rawEmail, ok := item["email"]; ok {
			if e, ok := rawEmail.(string); ok {
				email = e
			}
		}

		items = append(items, Persona{
			ID:    id,
			Name:  name,
			Email: email,
		})
	}

	return PersonasPage{
		Items:    items,
		NextPage: findNextPage(payload, headers),
	}, nil
}

func normalizeID(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return value
	case float64:
		return strconv.Itoa(int(value))
	case json.Number:
		return value.String()
	default:
		return ""
	}
}

func normalizeTitle(item map[string]interface{}) string {
	for _, key := range []string{"title", "name", "display_name"} {
		if raw, ok := item[key]; ok {
			if title, ok := raw.(string); ok && title != "" {
				return title
			}
		}
	}

	if id := normalizeID(item["id"]); id != "" {
		return id
	}
	return "Unknown show"
}

func findNextPage(payload map[string]json.RawMessage, headers http.Header) *int {
	for _, key := range []string{"next_page", "next"} {
		if raw, ok := payload[key]; ok {
			if page := parseNextPageValue(raw); page != nil {
				return page
			}
		}
	}

	for _, key := range []string{"_links", "links"} {
		if raw, ok := payload[key]; ok {
			if page := parseNextPageValue(raw); page != nil {
				return page
			}
		}
	}

	if metaRaw, ok := payload["meta"]; ok {
		if page := parseNextPageFromMeta(metaRaw); page != nil {
			return page
		}
	}

	return parseNextPageFromLinkHeader(headers.Get("Link"))
}

func parseNextPageFromMeta(raw json.RawMessage) *int {
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil
	}

	for _, key := range []string{"next_page", "next"} {
		if field, ok := meta[key]; ok {
			if page := parseNextPageValue(field); page != nil {
				return page
			}
		}
	}

	return nil
}

func parseNextPageValue(raw json.RawMessage) *int {
	var generic interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil
	}
	return pageFromGeneric(generic)
}

func pageFromGeneric(value interface{}) *int {
	switch v := value.(type) {
	case nil:
		return nil
	case float64:
		page := int(v)
		if page > 0 {
			return &page
		}
	case string:
		if page, ok := parsePageString(v); ok {
			return &page
		}
	case map[string]interface{}:
		if next, ok := v["next"]; ok {
			if page := pageFromGeneric(next); page != nil {
				return page
			}
		}
		for _, key := range []string{"next_page", "page", "href", "url"} {
			if field, ok := v[key]; ok {
				if page := pageFromGeneric(field); page != nil {
					return page
				}
			}
		}
	case []interface{}:
		for _, part := range v {
			if page := pageFromGeneric(part); page != nil {
				return page
			}
		}
	}

	return nil
}

func parsePageString(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}

	if page, err := strconv.Atoi(raw); err == nil && page > 0 {
		return page, true
	}

	parsedURL, err := url.Parse(raw)
	if err != nil {
		return 0, false
	}
	pageRaw := parsedURL.Query().Get("page")
	if pageRaw == "" {
		return 0, false
	}
	page, err := strconv.Atoi(pageRaw)
	if err != nil || page <= 0 {
		return 0, false
	}
	return page, true
}

func parseNextPageFromLinkHeader(linkHeader string) *int {
	if linkHeader == "" {
		return nil
	}

	parts := strings.Split(linkHeader, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if !strings.Contains(trimmed, `rel="next"`) {
			continue
		}
		start := strings.Index(trimmed, "<")
		end := strings.Index(trimmed, ">")
		if start < 0 || end <= start+1 {
			continue
		}

		if page, ok := parsePageString(trimmed[start+1 : end]); ok {
			return &page
		}
	}

	return nil
}
