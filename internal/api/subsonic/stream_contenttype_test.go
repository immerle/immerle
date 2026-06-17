package subsonic

import "testing"

func TestAudioContentType(t *testing.T) {
	cases := []struct {
		format, suffix, want string
	}{
		{"mp3", "flac", "audio/mpeg"}, // requested format wins (the "lie")
		{"", "flac", "audio/flac"},    // no format → provider suffix
		{"raw", "mp3", "audio/mpeg"},  // raw → provider suffix
		{"opus", "mp3", "audio/ogg"},  //
		{"", "weird", "application/octet-stream"},
	}
	for _, c := range cases {
		if got := audioContentType(c.format, c.suffix); got != c.want {
			t.Errorf("audioContentType(%q,%q)=%q want %q", c.format, c.suffix, got, c.want)
		}
	}
}
