package spotifyweb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/providers"
)

const pathfinderURL = "https://api-partner.spotify.com/pathfinder/v2/query"

// fetchPlaylistHash identifies, server-side, the persisted GraphQL query the
// web player uses to load a playlist (name, cover and a page of tracks). It's
// stable across web player releases (it names the query text, not this
// request) but may need updating if Spotify republishes the query under a new
// hash.
const fetchPlaylistHash = "21ea11a6ac75a5f3e01fd365d29c18e0a4a8b7dc09f89bbb5d73910190c6acaf"

type pathfinderRequest struct {
	Variables     map[string]any `json:"variables"`
	OperationName string         `json:"operationName"`
	Extensions    struct {
		PersistedQuery struct {
			Version    int    `json:"version"`
			SHA256Hash string `json:"sha256Hash"`
		} `json:"persistedQuery"`
	} `json:"extensions"`
}

type playlistContentsResponse struct {
	Data struct {
		PlaylistV2 struct {
			Typename string `json:"__typename"`
			Message  string `json:"message"` // set when Typename is e.g. "NotFound"
			Name     string `json:"name"`
			Images   struct {
				Items []struct {
					Sources []struct {
						URL string `json:"url"`
					} `json:"sources"`
				} `json:"items"`
			} `json:"images"`
			Content struct {
				Items      []playlistItem `json:"items"`
				TotalCount int            `json:"totalCount"`
			} `json:"content"`
		} `json:"playlistV2"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type playlistItem struct {
	ItemV2 struct {
		Data struct {
			Typename      string `json:"__typename"`
			Name          string `json:"name"`
			URI           string `json:"uri"`
			TrackDuration struct {
				TotalMilliseconds int64 `json:"totalMilliseconds"`
			} `json:"trackDuration"`
			AlbumOfTrack struct {
				Name string `json:"name"`
			} `json:"albumOfTrack"`
			Artists struct {
				Items []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
				} `json:"items"`
			} `json:"artists"`
		} `json:"data"`
	} `json:"itemV2"`
}

// pathfinderQuery runs one persisted GraphQL operation against the web
// player's pathfinder endpoint and decodes the JSON response into out.
func pathfinderQuery(ctx context.Context, client *http.Client, token, operation, hash string, vars map[string]any, out any) error {
	body := pathfinderRequest{Variables: vars, OperationName: operation}
	body.Extensions.PersistedQuery.Version = 1
	body.Extensions.PersistedQuery.SHA256Hash = hash

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pathfinderURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("App-Platform", "WebPlayer")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://open.spotify.com/")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("spotifyweb: %s: %w", operation, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("spotifyweb: %s returned %s", operation, resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, providers.MaxMetadataBytes)).Decode(out); err != nil {
		return fmt.Errorf("spotifyweb: decoding %s response: %w", operation, err)
	}
	return nil
}

// fetchPage fetches one page ([offset, offset+limit)) of a playlist: its
// tracks plus the playlist's name, cover URL and total track count.
func fetchPage(ctx context.Context, client *http.Client, token, playlistID string, offset, limit int) (Playlist, int, error) {
	vars := map[string]any{
		"uri":                            "spotify:playlist:" + playlistID,
		"offset":                         offset,
		"limit":                          limit,
		"enableWatchFeedEntrypoint":      false,
		"includeEpisodeContentRatingsV2": true,
	}
	var out playlistContentsResponse
	if err := pathfinderQuery(ctx, client, token, "fetchPlaylist", fetchPlaylistHash, vars, &out); err != nil {
		return Playlist{}, 0, err
	}
	if len(out.Errors) > 0 {
		return Playlist{}, 0, fmt.Errorf("spotifyweb: pathfinder: %s", out.Errors[0].Message)
	}
	if out.Data.PlaylistV2.Typename != "Playlist" {
		msg := out.Data.PlaylistV2.Message
		if msg == "" {
			msg = out.Data.PlaylistV2.Typename
		}
		return Playlist{}, 0, fmt.Errorf("spotifyweb: %s", msg)
	}

	tracks := make([]Track, 0, len(out.Data.PlaylistV2.Content.Items))
	for _, item := range out.Data.PlaylistV2.Content.Items {
		d := item.ItemV2.Data
		if d.Typename != "Track" {
			continue // ponytail: podcast episodes etc. skipped, importer only models music tracks
		}
		artists := make([]string, 0, len(d.Artists.Items))
		for _, a := range d.Artists.Items {
			artists = append(artists, a.Profile.Name)
		}
		tracks = append(tracks, Track{
			Title:    d.Name,
			Artist:   strings.Join(artists, ", "),
			Album:    d.AlbumOfTrack.Name,
			Duration: time.Duration(d.TrackDuration.TotalMilliseconds) * time.Millisecond,
			URI:      d.URI,
		})
	}
	pl := Playlist{Name: out.Data.PlaylistV2.Name, Tracks: tracks}
	if imgs := out.Data.PlaylistV2.Images.Items; len(imgs) > 0 && len(imgs[0].Sources) > 0 {
		pl.ImageURL = imgs[0].Sources[0].URL
	}
	return pl, out.Data.PlaylistV2.Content.TotalCount, nil
}
