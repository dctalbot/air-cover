package logger

import (
	"testing"

	"air-cover/internal/config"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name string
		env  string
	}{
		{
			name: "production",
			env:  "production",
		},
		{
			name: "development",
			env:  "development",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				ENV: tt.env,
			}
			logger := NewLogger(cfg)
			if logger == nil {
				t.Fatal("expected logger to not be nil")
			}
			logger.Info("test message", "env", tt.env)
		})
	}
}
