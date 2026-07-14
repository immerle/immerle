// Package charts syncs a small set of curated "chart" playlists (weekly top
// tracks per country, plus a global chart) from the kworb-net-api GitHub data
// set (https://github.com/ermos/kworb-net-api), materializing them as public
// playlists using the exact same federated-playlist mechanism as hub-imported
// playlists — so they behave identically everywhere in the app (read-only,
// public, tracks resolved lazily by artist/title at play time).
package charts

import (
	"regexp"
	"strings"
)

// Chart is one kworb source file to sync, and the display name of the
// playlist it materializes as.
type Chart struct {
	// Slug is the kworb file slug, e.g. "global" or "fr" — the source file is
	// "<slug>_weekly.json".
	Slug string
	Name string
}

// DefaultCharts is the curated set synced today: the worldwide chart plus a
// handful of major markets. Add more by appending a {slug, name} pair — the
// slug must match a "<slug>_weekly.json" file under kworb-net-api's
// data/spotify directory.
var DefaultCharts = []Chart{
	{Slug: "global", Name: "Top mondial"},
	{Slug: "fr", Name: "Top 50 France"},
	{Slug: "us", Name: "Top 50 États-Unis"},
	{Slug: "gb", Name: "Top 50 Royaume-Uni"},
	{Slug: "de", Name: "Top 50 Allemagne"},
	{Slug: "es", Name: "Top 50 Espagne"},
}

// maxTracksPerChart caps how many entries are kept per synced playlist — the
// source lists are already ranked, so this is simply "top N".
const maxTracksPerChart = 50

// kworbChart is the shape of a "<slug>_weekly.json" response — only the
// fields this package uses are declared; kworb's own metadata (source url,
// title, streams, etc.) is ignored.
type kworbChart struct {
	Chart []kworbEntry `json:"chart"`
}

type kworbEntry struct {
	// ArtistAndTitle is kworb's single combined field, formatted "Artist -
	// Title" (the title itself may contain further " - ", e.g. a remix tag).
	ArtistAndTitle string `json:"Artist and Title"`
}

// coAuthorPattern matches kworb's featured/co-artist annotation, e.g.
// "(w/ La Rvfleuze)". playlist_tracks has no co-author field to put this in
// (only a single portable artist string), so it's stripped from the title
// rather than left in as noise that would hurt title matching at play time.
var coAuthorPattern = regexp.MustCompile(`(?i)\s*\(w/[^)]*\)`)

// splitArtistAndTitle parses kworb's "Artist - Title" format, splitting on the
// first " - " so a title containing its own " - " (e.g. a remix suffix) stays
// intact, and stripping any "(w/ ...)" co-artist annotation from the title.
// Returns ("", "") if the format doesn't match.
func splitArtistAndTitle(s string) (artist, title string) {
	parts := strings.SplitN(s, " - ", 2)
	if len(parts) != 2 {
		return "", ""
	}
	title = coAuthorPattern.ReplaceAllString(strings.TrimSpace(parts[1]), "")
	return strings.TrimSpace(parts[0]), strings.TrimSpace(title)
}
