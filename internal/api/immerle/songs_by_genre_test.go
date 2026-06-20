package immerle

import (
	"net/http"
	"testing"
)

func TestSongsByGenre(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	var out struct {
		Songs []songView `json:"songs"`
	}
	if st := getJSON(t, srv, token, "/songs?genre=House", &out); st != http.StatusOK {
		t.Fatalf("songs by genre: status %d", st)
	}
	if len(out.Songs) != 2 { // Daft Punk / Discovery is tagged House
		t.Fatalf("expected 2 House songs, got %d", len(out.Songs))
	}
	// Missing genre is a validation error.
	if st := doStatus(t, srv, http.MethodGet, "/songs", token, nil); st != http.StatusBadRequest {
		t.Fatalf("missing genre: expected 400, got %d", st)
	}
}
