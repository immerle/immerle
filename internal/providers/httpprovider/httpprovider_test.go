package httpprovider

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func testService(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var authSeen []string
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		authSeen = append(authSeen, r.Header.Get("Authorization"))
		if r.URL.Query().Get("q") == "" {
			http.Error(w, "missing q", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"providerTrackId":"t1","title":"Song One","artist":"A","album":"Al","suffix":"flac"},
			{"providerTrackId":"","title":"skip me"}
		]}`))
	})
	mux.HandleFunc("/resolve", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") != "t1" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"providerTrackId":"t1","title":"Song One","artist":"A"}`))
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") != "t1" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("AUDIOBYTES"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &authSeen
}

func TestHTTPProviderSearchResolveDownload(t *testing.T) {
	srv, authSeen := testService(t)
	p, err := New("manual", srv.URL, `{"headers":{"Authorization":"Bearer xyz"},"quality":"lossless"}`)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "manual" || p.MaxQuality() != "lossless" {
		t.Fatalf("unexpected name/quality: %s/%s", p.Name(), p.MaxQuality())
	}

	ctx := context.Background()
	results, err := p.Search(ctx, "song", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 { // the empty-id row is dropped
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ProviderTrackID != "t1" || results[0].Suffix != "flac" {
		t.Fatalf("bad result: %+v", results[0])
	}
	if len(*authSeen) == 0 || (*authSeen)[0] != "Bearer xyz" {
		t.Fatalf("auth header not forwarded: %v", *authSeen)
	}

	res, err := p.Resolve(ctx, "t1")
	if err != nil || res.Title != "Song One" {
		t.Fatalf("resolve failed: %+v err=%v", res, err)
	}

	var buf bytes.Buffer
	if err := p.Download(ctx, "t1", &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "AUDIOBYTES" {
		t.Fatalf("unexpected audio: %q", buf.String())
	}
}

func TestHTTPProviderVerify(t *testing.T) {
	// Service whose /capabilities requires an "apikey" param and an "X-Token" header.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != capabilitiesPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":1,"name":"my-svc","config":{
			"apikey":{"type":"string","where":"params","required":true},
			"X-Token":{"type":"string","where":"header","required":true}
		}}`))
	}))
	t.Cleanup(srv.Close)
	ctx := context.Background()

	// All required fields supplied in their declared buckets → passes.
	ok, _ := New("svc", srv.URL, `{"params":{"apikey":"k"},"header":{"X-Token":"t"}}`)
	if err := ok.Verify(ctx); err != nil {
		t.Fatalf("Verify should pass with all fields: %v", err)
	}

	// Missing the header field → rejected.
	missing, _ := New("svc", srv.URL, `{"params":{"apikey":"k"}}`)
	if err := missing.Verify(ctx); err == nil {
		t.Fatal("Verify should fail when a required field is missing")
	}

	// No /capabilities endpoint at all → rejected (it is mandatory).
	none := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusNotFound)
	}))
	t.Cleanup(none.Close)
	p, _ := New("svc", none.URL, "{}")
	if err := p.Verify(ctx); err == nil {
		t.Fatal("Verify should fail without a /capabilities endpoint")
	}

	// Wrong protocol version → rejected.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":99,"name":"my-svc"}`))
	}))
	t.Cleanup(bad.Close)
	pv, _ := New("svc", bad.URL, "{}")
	if err := pv.Verify(ctx); err == nil {
		t.Fatal("Verify should fail on an unsupported protocol version")
	}
}

func TestHTTPProviderMergesStaticParams(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	t.Cleanup(srv.Close)

	// A static param (apikey) is appended; a static "q" must NOT override the
	// protocol's own q.
	p, err := New("manual", srv.URL, `{"params":{"apikey":"secret","q":"OVERRIDE"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Search(context.Background(), "realquery", 5); err != nil {
		t.Fatal(err)
	}
	if gotQuery.Get("apikey") != "secret" {
		t.Fatalf("static param not merged: %v", gotQuery)
	}
	if gotQuery.Get("q") != "realquery" {
		t.Fatalf("static param overrode protocol q: %q", gotQuery.Get("q"))
	}
}

func TestHTTPProviderRejectsBadInput(t *testing.T) {
	if _, err := New("", "https://x", "{}"); err == nil {
		t.Fatal("empty name should fail")
	}
	if _, err := New("manual", "not-a-url", "{}"); err == nil {
		t.Fatal("non-http endpoint should fail")
	}
	if _, err := New("manual", "https://x", "{bad json"); err == nil {
		t.Fatal("invalid config JSON should fail")
	}
}

func TestHTTPProviderDownloadError(t *testing.T) {
	srv, _ := testService(t)
	p, _ := New("manual", srv.URL, "{}")
	var buf bytes.Buffer
	if err := p.Download(context.Background(), "missing", &buf); err == nil {
		t.Fatal("download of unknown id should error")
	}
}

// flakyDownloadService fails the download `failBefore` times (502, mimicking the
// remote service momentarily failing to mint a token) before succeeding.
func flakyDownloadService(t *testing.T, failBefore int) (*httptest.Server, *int) {
	t.Helper()
	var calls int
	mux := http.NewServeMux()
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= failBefore {
			http.Error(w, `{"error":"Invalid CSRF token"}`, http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("AUDIOBYTES"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestHTTPProviderDownloadRetriesThenSucceeds(t *testing.T) {
	srv, calls := flakyDownloadService(t, 2) // fail twice, succeed on the 3rd
	p, _ := New("manual", srv.URL, "{}")     // default 3 attempts

	var buf bytes.Buffer
	if err := p.Download(context.Background(), "t1", &buf); err != nil {
		t.Fatalf("expected success on retry, got %v", err)
	}
	if buf.String() != "AUDIOBYTES" {
		t.Fatalf("unexpected audio: %q", buf.String())
	}
	if *calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", *calls)
	}
}

func TestHTTPProviderDownloadGivesUpAfterRetries(t *testing.T) {
	srv, calls := flakyDownloadService(t, 99) // always fails
	p, _ := New("manual", srv.URL, `{"downloadRetries":2}`)

	var buf bytes.Buffer
	if err := p.Download(context.Background(), "t1", &buf); err == nil {
		t.Fatal("expected failure after exhausting retries")
	}
	if buf.Len() != 0 {
		t.Fatalf("nothing should be written on failure, got %q", buf.String())
	}
	if *calls != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", *calls)
	}
}
