package logger

import (
	"log/slog"

	"air-cover/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a new slog.Logger backed by Zap.
func NewLogger(cfg *config.Config) *slog.Logger {
	production := cfg.ENV == "production"
	var zcfg zap.Config
	if production {
		zcfg = zap.NewProductionConfig()
		// GCP Cloud Logging expects "severity" (uppercase) instead of "level" (lowercase)
		zcfg.EncoderConfig.LevelKey = "severity"
		zcfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
		zcfg.EncoderConfig.MessageKey = "message"
		zcfg.EncoderConfig.TimeKey = "timestamp"
		zcfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		zcfg = zap.NewDevelopmentConfig()
	}

	// Build the Zap logger
	zl, err := zcfg.Build()
	if err != nil {
		// Fallback to default slog if zap fails (unlikely)
		return slog.Default()
	}

	// Create a slog handler backed by Zap
	handler := zapslog.NewHandler(zl.Core(), zapslog.WithCaller(true))

	return slog.New(handler)
}
