package core

import (
	"context"
	"sort"

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

// SimilarSongs is a lightweight heuristic: tracks sharing the seed item's genre
// (falling back to random) since no external recommendation source is wired.
func (s *LibraryService) SimilarSongs(ctx context.Context, userID, seedID string, count int) []TrackEntry {
	genre := ""
	if t, err := s.catalog.GetTrack(ctx, seedID); err == nil {
		genre = t.Genre
	}
	tracks, err := s.catalog.RandomTracks(ctx, count, genre, 0, 0)
	if err != nil {
		return nil
	}
	return s.annotateTracks(ctx, userID, tracks)
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
