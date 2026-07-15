package charts

import "testing"

func TestGeneratorParamsSetsIconTitleAndGradient(t *testing.T) {
	vals := GeneratorParams("fr")
	if vals.Get("icon") != "1f1eb-1f1f7" {
		t.Errorf("icon = %q, want the French flag codepoint", vals.Get("icon"))
	}
	if vals.Get("title") != "charts.top50" {
		t.Errorf("title = %q, want %q", vals.Get("title"), "charts.top50")
	}
	if vals.Get("subTitle") != "charts.country.fr" {
		t.Errorf("subTitle = %q, want %q", vals.Get("subTitle"), "charts.country.fr")
	}
	if vals.Get("color") == "" || vals.Get("color2") == "" || vals.Get("angle") == "" {
		t.Errorf("expected a gradient, got %v", vals)
	}
}

func TestGeneratorParamsFallsBackForAnUnknownSlug(t *testing.T) {
	// No matching gradient/emoji/label entry — still yields a plain gradient,
	// with no icon or subtitle.
	vals := GeneratorParams("does-not-exist")
	if vals.Get("icon") != "" || vals.Get("subTitle") != "" {
		t.Errorf("expected no icon/subTitle for an unknown slug, got %v", vals)
	}
	if vals.Get("color") == "" || vals.Get("color2") == "" {
		t.Errorf("expected a fallback gradient, got %v", vals)
	}
}

func TestResolveLabel(t *testing.T) {
	if got := ResolveLabel("charts.country.fr", "en"); got != "French" {
		t.Errorf("ResolveLabel(fr, en) = %q, want %q", got, "French")
	}
	if got := ResolveLabel("charts.country.fr", "fr"); got != "France" {
		t.Errorf("ResolveLabel(fr, fr) = %q, want %q", got, "France")
	}
}

func TestResolveLabelFallsBackToFrenchForAnUnknownLocale(t *testing.T) {
	if got := ResolveLabel("charts.country.fr", "xx"); got != "France" {
		t.Errorf("ResolveLabel with unknown locale = %q, want French fallback %q", got, "France")
	}
}

func TestResolveLabelReturnsUnknownKeysAsLiteralText(t *testing.T) {
	if got := ResolveLabel("Road Trip", "en"); got != "Road Trip" {
		t.Errorf("ResolveLabel(literal) = %q, want unchanged", got)
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
