package api

import (
	"testing"
	"time"

	"air-cover/internal/presenter"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{30 * time.Minute, "30 min"},
		{59 * time.Minute, "59 min"},
		{60 * time.Minute, "1 hours"},
		{90 * time.Minute, "1.5 hours"},
		{105 * time.Minute, "1.75 hours"},
		{120 * time.Minute, "2 hours"},
		{125 * time.Minute, "2.08 hours"}, // 125/60 = 2.08333... rounded to 2 decimal places is 2.08
		{126 * time.Minute, "2.1 hours"},  // 126/60 = 2.1
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := presenter.FormatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}
