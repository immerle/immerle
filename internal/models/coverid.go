package models

import (
	"encoding/base64"
	"net/url"
	"strings"
)

// RemoteCoverPrefix marks a cover-art id that points to a provider's public image
// URL rather than a local file. The URL is embedded (base64url) so getCoverArt
// can fetch and serve it (subject to a host allowlist).
const RemoteCoverPrefix = "rcov:"

// RemoteCoverID encodes an image URL as a remote cover-art id. An empty URL
// yields an empty id (no cover).
func RemoteCoverID(imageURL string) string {
	if imageURL == "" {
		return ""
	}
	return RemoteCoverPrefix + base64.RawURLEncoding.EncodeToString([]byte(imageURL))
}

// IsRemoteCoverID reports whether id is a remote cover-art id.
func IsRemoteCoverID(id string) bool {
	return strings.HasPrefix(id, RemoteCoverPrefix)
}

// DecodeRemoteCoverID extracts the image URL from a remote cover-art id.
func DecodeRemoteCoverID(id string) (string, bool) {
	if !IsRemoteCoverID(id) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(id, RemoteCoverPrefix))
	if err != nil {
		return "", false
	}
	return string(raw), true
}

// GeneratorCoverPrefix marks a cover-art id as generated on demand (not a
// stored file) from a small set of builder params — icon, title, subTitle,
// color, color2, angle — rather than referring to a specific resource. This
// is the id form behind GET /cover/generator, which reads those same params
// straight off the request's query string; see models.GeneratorCoverID.
const GeneratorCoverPrefix = "generator:"

// generatorCoverKeys are the builder params kept in a generator cover id.
// Everything else on the request (size, locale) is handled separately by
// CoverService, not part of the id/cache key itself.
var generatorCoverKeys = []string{"icon", "title", "subTitle", "color", "color2", "angle"}

// GeneratorCoverID builds a cover-art id from a generator builder's query
// params (a subset of q — size/locale are dropped).
func GeneratorCoverID(q url.Values) string {
	vals := url.Values{}
	for _, k := range generatorCoverKeys {
		if v := q.Get(k); v != "" {
			vals.Set(k, v)
		}
	}
	return GeneratorCoverPrefix + vals.Encode()
}

// IsGeneratorCoverID reports whether id is a generator cover-art id.
func IsGeneratorCoverID(id string) bool { return strings.HasPrefix(id, GeneratorCoverPrefix) }

// DecodeGeneratorCoverID extracts the builder params from a generator
// cover-art id.
func DecodeGeneratorCoverID(id string) (url.Values, bool) {
	if !IsGeneratorCoverID(id) {
		return nil, false
	}
	vals, err := url.ParseQuery(strings.TrimPrefix(id, GeneratorCoverPrefix))
	if err != nil {
		return nil, false
	}
	return vals, true
}
