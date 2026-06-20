package media

import "testing"

func TestAudioContentType(t *testing.T) {
	cases := []struct {
		suffix, want string
	}{
		{"flac", "audio/flac"},
		{"mp3", "audio/mpeg"},
		{"opus", "audio/ogg"},
		{"m4a", "audio/mp4"},
		{"weird", "application/octet-stream"},
	}
	for _, c := range cases {
		if got := audioContentType(c.suffix); got != c.want {
			t.Errorf("audioContentType(%q)=%q want %q", c.suffix, got, c.want)
		}
	}
}
