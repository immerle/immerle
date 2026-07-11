package core

import (
	"context"
	"io"
	"testing"

	"github.com/immerle/immerle/internal/providers"
	"github.com/immerle/immerle/internal/testutil"
)

// discographyProvider implements ArtistSearcher + ArtistAlbumLister + AlbumBrowser.
type discographyProvider struct {
	artists []providers.ArtistResult
	albums  map[string][]providers.ProviderAlbum // by artist id
	tracks  map[string][]providers.Result        // by album id
}

func (d *discographyProvider) Name() string       { return "disco" }
func (d *discographyProvider) MaxQuality() string { return "meta" }
func (d *discographyProvider) Search(context.Context, string, int) ([]providers.Result, error) {
	return nil, nil
}
func (d *discographyProvider) Resolve(_ context.Context, id string) (providers.Result, error) {
	return providers.Result{ProviderTrackID: id}, nil
}
func (d *discographyProvider) Download(context.Context, string, io.Writer) error { return nil }
func (d *discographyProvider) SearchArtists(context.Context, string, int) ([]providers.ArtistResult, error) {
	return d.artists, nil
}
func (d *discographyProvider) ArtistAlbums(_ context.Context, artistID string, _ int) ([]providers.ProviderAlbum, error) {
	return d.albums[artistID], nil
}
func (d *discographyProvider) AlbumTracks(_ context.Context, albumID string, _ int) ([]providers.Result, error) {
	return d.tracks[albumID], nil
}

func newDisco(t *testing.T) *CatalogService {
	store := testutil.NewStore(t)
	reg := NewProviderRegistry()
	reg.Register(&discographyProvider{
		artists: []providers.ArtistResult{{ProviderArtistID: "27", Name: "Daft Punk", AlbumCount: 2}},
		albums: map[string][]providers.ProviderAlbum{
			"27": {
				{ProviderAlbumID: "301", Title: "Discovery", Year: 2001, CoverImageURL: "https://e-cdns-images.dzcdn.net/images/cover/h1/500x500.jpg"},
				{ProviderAlbumID: "302", Title: "Homework", Year: 1997},
			},
		},
		tracks: map[string][]providers.Result{
			"301": {
				{ProviderTrackID: "9", Title: "One More Time", Artist: "Daft Punk", Album: "Discovery", TrackNo: 1},
				{ProviderTrackID: "10", Title: "Aerodynamic", Artist: "Daft Punk", Album: "Discovery", TrackNo: 2},
			},
		},
	})
	return NewCatalogService(CatalogServiceConfig{
		Catalog: store.Catalog, Downloads: store.Downloads, Registry: reg,
		Settings: StaticProviderSettings{}, Logger: testutil.NewLogger(),
	})
}

func TestRemoteAlbumsForArtistDiscography(t *testing.T) {
	svc := newDisco(t)
	ctx := context.Background()

	albums, err := svc.RemoteAlbumsForArtist(ctx, "Daft Punk")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 2 {
		t.Fatalf("expected 2 remote albums, got %d", len(albums))
	}
	var discoveryID string
	for _, a := range albums {
		if !IsRemoteAlbumID(a.ID) {
			t.Fatalf("album id should be remote: %q", a.ID)
		}
		if a.Name == "Discovery" {
			discoveryID = a.ID
			if a.Year != 2001 || a.CoverArt == "" {
				t.Fatalf("Discovery metadata wrong: %+v", a)
			}
		}
	}
	if discoveryID == "" {
		t.Fatal("Discovery album not found")
	}

	// Browse the remote album → its tracks (via AlbumBrowser).
	album, tracks, err := svc.RemoteAlbum(ctx, discoveryID)
	if err != nil {
		t.Fatal(err)
	}
	if album.Name != "Discovery" || len(tracks) != 2 || tracks[0].Title != "One More Time" {
		t.Fatalf("unexpected album browse: %+v / %d tracks", album, len(tracks))
	}
	if !tracks[0].Remote || !IsRemoteID(tracks[0].ID) {
		t.Fatalf("album tracks should be remote: %+v", tracks[0])
	}

	// Unknown artist → no albums.
	if a, _ := svc.RemoteAlbumsForArtist(ctx, "Nobody"); len(a) != 0 {
		t.Fatalf("unknown artist should yield no albums, got %d", len(a))
	}
}

func TestRemoteTracksForAlbum(t *testing.T) {
	svc := newDisco(t)
	ctx := context.Background()

	// Matched by artist + album name → the album's full tracklist as remote.
	tracks, err := svc.RemoteTracksForAlbum(ctx, "Daft Punk", "Discovery")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 || tracks[0].Title != "One More Time" || tracks[1].Title != "Aerodynamic" {
		t.Fatalf("unexpected album tracks: %+v", tracks)
	}
	for _, tr := range tracks {
		if !tr.Remote || !IsRemoteID(tr.ID) {
			t.Fatalf("album tracks should be remote/playable: %+v", tr)
		}
	}

	// Unknown artist or album → nothing (caller falls back to local-only).
	if got, _ := svc.RemoteTracksForAlbum(ctx, "Nobody", "Discovery"); len(got) != 0 {
		t.Fatalf("unknown artist should yield no tracks, got %d", len(got))
	}
	if got, _ := svc.RemoteTracksForAlbum(ctx, "Daft Punk", "Random Access Memories"); len(got) != 0 {
		t.Fatalf("unknown album should yield no tracks, got %d", len(got))
	}
}

// TestRemoteTracksForAlbumFallsBackToSearch covers a real bug: a provider that
// only implements the mandatory Search capability (no ArtistSearcher/
// ArtistAlbumLister/AlbumBrowser — true of most on-demand HTTP providers, and
// all three shipped built-ins) made RemoteTracksForAlbum silently return
// nothing, forever, no matter how many times a partially-downloaded album was
// reopened. It must fall back to a plain search, filtered to the requested
// album (and artist, when known).
func TestRemoteTracksForAlbumFallsBackToSearch(t *testing.T) {
	store := testutil.NewStore(t)
	reg := NewProviderRegistry()
	reg.Register(&fakeProvider{name: "basic", results: []providers.Result{
		{ProviderTrackID: "t1", Title: "One More Time", Artist: "Daft Punk", Album: "Discovery"},
		{ProviderTrackID: "t2", Title: "Aerodynamic", Artist: "Daft Punk", Album: "Discovery"},
		{ProviderTrackID: "t3", Title: "Around the World", Artist: "Daft Punk", Album: "Homework"},
	}})
	svc := NewCatalogService(CatalogServiceConfig{
		Catalog: store.Catalog, Downloads: store.Downloads, Registry: reg,
		Settings: StaticProviderSettings{}, Logger: testutil.NewLogger(),
	})
	ctx := context.Background()

	tracks, err := svc.RemoteTracksForAlbum(ctx, "Daft Punk", "Discovery")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks from the Discovery album, got %+v", tracks)
	}
	for _, tr := range tracks {
		if tr.AlbumName != "Discovery" {
			t.Fatalf("a track from a different album leaked in: %+v", tr)
		}
		if !tr.Remote || !IsRemoteID(tr.ID) {
			t.Fatalf("fallback tracks should be remote/playable: %+v", tr)
		}
	}
}
