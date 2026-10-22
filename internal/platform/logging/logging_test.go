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
	logger, _ := New("warn")
	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("info should be disabled at warn level")
	}
	if !logger.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("warn should be enabled at warn level")
	}
}

func TestNewIsJSON(t *testing.T) {
	logger, _ := New("info")
	if _, ok := logger.Handler().(*slog.JSONHandler); !ok {
		t.Error("expected a *slog.JSONHandler")
	}
}

func TestHubBroadcastsAndKeepsHistory(t *testing.T) {
	logger, hub := New("info")
	logger.Info("first")

	ch, history, unsubscribe := hub.Subscribe()
	defer unsubscribe()
	if len(history) != 1 {
		t.Fatalf("expected 1 history line, got %d", len(history))
	}

	logger.Info("second")
	select {
	case line := <-ch:
		if len(line) == 0 {
			t.Error("expected a non-empty broadcast line")
		}
	default:
		t.Error("expected the new log line to be broadcast to the subscriber")
	}
}
