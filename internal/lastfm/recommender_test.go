package lastfm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/reccobeats"
)

func TestGetSimilarTracks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("method") != "track.getSimilar" {
			t.Errorf("method = %q, want track.getSimilar", r.URL.Query().Get("method"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"similartracks": map[string]any{
				"track": []map[string]any{
					{"name": "Similar Song", "artist": map[string]string{"name": "Similar Artist"}},
				},
			},
		})
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	got, err := c.GetSimilarTracks(context.Background(), "key", "secret", "Seed Artist", "Seed Song", 10)
	if err != nil {
		t.Fatalf("GetSimilarTracks() error = %v", err)
	}
	want := []SimilarTrack{{Artist: "Similar Artist", Title: "Similar Song"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestRecommenderNotConfigured(t *testing.T) {
	r := NewRecommender(NewClient(nil), func() models.LastFmRuntime { return models.LastFmRuntime{} })
	_, err := r.Recommend(context.Background(), []reccobeats.Seed{{Artist: "A", Title: "T"}}, 10)
	if err == nil {
		t.Fatal("expected an error when the admin hasn't configured Last.fm")
	}
}

func TestRecommenderDedupesAgainstSeedsAndCapsSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"similartracks": map[string]any{
				"track": []map[string]any{
					{"name": "Seed Song", "artist": map[string]string{"name": "Seed Artist"}}, // echoes the seed
					{"name": "New Song", "artist": map[string]string{"name": "New Artist"}},
					{"name": "New Song", "artist": map[string]string{"name": "New Artist"}}, // duplicate
					{"name": "Second Song", "artist": map[string]string{"name": "Second Artist"}},
				},
			},
		})
	}))
	defer srv.Close()

	r := NewRecommender(newClient(srv.URL, nil), func() models.LastFmRuntime {
		return models.LastFmRuntime{Enabled: true, APIKey: "k", APISecret: "s"}
	})
	got, err := r.Recommend(context.Background(), []reccobeats.Seed{{Artist: "Seed Artist", Title: "Seed Song"}}, 1)
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}
	if len(got) != 1 || got[0].Artist != "New Artist" || got[0].Title != "New Song" {
		t.Fatalf("got %+v, want a single [New Artist/New Song] (seed echoed, duplicate deduped, size capped)", got)
	}
}
