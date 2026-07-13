package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func strptr(s string) *string { return &s }

func mustPort(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Port()
}

func TestBootstrapSendsRequestAndDecodesResponse(t *testing.T) {
	var gotAuth, gotInstanceID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotInstanceID = r.Header.Get("X-Instance-ID")
		if r.URL.Path != "/api/v1/instances" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req PublicBootstrapRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.UserId == nil || *req.UserId != "user-1" {
			t.Errorf("unexpected request body: %+v", req)
		}
		_ = json.NewEncoder(w).Encode(PublicBootstrapResponse{Id: strptr("instance-1"), Ok: boolptr(true)})
	}))
	defer srv.Close()

	c := New(func() string { return srv.URL }, nil)
	// Bootstrap is unauthenticated, so no auth headers should be sent.
	resp, err := c.Bootstrap(context.Background(), PublicBootstrapRequest{UserId: strptr("user-1")})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if resp.Id == nil || *resp.Id != "instance-1" {
		t.Errorf("resp.Id = %v, want instance-1", resp.Id)
	}
	if gotAuth != "" || gotInstanceID != "" {
		t.Errorf("Bootstrap should not send auth headers, got Authorization=%q X-Instance-ID=%q", gotAuth, gotInstanceID)
	}
}

func boolptr(b bool) *bool { return &b }

func TestAuthenticatedCallSendsHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer priv-key" {
			t.Errorf("Authorization = %q, want Bearer priv-key", got)
		}
		if got := r.Header.Get("X-Instance-ID"); got != "inst-1" {
			t.Errorf("X-Instance-ID = %q, want inst-1", got)
		}
		_ = json.NewEncoder(w).Encode(PublicProfileResponse{Ok: boolptr(true)})
	}))
	defer srv.Close()

	c := New(func() string { return srv.URL }, nil)
	if _, err := c.Me(context.Background(), Auth{InstanceID: "inst-1", PrivateKey: "priv-key"}); err != nil {
		t.Fatalf("Me: %v", err)
	}
}

func TestNonSuccessStatusBecomesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(HttpxErrorResponse{Error: strptr("instance not found")})
	}))
	defer srv.Close()

	c := New(func() string { return srv.URL }, nil)
	_, err := c.Me(context.Background(), Auth{InstanceID: "x", PrivateKey: "y"})
	if err == nil {
		t.Fatal("expected error")
	}
	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("error type = %T, want *HTTPError", err)
	}
	if httpErr.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want 404", httpErr.Status)
	}
	if httpErr.Message != "instance not found" {
		t.Errorf("Message = %q, want %q", httpErr.Message, "instance not found")
	}
	if !strings.Contains(httpErr.Error(), "instance not found") || !strings.Contains(httpErr.Error(), "404") {
		t.Errorf("Error() = %q, missing message or status", httpErr.Error())
	}
}

func TestRedirectToSameHostAllowed(t *testing.T) {
	var final string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/instances/me", func(w http.ResponseWriter, r *http.Request) {
		final = r.URL.Path
		_ = json.NewEncoder(w).Encode(PublicProfileResponse{Ok: boolptr(true)})
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/v1/instances/me", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(func() string { return srv.URL }, nil)
	if err := c.doRaw(context.Background(), http.MethodGet, "/redirect", Auth{}, "", nil, nil); err != nil {
		t.Fatalf("doRaw via same-host redirect: %v", err)
	}
	if final != "/api/v1/instances/me" {
		t.Errorf("request did not follow redirect, final path = %q", final)
	}
}

func TestRedirectToDifferentHostRejected(t *testing.T) {
	var hubPort string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/steal" {
			t.Error("evil path should never be reached")
			return
		}
		// Same port, different hostname string: exercises the client's
		// EqualFold(hostname) check rather than relying on two loopback
		// httptest servers, which would both report Hostname()=="127.0.0.1".
		http.Redirect(w, r, "http://localhost:"+hubPort+"/steal", http.StatusFound)
	}))
	defer hub.Close()
	hubPort = mustPort(t, hub.URL)

	c := New(func() string { return hub.URL }, nil)
	err := c.doRaw(context.Background(), http.MethodGet, "/redirect-away", Auth{}, "", nil, nil)
	if err == nil {
		t.Fatal("expected redirect to a different host to be rejected")
	}
	if !strings.Contains(err.Error(), "disallowed host") {
		t.Errorf("error = %v, want it to mention disallowed host", err)
	}
}

func TestMissingCoversNilVsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PublicMissingCoversResponse{})
	}))
	defer srv.Close()

	c := New(func() string { return srv.URL }, nil)
	got, err := c.MissingCovers(context.Background(), Auth{}, []string{"a", "b"})
	if err != nil {
		t.Fatalf("MissingCovers: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil when hub omits missing", got)
	}
}

func TestGetFeedPlaylistDecodesTracks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/instances/inst-1/playlists/ext-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"playlist":{"author":{"id":"inst-1","name":"Alice"},"externalId":"ext-1","name":"Faves","description":"desc","image":"img","tracks":[{"mbid":"m1","artist":"A","title":"T"}]}}`))
	}))
	defer srv.Close()

	c := New(func() string { return srv.URL }, nil)
	got, err := c.GetFeedPlaylist(context.Background(), Auth{}, "inst-1", "ext-1")
	if err != nil {
		t.Fatalf("GetFeedPlaylist: %v", err)
	}
	if got.InstanceName != "Alice" || got.Name != "Faves" || len(got.Tracks) != 1 || got.Tracks[0].Mbid != "m1" {
		t.Errorf("unexpected result: %+v", got)
	}
}
