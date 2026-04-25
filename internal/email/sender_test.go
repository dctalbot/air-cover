package email

import "testing"

func TestSendGridSender(t *testing.T) {
	s := NewSender("fake-key", "production")
	// Send to console if empty key
	s2 := NewSender("", "production")
	_ = s2.SendMagicLink("test@example.com", "http://test")

	s3 := NewSender("", "development")
	_ = s3.SendMagicLink("test@example.com", "http://test")

	// We won't actually hit the SendGrid API in unit tests since it requires network and valid keys.
	// But we can test that it builds correctly.
	sg, ok := s.(*SendGridSender)
	if !ok {
		t.Fatal("expected SendGridSender")
	}
	if sg.APIKey != "fake-key" {
		t.Fatal("expected fake-key")
	}
}
