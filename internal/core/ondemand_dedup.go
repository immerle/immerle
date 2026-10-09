package core

import (
	"context"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/providers"
)

// splitArtists splits a provider artist credit "Main/Feat1/Feat2" into the main
// artist and its featured artists.
func splitArtists(s string) (string, []string) {
	var names []string
	for _, n := range strings.Split(s, "/") {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return strings.TrimSpace(s), nil
	}
	return names[0], names[1:]
}

// matchKey folds case, diacritics and punctuation so "TOTO"/"Toto",
// "RŮDE"/"Rude." or "JAY Z"/"JAŸ-Z" compare equal.
func matchKey(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// canonicalizeOnDemand rewrites a provider result before it is tagged and
// ingested: the main artist is split from its featurings, and the artist and
// album names reuse the spelling already in the library, so a download never
// creates a case/diacritic variant of an existing artist or album. It returns
// the matching existing album id ("" when the album is new).
func (s *CatalogService) canonicalizeOnDemand(ctx context.Context, meta *providers.Result) string {
	st := s.state
	meta.Artist, meta.Featuring = splitArtists(meta.Artist)
	meta.AlbumArtist, _ = splitArtists(meta.AlbumArtist)

	// ponytail: full artist scan per non-local play, add a normalized-name column if the library gets huge.
	artists, err := st.catalog.ListArtists(ctx)
	if err != nil {
		return ""
	}
	canon := func(name string) (models.Artist, bool) {
		k := matchKey(name)
		for _, a := range artists {
			if k != "" && matchKey(a.Name) == k {
				return a, true
			}
		}
		return models.Artist{}, false
	}
	if a, ok := canon(meta.Artist); ok {
		meta.Artist = a.Name
	}
	if a, ok := canon(meta.AlbumArtist); ok {
		meta.AlbumArtist = a.Name
	}

	albumArtist, ok := canon(firstNonEmpty(meta.AlbumArtist, meta.Artist))
	if !ok {
		return ""
	}
	albums, err := st.catalog.ListAlbumsByArtist(ctx, albumArtist.ID)
	if err != nil {
		return ""
	}
	k := matchKey(meta.Album)
	for _, al := range albums {
		if matchKey(al.Name) == k {
			meta.Album = al.Name
			return al.ID
		}
	}
	return ""
}

// findOnDemandDuplicate canonicalizes meta and returns an already downloaded
// track for the same song (same album, same title), even when it came from
// another provider or provider track id. Only on-demand tracks (those produced
// by a download job) are considered: a manually scanned library is left alone.
// The returned track may be evicted (Remote): re-download it to its path.
func (s *CatalogService) findOnDemandDuplicate(ctx context.Context, meta *providers.Result) (models.Track, bool) {
	albumID := s.canonicalizeOnDemand(ctx, meta)
	if albumID == "" {
		return models.Track{}, false
	}
	tracks, err := s.state.catalog.ListTracksByAlbum(ctx, albumID)
	if err != nil {
		return models.Track{}, false
	}
	k := matchKey(meta.Title)
	for _, t := range tracks {
		if matchKey(t.Title) != k {
			continue
		}
		if _, err := s.state.downloads.GetByTrack(ctx, t.ID); err == nil {
			return t, true
		}
	}
	return models.Track{}, false
}
