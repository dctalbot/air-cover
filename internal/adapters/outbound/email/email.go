package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/resend/resend-go/v3"
)

type Sender interface {
	SendMagicLink(toEmail, magicLink string) error
	SendSubRequestCreated(bccEmails []string, message SubRequestCreatedMessage) error
	SendSubRequestTaken(toEmail string, ccEmails []string, message SubRequestTakenMessage) error
	SendSubRequestUntaken(toEmail string, ccEmails []string, message SubRequestUntakenMessage) error
}

type ConsoleSender struct{}

func (c *ConsoleSender) SendMagicLink(toEmail, magicLink string) error {
	slog.Info("Simulating email send", "to", toEmail, "magicLink", magicLink)
	return nil
}

func (c *ConsoleSender) SendSubRequestCreated(bccEmails []string, message SubRequestCreatedMessage) error {
	slog.Info("Simulating sub request notification email send", "bcc_count", len(bccEmails), "detailURL", message.DetailURL)
	return nil
}

func (c *ConsoleSender) SendSubRequestTaken(toEmail string, ccEmails []string, message SubRequestTakenMessage) error {
	slog.Info("Simulating sub request taken confirmation email send", "to", toEmail, "cc_count", len(ccEmails), "detailURL", message.DetailURL)
	return nil
}

func (c *ConsoleSender) SendSubRequestUntaken(toEmail string, ccEmails []string, message SubRequestUntakenMessage) error {
	slog.Info("Simulating sub request untaken confirmation email send", "to", toEmail, "cc_count", len(ccEmails), "detailURL", message.DetailURL)
	return nil
}

type resendEmailSender interface {
	Send(params *resend.SendEmailRequest) (*resend.SendEmailResponse, error)
}

type emailMessage struct {
	Subject string
	Preview string
	Heading string
	Body    []string
	CTA     emailCTA
	Footer  string
}

type emailCTA struct {
	Label string
	URL   string
}

type SubRequestCreatedMessage struct {
	ShowTitle      string
	RequesterEmail string
	StartTime      time.Time
	EndTime        time.Time
	Notes          string
	DetailURL      string
}

type SubRequestTakenMessage struct {
	ShowTitle  string
	TakerEmail string
	StartTime  time.Time
	EndTime    time.Time
	DetailURL  string
}

type SubRequestUntakenMessage struct {
	ShowTitle    string
	UntakerEmail string
	StartTime    time.Time
	EndTime      time.Time
	DetailURL    string
}

type renderedEmail struct {
	Subject string
	HTML    string
	Text    string
}

