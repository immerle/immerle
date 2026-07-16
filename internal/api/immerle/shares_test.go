package immerle

import (
	"net/http"
	"strings"
	"testing"
)

func TestShareCRUD(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	// A track to share.
	var search searchView
	if st := getJSON(t, srv, token, "/search?q=So+What", &search); st != http.StatusOK || len(search.Songs()) == 0 {
		t.Fatalf("search: status %d, songs %d", st, len(search.Songs()))
	}
	songID := search.Songs()[0].ID

	var created shareView
	resp := do(t, srv, http.MethodPost, "/shares", token, map[string]any{"itemId": songID, "description": "listen!"})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create share: status %d", resp.StatusCode)
	}
	decode(t, resp, &created)
	if !strings.HasPrefix(created.URL, "https://music.example/share/") || len(created.Entries) != 1 {
		t.Fatalf("created share: %+v", created)
	}
	id := created.ID

	var list struct {
		Shares []shareView `json:"shares"`
	}
	if st := getJSON(t, srv, token, "/shares", &list); st != http.StatusOK || len(list.Shares) != 1 {
		t.Fatalf("list: status %d, count %d", st, len(list.Shares))
	}

	if st := doStatus(t, srv, http.MethodPatch, "/shares/"+id, token, map[string]any{"description": "updated"}); st != http.StatusNoContent {
		t.Fatalf("update: status %d", st)
	}
	if st := getJSON(t, srv, token, "/shares", &list); st != http.StatusOK || len(list.Shares) != 1 || list.Shares[0].Description != "updated" {
		t.Fatalf("update not applied: %+v", list.Shares)
	}

	if st := doStatus(t, srv, http.MethodDelete, "/shares/"+id, token, nil); st != http.StatusNoContent {
		t.Fatalf("delete: status %d", st)
	}
	if st := getJSON(t, srv, token, "/shares", &list); st != http.StatusOK || len(list.Shares) != 0 {
		t.Fatalf("after delete: count %d", len(list.Shares))
	}

	// Missing itemId is a validation error.
	if st := doStatus(t, srv, http.MethodPost, "/shares", token, map[string]any{}); st != http.StatusBadRequest {
		t.Fatalf("empty create: expected 400, got %d", st)
	}
}
