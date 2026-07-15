package models

import (
	"encoding/base64"
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

// ChartCoverPrefix marks a cover-art id as a curated chart-playlist cover:
// generated on demand (not a stored file), since its text label is rendered
// in the requesting client's locale — see internal/charts.GenerateCover.
const ChartCoverPrefix = "chart:"

// ChartCoverID builds the cover-art id for a chart playlist's slug.
func ChartCoverID(slug string) string { return ChartCoverPrefix + slug }

// IsChartCoverID reports whether id is a chart cover-art id.
func IsChartCoverID(id string) bool { return strings.HasPrefix(id, ChartCoverPrefix) }

// DecodeChartCoverID extracts the chart slug from a chart cover-art id.
func DecodeChartCoverID(id string) (string, bool) {
	if !IsChartCoverID(id) {
		return "", false
	}
	return strings.TrimPrefix(id, ChartCoverPrefix), true
}