type ResendSender struct {
	FromEmail string
	Emails    resendEmailSender
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

func renderEmail(message emailMessage) renderedEmail {
	return renderedEmail{
		Subject: message.Subject,
		HTML:    renderHTML(message),
		Text:    renderText(message),
	}
}

func renderHTML(message emailMessage) string {
	var body strings.Builder
	body.WriteString(`<!doctype html><html><body style="margin:0;padding:0;background:#f8fafc;color:#0f172a;font-family:Arial,sans-serif;">`)
	if message.Preview != "" {
		body.WriteString(`<div style="display:none;max-height:0;overflow:hidden;">`)
		body.WriteString(html.EscapeString(message.Preview))
		body.WriteString(`</div>`)
	}
	body.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#f8fafc;padding:32px 16px;"><tr><td align="center">`)
	body.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:560px;background:#ffffff;border:1px solid #e2e8f0;border-radius:12px;padding:32px;">`)
	body.WriteString(`<tr><td style="font-size:14px;font-weight:700;letter-spacing:0.08em;text-transform:uppercase;color:#2563eb;padding-bottom:24px;">Air Cover</td></tr>`)
	body.WriteString(`<tr><td><h1 style="margin:0 0 16px;font-size:24px;line-height:1.25;color:#0f172a;">`)
	body.WriteString(html.EscapeString(message.Heading))
	body.WriteString(`</h1></td></tr>`)
	for _, paragraph := range message.Body {
		body.WriteString(`<tr><td><p style="margin:0 0 16px;font-size:16px;line-height:1.6;color:#334155;">`)
		body.WriteString(html.EscapeString(paragraph))
		body.WriteString(`</p></td></tr>`)
	}
	if message.CTA.Label != "" && message.CTA.URL != "" {
		escapedURL := html.EscapeString(message.CTA.URL)
		body.WriteString(`<tr><td style="padding:8px 0 24px;"><a href="`)
		body.WriteString(escapedURL)
		body.WriteString(`" style="display:inline-block;background:#2563eb;color:#ffffff;text-decoration:none;font-weight:700;padding:12px 18px;border-radius:8px;">`)
		body.WriteString(html.EscapeString(message.CTA.Label))
		body.WriteString(`</a></td></tr>`)
		body.WriteString(`<tr><td><p style="margin:0 0 16px;font-size:14px;line-height:1.6;color:#64748b;">If the button does not work, copy and paste this link into your browser:<br><a href="`)
		body.WriteString(escapedURL)
		body.WriteString(`" style="color:#2563eb;word-break:break-all;">`)
		body.WriteString(escapedURL)
		body.WriteString(`</a></p></td></tr>`)
	}
	if message.Footer != "" {
		body.WriteString(`<tr><td style="border-top:1px solid #e2e8f0;padding-top:16px;"><p style="margin:0;font-size:13px;line-height:1.5;color:#64748b;">`)
		body.WriteString(html.EscapeString(message.Footer))
		body.WriteString(`</p></td></tr>`)
	}
	body.WriteString(`</table></td></tr></table></body></html>`)
	return body.String()
}

func renderText(message emailMessage) string {
	var body strings.Builder
	body.WriteString("Air Cover\n\n")
	body.WriteString(message.Heading)
	body.WriteString("\n\n")
	for _, paragraph := range message.Body {
		body.WriteString(paragraph)
		body.WriteString("\n\n")
	}
	if message.CTA.Label != "" && message.CTA.URL != "" {
		body.WriteString(message.CTA.Label)
		body.WriteString(": ")
		body.WriteString(message.CTA.URL)
		body.WriteString("\n\n")
	}
	if message.Footer != "" {
		body.WriteString(message.Footer)
		body.WriteString("\n")
	}
	return body.String()
}

func magicLinkMessage(magicLink string) emailMessage {
	return emailMessage{
		Subject: "Log in to Air Cover",
		Preview: "Use your secure link to log in to Air Cover.",
		Heading: "Log in to Air Cover",
		Body: []string{
			"Use the secure link below to log in to Air Cover. This link expires soon and can only be used once.",
		},
		CTA: emailCTA{
			Label: "Log in to Air Cover",
			URL:   magicLink,
		},
		Footer: "If you did not request this email, you can ignore it.",
	}
}

func subRequestCreatedMessage(message SubRequestCreatedMessage) emailMessage {
	body := []string{
		fmt.Sprintf("%s posted a new sub request for %s.", message.RequesterEmail, message.ShowTitle),
		fmt.Sprintf("When: %s to %s", formatEmailTime(message.StartTime), formatEmailTime(message.EndTime)),
	}
	if strings.TrimSpace(message.Notes) != "" {
		body = append(body, "Notes: "+strings.TrimSpace(message.Notes))
	}

	return emailMessage{
		Subject: "New sub request: " + message.ShowTitle,
		Preview: "A new sub request is available in Air Cover.",
		Heading: "New sub request",
		Body:    body,
		CTA: emailCTA{
			Label: "View sub request",
			URL:   message.DetailURL,
		},
		Footer: "You are receiving this because you are an active Air Cover user.",
	}
}

func subRequestTakenMessage(message SubRequestTakenMessage) emailMessage {
	return emailMessage{
		Subject: "Sub request taken: " + message.ShowTitle,
		Preview: "Your sub request has been taken in Air Cover.",
		Heading: "Sub request taken",
		Body: []string{
			fmt.Sprintf("%s took your sub request for %s.", message.TakerEmail, message.ShowTitle),
			fmt.Sprintf("When: %s to %s", formatEmailTime(message.StartTime), formatEmailTime(message.EndTime)),
		},
		CTA: emailCTA{
			Label: "View sub request",
			URL:   message.DetailURL,
		},
		Footer: "You are receiving this because you posted this sub request in Air Cover.",
	}
}

func subRequestUntakenMessage(message SubRequestUntakenMessage) emailMessage {
	return emailMessage{
		Subject: "Sub request no longer covered: " + message.ShowTitle,
		Preview: "Your sub request is no longer covered in Air Cover.",
		Heading: "Sub request no longer covered",
		Body: []string{
			fmt.Sprintf("%s is no longer covering your sub request for %s.", message.UntakerEmail, message.ShowTitle),
			fmt.Sprintf("When: %s to %s", formatEmailTime(message.StartTime), formatEmailTime(message.EndTime)),
		},
		CTA: emailCTA{
			Label: "View sub request",
			URL:   message.DetailURL,
		},
		Footer: "You are receiving this because you posted this sub request in Air Cover.",
	}
}

func formatEmailTime(value time.Time) string {
	return value.Format("Jan 2, 2006 3:04 PM")
}

func (s *ResendSender) SendMagicLink(toEmail, magicLink string) error {
	if err := s.send(toEmail, renderEmail(magicLinkMessage(magicLink))); err != nil {
		return err
	}

	slog.Info("Successfully sent magic link via Resend", "to", toEmail)
	return nil
}

func (s *ResendSender) SendSubRequestCreated(bccEmails []string, subRequestMessage SubRequestCreatedMessage) error {
	if len(bccEmails) == 0 {
		return nil
	}
	if err := s.sendBCC(bccEmails, renderEmail(subRequestCreatedMessage(subRequestMessage))); err != nil {
		return fmt.Errorf("failed to send sub request email via resend: %w", err)
	}

	slog.Info("Successfully sent sub request notification via Resend", "bcc_count", len(bccEmails))
	return nil
}

func (s *ResendSender) SendSubRequestTaken(toEmail string, ccEmails []string, subRequestMessage SubRequestTakenMessage) error {
	if toEmail == "" {
		return nil
	}
	if err := s.sendCC(toEmail, ccEmails, renderEmail(subRequestTakenMessage(subRequestMessage))); err != nil {
		return fmt.Errorf("failed to send sub request taken email via resend: %w", err)
	}

	slog.Info("Successfully sent sub request taken notification via Resend", "to", toEmail, "cc_count", len(ccEmails))
	return nil
}

func (s *ResendSender) SendSubRequestUntaken(toEmail string, ccEmails []string, subRequestMessage SubRequestUntakenMessage) error {
	if toEmail == "" {
		return nil
	}
	if err := s.sendCC(toEmail, ccEmails, renderEmail(subRequestUntakenMessage(subRequestMessage))); err != nil {
		return fmt.Errorf("failed to send sub request untaken email via resend: %w", err)
	}

	slog.Info("Successfully sent sub request untaken notification via Resend", "to", toEmail, "cc_count", len(ccEmails))
	return nil
}

func (s *ResendSender) send(toEmail string, message renderedEmail) error {
	if _, err := s.Emails.Send(&resend.SendEmailRequest{
		From:    s.FromEmail,
		To:      []string{toEmail},
		Subject: message.Subject,
		Html:    message.HTML,
		Text:    message.Text,
	}); err != nil {
		return fmt.Errorf("failed to send email via resend: %w", err)
	}
	return nil
}

func (s *ResendSender) sendCC(toEmail string, ccEmails []string, message renderedEmail) error {
	if _, err := s.Emails.Send(&resend.SendEmailRequest{
		From:    s.FromEmail,
		To:      []string{toEmail},
		Cc:      ccEmails,
		Subject: message.Subject,
		Html:    message.HTML,
		Text:    message.Text,
	}); err != nil {
		return fmt.Errorf("failed to send email via resend: %w", err)
	}
	return nil
}

func (s *ResendSender) sendBCC(bccEmails []string, message renderedEmail) error {
	if _, err := s.Emails.Send(&resend.SendEmailRequest{
		From:    s.FromEmail,
		To:      []string{s.FromEmail},
		Bcc:     bccEmails,
		Subject: message.Subject,
		Html:    message.HTML,
		Text:    message.Text,
	}); err != nil {
		return fmt.Errorf("failed to send email via resend: %w", err)
	}
	return nil
}

func (s *SendGridSender) SendMagicLink(toEmail, magicLink string) error {
	if err := s.send(toEmail, renderEmail(magicLinkMessage(magicLink))); err != nil {
		return err
	}

	slog.Info("Successfully sent magic link via SendGrid", "to", toEmail)
	return nil
}

func (s *SendGridSender) SendSubRequestCreated(bccEmails []string, subRequestMessage SubRequestCreatedMessage) error {
	if len(bccEmails) == 0 {
		return nil
	}
	if err := s.sendBCC(bccEmails, renderEmail(subRequestCreatedMessage(subRequestMessage))); err != nil {
		return fmt.Errorf("failed to send sub request email via sendgrid: %w", err)
	}

	slog.Info("Successfully sent sub request notification via SendGrid", "bcc_count", len(bccEmails))
	return nil
}

func (s *SendGridSender) SendSubRequestTaken(toEmail string, ccEmails []string, subRequestMessage SubRequestTakenMessage) error {
	if toEmail == "" {
		return nil
	}
	if err := s.sendCC(toEmail, ccEmails, renderEmail(subRequestTakenMessage(subRequestMessage))); err != nil {
		return fmt.Errorf("failed to send sub request taken email via sendgrid: %w", err)
	}

	slog.Info("Successfully sent sub request taken notification via SendGrid", "to", toEmail, "cc_count", len(ccEmails))
	return nil
}

func (s *SendGridSender) SendSubRequestUntaken(toEmail string, ccEmails []string, subRequestMessage SubRequestUntakenMessage) error {
	if toEmail == "" {
		return nil
	}
	if err := s.sendCC(toEmail, ccEmails, renderEmail(subRequestUntakenMessage(subRequestMessage))); err != nil {
		return fmt.Errorf("failed to send sub request untaken email via sendgrid: %w", err)
	}

	slog.Info("Successfully sent sub request untaken notification via SendGrid", "to", toEmail, "cc_count", len(ccEmails))
	return nil
}

func (s *SendGridSender) send(toEmail string, message renderedEmail) error {
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
		"subject": message.Subject,
		"content": []map[string]string{
			{
				"type":  "text/plain",
				"value": message.Text,
			},
			{
				"type":  "text/html",
				"value": message.HTML,
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
	return nil
}

func (s *SendGridSender) sendCC(toEmail string, ccEmails []string, message renderedEmail) error {
	personalization := map[string]interface{}{
		"to": []map[string]string{
			{"email": toEmail},
		},
	}
	if len(ccEmails) > 0 {
		cc := make([]map[string]string, 0, len(ccEmails))
		for _, email := range ccEmails {
			cc = append(cc, map[string]string{"email": email})
		}
		personalization["cc"] = cc
	}

	payload := map[string]interface{}{
		"personalizations": []map[string]interface{}{personalization},
		"from": map[string]string{
			"email": s.FromEmail,
			"name":  "Air Cover",
		},
		"subject": message.Subject,
		"content": []map[string]string{
			{
				"type":  "text/plain",
				"value": message.Text,
			},
			{
				"type":  "text/html",
				"value": message.HTML,
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
	return nil
}

func (s *SendGridSender) sendBCC(bccEmails []string, message renderedEmail) error {
	bcc := make([]map[string]string, 0, len(bccEmails))
	for _, email := range bccEmails {
		bcc = append(bcc, map[string]string{"email": email})
	}

	payload := map[string]interface{}{
		"personalizations": []map[string]interface{}{
			{
				"to": []map[string]string{
					{"email": s.FromEmail},
				},
				"bcc": bcc,
			},
		},
		"from": map[string]string{
			"email": s.FromEmail,
			"name":  "Air Cover",
		},
		"subject": message.Subject,
		"content": []map[string]string{
			{
				"type":  "text/plain",
				"value": message.Text,
			},
			{
				"type":  "text/html",
				"value": message.HTML,
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
	return nil
}

func NewSender(resendAPIKey, sendGridAPIKey, fromEmail string) Sender {
	if resendAPIKey != "" {
		return &ResendSender{
			FromEmail: fromEmail,
			Emails:    resend.NewClient(resendAPIKey).Emails,
		}
	}
	if sendGridAPIKey != "" {
		return &SendGridSender{
			APIKey:    sendGridAPIKey,
			FromEmail: fromEmail,
			HTTPClient: &http.Client{
				Timeout: 10 * time.Second,
			},
		}
	}
	return &ConsoleSender{}
}
