package spinitron

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	apiKey := "test-api-key"
	client := NewClient(apiKey)

	if client.APIKey != apiKey {
		t.Errorf("expected API key %s, got %s", apiKey, client.APIKey)
	}

	if client.BaseURL != "https://spinitron.com/api" {
		t.Errorf("expected base URL https://spinitron.com/api, got %s", client.BaseURL)
	}

	if client.HTTPClient == nil {
		t.Error("expected HTTP client to be initialized")
	}
}
