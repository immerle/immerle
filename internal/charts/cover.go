package charts

import (
	"bytes"
	"embed"
	"image"
	"image/color"
	_ "image/png"
	"strings"

	"github.com/immerle/immerle/internal/covergen"
)

//go:embed assets/*.png
var emojiAssets embed.FS

// coverGradient is the background gradient used behind a chart's flag/globe
// icon — loosely evoking that country's flag colors (two of them, since the
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

// countryLabels give each chart slug a localized label, used as "Top 50
// <label>" on the cover — e.g. "Top 50 France" (fr) vs "Top 50 French" (en),
// matching how English favors the demonym where French favors the country
// name. Extend with more locales/slugs as needed; chartLabel falls back to
// "fr" for an unlisted locale.
var countryLabels = map[string]map[string]string{
	"global": {"fr": "Mondial", "en": "Global"},
	"fr":     {"fr": "France", "en": "French"},
	"us":     {"fr": "États-Unis", "en": "American"},
	"gb":     {"fr": "Royaume-Uni", "en": "British"},
	"de":     {"fr": "Allemagne", "en": "German"},
	"es":     {"fr": "Espagne", "en": "Spanish"},
}

// emojiInset is how far the flag/globe icon is inset from each edge
// horizontally, as a fraction of covergen.Size — the icon is emojiIconFrac
// wide/tall (36% of the cover).
const emojiInset = 0.32
const emojiIconFrac = 1 - 2*emojiInset

// titleFontFrac/countryFontFrac are the "Top 50" / country-label font sizes;
// groupGapFrac is the gap between the icon and "Top 50", textGapFrac the
// (tighter) gap between "Top 50" and the country label — all fractions of
// covergen.Size. The icon+"Top 50"+country trio is centered as one group
// (see GenerateCover), not each line centered independently, so the whole
// composition reads as a single mark.
const titleFontFrac = 0.13
const countryFontFrac = 0.07
const groupGapFrac = 0.045
const textGapFrac = 0.02

// NormalizeLocale reduces a BCP47-ish tag ("en-US", "FR") to the bare
// lowercase language code ("en", "fr") this package's tables key on.
func NormalizeLocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if i := strings.IndexAny(locale, "-_"); i >= 0 {
		locale = locale[:i]
	}
	return locale
}

// chartLabel returns slug's localized label, falling back to French for an
// unlisted locale (or slug) — same default the playlist's own Name uses.
func chartLabel(slug, locale string) string {
	labels, ok := countryLabels[slug]
	if !ok {
		return ""
	}
	if l, ok := labels[NormalizeLocale(locale)]; ok {
		return l
	}
	return labels["fr"]
}

// GenerateCover renders a gradient cover for slug with its flag/globe icon
// (country flags clipped to a circle), a large "Top 50" line, and the
// (smaller) country label below it in the given locale, PNG-encoded. Falls
// back to a plain gradient (no error) if the slug has no matching emoji
// asset; falls back to French text for an unknown locale.
func GenerateCover(slug, locale string) ([]byte, error) {
	img := covergen.NewCanvas()

	g, ok := coverGradients[slug]
	if !ok {
		g = coverGradient{"#333333", "#111111"}
	}
	covergen.FillGradient(img, covergen.ParseHex(g.from, color.Black), covergen.ParseHex(g.to, color.Black), 45)

	// The icon, "Top 50" and the country label are centered as one group, not
	// each centered independently — otherwise they'd read as "a centered icon
	// with text tacked on after it" rather than a single composed mark.
	label := chartLabel(slug, locale)
	titleLineHFrac := titleFontFrac * 1.25
	groupHeightFrac := emojiIconFrac + groupGapFrac + titleLineHFrac
	if label != "" {
		groupHeightFrac += textGapFrac + countryFontFrac*1.25
	}
	groupTopFrac := (1 - groupHeightFrac) / 2

	if data, err := emojiAssets.ReadFile("assets/" + slug + ".png"); err == nil {
		if icon, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			hInset := int(emojiInset * covergen.Size)
			top := int(groupTopFrac * covergen.Size)
			r := image.Rect(hInset, top, covergen.Size-hInset, top+int(emojiIconFrac*covergen.Size))
			if slug == "global" {
				covergen.DrawImage(img, icon, r) // globe is already round
			} else {
				covergen.DrawImageRounded(img, icon, r, 0.5) // flag: circular crop
			}
		}
	}

	titleTop := groupTopFrac + emojiIconFrac + groupGapFrac
	_ = covergen.DrawText(img, covergen.TextSpec{
		Text: "Top 50", Color: color.White,
		FontFrac: titleFontFrac, Align: "center", TopFrac: &titleTop,
	})

	if label != "" {
		countryTop := titleTop + titleLineHFrac + textGapFrac
		_ = covergen.DrawText(img, covergen.TextSpec{
			Text: label, Color: color.White,
			FontFrac: countryFontFrac, Align: "center", TopFrac: &countryTop,
		})
	}

	return covergen.Encode(img)
}
