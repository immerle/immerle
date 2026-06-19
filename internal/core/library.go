package core

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// remoteFetchTimeout bounds the provider calls made while assembling an
// artist's or album's tracklist, so a slow provider can't hang a request.
const remoteFetchTimeout = 12 * time.Second

// LibraryService holds the catalog browsing/search business logic shared by every
// presentation layer (Subsonic, REST). Methods return domain models and per-user
// annotation maps; the presentation layers own all serialization. As more
// endpoints are extracted from the handlers, their orchestration lands here.
type LibraryService struct {
	catalog     *persistence.CatalogRepo
	annotations *persistence.AnnotationRepo
	// onDemand is optional (remote provider integration); may be nil when disabled.
	onDemand *CatalogService
}

// NewLibraryService wires the library application service. onDemand may be nil.
func NewLibraryService(catalog *persistence.CatalogRepo, annotations *persistence.AnnotationRepo, onDemand *CatalogService) *LibraryService {
	return &LibraryService{catalog: catalog, annotations: annotations, onDemand: onDemand}
}

// Final search result caps applied to the merged local+remote lists after
// re-sorting by relevance.
const (
	maxSearchArtists = 4
	maxSearchAlbums  = 10
	maxSearchSongs   = 10
)

// SearchResults is a presentation-neutral search result. The annotation maps
// carry per-user state (star/rating/play count) keyed by item id; presentation
// layers attach them when serializing. Remote-provider results have no
// annotations, so a missing key simply means "no per-user state".
type SearchResults struct {
	Artists []models.Artist
	Albums  []models.Album
	Tracks  []models.Track

	AlbumAnnotations map[string]models.Annotation
	TrackAnnotations map[string]models.Annotation
}

// Search runs a catalog search, merges in remote-provider results (when on-demand
// is enabled and the query is non-empty), re-sorts the merged lists by relevance
// and caps them. Counts bound the local query; the final caps are fixed.
func (s *LibraryService) Search(ctx context.Context, userID, query string, artistCount, albumCount, songCount int) (SearchResults, error) {
	// Subsonic clients sometimes quote queries or use "" to mean "everything".
	query = strings.Trim(strings.TrimSpace(query), "\"")

	artists, albums, tracks, err := s.catalog.Search(ctx, query, artistCount, albumCount, songCount)
	if err != nil {
		return SearchResults{}, err
	}

	albumAnn, _ := s.annotations.AnnotationMap(ctx, userID, models.ItemAlbum)
	trackAnn, _ := s.annotations.AnnotationMap(ctx, userID, models.ItemTrack)

	// Merge remote results from every active provider, deduplicated by name for
	// artists/albums and by id for songs.
	if s.onDemand != nil && query != "" {
		rArtists, rAlbums, rSongs := s.onDemand.RemoteSearch3(ctx, query, maxSearchArtists, maxSearchAlbums, maxSearchSongs)

		seenA := make(map[string]bool, len(artists))
		for _, a := range artists {
			seenA[strings.ToLower(a.Name)] = true
		}
		for _, a := range rArtists {
			if seenA[strings.ToLower(a.Name)] {
				continue
			}
			seenA[strings.ToLower(a.Name)] = true
			artists = append(artists, a)
		}

		seenAl := make(map[string]bool, len(albums))
		for _, a := range albums {
			seenAl[strings.ToLower(a.ArtistName+"|"+a.Name)] = true
		}
		for _, a := range rAlbums {
			if seenAl[strings.ToLower(a.ArtistName+"|"+a.Name)] {
				continue
			}
			seenAl[strings.ToLower(a.ArtistName+"|"+a.Name)] = true
			albums = append(albums, a)
		}

		seenS := make(map[string]bool, len(tracks))
		for _, t := range tracks {
			seenS[t.ID] = true
		}
		for _, t := range rSongs {
			if seenS[t.ID] {
				continue
			}
			seenS[t.ID] = true
			tracks = append(tracks, t)
		}
	}

	sort.SliceStable(artists, func(i, j int) bool {
		return relevance(query, artists[i].Name) < relevance(query, artists[j].Name)
	})
	sort.SliceStable(albums, func(i, j int) bool {
		return relevance(query, albums[i].Name) < relevance(query, albums[j].Name)
	})
	sort.SliceStable(tracks, func(i, j int) bool {
		return relevance(query, tracks[i].Title) < relevance(query, tracks[j].Title)
	})

	return SearchResults{
		Artists:          capSlice(artists, maxSearchArtists),
		Albums:           capSlice(albums, maxSearchAlbums),
		Tracks:           capSlice(tracks, maxSearchSongs),
		AlbumAnnotations: albumAnn,
		TrackAnnotations: trackAnn,
	}, nil
}

