package covergen_test

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"

	"github.com/immerle/immerle/internal/covergen"
)

func TestParseHex(t *testing.T) {
	if got := covergen.ParseHex("#ff0000", color.Black).(color.RGBA); got != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("ff0000 -> %v", got)
	}
	if got := covergen.ParseHex("#0f0", color.Black).(color.RGBA); got != (color.RGBA{0, 255, 0, 255}) {
		t.Fatalf("short hex #0f0 -> %v", got)
	}
	if got := covergen.ParseHex("nope", color.Black); got != color.Black {
		t.Fatalf("bad hex should fall back, got %v", got)
	}
}

func TestFillGradientAndEncode(t *testing.T) {
	img := covergen.NewCanvas()
	covergen.FillGradient(img, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}, 45)
	data, err := covergen.Encode(img)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("output is not a PNG: %v", err)
	}
	if b := decoded.Bounds(); b.Dx() != covergen.Size || b.Dy() != covergen.Size {
		t.Fatalf("size = %v, want %d square", b, covergen.Size)
	}
}
