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
		if s.Country == "" {
			t.Fatalf("station %q has no country", s.Name)
		}
		if seen[s.ID] {
			t.Fatalf("duplicate station id: %q", s.ID)
		}
		seen[s.ID] = true
		// Embedded logos are standardized as .webp and must resolve to real bytes.
		if s.CoverArt != "" {
			if !strings.HasSuffix(s.CoverArt, ".webp") {
				t.Fatalf("station %q cover %q is not .webp", s.Name, s.CoverArt)
			}
			data, ct, ok := CoverFile(s.CoverArt)
			if !ok || len(data) == 0 || ct != "image/webp" {
				t.Fatalf("station %q cover %q does not resolve to webp", s.Name, s.CoverArt)
			}
		}
	}
}
