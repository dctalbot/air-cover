package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type Sender interface {
	SendMagicLink(toEmail, magicLink string) error
}

type ConsoleSender struct{}

func (c *ConsoleSender) SendMagicLink(toEmail, magicLink string) error {
	slog.Info("Simulating email send", "to", toEmail, "magicLink", magicLink)
	return nil
}

type SendGridSender struct {
	APIKey     string
	FromEmail  string
	HTTPClient *http.Client
}

var (
	jsonMarshal    = json.Marshal
	newHTTPRequest = func(method, url string, body io.Reader) (*http.Request, error) {
		return http.NewRequest(method, url, body) //nolint:noctx
	}
)

func (s *SendGridSender) SendMagicLink(toEmail, magicLink string) error {
	// SendGrid v3 API minimal payload
	payload := map[string]interface{}{
		"personalizations": []map[string]interface{}{
			{
				"to": []map[string]string{
					{"email": toEmail},
				},
			},
		},
		"from": map[string]string{
			"email": s.FromEmail,
			"name":  "Air Cover",
		},
		"subject": "Your Air Cover Login Link",
		"content": []map[string]string{
			{
				"type":  "text/plain",
				"value": fmt.Sprintf("Click here to log in: %s", magicLink),
			},
		},
	}

	body, err := jsonMarshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal sendgrid payload: %w", err)
	}

	req, err := newHTTPRequest("POST", "https://api.sendgrid.com/v3/mail/send", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create sendgrid request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email via sendgrid: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("sendgrid api returned error status: %d", resp.StatusCode)
	}

	slog.Info("Successfully sent magic link via SendGrid", "to", toEmail)
	return nil
}

func NewSender(apiKey, fromEmail string) Sender {
	if apiKey != "" {
		return &SendGridSender{
			APIKey:    apiKey,
			FromEmail: fromEmail,
			HTTPClient: &http.Client{
				Timeout: 10 * time.Second,
			},
		}
	}
	return &ConsoleSender{}
}
