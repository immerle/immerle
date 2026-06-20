package immerle

import (
	"encoding/json"
	"net/http"
	"testing"
)

// decode reads a JSON response body into out and closes it.
func decode(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestPlaylistCRUD(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	// A track to seed the playlist with.
	var search searchView
	if st := getJSON(t, srv, token, "/search?q=So+What", &search); st != http.StatusOK || len(search.Songs) == 0 {
		t.Fatalf("search: status %d, songs %d", st, len(search.Songs))
	}
	songID := search.Songs[0].ID

	// Create.
	var created playlistView
	resp := do(t, srv, http.MethodPost, "/playlists", token, map[string]any{"name": "Mine", "ids": []string{songID}})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create: status %d", resp.StatusCode)
	}
	decode(t, resp, &created)
	if created.SongCount != 1 || len(created.Tracks) != 1 {
		t.Fatalf("created playlist: %+v", created)
	}
	id := created.ID

	// List shows it.
	var list struct {
		Playlists []playlistView `json:"playlists"`
	}
	if st := getJSON(t, srv, token, "/playlists", &list); st != http.StatusOK || len(list.Playlists) != 1 {
		t.Fatalf("list: status %d, count %d", st, len(list.Playlists))
	}

	// Update metadata.
	if st := doStatus(t, srv, http.MethodPatch, "/playlists/"+id, token, map[string]any{"name": "Renamed", "public": true}); st != http.StatusNoContent {
		t.Fatalf("update: status %d", st)
	}
	var got playlistView
	if st := getJSON(t, srv, token, "/playlists/"+id, &got); st != http.StatusOK {
		t.Fatalf("get: status %d", st)
	}
	if got.Name != "Renamed" || !got.Public {
		t.Fatalf("update not applied: %+v", got)
	}

	// Replace tracks with an empty list.
	var replaced playlistView
	rr := do(t, srv, http.MethodPut, "/playlists/"+id+"/tracks", token, map[string]any{"ids": []string{}})
	if rr.StatusCode != http.StatusOK {
		rr.Body.Close()
		t.Fatalf("replace tracks: status %d", rr.StatusCode)
	}
	decode(t, rr, &replaced)
	if replaced.SongCount != 0 {
		t.Fatalf("expected empty tracklist, got %d", replaced.SongCount)
	}

	// Delete, then it is gone.
	if st := doStatus(t, srv, http.MethodDelete, "/playlists/"+id, token, nil); st != http.StatusNoContent {
		t.Fatalf("delete: status %d", st)
	}
	if st := doStatus(t, srv, http.MethodGet, "/playlists/"+id, token, nil); st != http.StatusNotFound {
		t.Fatalf("get deleted: expected 404, got %d", st)
	}
}
