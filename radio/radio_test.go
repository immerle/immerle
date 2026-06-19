package radio

import (
	"strings"
	"testing"
)

func TestBuiltinsParse(t *testing.T) {
	stations := Builtins()
	if len(stations) < 10 {
		t.Fatalf("expected the embedded list to parse to many stations, got %d", len(stations))
	}
	seen := map[string]bool{}
	for _, s := range stations {
		if s.ID == "" || s.Name == "" {
			t.Fatalf("station with empty id/name: %+v", s)
		}
		if !strings.HasPrefix(s.StreamURL, "http") {
			t.Fatalf("station %q has a non-http stream URL: %q", s.Name, s.StreamURL)
		}
		if seen[s.ID] {
			t.Fatalf("duplicate station id: %q", s.ID)
		}
		seen[s.ID] = true
	}
}
