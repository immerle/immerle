package listenbrainz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// createdForBody is a trimmed real response shape (captured from
// GET /1/user/{username}/playlists/createdfor), keeping only what
// RecommendedPlaylists reads: one daily-jams and two weekly-exploration
// entries (to verify the newest-by-date one wins) plus an unrelated yearly
// "wrapped" playlist that must be ignored.
const createdForBody = `{
  "playlists": [
    {"playlist": {
      "identifier": "https://listenbrainz.org/playlist/0fda81ea-1be8-4d91-b139-e9e9f60d983a",
      "title": "Daily Jams for kilian, 2026-07-25",
      "date": "2026-07-25T19:00:18.771589+00:00",
      "extension": {"https://musicbrainz.org/doc/jspf#playlist": {
        "additional_metadata": {"algorithm_metadata": {"source_patch": "daily-jams"}}
      }}
    }},
    {"playlist": {
      "identifier": "https://listenbrainz.org/playlist/older-exploration",
      "title": "Weekly Exploration (old)",
      "date": "2026-07-10T19:00:00.000000+00:00",
      "extension": {"https://musicbrainz.org/doc/jspf#playlist": {
        "additional_metadata": {"algorithm_metadata": {"source_patch": "weekly-exploration"}}
      }}
    }},
    {"playlist": {
      "identifier": "https://listenbrainz.org/playlist/newer-exploration",
      "title": "Weekly Exploration (new)",
      "date": "2026-07-24T19:00:00.000000+00:00",
      "extension": {"https://musicbrainz.org/doc/jspf#playlist": {
        "additional_metadata": {"algorithm_metadata": {"source_patch": "weekly-exploration"}}
      }}
    }},
    {"playlist": {
      "identifier": "https://listenbrainz.org/playlist/wrapped-2024",
      "title": "Top Discoveries of 2024",
      "date": "2024-01-01T00:00:00.000000+00:00",
      "extension": {"https://musicbrainz.org/doc/jspf#playlist": {
        "additional_metadata": {"algorithm_metadata": {"source_patch": "top-discoveries-of-2024"}}
      }}
    }}
  ]
}`

// playlistTrackBody is a trimmed real GET /1/playlist/{mbid} response.
const playlistTrackBody = `{
  "playlist": {
    "title": "Daily Jams",
    "track": [
      {
        "identifier": ["https://musicbrainz.org/recording/f1f21661-d4be-49c6-aeb6-050d35638d43"],
        "title": "Diet Mountain Dew",
        "creator": "Lana Del Rey",
        "album": "Born to Die"
      },
      {"title": "No creator, must be skipped", "creator": ""}
    ]
  }
}`

func TestRecommendedPlaylists(t *testing.T) {
	var gotAuth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/1/validate-token":
			w.Write([]byte(`{"valid":true,"user_name":"kilian"}`))
		case "/1/user/kilian/playlists/createdfor":
			w.Write([]byte(createdForBody))
		case "/1/playlist/0fda81ea-1be8-4d91-b139-e9e9f60d983a":
			w.Write([]byte(playlistTrackBody))
		case "/1/playlist/newer-exploration":
			w.Write([]byte(`{"playlist":{"track":[{"identifier":["https://musicbrainz.org/recording/aaa"],"title":"Explore Me","creator":"Someone","album":""}]}}`))
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	got, err := c.RecommendedPlaylists(context.Background(), "my-token")
	if err != nil {
		t.Fatal(err)
	}

	for _, a := range gotAuth {
		if a != "Token my-token" {
			t.Errorf("Authorization = %q, want %q", a, "Token my-token")
		}
	}

	// weekly-jams was never generated for this user -- absent, not an error.
	if _, ok := got[WeeklyJams]; ok {
		t.Error("weekly-jams should be absent")
	}
	// the yearly wrapped playlist must never surface under any known kind.
	for kind, tracks := range got {
		for _, tr := range tracks {
			if strings.Contains(tr.Title, "Discoveries") {
				t.Errorf("yearly wrapped playlist leaked into %s", kind)
			}
		}
	}

	daily := got[DailyJams]
	if len(daily) != 1 {
		t.Fatalf("daily-jams: expected 1 track (the one with no creator skipped), got %d", len(daily))
	}
	if daily[0].MBID != "f1f21661-d4be-49c6-aeb6-050d35638d43" || daily[0].Artist != "Lana Del Rey" || daily[0].Title != "Diet Mountain Dew" || daily[0].Album != "Born to Die" {
		t.Errorf("daily-jams track = %+v", daily[0])
	}

	// The newer of the two weekly-exploration entries must be the one fetched.
	exploration := got[WeeklyExploration]
	if len(exploration) != 1 || exploration[0].Title != "Explore Me" {
		t.Fatalf("expected the newer weekly-exploration playlist's track, got %+v", exploration)
	}
}

func TestRecommendedPlaylistsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"valid":false}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	if _, err := c.RecommendedPlaylists(context.Background(), "bad"); err == nil {
		t.Fatal("expected an error for an invalid token")
	}
}
