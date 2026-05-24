package email

import "testing"

func TestNewSender(t *testing.T) {
	if _, ok := NewSender("", "", "development").(*ConsoleSender); !ok {
		t.Fatal("expected console sender outside production")
	}
	if _, ok := NewSender("key", "from@example.com", "production").(*SendGridSender); !ok {
		t.Fatal("expected SendGrid sender in production with API key")
	}
}
