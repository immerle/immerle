package charts

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/immerle/immerle/internal/covergen"
)

func TestGenerateCoverProducesADecodableSquarePNGForEveryDefaultChart(t *testing.T) {
	for _, c := range DefaultCharts {
		for _, locale := range []string{"fr", "en"} {
			data, err := GenerateCover(c.Slug, locale)
			if err != nil {
				t.Fatalf("%s/%s: GenerateCover: %v", c.Slug, locale, err)
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("%s/%s: output is not a PNG: %v", c.Slug, locale, err)
			}
			if b := img.Bounds(); b.Dx() != covergen.Size || b.Dy() != covergen.Size {
				t.Fatalf("%s/%s: size = %v, want %d square", c.Slug, locale, b, covergen.Size)
			}
		}
	}
}

func TestGenerateCoverDiffersByLocale(t *testing.T) {
	// "Top 50 France" (fr) vs "Top 50 French" (en) — different text drawn
	// onto the canvas, so the encoded bytes must differ.
	fr, err := GenerateCover("fr", "fr")
	if err != nil {
		t.Fatal(err)
	}
	en, err := GenerateCover("fr", "en")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(fr, en) {
		t.Fatal("expected different cover bytes between locales")
	}
}

func TestGenerateCoverFallsBackToFrenchForAnUnknownLocale(t *testing.T) {
	fr, err := GenerateCover("fr", "fr")
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := GenerateCover("fr", "xx")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fr, unknown) {
		t.Fatal("expected an unknown locale to fall back to the French label")
	}
}

func TestGenerateCoverFallsBackForAnUnknownSlug(t *testing.T) {
	// No matching emoji asset or gradient entry — should still produce a
	// plain gradient cover rather than erroring.
	data, err := GenerateCover("does-not-exist", "fr")
	if err != nil {
		t.Fatalf("GenerateCover: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("output is not a PNG: %v", err)
	}
}

func TestNormalizeLocale(t *testing.T) {
	cases := map[string]string{
		"en":    "en",
		"EN":    "en",
		"en-US": "en",
		"fr_FR": "fr",
		" fr ":  "fr",
	}
	for in, want := range cases {
		if got := NormalizeLocale(in); got != want {
			t.Errorf("NormalizeLocale(%q) = %q, want %q", in, got, want)
		}
	}
}
