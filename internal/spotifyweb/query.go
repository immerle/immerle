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

// fetchPlaylistContentsHash identifies, server-side, the persisted GraphQL
// query the web player uses to page through a playlist's tracks. It's stable
// across web player releases (it names the query text, not this request) but
// may need updating if Spotify republishes the query under a new hash.
const fetchPlaylistContentsHash = "a65e12194ed5fc443a1cdebed5fabe33ca5b07b987185d63c72483867ad13cb4"

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
			Content  struct {
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

// fetchTracksPage fetches one page ([offset, offset+limit)) of a playlist's
// tracks, returning it alongside the playlist's total track count.
func fetchTracksPage(ctx context.Context, client *http.Client, token, playlistID string, offset, limit int) ([]Track, int, error) {
	body := pathfinderRequest{
		Variables: map[string]any{
			"uri":                            "spotify:playlist:" + playlistID,
			"offset":                         offset,
			"limit":                          limit,
			"includeEpisodeContentRatingsV2": true,
		},
		OperationName: "fetchPlaylistContents",
	}
	body.Extensions.PersistedQuery.Version = 1
	body.Extensions.PersistedQuery.SHA256Hash = fetchPlaylistContentsHash

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pathfinderURL, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("App-Platform", "WebPlayer")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://open.spotify.com/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("spotifyweb: fetching playlist tracks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("spotifyweb: pathfinder query returned %s", resp.Status)
	}

	var out playlistContentsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, providers.MaxMetadataBytes)).Decode(&out); err != nil {
		return nil, 0, fmt.Errorf("spotifyweb: decoding pathfinder response: %w", err)
	}
	if len(out.Errors) > 0 {
		return nil, 0, fmt.Errorf("spotifyweb: pathfinder: %s", out.Errors[0].Message)
	}
	if out.Data.PlaylistV2.Typename != "Playlist" {
		msg := out.Data.PlaylistV2.Message
		if msg == "" {
			msg = out.Data.PlaylistV2.Typename
		}
		return nil, 0, fmt.Errorf("spotifyweb: %s", msg)
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
	return tracks, out.Data.PlaylistV2.Content.TotalCount, nil
}
