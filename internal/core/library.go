package core

import (
	"context"
	"sort"
	"strings"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

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