// relevance scores how well s matches the query for search ordering: exact (0),
// prefix (1), substring (2), otherwise (3). Lower is better; ties keep input
// order (stable sort).
func relevance(query, s string) int {
	q, x := strings.ToLower(strings.TrimSpace(query)), strings.ToLower(s)
	switch {
	case q == "" || x == q:
		return 0
	case strings.HasPrefix(x, q):
		return 1
	case strings.Contains(x, q):
		return 2
	default:
		return 3
	}
}

// capSlice truncates s to at most n elements.
func capSlice[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Song returns a single track with the caller's annotation attached (always
// non-nil, so absent per-user state reads as zero). Returns
// persistence.ErrNotFound when the track does not exist.
func (s *LibraryService) Song(ctx context.Context, userID, id string) (TrackEntry, error) {
	t, err := s.catalog.GetTrack(ctx, id)
	if err != nil {
		return TrackEntry{}, err
	}
	ann, _ := s.annotations.Get(ctx, userID, models.ItemTrack, id)
	return TrackEntry{Track: t, Annotation: &ann}, nil
}

// Artists returns every artist plus the caller's per-artist starred state. The
// presentation layer decides how to group/paginate them.
func (s *LibraryService) Artists(ctx context.Context, userID string) ([]models.Artist, map[string]models.Annotation, error) {
	artists, err := s.catalog.ListArtists(ctx)
	if err != nil {
		return nil, nil, err
	}
	starred, _ := s.annotations.AnnotationMap(ctx, userID, models.ItemArtist)
	return artists, starred, nil
}

// ---- artist / album detail ----

// TrackEntry pairs a track with the caller's resolved annotation (star/rating/
// play count). The annotation is nil when there is no per-user state.
type TrackEntry struct {
	Track      models.Track
	Annotation *models.Annotation
}

// AlbumEntry pairs an album with its annotation and, when songs were requested,
// its tracks. Tracks is nil when songs were not loaded.
type AlbumEntry struct {
	Album      models.Album
	Annotation *models.Annotation
	Tracks     []TrackEntry
}

// ArtistResult is an artist with its (local + remote) albums.
type ArtistResult struct {
	Artist     models.Artist
	Annotation *models.Annotation
	Albums     []AlbumEntry
}

// AlbumResult is an album with its merged (local + remote) tracklist.
type AlbumResult struct {
	Album      models.Album
	Annotation *models.Annotation
	Tracks     []TrackEntry
}

// GetArtist returns an artist with its albums. Local albums come from the
// catalog; when a provider is configured, the rest of the discography is merged
// in (deduped by name) as remote play-on-demand entries. With includeSongs each
// local album's tracks are loaded and remote albums fetched concurrently.
// Returns persistence.ErrNotFound when the artist does not exist.
func (s *LibraryService) GetArtist(ctx context.Context, userID, id string, includeSongs bool) (ArtistResult, error) {
	if IsRemoteArtistID(id) && s.onDemand != nil {
		return s.remoteArtist(ctx, id)
	}

	artist, err := s.catalog.GetArtist(ctx, id)
	if err != nil {
		return ArtistResult{}, err
	}
	albums, err := s.catalog.ListAlbumsByArtist(ctx, id)
	if err != nil {
		return ArtistResult{}, err
	}

	albumAnn, _ := s.annotations.AnnotationMap(ctx, userID, models.ItemAlbum)
	artistAnn, _ := s.annotations.Get(ctx, userID, models.ItemArtist, id)

	entries := make([]AlbumEntry, 0, len(albums))
	seen := make(map[string]bool, len(albums))
	for _, a := range albums {
		entries = append(entries, AlbumEntry{Album: a, Annotation: annPtr(albumAnn, a.ID)})
		seen[strings.ToLower(a.Name)] = true
	}
	if s.onDemand != nil {
		if remote, err := s.onDemand.RemoteAlbumsForArtist(ctx, artist.Name); err == nil {
			for _, ra := range remote {
				if seen[strings.ToLower(ra.Name)] {
					continue
				}
				seen[strings.ToLower(ra.Name)] = true
				entries = append(entries, AlbumEntry{Album: ra})
			}
		}
	}
	artist.AlbumCount = len(entries)

	if includeSongs {
		s.fillAlbumSongs(ctx, userID, entries)
	}
	return ArtistResult{Artist: artist, Annotation: &artistAnn, Albums: entries}, nil
}

// remoteArtist renders a provider (remote) artist with its albums (songs are not
// inlined, matching the historical behavior).
func (s *LibraryService) remoteArtist(ctx context.Context, id string) (ArtistResult, error) {
	artist, albums, err := s.onDemand.RemoteArtist(ctx, id)
	if err != nil || artist.Name == "" {
		return ArtistResult{}, persistence.ErrNotFound
	}
	entries := make([]AlbumEntry, 0, len(albums))
	for _, a := range albums {
		entries = append(entries, AlbumEntry{Album: a})
	}
	return ArtistResult{Artist: artist, Albums: entries}, nil
}

// fillAlbumSongs populates each album's Tracks: local albums from the catalog
// (cheap), remote albums from the provider (fetched concurrently, bounded, with
// an overall timeout). Each goroutine writes a distinct index — no race.
func (s *LibraryService) fillAlbumSongs(ctx context.Context, userID string, entries []AlbumEntry) {
	trackAnn, _ := s.annotations.AnnotationMap(ctx, userID, models.ItemTrack)

	var remote []int
	for i := range entries {
		if IsRemoteAlbumID(entries[i].Album.ID) {
			remote = append(remote, i)
			continue
		}
		tracks, err := s.catalog.ListTracksByAlbum(ctx, entries[i].Album.ID)
		if err != nil {
			continue
		}
		entries[i].Tracks = toTrackEntries(tracks, trackAnn)
	}

	if s.onDemand == nil || len(remote) == 0 {
		return
	}
	rctx, cancel := context.WithTimeout(ctx, remoteFetchTimeout)
	defer cancel()
	sem := make(chan struct{}, 6) // bound concurrent provider calls
	var wg sync.WaitGroup
	for _, idx := range remote {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			_, tracks, err := s.onDemand.RemoteAlbum(rctx, entries[i].Album.ID)
			if err != nil {
				return
			}
			out := make([]TrackEntry, 0, len(tracks))
			for _, t := range tracks {
				out = append(out, TrackEntry{Track: t, Annotation: s.localAnn(rctx, trackAnn, t.ID)})
			}
			entries[i].Tracks = out // distinct index per goroutine — no race
		}(idx)
	}
	wg.Wait()
}

