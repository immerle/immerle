package charts

import (
	"net/url"
	"strings"

	"github.com/immerle/immerle/internal/covergen"
)

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

// flagEmoji gives each chart slug its flag/globe emoji; GeneratorParams
// converts it to the Twemoji codepoint stored in the generator cover id (see
// covergen.EmojiCodepoint), so no PNG is bundled for it.
var flagEmoji = map[string]string{
	"global": "🌍",
	"fr":     "🇫🇷",
	"us":     "🇺🇸",
	"gb":     "🇬🇧",
	"de":     "🇩🇪",
	"es":     "🇪🇸",
}

// labelKeys is the i18n dictionary behind the generator cover's `title`/
// `subTitle` params — GET /cover/generator resolves a param through this
// table (via ResolveLabel) when it matches a known key, e.g.
// "charts.country.fr" -> "France" (fr) / "French" (en); an unrecognized
// param is used as literal text instead. ResolveLabel falls back to French
// for an unknown locale.
var labelKeys = map[string]map[string]string{
	"charts.top50":          {"fr": "Top 50", "en": "Top 50"},
	"charts.country.global": {"fr": "Mondial", "en": "Global"},
	"charts.country.fr":     {"fr": "France", "en": "French"},
	"charts.country.us":     {"fr": "États-Unis", "en": "American"},
	"charts.country.gb":     {"fr": "Royaume-Uni", "en": "British"},
	"charts.country.de":     {"fr": "Allemagne", "en": "German"},
	"charts.country.es":     {"fr": "Espagne", "en": "Spanish"},
}

// NormalizeLocale reduces a BCP47-ish tag ("en-US", "FR") to the bare
// lowercase language code ("en", "fr") this package's tables key on.
func NormalizeLocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if i := strings.IndexAny(locale, "-_"); i >= 0 {
		locale = locale[:i]
	}
	return locale
}

// ResolveLabel translates key through labelKeys in locale, falling back to
// French for an unlisted locale. A key with no entry (e.g. literal text
// passed straight through by a caller) is returned unchanged.
func ResolveLabel(key, locale string) string {
	labels, ok := labelKeys[key]
	if !ok {
		return key
	}
	if l, ok := labels[NormalizeLocale(locale)]; ok {
		return l
	}
	return labels["fr"]
}

// GeneratorParams builds the GET /cover/generator query params for a chart
// slug's cover: its flag/globe icon, "charts.top50"/"charts.country.<slug>"
// title keys, and gradient. Stored as the chart playlist's CoverArt (see
// service.go), it's resolved into a rendered PNG on demand, in the
// requesting client's locale — an unknown slug still yields a plain
// gradient with no icon/subtitle.
func GeneratorParams(slug string) url.Values {
	g, ok := coverGradients[slug]
	if !ok {
		g = coverGradient{"#333333", "#111111"}
	}

	vals := url.Values{}
	if icon := flagEmoji[slug]; icon != "" {
		vals.Set("icon", covergen.EmojiCodepoint(icon))
	}
	vals.Set("title", "charts.top50")
	if _, ok := labelKeys["charts.country."+slug]; ok {
		vals.Set("subTitle", "charts.country."+slug)
	}
	vals.Set("color", g.from)
	vals.Set("color2", g.to)
	vals.Set("angle", "45")
	return vals
}
