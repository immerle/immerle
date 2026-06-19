package immerle

import (
	"context"
	"net/http"
	"testing"

	"github.com/immerle/immerle/internal/models"
)

func TestFavoriteRatingScrobble(t *testing.T) {
	srv, token, store := newBrowseEnv(t)
	ctx := context.Background()

	admin, err := store.Users.GetByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}

	// Locate a track id via search.
	var search searchView
	if st := getJSON(t, srv, token, "/search?q=So+What", &search); st != http.StatusOK || len(search.Songs) == 0 {
		t.Fatalf("search: status %d, songs %d", st, len(search.Songs))
	}
	id := search.Songs[0].ID

	ann := func() models.Annotation {
		a, _ := store.Annotations.Get(ctx, admin.ID, models.ItemTrack, id)
		return a
	}

	// Favorite then unfavorite.
	if st := doStatus(t, srv, http.MethodPut, "/songs/"+id+"/star", token, nil); st != http.StatusNoContent {
		t.Fatalf("star: status %d", st)
	}
	if ann().Starred == nil {
		t.Fatal("expected song to be starred")
	}
	if st := doStatus(t, srv, http.MethodDelete, "/songs/"+id+"/star", token, nil); st != http.StatusNoContent {
		t.Fatalf("unstar: status %d", st)
	}
	if ann().Starred != nil {
		t.Fatal("expected song to be unstarred")
	}

	// Rate then clear.
	if st := doStatus(t, srv, http.MethodPut, "/songs/"+id+"/rating", token, map[string]any{"rating": 4}); st != http.StatusNoContent {
		t.Fatalf("rate: status %d", st)
	}
	if ann().Rating != 4 {
		t.Fatalf("expected rating 4, got %d", ann().Rating)
	}
	if st := doStatus(t, srv, http.MethodDelete, "/songs/"+id+"/rating", token, nil); st != http.StatusNoContent {
		t.Fatalf("clear rating: status %d", st)
	}
	if ann().Rating != 0 {
		t.Fatalf("expected rating cleared, got %d", ann().Rating)
	}

	// Scrobble (submission) increments the play count.
	if st := doStatus(t, srv, http.MethodPost, "/scrobbles", token, map[string]any{"ids": []string{id}, "submission": true}); st != http.StatusNoContent {
		t.Fatalf("scrobble: status %d", st)
	}
	if ann().PlayCount != 1 {
		t.Fatalf("expected play count 1, got %d", ann().PlayCount)
	}

	// Missing ids is a validation error.
	if st := doStatus(t, srv, http.MethodPost, "/scrobbles", token, map[string]any{}); st != http.StatusBadRequest {
		t.Fatalf("empty scrobble: expected 400, got %d", st)
	}
}
