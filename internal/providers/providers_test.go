package providers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJamendoSearchResolveDownload(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tracks/", func(w http.ResponseWriter, r *http.Request) {
		dl := "http://" + r.Host + "/file/track.mp3"
		// Resolve-by-id and search both hit /tracks/.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"headers":{"status":"success"},"results":[
			{"id":"42","name":"Sunshine","artist_name":"The Band","album_name":"Day","position":3,
			 "releasedate":"2012-05-01","duration":180,"audiodownload":"` + dl + `",
			 "musicinfo":{"tags":{"genres":["pop"]}}}]}`))
	})
	mux.HandleFunc("/file/track.mp3", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("AUDIO-BYTES"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := NewJamendoProvider("test-client", "mp32", srv.URL)
	ctx := context.Background()

	res, err := p.Search(ctx, "sunshine", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Title != "Sunshine" || res[0].Artist != "The Band" || res[0].TrackNo != 3 {
		t.Fatalf("unexpected search result: %+v", res)
	}
	if res[0].Year != 2012 || res[0].Genre != "pop" || res[0].Suffix != "mp3" {
		t.Fatalf("unexpected metadata: %+v", res[0])
	}

	got, err := p.Resolve(ctx, "42")
	if err != nil || got.Title != "Sunshine" {
		t.Fatalf("resolve failed: %+v %v", got, err)
	}

	var buf bytes.Buffer
	if err := p.Download(ctx, "42", &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "AUDIO-BYTES" {
		t.Fatalf("download mismatch: %q", buf.String())
	}
}

func TestJamendoFactoryRequiresClientID(t *testing.T) {
	if _, err := Build("jamendo", map[string]string{}); err == nil {
		t.Fatal("expected error without client_id")
	}
	if _, err := Build("jamendo", map[string]string{"client_id": "x"}); err != nil {
		t.Fatalf("unexpected error with client_id: %v", err)
	}
}

func TestInternetArchiveSearchResolveDownload(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/advancedsearch.php", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"response":{"docs":[{"identifier":"liveset1","title":"Live Set","creator":"Jam Band"}]}}`))
	})
	mux.HandleFunc("/metadata/liveset1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"metadata":{"title":"Live Set","creator":"Jam Band","date":"2009-08-15"},
			"files":[
			  {"name":"gd1.mp3","format":"VBR MP3","title":"Opener","track":"1","length":"200.5"},
			  {"name":"cover.jpg","format":"JPEG"},
			  {"name":"gd2.flac","format":"Flac","title":"Closer","track":"2"}
			]}`))
	})
	mux.HandleFunc("/download/liveset1/gd1.mp3", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("IA-AUDIO"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := NewInternetArchiveProvider(srv.URL, 8)
	ctx := context.Background()

	res, err := p.Search(ctx, "jam band", 10)
	if err != nil {
		t.Fatal(err)
	}
	// Two audio files (mp3 + flac), cover.jpg excluded.
	if len(res) != 2 {
		t.Fatalf("expected 2 audio tracks, got %d: %+v", len(res), res)
	}
	if res[0].Title != "Opener" || res[0].Suffix != "mp3" || res[0].Artist != "Jam Band" || res[0].Year != 2009 {
		t.Fatalf("unexpected first track: %+v", res[0])
	}
	if !strings.Contains(res[0].ProviderTrackID, "liveset1"+iaSep+"gd1.mp3") {
		t.Fatalf("unexpected provider track id: %q", res[0].ProviderTrackID)
	}

	got, err := p.Resolve(ctx, res[0].ProviderTrackID)
	if err != nil || got.Title != "Opener" {
		t.Fatalf("resolve failed: %+v %v", got, err)
	}

	var buf bytes.Buffer
	if err := p.Download(ctx, res[0].ProviderTrackID, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "IA-AUDIO" {
		t.Fatalf("download mismatch: %q", buf.String())
	}
}

func TestProviderFactoryRegistry(t *testing.T) {
	for _, name := range []string{"jamendo", "internet-archive"} {
		if !HasFactory(name) {
			t.Errorf("expected factory %q to be registered", name)
		}
	}
}
