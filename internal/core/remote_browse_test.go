package core

import (
	"context"
	"io"
	"testing"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/providers"
	"github.com/immerle/immerle/internal/testutil"
)

// browsableProvider implements Provider + ArtistBrowser for remote-browse tests.
type browsableProvider struct {
	name         string
	searchResult []providers.Result
	artistTracks map[string][]providers.Result
}

func (b *browsableProvider) Name() string       { return b.name }
func (b *browsableProvider) MaxQuality() string { return "test" }
func (b *browsableProvider) Search(_ context.Context, _ string, _ int) ([]providers.Result, error) {
	return b.searchResult, nil
}
func (b *browsableProvider) Resolve(_ context.Context, id string) (providers.Result, error) {
	return providers.Result{ProviderTrackID: id}, nil
}
func (b *browsableProvider) Download(_ context.Context, _ string, _ io.Writer) error { return nil }
func (b *browsableProvider) ArtistTracks(_ context.Context, artistID string, _ int) ([]providers.Result, error) {
	return b.artistTracks[artistID], nil
}

// searcherProvider implements ArtistSearcher (accurate album counts).
type searcherProvider struct {
	browsableProvider
	artists []providers.ArtistResult
}

func (s *searcherProvider) SearchArtists(_ context.Context, _ string, _ int) ([]providers.ArtistResult, error) {
	return s.artists, nil
}

func TestRemoteSearch3ArtistAlbumCount(t *testing.T) {
	store := testutil.NewStore(t)
	registry := NewProviderRegistry()
	registry.Register(&searcherProvider{
		browsableProvider: browsableProvider{name: "prov"},
		artists: []providers.ArtistResult{
			{ProviderArtistID: "27", Name: "Famous", AlbumCount: 14, ImageURL: "https://e-cdns-images.dzcdn.net/images/artist/x/500x500.jpg"},
		},
	})
	svc := NewCatalogService(CatalogServiceConfig{
		Catalog: store.Catalog, Downloads: store.Downloads, Registry: registry,
		Settings: StaticProviderSettings{}, Logger: testutil.NewLogger(),
	})

	artists, _, _ := svc.RemoteSearch3(context.Background(), "famous", 10, 10, 10)
	if len(artists) != 1 || artists[0].AlbumCount != 14 {
		t.Fatalf("expected remote artist with albumCount=14, got %+v", artists)
	}
	if artists[0].CoverArt == "" {
		t.Fatal("remote artist should carry a cover id")
	}
}

func newBrowseService(t *testing.T) *CatalogService {
	store := testutil.NewStore(t)
	registry := NewProviderRegistry()
	registry.Register(&browsableProvider{
		name: "prov",
		searchResult: []providers.Result{
			{ProviderTrackID: "t1", Title: "Hit One", Artist: "Famous", ProviderArtistID: "777", Album: "Greatest"},
		},
		artistTracks: map[string][]providers.Result{
			"777": {
				{ProviderTrackID: "t1", Title: "Hit One", Artist: "Famous", ProviderArtistID: "777", Album: "Greatest"},
				{ProviderTrackID: "t2", Title: "Hit Two", Artist: "Famous", ProviderArtistID: "777", Album: "Greatest"},
				{ProviderTrackID: "t3", Title: "Deep Cut", Artist: "Famous", ProviderArtistID: "777", Album: "Early Days"},
			},
		},
	})
	return NewCatalogService(CatalogServiceConfig{
		Catalog: store.Catalog, Downloads: store.Downloads, Registry: registry,
		Settings: StaticProviderSettings{}, Logger: testutil.NewLogger(),
	})
}

func TestRemoteSearch3DerivesArtists(t *testing.T) {
	svc := newBrowseService(t)
	artists, _, _ := svc.RemoteSearch3(context.Background(), "famous", 10, 10, 10)
	if len(artists) != 1 || artists[0].Name != "Famous" {
		t.Fatalf("expected 1 remote artist 'Famous', got %+v", artists)
	}
	if !IsRemoteArtistID(artists[0].ID) {
		t.Fatalf("artist id should be a remote artist id: %q", artists[0].ID)
	}
}

