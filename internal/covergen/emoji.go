package covergen

import (
	"context"
	"fmt"
	"image"
	_ "image/png"
	"net/http"
	"strings"
	"time"
)

// TwemojiCDN is the base URL emoji PNGs are fetched from (Twemoji, CC-BY 4.0,
// https://github.com/jdecked/twemoji) — icons are fetched on demand instead
// of being bundled as PNGs in the repo. Tests point this at an httptest
// server instead of the real CDN.
var TwemojiCDN = "https://cdn.jsdelivr.net/gh/jdecked/twemoji@latest/assets/72x72/"

var emojiHTTPClient = &http.Client{Timeout: 10 * time.Second}

// FetchEmoji downloads and decodes the Twemoji PNG for a codepoint sequence
// (see EmojiCodepoint), e.g. "1f30d" (🌍) or "1f1eb-1f1f7" (🇫🇷).
func FetchEmoji(ctx context.Context, codepoint string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, TwemojiCDN+codepoint+".png", nil)
	if err != nil {
		return nil, err
	}
	resp, err := emojiHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("covergen: twemoji fetch %q: %s", codepoint, resp.Status)
	}
	img, _, err := image.Decode(resp.Body)
	return img, err
}

// EmojiCodepoint turns an emoji glyph into Twemoji's filename form: lowercase
// hex codepoints joined by "-", dropping variation selectors (U+FE0F) — e.g.
// "🇫🇷" -> "1f1eb-1f1f7", "🌍" -> "1f30d".
func EmojiCodepoint(emoji string) string {
	parts := make([]string, 0, len(emoji))
	for _, r := range emoji {
		if r == 0xfe0f {
			continue
		}
		parts = append(parts, fmt.Sprintf("%x", r))
	}
	return strings.Join(parts, "-")
}
