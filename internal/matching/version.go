// Package matching holds small, dependency-free heuristics for auto-matching
// a wanted (artist, title) against track candidates — shared by the local
// catalog lookup (persistence.CatalogRepo.FindByArtistTitle) and the remote
// provider search (core.CatalogService.ResolveBestRemoteMatch), so a
// candidate can't dodge one path's disambiguation just by winning the other.
package matching

import "strings"

// VersionMarkers are qualifiers that typically indicate an alternate version
// of a track (remix, live recording, cover, instrumental, ...) rather than
// the original.
var VersionMarkers = []string{
	"remix", "live", "acoustic", "instrumental", "karaoke", "cover version",
	"tribute", "a cappella", "acapella", "gospel", "reprise", "extended mix",
	"radio edit", "sped up", "slowed", "nightcore", "mashup",
}

// VersionMarkerPenalty returns 1 if title/album look like an alternate
// version the wanted title doesn't itself ask for (so wanting "Song (Live)"
// isn't penalized against itself), else 0. Scraped metadata for an alternate
// version is sometimes *cleaner* than the original's own listing (e.g. a
// "Gospel" cover simply titled "Song", while the original carries extra
// qualifiers) — a plain title-closeness score alone can't tell them apart.
func VersionMarkerPenalty(wanted, title, album string) int {
	w := strings.ToLower(wanted)
	t, a := strings.ToLower(title), strings.ToLower(album)
	for _, m := range VersionMarkers {
		if strings.Contains(w, m) {
			continue
		}
		if strings.Contains(t, m) || strings.Contains(a, m) {
			return 1
		}
	}
	return 0
}
