package subsonic

import "testing"

func TestParseStructuredLyrics(t *testing.T) {
	synced := parseStructuredLyrics("[00:12.50]Hello\n[01:05]World")
	if !synced.Synced {
		t.Fatal("expected synced=true")
	}
	if len(synced.Line) != 2 {
		t.Fatalf("got %d lines, want 2", len(synced.Line))
	}
	if synced.Line[0].Start != 12500 || synced.Line[0].Value != "Hello" {
		t.Errorf("line0 = %+v, want start=12500 value=Hello", synced.Line[0])
	}
	if synced.Line[1].Start != 65000 || synced.Line[1].Value != "World" {
		t.Errorf("line1 = %+v, want start=65000 value=World", synced.Line[1])
	}

	plain := parseStructuredLyrics("just\ntext")
	if plain.Synced {
		t.Error("expected synced=false for plain text")
	}
	if len(plain.Line) != 2 || plain.Line[0].Value != "just" {
		t.Errorf("plain lines = %+v", plain.Line)
	}
}
