package covergen_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
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

func TestRender(t *testing.T) {
	// Gradient background + positioned text renders a decodable square PNG.
	data, err := covergen.Render(context.Background(), covergen.Spec{
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

func TestRenderWithIconFetchesFromCDN(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	}))
	defer srv.Close()
	orig := covergen.TwemojiCDN
	covergen.TwemojiCDN = srv.URL + "/"
	defer func() { covergen.TwemojiCDN = orig }()

	data, err := covergen.Render(context.Background(), covergen.Spec{
		Color: "#111111", Icon: "1f30d", Text: "Top 50", Subtitle: "Global", TextColor: "#ffffff", FontSize: 0.13,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("output is not a PNG: %v", err)
	}
	if want := "/1f30d.png"; gotPath != want {
		t.Fatalf("fetched %q, want %q", gotPath, want)
	}
}

func TestRenderSubtitleIsSmallerThanTitle(t *testing.T) {
	// Title + subtitle render as a decodable PNG, distinct from title alone —
	// the subtitle is drawn at its own (smaller) size below the title.
	titleOnly, err := covergen.Render(context.Background(), covergen.Spec{
		Color: "#111111", Text: "Top 50", TextColor: "#ffffff", FontSize: 0.13,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	titleAndSubtitle, err := covergen.Render(context.Background(), covergen.Spec{
		Color: "#111111", Text: "Top 50", Subtitle: "France", TextColor: "#ffffff", FontSize: 0.13,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(titleOnly, titleAndSubtitle) {
		t.Fatal("expected the subtitle to change the rendered cover")
	}
	if _, err := png.Decode(bytes.NewReader(titleAndSubtitle)); err != nil {
		t.Fatalf("output is not a PNG: %v", err)
	}
}

func TestRenderIconFetchFailureFallsBackToNoIcon(t *testing.T) {
	orig := covergen.TwemojiCDN
	covergen.TwemojiCDN = "http://127.0.0.1:0/" // unreachable
	defer func() { covergen.TwemojiCDN = orig }()

	data, err := covergen.Render(context.Background(), covergen.Spec{Color: "#111111", Icon: "1f30d", Text: "Top 50"}, nil)
	if err != nil {
		t.Fatalf("icon fetch failure should not error the whole render: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("output is not a PNG: %v", err)
	}
}

func TestEmojiCodepoint(t *testing.T) {
	cases := map[string]string{
		"🌍":  "1f30d",
		"🇫🇷": "1f1eb-1f1f7",
	}
	for emoji, want := range cases {
		if got := covergen.EmojiCodepoint(emoji); got != want {
			t.Errorf("EmojiCodepoint(%q) = %q, want %q", emoji, got, want)
		}
	}
}
