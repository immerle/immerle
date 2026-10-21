package core

import (
	"context"
	"sort"

	"github.com/immerle/immerle/internal/discover"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// annotateTracks attaches the caller's per-user track annotations to a list of
// tracks (all local, so a plain map lookup suffices).
func (s *LibraryService) annotateTracks(ctx context.Context, userID string, tracks []models.Track) []TrackEntry {
	ann, _ := s.annotations.AnnotationMap(ctx, userID, models.ItemTrack)
	return toTrackEntries(tracks, ann)
}

// RandomSongs returns random tracks, optionally filtered by genre and year range.
func (s *LibraryService) RandomSongs(ctx context.Context, userID string, size int, genre string, fromYear, toYear int) ([]TrackEntry, error) {
	tracks, err := s.catalog.RandomTracks(ctx, size, genre, fromYear, toYear)
	if err != nil {
		return nil, err
	}
	return s.annotateTracks(ctx, userID, tracks), nil
}

// SongsByGenre returns a page of tracks tagged with genre.
func (s *LibraryService) SongsByGenre(ctx context.Context, userID, genre string, count, offset int) ([]TrackEntry, error) {
	tracks, err := s.catalog.ListTracksByGenre(ctx, genre, count, offset)
	if err != nil {
		return nil, err
	}
	return s.annotateTracks(ctx, userID, tracks), nil
}

// AlbumList returns albums for getAlbumList/getAlbumList2. opt carries the sort
// type, paging, filters and the user id (per-user play stats drive some sorts).
func (s *LibraryService) AlbumList(ctx context.Context, opt persistence.AlbumListOptions) ([]AlbumEntry, error) {
	albums, err := s.catalog.ListAlbums(ctx, opt)
	if err != nil {
		return nil, err
	}
	albumAnn, _ := s.annotations.AnnotationMap(ctx, opt.UserID, models.ItemAlbum)
	out := make([]AlbumEntry, 0, len(albums))
	for _, a := range albums {
		out = append(out, AlbumEntry{Album: a, Annotation: annPtr(albumAnn, a.ID)})
	}
	return out, nil
}

// TopSongs returns an artist's tracks ranked by the caller's play count. An
// unknown artist yields an empty list (not an error), matching the API.
func (s *LibraryService) TopSongs(ctx context.Context, userID, artistName string, count int) []TrackEntry {
	artist, err := s.catalog.FindArtistByName(ctx, artistName)
	if err != nil {
		return nil
	}
	tracks, _ := s.catalog.ListTracksByArtist(ctx, artist.ID, 1000)
	ann, _ := s.annotations.AnnotationMap(ctx, userID, models.ItemTrack)
	sort.SliceStable(tracks, func(i, j int) bool {
		return ann[tracks[i].ID].PlayCount > ann[tracks[j].ID].PlayCount
	})
	if len(tracks) > count {
		tracks = tracks[:count]
	}
	return toTrackEntries(tracks, ann)
}

// SimilarSongs returns tracks that sound like the seed (a track, album or
// artist id) when the sonic radio is on and the seed is analyzed. Otherwise it
// asks immerle discover for tracks similar to a seed track (keeping the ones in
// the library), then falls back to random tracks sharing its genre.
func (s *LibraryService) SimilarSongs(ctx context.Context, userID, seedID string, count int) []TrackEntry {
	if s.sonic.RadioEnabled() {
		// Best effort: on a failure too, fall back to discover / same genre.
		if tracks, ok, err := s.SonicRadio(ctx, userID, []string{seedID}, nil, count); err == nil && ok {
			return tracks
		}
	}
	genre := ""
	if t, err := s.catalog.GetTrack(ctx, seedID); err == nil {
		genre = t.Genre
		if tracks := s.discoverSimilar(ctx, t, count); len(tracks) > 0 {
			return s.annotateTracks(ctx, userID, tracks)
		}
	}
	tracks, err := s.catalog.RandomTracks(ctx, count, genre, 0, 0)
	if err != nil {
		return nil
	}
	return s.annotateTracks(ctx, userID, tracks)
}

// discoverFetchSize over-asks discover: most of its global catalog isn't in a
// given library.
const discoverFetchSize = 100

// discoverSimilar returns up to count library tracks discover finds similar to
// seed, nil when it's off or unreachable.
func (s *LibraryService) discoverSimilar(ctx context.Context, seed models.Track, count int) []models.Track {
	if s.discover == nil || !s.discover.SimilarTracksEnabled() {
		return nil
	}
	ref := discover.Track{Artist: seed.ArtistName, Title: seed.Title}
	if seed.MBID != "" {
		ref.IDs = []string{"mbid:" + seed.MBID}
	}
	similar, err := s.discover.SimilarTracks(ctx, []discover.Track{ref}, discoverFetchSize)
	if err != nil {
		return nil
	}
	var out []models.Track
	for _, t := range similar {
		if len(out) >= count {
			break
		}
		// ponytail: one lookup per candidate (up to discoverFetchSize), batch it if getSimilarSongs gets slow.
		if local, ok, err := s.catalog.FindByArtistTitle(ctx, t.MainArtist(), t.Title); err == nil && ok && local.ID != seed.ID {
			out = append(out, local)
		}
	}
	return out
}

// StarredResult is the caller's starred catalog. No annotations are attached:
// the starred lists historically render without per-item state.
type StarredResult struct {
	Artists []models.Artist
	Albums  []models.Album
	Songs   []models.Track
}

// Starred returns the items the user has starred. Lookups that fail (e.g. an item
// removed since it was starred) are skipped, matching the API.
func (s *LibraryService) Starred(ctx context.Context, userID string) StarredResult {
	var out StarredResult
	artistIDs, _ := s.annotations.ListStarred(ctx, userID, models.ItemArtist)
	for _, id := range artistIDs {
		if a, err := s.catalog.GetArtist(ctx, id); err == nil {
			out.Artists = append(out.Artists, a)
		}
	}
	albumIDs, _ := s.annotations.ListStarred(ctx, userID, models.ItemAlbum)
	for _, id := range albumIDs {
		if a, err := s.catalog.GetAlbum(ctx, id); err == nil {
			out.Albums = append(out.Albums, a)
		}
	}
	songIDs, _ := s.annotations.ListStarred(ctx, userID, models.ItemTrack)
	for _, id := range songIDs {
		if t, err := s.catalog.GetTrack(ctx, id); err == nil {
			out.Songs = append(out.Songs, t)
		}
	}
	return out
}

// SonicRadio returns up to count tracks that sound like the seeds (track,
// album or artist ids, mixed freely), excluding the exclude track ids. ok is
// false when none of the seeds is analyzed; err reports a failure (kept apart so
// it never reads as "nothing analyzed"). Callers check the feature toggles.
func (s *LibraryService) SonicRadio(ctx context.Context, userID string, seeds, exclude []string, count int) ([]TrackEntry, bool, error) {
	if s.sonic == nil {
		return nil, false, nil
	}
	trackSeeds := s.resolveSeedTracks(ctx, seeds)
	ids, ok, err := s.sonic.Similar(ctx, trackSeeds, exclude, count)
	if err != nil || !ok {
		return nil, false, err
	}
	byID, err := s.catalog.TracksByIDs(ctx, ids)
	if err != nil {
		return nil, false, err
	}
	tracks := make([]models.Track, 0, len(ids))
	for _, id := range ids {
		if t, found := byID[id]; found {
			tracks = append(tracks, t)
		}
	}
	return s.annotateTracks(ctx, userID, tracks), true, nil
}

// resolveSeedTracks expands seed ids into local track ids: a track stays as is
// (a downloaded remote track maps to its local copy), an album or an artist
// becomes its tracks.
func (s *LibraryService) resolveSeedTracks(ctx context.Context, seeds []string) []string {
	var out []string
	for _, id := range seeds {
		if IsRemoteID(id) {
			if s.onDemand != nil {
				if local, ok := s.onDemand.LocalTrackIDForRemote(ctx, id); ok {
					out = append(out, local)
				}
			}
			continue
		}
		if _, err := s.catalog.GetTrack(ctx, id); err == nil {
			out = append(out, id)
			continue
		}
		tracks, _ := s.catalog.ListTracksByAlbum(ctx, id)
		if len(tracks) == 0 {
			tracks, _ = s.catalog.ListTracksByArtist(ctx, id, 200)
		}
		for _, t := range tracks {
			out = append(out, t.ID)
		}
	}
	return out
}
