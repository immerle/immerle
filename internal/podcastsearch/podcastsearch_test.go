package podcastsearch

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPodcastIndexSearch checks the signed-auth headers and response parsing
// against a stub server (the auth is computed per-request, so it can't be a
// static config — this guards that the signature is built correctly).
func TestPodcastIndexSearch(t *testing.T) {
	const key, secret = "K", "S"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		date := r.Header.Get("X-Auth-Date")
		sum := sha1.Sum([]byte(key + secret + date))
		if r.Header.Get("X-Auth-Key") != key || r.Header.Get("Authorization") != hex.EncodeToString(sum[:]) {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"feeds":[{"title":"Show","author":"Me","url":"https://f/rss","image":"https://f/i.png"},{"url":""}]}`))
	}))
	defer srv.Close()

	// Point the adapter at the stub by rewriting its hardcoded host via a transport.
	p := &podcastIndex{client: &http.Client{Transport: rewriteHost{srv.URL, srv.Client().Transport}}}

	res, err := p.Search(context.Background(), "x", map[string]string{"apiKey": key, "apiSecret": secret})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].FeedURL != "https://f/rss" || res[0].Source != "podcastindex" {
		t.Fatalf("unexpected results: %+v", res)
	}

	if _, err := p.Search(context.Background(), "x", map[string]string{"apiKey": key}); err == nil {
		t.Fatal("expected error when secret missing")
	}
}

// rewriteHost redirects every request to base, preserving the path/query, so the
// adapter's hardcoded api.podcastindex.org host hits the stub instead.
type rewriteHost struct {
	base string
	next http.RoundTripper
}

func (rw rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	target, _ := http.NewRequestWithContext(req.Context(), req.Method, rw.base+req.URL.Path+"?"+req.URL.RawQuery, req.Body)
	target.Header = req.Header
	next := rw.next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(target)
}
