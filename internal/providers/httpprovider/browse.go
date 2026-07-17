package httpprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/immerle/immerle/internal/providers"
)

// errUnsupported marks an endpoint the remote service does not implement (404).
// Capability methods translate it to an empty result so the artist page simply
// isn't enriched, instead of surfacing an error.
var errUnsupported = errors.New("endpoint not supported")

// getJSON performs a GET against path?q and decodes the body into out. A 404 is
// reported as errUnsupported.
func (p *Provider) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	req, err := p.newRequest(ctx, path, q)
	if err != nil {
		return err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errUnsupported
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s status %d", p.name, path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// wire shapes for browsing.

type artistWire struct {
	ProviderArtistID string `json:"providerArtistId"`
	Name             string `json:"name"`
	AlbumCount       int    `json:"albumCount"`
	ImageURL         string `json:"imageUrl"`
}

type albumWire struct {
	ProviderAlbumID string `json:"providerAlbumId"`
	Title           string `json:"title"`
	Year            int    `json:"year"`
	CoverImageURL   string `json:"coverImageUrl"`
}

type playlistWire struct {
	ProviderPlaylistID string  `json:"providerPlaylistId"`
	Name               string  `json:"name"`
	CoverImageURL      string  `json:"coverImageUrl"`
	Tracks             []track `json:"tracks"`
}

// SearchArtists implements providers.ArtistSearcher.
//
//	GET {searchArtistsPath}?q=&limit= → {"artists":[<artist>]}
func (p *Provider) SearchArtists(ctx context.Context, query string, limit int) ([]providers.ArtistResult, error) {
	if limit <= 0 {
		limit = 20
	}
	q := url.Values{"q": {query}, "limit": {strconv.Itoa(limit)}}
	var body struct {
		Artists []artistWire `json:"artists"`
	}
	if err := p.getJSON(ctx, searchArtistsPath, q, &body); err != nil {
		if errors.Is(err, errUnsupported) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]providers.ArtistResult, 0, len(body.Artists))
	for _, a := range body.Artists {
		if a.ProviderArtistID == "" && a.Name == "" {
			continue
		}
		out = append(out, providers.ArtistResult{
			ProviderArtistID: a.ProviderArtistID,
			Name:             a.Name,
			AlbumCount:       a.AlbumCount,
			ImageURL:         a.ImageURL,
		})
	}
	return out, nil
}

// ArtistAlbums implements providers.ArtistAlbumLister.
//
//	GET {artistAlbumsPath}?id=&limit= → {"albums":[<album>]}
func (p *Provider) ArtistAlbums(ctx context.Context, providerArtistID string, limit int) ([]providers.ProviderAlbum, error) {
	if limit <= 0 {
		limit = 100
	}
	q := url.Values{"id": {providerArtistID}, "limit": {strconv.Itoa(limit)}}
	var body struct {
		Albums []albumWire `json:"albums"`
	}
	if err := p.getJSON(ctx, artistAlbumsPath, q, &body); err != nil {
		if errors.Is(err, errUnsupported) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]providers.ProviderAlbum, 0, len(body.Albums))
	for _, a := range body.Albums {
		if a.ProviderAlbumID == "" {
			continue
		}
		out = append(out, providers.ProviderAlbum{
			ProviderAlbumID: a.ProviderAlbumID,
			Title:           a.Title,
			Year:            a.Year,
			CoverImageURL:   a.CoverImageURL,
		})
	}
	return out, nil
}

// ArtistTracks implements providers.ArtistBrowser (an artist's top tracks).
//
//	GET {artistTracksPath}?id=&limit= → {"results":[<track>]}
func (p *Provider) ArtistTracks(ctx context.Context, providerArtistID string, limit int) ([]providers.Result, error) {
	if limit <= 0 {
		limit = 50
	}
	return p.tracksAt(ctx, artistTracksPath, providerArtistID, limit)
}

// AlbumTracks implements providers.AlbumBrowser.
//
//	GET {albumTracksPath}?id=&limit= → {"results":[<track>]}
func (p *Provider) AlbumTracks(ctx context.Context, providerAlbumID string, limit int) ([]providers.Result, error) {
	if limit <= 0 {
		limit = 200
	}
	return p.tracksAt(ctx, albumTracksPath, providerAlbumID, limit)
}

// Playlists implements providers.PlaylistBrowser.
//
//	GET {playlistsPath}?limit= → {"playlists":[<playlist>]}
func (p *Provider) Playlists(ctx context.Context, limit int) ([]providers.ProviderPlaylist, error) {
	if limit <= 0 {
		limit = 20
	}
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	var body struct {
		Playlists []playlistWire `json:"playlists"`
	}
	if err := p.getJSON(ctx, playlistsPath, q, &body); err != nil {
		if errors.Is(err, errUnsupported) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]providers.ProviderPlaylist, 0, len(body.Playlists))
	for _, pl := range body.Playlists {
		if pl.ProviderPlaylistID == "" {
			continue
		}
		tracks := make([]providers.Result, 0, len(pl.Tracks))
		for _, t := range pl.Tracks {
			if t.ProviderTrackID == "" {
				continue
			}
			tracks = append(tracks, t.toResult())
		}
		out = append(out, providers.ProviderPlaylist{
			ProviderPlaylistID: pl.ProviderPlaylistID,
			Name:               pl.Name,
			CoverImageURL:      pl.CoverImageURL,
			Tracks:             tracks,
		})
	}
	return out, nil
}

// ArtistImage implements providers.ArtistImageSearcher.
//
//	GET {artistImagePath}?name= → {"imageUrl":"..."}
func (p *Provider) ArtistImage(ctx context.Context, name string) (string, error) {
	var body struct {
		ImageURL string `json:"imageUrl"`
	}
	if err := p.getJSON(ctx, artistImagePath, url.Values{"name": {name}}, &body); err != nil {
		if errors.Is(err, errUnsupported) {
			return "", nil
		}
		return "", err
	}
	return body.ImageURL, nil
}

// tracksAt GETs a {"results":[<track>]} endpoint keyed by id.
func (p *Provider) tracksAt(ctx context.Context, path, id string, limit int) ([]providers.Result, error) {
	q := url.Values{"id": {id}, "limit": {strconv.Itoa(limit)}}
	var body struct {
		Results []track `json:"results"`
	}
	if err := p.getJSON(ctx, path, q, &body); err != nil {
		if errors.Is(err, errUnsupported) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]providers.Result, 0, len(body.Results))
	for _, t := range body.Results {
		if t.ProviderTrackID == "" {
			continue
		}
		out = append(out, t.toResult())
	}
	return out, nil
}