// GetAlbum returns an album with its merged (local + remote) tracklist, ordered
// by disc/track. SongCount and Duration reflect the enriched list. Returns
// persistence.ErrNotFound when the album does not exist.
func (s *LibraryService) GetAlbum(ctx context.Context, userID, id string) (AlbumResult, error) {
	if IsRemoteAlbumID(id) && s.onDemand != nil {
		return s.remoteAlbum(ctx, userID, id)
	}

	album, err := s.catalog.GetAlbum(ctx, id)
	if err != nil {
		return AlbumResult{}, err
	}
	albumAnn, _ := s.annotations.Get(ctx, userID, models.ItemAlbum, id)
	trackAnn, _ := s.annotations.AnnotationMap(ctx, userID, models.ItemTrack)

	tracks := s.albumSongs(ctx, album, trackAnn)
	album.SongCount = len(tracks)
	album.Duration = 0
	for _, te := range tracks {
		album.Duration += te.Track.Duration
	}
	return AlbumResult{Album: album, Annotation: &albumAnn, Tracks: tracks}, nil
}

// remoteAlbum renders a provider (remote) album with its tracks.
func (s *LibraryService) remoteAlbum(ctx context.Context, userID, id string) (AlbumResult, error) {
	album, tracks, err := s.onDemand.RemoteAlbum(ctx, id)
	if err != nil || album.Name == "" {
		return AlbumResult{}, persistence.ErrNotFound
	}
	trackAnn, _ := s.annotations.AnnotationMap(ctx, userID, models.ItemTrack)
	out := make([]TrackEntry, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, TrackEntry{Track: t, Annotation: s.localAnn(ctx, trackAnn, t.ID)})
	}
	return AlbumResult{Album: album, Tracks: out}, nil
}

