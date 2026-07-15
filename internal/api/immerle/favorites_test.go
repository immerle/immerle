package immerle

import (
	"net/http"
	"testing"
)

func TestFavorites(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	// Find a song and its album, star both, plus an artist.
	var search searchView
	if st := getJSON(t, srv, token, "/search?q=So+What", &search); st != http.StatusOK || len(search.Songs()) == 0 {
		t.Fatalf("search: status %d", st)
	}
	song := search.Songs()[0]

	if st := doStatus(t, srv, http.MethodPut, "/songs/"+song.ID+"/star", token, nil); st != http.StatusNoContent {
		t.Fatalf("star song: status %d", st)
	}
	if st := doStatus(t, srv, http.MethodPut, "/albums/"+song.AlbumID+"/star", token, nil); st != http.StatusNoContent {
		t.Fatalf("star album: status %d", st)
	}
	if st := doStatus(t, srv, http.MethodPut, "/artists/"+song.ArtistID+"/star", token, nil); st != http.StatusNoContent {
		t.Fatalf("star artist: status %d", st)
	}

	var fav favoritesView
	if st := getJSON(t, srv, token, "/me/favorites", &fav); st != http.StatusOK {
		t.Fatalf("favorites: status %d", st)
	}
	if len(fav.Songs) != 1 || len(fav.Albums) != 1 || len(fav.Artists) != 1 {
		t.Fatalf("favorites counts: songs=%d albums=%d artists=%d", len(fav.Songs), len(fav.Albums), len(fav.Artists))
	}
	if fav.Songs[0].ID != song.ID {
		t.Fatalf("favorite song mismatch: %+v", fav.Songs[0])
	}
}
