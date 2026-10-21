package matching

import "testing"

func TestArtistOverlap(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want float64
	}{
		{"Sindika", "Sindika, TRK., Ameka Zrai", 1},
		{"Sindika, TRK., Ameka Zrai", "Sindika", 1},
		{"Vacra feat. La Rvfleuze", "vacra", 1},
		{"Damso & Hamza", "Hamza, Ninho", 0.5},
		{"Ninho", "Jul", 0},
	} {
		if got := ArtistOverlap(c.a, c.b); got != c.want {
			t.Errorf("ArtistOverlap(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
