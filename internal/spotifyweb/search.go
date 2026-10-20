package spotifyweb

import (
	"context"
	"fmt"
	"strings"
)

// searchPlaylistsHash is the persisted query behind the web player's
// "Playlists" search tab. Like fetchPlaylistContentsHash, it may need
// updating if Spotify republishes the query.
const searchPlaylistsHash = "d520014e748f9ea44f7707d8df1819867ac1205e8b7f3e28f22fe5fc858921b1"

// PlaylistSummary is one public playlist found by SearchPlaylists.
type PlaylistSummary struct {
	ID       string
	Name     string
	Owner    string
	ImageURL string
}

type searchPlaylistsResponse struct {
	Data struct {
		SearchV2 struct {
			Playlists struct {
				Items []struct {
					Data struct {
						Typename string `json:"__typename"`
						Name     string `json:"name"`
						URI      string `json:"uri"`
						Images   struct {
							Items []struct {
								Sources []struct {
									URL string `json:"url"`
								} `json:"sources"`
							} `json:"items"`
						} `json:"images"`
						OwnerV2 struct {
							Data struct {
								Name string `json:"name"`
							} `json:"data"`
						} `json:"ownerV2"`
					} `json:"data"`
				} `json:"items"`
			} `json:"playlists"`
		} `json:"searchV2"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// SearchPlaylists returns up to limit public playlists matching query, in
// Spotify's own relevance order.
func (c *Client) SearchPlaylists(ctx context.Context, query string, limit int) ([]PlaylistSummary, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	vars := map[string]any{
		"searchTerm":         query,
		"offset":             0,
		"limit":              limit,
		"numberOfTopResults": 5,
		"includeAudiobooks":  false,
	}
	var out searchPlaylistsResponse
	if err := pathfinderQuery(ctx, c.http, tok, "searchPlaylists", searchPlaylistsHash, vars, &out); err != nil {
		return nil, err
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("spotifyweb: searchPlaylists: %s", out.Errors[0].Message)
	}
	return toSummaries(out), nil
}

func toSummaries(out searchPlaylistsResponse) []PlaylistSummary {
	items := out.Data.SearchV2.Playlists.Items
	res := make([]PlaylistSummary, 0, len(items))
	for _, it := range items {
		d := it.Data
		id := strings.TrimPrefix(d.URI, "spotify:playlist:")
		if d.Typename != "Playlist" || !playlistIDRe.MatchString(id) {
			continue
		}
		s := PlaylistSummary{ID: id, Name: d.Name, Owner: d.OwnerV2.Data.Name}
		if len(d.Images.Items) > 0 && len(d.Images.Items[0].Sources) > 0 {
			s.ImageURL = d.Images.Items[0].Sources[0].URL
		}
		res = append(res, s)
	}
	return res
}
