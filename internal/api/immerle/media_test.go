package immerle

import (
	"net/http"
	"testing"
)

func TestStreamAndDownload(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	var search searchView
	if st := getJSON(t, srv, token, "/search?q=So+What", &search); st != http.StatusOK || len(search.Songs()) == 0 {
		t.Fatalf("search: status %d, songs %d", st, len(search.Songs()))
	}
	id := search.Songs()[0].ID

	// A range request yields 206 Partial Content.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+apiBase+"/songs/"+id+"/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Range", "bytes=0-99")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("stream range: expected 206, got %d", resp.StatusCode)
	}

	// Download serves the original bytes.
	if st := doStatus(t, srv, http.MethodGet, "/songs/"+id+"/download", token, nil); st != http.StatusOK {
		t.Fatalf("download: status %d", st)
	}

	// A missing track is a 404 (no bytes written before the error).
	if st := doStatus(t, srv, http.MethodGet, "/songs/does-not-exist/stream", token, nil); st != http.StatusNotFound {
		t.Fatalf("missing stream: expected 404, got %d", st)
	}
}
