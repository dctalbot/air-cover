package logger

import (
	"errors"
	"log/slog"
	"testing"

	"air-cover/internal/platform/config"
	"go.uber.org/zap"
)

func TestNewLogger(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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

func TestNewLogger_BuildErrorFallback(t *testing.T) {
	originalBuildZapLogger := buildZapLogger
	t.Cleanup(func() { buildZapLogger = originalBuildZapLogger })
	buildZapLogger = func(cfg zapBuilder) (*zap.Logger, error) {
		return nil, errors.New("build failed")
	}

	logger := NewLogger(&config.Config{ENV: "production"})
	if logger != slog.Default() {
		t.Fatal("expected slog default fallback")
	}
}
