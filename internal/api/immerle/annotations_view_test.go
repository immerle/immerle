package immerle

import (
	"net/http"
	"testing"
)

func TestTrackAnnotationsSurfaced(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	var search searchView
	if st := getJSON(t, srv, token, "/search?q=So+What", &search); st != http.StatusOK || len(search.Songs()) == 0 {
		t.Fatalf("search: status %d, songs %d", st, len(search.Songs()))
	}
	song := search.Songs()[0]

	// Star, rate and scrobble the track.
	if st := doStatus(t, srv, http.MethodPut, "/songs/"+song.ID+"/star", token, nil); st != http.StatusNoContent {
		t.Fatalf("star: status %d", st)
	}
	if st := doStatus(t, srv, http.MethodPut, "/songs/"+song.ID+"/rating", token, map[string]any{"rating": 4}); st != http.StatusNoContent {
		t.Fatalf("rate: status %d", st)
	}
	if st := doStatus(t, srv, http.MethodPost, "/scrobbles", token, map[string]any{"ids": []string{song.ID}, "submission": true}); st != http.StatusNoContent {
		t.Fatalf("scrobble: status %d", st)
	}

	// The single-song resource reflects the per-user state.
	var got songView
	if st := getJSON(t, srv, token, "/songs/"+song.ID, &got); st != http.StatusOK {
		t.Fatalf("get song: status %d", st)
	}
	if got.Starred == nil || got.Rating != 4 || got.PlayCount != 1 {
		t.Fatalf("song annotation not surfaced: starred=%v rating=%d play=%d", got.Starred, got.Rating, got.PlayCount)
	}

	// And so does the track inside its album.
	var album albumView
	if st := getJSON(t, srv, token, "/albums/"+song.AlbumID, &album); st != http.StatusOK {
		t.Fatalf("get album: status %d", st)
	}
	var found *songView
	for i := range album.Tracks {
		if album.Tracks[i].ID == song.ID {
			found = &album.Tracks[i]
		}
	}
	if found == nil {
		t.Fatal("track not found in album")
	}
	if found.Starred == nil || found.Rating != 4 || found.PlayCount != 1 {
		t.Fatalf("album track annotation not surfaced: %+v", found)
	}
}
