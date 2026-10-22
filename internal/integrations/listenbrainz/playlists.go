package listenbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// RecommendedPlaylistKind identifies one of ListenBrainz's ongoing
// (non-yearly) recommendation playlists -- the "source_patch" identifiers its
// own troi-bot tags each one with. ListenBrainz also generates yearly
// "wrapped"-style playlists (top discoveries/missed recordings of <year>,
// ...); those are a once-a-year historical export, not a rolling
// recommendation, so they're deliberately not synced here.
type RecommendedPlaylistKind string

// The ListenBrainz-generated recommendation playlist kinds RecommendedPlaylists
// looks for -- their "source_patch" values, one per rolling playlist troi-bot
// keeps refreshed.
const (
	DailyJams         RecommendedPlaylistKind = "daily-jams"
	WeeklyJams        RecommendedPlaylistKind = "weekly-jams"
	WeeklyExploration RecommendedPlaylistKind = "weekly-exploration"
)

// recommendedKinds is every kind RecommendedPlaylists looks for.
var recommendedKinds = []RecommendedPlaylistKind{DailyJams, WeeklyJams, WeeklyExploration}

// RecommendedPlaylistTrack is one track of a ListenBrainz-generated
// recommendation playlist, portable (no local track id) -- MBID is the
// MusicBrainz recording id, when ListenBrainz has one for it.
type RecommendedPlaylistTrack struct {
	MBID, Artist, Title, Album string
}

// createdForPlaylist is the shape of one entry in
// GET /1/user/{username}/playlists/createdfor.
type createdForPlaylist struct {
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
	Date       string `json:"date"`
	Extension  struct {
		JSPF struct {
			AdditionalMetadata struct {
				AlgorithmMetadata struct {
					SourcePatch string `json:"source_patch"`
				} `json:"algorithm_metadata"`
			} `json:"additional_metadata"`
		} `json:"https://musicbrainz.org/doc/jspf#playlist"`
	} `json:"extension"`
}

// RecommendedPlaylists resolves token to its ListenBrainz username, then
// fetches the tracks of whichever of that user's Daily Jams/Weekly
// Jams/Weekly Exploration playlists currently exist -- a kind ListenBrainz
// hasn't generated yet (e.g. too little listening history) is simply absent
// from the result map, same as any other empty recommendation source.
func (c *Client) RecommendedPlaylists(ctx context.Context, token string) (map[RecommendedPlaylistKind][]RecommendedPlaylistTrack, error) {
	username, err := c.ValidateToken(ctx, token)
	if err != nil {
		return nil, err
	}

	var list struct {
		Playlists []struct {
			Playlist createdForPlaylist `json:"playlist"`
		} `json:"playlists"`
	}
	// count=50 comfortably covers the 3 rolling kinds alongside years of
	// yearly wrapped-style ones (verified against a long-time real account:
	// 34 total playlists, all 3 rolling kinds present within 50).
	path := fmt.Sprintf("/1/user/%s/playlists/createdfor?count=50", url.PathEscape(username))
	if err := c.get(ctx, token, path, &list); err != nil {
		return nil, err
	}

	// Keep only the latest (by date) entry per wanted kind.
	latest := make(map[RecommendedPlaylistKind]createdForPlaylist)
	for _, e := range list.Playlists {
		kind := RecommendedPlaylistKind(e.Playlist.Extension.JSPF.AdditionalMetadata.AlgorithmMetadata.SourcePatch)
		cur, ok := latest[kind]
		if !ok || e.Playlist.Date > cur.Date {
			latest[kind] = e.Playlist
		}
	}

	out := make(map[RecommendedPlaylistKind][]RecommendedPlaylistTrack, len(recommendedKinds))
	for _, kind := range recommendedKinds {
		p, ok := latest[kind]
		if !ok {
			continue
		}
		mbid := lastPathSegment(p.Identifier)
		if mbid == "" {
			continue
		}
		tracks, err := c.playlistTracks(ctx, token, mbid)
		if err != nil {
			return nil, fmt.Errorf("listenbrainz: fetch %s playlist: %w", kind, err)
		}
		if len(tracks) > 0 {
			out[kind] = tracks
		}
	}
	return out, nil
}

// jspfTrack is one track as returned by GET /1/playlist/{mbid} (JSPF format).
type jspfTrack struct {
	Identifier []string `json:"identifier"`
	Title      string   `json:"title"`
	Creator    string   `json:"creator"`
	Album      string   `json:"album"`
}

// playlistTracks fetches one playlist's tracks by its mbid.
func (c *Client) playlistTracks(ctx context.Context, token, playlistMBID string) ([]RecommendedPlaylistTrack, error) {
	var out struct {
		Playlist struct {
			Track []jspfTrack `json:"track"`
		} `json:"playlist"`
	}
	if err := c.get(ctx, token, "/1/playlist/"+url.PathEscape(playlistMBID), &out); err != nil {
		return nil, err
	}
	tracks := make([]RecommendedPlaylistTrack, 0, len(out.Playlist.Track))
	for _, t := range out.Playlist.Track {
		if t.Title == "" || t.Creator == "" {
			continue
		}
		var mbid string
		if len(t.Identifier) > 0 {
			mbid = lastPathSegment(t.Identifier[0])
		}
		tracks = append(tracks, RecommendedPlaylistTrack{MBID: mbid, Artist: t.Creator, Title: t.Title, Album: t.Album})
	}
	return tracks, nil
}

// get performs an authenticated GET against the ListenBrainz API and decodes
// the JSON response into out.
func (c *Client) get(ctx context.Context, token, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Token "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("listenbrainz: %s: unexpected status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return fmt.Errorf("listenbrainz: %s: decode: %w", path, err)
	}
	return nil
}

// lastPathSegment extracts the trailing UUID from an entity URI, e.g.
// "https://musicbrainz.org/recording/<mbid>" or
// "https://listenbrainz.org/playlist/<mbid>".
func lastPathSegment(uri string) string {
	i := strings.LastIndex(uri, "/")
	if i < 0 {
		return uri
	}
	return uri[i+1:]
}
