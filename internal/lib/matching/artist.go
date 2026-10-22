package matching

import (
	"math"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// artistSeparators split an artist credit into individual artists.
var artistSeparators = strings.NewReplacer(
	" featuring ", ",", " feat. ", ",", " feat ", ",", " ft. ", ",", " ft ", ",",
	"&", ",", ";", ",", "/", ",",
)

// splitArtists lowercases (NFC) a credit and splits it into its artists.
func splitArtists(credit string) []string {
	credit = artistSeparators.Replace(strings.ToLower(norm.NFC.String(credit)))
	var out []string
	for _, a := range strings.Split(credit, ",") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// ArtistOverlap scores how well two artist credits match, from 0 to 1: the
// share of one side's artists found in the other, checked in both directions
// and keeping the higher. "Sindika" vs "Sindika, TRK., Ameka Zrai" scores 1
// (Sindika is fully in the second credit), even though only a third of the
// second credit is in the first.
func ArtistOverlap(a, b string) float64 {
	as, bs := splitArtists(a), splitArtists(b)
	return max(coverage(as, bs), coverage(bs, as))
}

// ArtistMismatch turns ArtistOverlap into a 0 (full match) to 3 (no artist
// in common) penalty, on the same scale as a title relevance score.
func ArtistMismatch(a, b string) int {
	return int(math.Round((1 - ArtistOverlap(a, b)) * 3))
}

// coverage is the share of from's artists present in to.
func coverage(from, to []string) float64 {
	if len(from) == 0 {
		return 0
	}
	found := 0
	for _, f := range from {
		for _, t := range to {
			if f == t {
				found++
				break
			}
		}
	}
	return float64(found) / float64(len(from))
}
