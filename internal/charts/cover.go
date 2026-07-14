package charts

import (
	"bytes"
	"embed"
	"image"
	"image/color"
	_ "image/png"

	"github.com/immerle/immerle/internal/covergen"
)

//go:embed assets/*.png
var emojiAssets embed.FS

// coverGradient is the background gradient used behind a chart's flag/globe
// emoji — loosely evoking that country's flag colors (two of them, since the
// gradient only has two stops), or the app's own green for the global chart.
type coverGradient struct{ from, to string }

var coverGradients = map[string]coverGradient{
	"global": {"#1db954", "#0b3d20"},
	"fr":     {"#0055a4", "#ef4135"},
	"us":     {"#3c3b6e", "#b22234"},
	"gb":     {"#00247d", "#cf142b"},
	"de":     {"#1a1a1a", "#dd0000"},
	"es":     {"#aa151b", "#f1bf00"},
}

// emojiInset is how far the emoji is inset from each edge, as a fraction of
// covergen.Size — the flag/globe ends up centered at roughly 55% of the cover.
const emojiInset = 0.225

// generateCover renders a gradient cover with the chart's flag/globe emoji
// centered on top, PNG-encoded. Falls back to a plain gradient (no error) if
// the slug has no matching emoji asset.
func generateCover(slug string) ([]byte, error) {
	img := covergen.NewCanvas()

	g, ok := coverGradients[slug]
	if !ok {
		g = coverGradient{"#333333", "#111111"}
	}
	covergen.FillGradient(img, covergen.ParseHex(g.from, color.Black), covergen.ParseHex(g.to, color.Black), 45)

	if data, err := emojiAssets.ReadFile("assets/" + slug + ".png"); err == nil {
		if emoji, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			inset := int(emojiInset * covergen.Size)
			r := image.Rect(inset, inset, covergen.Size-inset, covergen.Size-inset)
			covergen.DrawImage(img, emoji, r)
		}
	}

	return covergen.Encode(img)
}
