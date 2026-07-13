package logging

import (
	"context"
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"bogus", slog.LevelInfo},
	}
	for _, tt := range tests {
		if got := parseLevel(tt.in); got != tt.want {
			t.Errorf("parseLevel(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestNewRespectsLevel(t *testing.T) {
	logger := New("warn", "text")
	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("info should be disabled at warn level")
	}
	if !logger.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("warn should be enabled at warn level")
	}
}

func TestNewFormats(t *testing.T) {
	if _, ok := New("info", "json").Handler().(*slog.JSONHandler); !ok {
		t.Error("expected a *slog.JSONHandler for json format")
	}
	if _, ok := New("info", "JSON").Handler().(*slog.JSONHandler); !ok {
		t.Error("format should be matched case-insensitively")
	}
	if _, ok := New("info", "text").Handler().(*slog.TextHandler); !ok {
		t.Error("expected a *slog.TextHandler for text format")
	}
	if _, ok := New("info", "anything-else").Handler().(*slog.TextHandler); !ok {
		t.Error("unknown format should default to text")
	}
}
