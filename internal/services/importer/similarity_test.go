package importer

import "testing"

func TestSimilarityIgnoresNoise(t *testing.T) {
	// Same song with remaster/edit annotations and punctuation should match high.
	s := trackSimilarity("Daft Punk", "Da Funk", "Daft Punk", "Da Funk (Radio Edit)")
	if s < MatchThreshold {
		t.Fatalf("expected high similarity, got %.3f", s)
	}
	// Exact (after normalization) is 1.
	if s := trackSimilarity("Daft Punk", "Da Funk", "daft punk", "da funk"); s != 1 {
		t.Fatalf("expected 1.0 for normalized-equal, got %.3f", s)
	}
}

func TestSimilarityRejectsDifferent(t *testing.T) {
	s := trackSimilarity("Daft Punk", "Da Funk", "Taylor Swift", "Blank Space")
	if s >= MatchThreshold {
		t.Fatalf("expected low similarity for a different song, got %.3f", s)
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"  Daft Punk - Da Funk! ":     "daft punk da funk",
		"Song (feat. X) [Remastered]": "song",
		"AC/DC":                       "ac dc",
	}
	for in, want := range cases {
		if got := normalize(in); got != want {
			t.Fatalf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