// albumSongs returns an album's songs: the local tracks plus — when a provider
// is configured — the rest of the album's tracks fetched from the provider (the
// ones the user does not own, as remote play-on-demand entries), deduped by
// title and ordered by disc/track.
func (s *LibraryService) albumSongs(ctx context.Context, album models.Album, trackAnn map[string]models.Annotation) []TrackEntry {
	local, _ := s.catalog.ListTracksByAlbum(ctx, album.ID)
	out := make([]TrackEntry, 0, len(local))
	seen := make(map[string]bool, len(local))
	for _, t := range local {
		out = append(out, TrackEntry{Track: t, Annotation: annPtr(trackAnn, t.ID)})
		if k := albumTrackKey(t.Title); k != "" {
			seen[k] = true
		}
	}

	if s.onDemand != nil && strings.TrimSpace(album.Name) != "" {
		rctx, cancel := context.WithTimeout(ctx, remoteFetchTimeout)
		defer cancel()
		if remote, err := s.onDemand.RemoteTracksForAlbum(rctx, album.ArtistName, album.Name); err == nil {
			for _, t := range remote {
				k := albumTrackKey(t.Title)
				if k == "" || seen[k] {
					continue
				}
				seen[k] = true
				t.AlbumID = album.ID // keep the client on this album page
				out = append(out, TrackEntry{Track: t, Annotation: s.localAnn(rctx, trackAnn, t.ID)})
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Track.DiscNo != out[j].Track.DiscNo {
			return out[i].Track.DiscNo < out[j].Track.DiscNo
		}
		return out[i].Track.TrackNo < out[j].Track.TrackNo
	})
	return out
}

// localAnn resolves a track's annotation. For a remote (provider) track already
// downloaded locally, it falls back to the local copy's state so a liked title
// still shows as liked under its remote id.
func (s *LibraryService) localAnn(ctx context.Context, trackAnn map[string]models.Annotation, id string) *models.Annotation {
	if a := annPtr(trackAnn, id); a != nil {
		return a
	}
	if s.onDemand != nil && IsRemoteID(id) {
		if localID, ok := s.onDemand.LocalTrackIDForRemote(ctx, id); ok {
			return annPtr(trackAnn, localID)
		}
	}
	return nil
}

func toTrackEntries(tracks []models.Track, ann map[string]models.Annotation) []TrackEntry {
	out := make([]TrackEntry, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, TrackEntry{Track: t, Annotation: annPtr(ann, t.ID)})
	}
	return out
}

// albumTrackKey normalizes a title for matching a remote track against an owned one.
func albumTrackKey(title string) string {
	return strings.ToLower(strings.TrimSpace(title))
}

func annPtr(m map[string]models.Annotation, id string) *models.Annotation {
	if a, ok := m[id]; ok {
		return &a
	}
	return nil
}
