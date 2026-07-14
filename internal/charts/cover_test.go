package charts

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/immerle/immerle/internal/covergen"
)

func TestGenerateCoverProducesADecodableSquarePNGForEveryDefaultChart(t *testing.T) {
	for _, c := range DefaultCharts {
		data, err := generateCover(c.Slug)
		if err != nil {
			t.Fatalf("%s: generateCover: %v", c.Slug, err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: output is not a PNG: %v", c.Slug, err)
		}
		if b := img.Bounds(); b.Dx() != covergen.Size || b.Dy() != covergen.Size {
			t.Fatalf("%s: size = %v, want %d square", c.Slug, b, covergen.Size)
		}
	}
}

func TestGenerateCoverFallsBackForAnUnknownSlug(t *testing.T) {
	// No matching emoji asset or gradient entry — should still produce a
	// plain gradient cover rather than erroring.
	data, err := generateCover("does-not-exist")
	if err != nil {
		t.Fatalf("generateCover: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("output is not a PNG: %v", err)
	}
}
