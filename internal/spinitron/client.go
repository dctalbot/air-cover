package spinitron

import (
	"net/http"
)

// Client is used to interact with the Spinitron API.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewClient creates a new Spinitron API client.
func NewClient(apiKey string) *Client {
	return &Client{
		BaseURL:    "https://spinitron.com/api",
		APIKey:     apiKey,
		HTTPClient: &http.Client{},
	}
}

// TODO: Add methods for getting personas, shows, etc.
