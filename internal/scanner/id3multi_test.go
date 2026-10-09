package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// id3Frame builds an ID3v2.4 text frame with a pre-encoded body.
func id3Frame(id string, enc byte, text []byte) []byte {
	body := append([]byte{enc}, text...)
	n := len(body)
	return append([]byte{id[0], id[1], id[2], id[3],
		byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f), 0, 0}, body...)
}

func writeID3(t *testing.T, frames ...[]byte) string {
	var body []byte
	for _, f := range frames {
		body = append(body, f...)
	}
	n := len(body)
	tag := append([]byte{'I', 'D', '3', 4, 0, 0,
		byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}, body...)
	path := filepath.Join(t.TempDir(), "t.mp3")
	if err := os.WriteFile(path, append(tag, make([]byte, 512)...), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Regression for #270: a multi-value ID3v2.4 TPE1 used to be read as "SantanaJuanes".
func TestExtractMultiValueID3Artist(t *testing.T) {
	utf16le := []byte{0xff, 0xfe, 'S', 0, 'a', 0, 'n', 0, 't', 0, 'a', 0, 'n', 0, 'a', 0, 0, 0,
		0xff, 0xfe, 'J', 0, 'u', 0, 'a', 0, 'n', 0, 'e', 0, 's', 0}
	for name, tc := range map[string]struct {
		frame     []byte
		artist    string
		featuring []string
	}{
		"utf8":          {id3Frame("TPE1", 3, []byte("Santana\x00Juanes")), "Santana", []string{"Juanes"}},
		"utf16":         {id3Frame("TPE1", 1, utf16le), "Santana", []string{"Juanes"}},
		"slash in name": {id3Frame("TPE1", 3, []byte("AC/DC")), "AC/DC", nil},
	} {
		t.Run(name, func(t *testing.T) {
			path := writeID3(t, id3Frame("TIT2", 3, []byte("La Flaca")), tc.frame,
				id3Frame("TPE2", 3, []byte("Santana\x00Juanes")))
			md, err := NewExtractor("").Extract(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			var feats []string
			for _, p := range md.Participants {
				if p.Role == "featuring" {
					feats = append(feats, p.Name)
				}
			}
			if md.Artist != tc.artist || len(feats) != len(tc.featuring) || (len(feats) > 0 && feats[0] != tc.featuring[0]) {
				t.Fatalf("artist=%q featuring=%v, want %q %v", md.Artist, feats, tc.artist, tc.featuring)
			}
			if md.AlbumArtist != "Santana" {
				t.Fatalf("album artist=%q, want Santana", md.AlbumArtist)
			}
		})
	}
}
