package immerle

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/immerle/immerle/internal/covergen"
)

func TestRenderCover(t *testing.T) {
	// Gradient background + positioned text renders a decodable square PNG.
	data, err := renderCover(coverSpec{
		Color: "#1db954", Color2: "#000000", Angle: 45,
		Text: "Road\nTrip", TextColor: "#ffffff", FontSize: 0.18, Align: "center", Valign: "middle",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("output is not a PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != covergen.Size || b.Dy() != covergen.Size {
		t.Fatalf("size = %v, want %d square", b, covergen.Size)
	}
}
