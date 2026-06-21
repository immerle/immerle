package immerle

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
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
	if b := img.Bounds(); b.Dx() != renderSize || b.Dy() != renderSize {
		t.Fatalf("size = %v, want %d square", b, renderSize)
	}
}

func TestParseHex(t *testing.T) {
	if got := parseHex("#ff0000", color.Black).(color.RGBA); got != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("ff0000 -> %v", got)
	}
	if got := parseHex("#0f0", color.Black).(color.RGBA); got != (color.RGBA{0, 255, 0, 255}) {
		t.Fatalf("short hex #0f0 -> %v", got)
	}
	if got := parseHex("nope", color.Black); got != color.Black {
		t.Fatalf("bad hex should fall back, got %v", got)
	}
}
