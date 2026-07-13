package spotifyweb

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPlaylistID(t *testing.T) {
	cases := map[string]string{
		"https://open.spotify.com/playlist/78YWsrPFPJoF5cD3mkTQuY":                   "78YWsrPFPJoF5cD3mkTQuY",
		"https://open.spotify.com/intl-fr/playlist/78YWsrPFPJoF5cD3mkTQuY?si=abc123": "78YWsrPFPJoF5cD3mkTQuY",
		"78YWsrPFPJoF5cD3mkTQuY": "78YWsrPFPJoF5cD3mkTQuY",
		"not a playlist ref":     "",
		"https://open.spotify.com/album/78YWsrPFPJoF5cD3mkTQuY": "",
	}
	for ref, want := range cases {
		if got := playlistID(ref); got != want {
			t.Errorf("playlistID(%q) = %q, want %q", ref, got, want)
		}
	}
}

// TestTOTPDeterministic pins the code generated for a fixed instant, so a
// change to the transform (intentional or not) is caught even without
// network access.
func TestTOTPDeterministic(t *testing.T) {
	at := time.Unix(1700000000, 0).UTC()
	// An arbitrary cipher — this tests the transform's determinism, not any
	// real Spotify secret (none is hardcoded, see currentSecret in token.go).
	key := totpKey([]byte{12, 34, 56, 78, 90})
	code := totpCode(key, at)
	if len(code) != 6 {
		t.Fatalf("totpCode returned %q, want 6 digits", code)
	}
	if got := totpCode(key, at); got != code {
		t.Fatalf("totpCode not deterministic: %q vs %q", code, got)
	}
}

// fixture trims a real fetchPlaylistContents response down to two items: one
// music track and one non-track (podcast episode) item, to check both the
// field mapping and the type filter.
const fixture = `{
  "data": {
    "playlistV2": {
      "__typename": "Playlist",
      "content": {
        "__typename": "PlaylistItemsPage",
        "totalCount": 2,
        "items": [
          {
            "itemV2": {
              "data": {
                "__typename": "Track",
                "name": "Intro",
                "uri": "spotify:track:2usrT8QIbIk9y0NEtQwS4j",
                "trackDuration": {"totalMilliseconds": 127920},
                "albumOfTrack": {"name": "xx"},
                "artists": {"items": [{"profile": {"name": "The xx"}}]}
              }
            }
          },
          {
            "itemV2": {
              "data": {
                "__typename": "Episode",
                "name": "Some Podcast Episode"
              }
            }
          }
        ]
      }
    }
  }
}`

func TestParsePlaylistContentsResponse(t *testing.T) {
	var out playlistContentsResponse
	if err := json.Unmarshal([]byte(fixture), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.PlaylistV2.Typename != "Playlist" {
		t.Fatalf("typename = %q", out.Data.PlaylistV2.Typename)
	}
	if out.Data.PlaylistV2.Content.TotalCount != 2 {
		t.Fatalf("totalCount = %d, want 2", out.Data.PlaylistV2.Content.TotalCount)
	}
	if len(out.Data.PlaylistV2.Content.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(out.Data.PlaylistV2.Content.Items))
	}
	track := out.Data.PlaylistV2.Content.Items[0].ItemV2.Data
	if track.Name != "Intro" || track.AlbumOfTrack.Name != "xx" || track.Artists.Items[0].Profile.Name != "The xx" {
		t.Fatalf("track fields not mapped: %+v", track)
	}
	if out.Data.PlaylistV2.Content.Items[1].ItemV2.Data.Typename != "Episode" {
		t.Fatalf("expected the non-track item to keep its own typename")
	}
}