// hasProvider reports whether any track came from the named provider.
func hasProvider(tracks []models.Track, name string) bool {
	for _, t := range tracks {
		if t.Provider == name {
			return true
		}
	}
	return false
}

func TestRemoteSearch3QueriesAllProviders(t *testing.T) {
	store := testutil.NewStore(t)
	registry := NewProviderRegistry()
	registry.Register(&browsableProvider{
		name:         "first",
		searchResult: []providers.Result{{ProviderTrackID: "f1", Title: "Solo", Artist: "Alpha", Album: "A"}},
	})
	registry.Register(&browsableProvider{
		name:         "second",
		searchResult: []providers.Result{{ProviderTrackID: "s1", Title: "Hit", Artist: "Beta", Album: "B"}},
	})
	svc := NewCatalogService(CatalogServiceConfig{
		Catalog: store.Catalog, Downloads: store.Downloads, Registry: registry,
		Settings: StaticProviderSettings{}, Logger: testutil.NewLogger(),
	})

	_, _, tracks := svc.RemoteSearch3(context.Background(), "x", 4, 10, 10)
	if !hasProvider(tracks, "first") || !hasProvider(tracks, "second") {
		t.Fatalf("expected every active provider to contribute, got %+v", tracks)
	}
}

func TestRemoteSearch3DerivesAlbums(t *testing.T) {
	store := testutil.NewStore(t)
	registry := NewProviderRegistry()
	registry.Register(&browsableProvider{
		name: "prov",
		searchResult: []providers.Result{
			{ProviderTrackID: "t1", Title: "One", Artist: "Beta", Album: "Greatest"},
			{ProviderTrackID: "t2", Title: "Two", Artist: "Beta", Album: "Greatest"},
			{ProviderTrackID: "t3", Title: "Three", Artist: "Beta", Album: "Early"},
		},
	})
	svc := NewCatalogService(CatalogServiceConfig{
		Catalog: store.Catalog, Downloads: store.Downloads, Registry: registry,
		Settings: StaticProviderSettings{}, Logger: testutil.NewLogger(),
	})

	_, albums, _ := svc.RemoteSearch3(context.Background(), "x", 4, 10, 10)
	if len(albums) != 2 {
		t.Fatalf("expected 2 distinct derived albums, got %d (%+v)", len(albums), albums)
	}
	for _, a := range albums {
		if !IsRemoteAlbumID(a.ID) {
			t.Fatalf("derived album should carry a remote album id: %q", a.ID)
		}
	}
}

func TestRemoteArtistAndAlbumBrowse(t *testing.T) {
	svc := newBrowseService(t)
	ctx := context.Background()

	artists, _, _ := svc.RemoteSearch3(ctx, "famous", 10, 10, 10)
	rartID := artists[0].ID

	// Browse the artist → 2 albums (Greatest: 2 songs, Early Days: 1 song).
	artist, albums, err := svc.RemoteArtist(ctx, rartID)
	if err != nil {
		t.Fatal(err)
	}
	if artist.Name != "Famous" || len(albums) != 2 {
		t.Fatalf("expected 2 albums, got %d (%+v)", len(albums), albums)
	}
	var greatest string
	for _, a := range albums {
		if a.Name == "Greatest" {
			greatest = a.ID
			if a.SongCount != 2 {
				t.Fatalf("Greatest should have 2 songs, got %d", a.SongCount)
			}
		}
		if !IsRemoteAlbumID(a.ID) {
			t.Fatalf("album id should be remote: %q", a.ID)
		}
	}
	if greatest == "" {
		t.Fatal("Greatest album not found")
	}

	// Browse the album → its 2 tracks.
	album, tracks, err := svc.RemoteAlbum(ctx, greatest)
	if err != nil {
		t.Fatal(err)
	}
	if album.Name != "Greatest" || len(tracks) != 2 {
		t.Fatalf("expected 2 tracks in Greatest, got %d", len(tracks))
	}
	for _, tr := range tracks {
		if !tr.Remote || !IsRemoteID(tr.ID) {
			t.Fatalf("remote album track should have a remote id: %+v", tr)
		}
	}
}
