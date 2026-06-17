package immerle

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

func newEnv(t *testing.T) (*httptest.Server, *persistence.Store) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	for _, u := range []string{"alice", "bob"} {
		if _, err := auth.CreateUser(ctx, u, u+"pw", "", "", false); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(Deps{
		Auth:      auth,
		Users:     store.Users,
		Friends:   store.Friends,
		Activity:  core.NewActivityService(store.Activity, store.Friends, store.Users),
		Playlists: store.Playlists,
		Jam:       core.NewJamService(store.Jam),
		Logger:    testutil.NewLogger(),
	})
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, store
}

func creds(u string) url.Values {
	return url.Values{"u": {u}, "p": {u + "pw"}, "c": {"test"}}
}

func postForm(t *testing.T, srv *httptest.Server, path string, v url.Values) map[string]any {
	t.Helper()
	resp, err := http.PostForm(srv.URL+path, v)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func TestCapabilitiesUnauthenticated(t *testing.T) {
	srv, _ := newEnv(t)
	resp, err := http.Get(srv.URL + "/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["server"] != "immerle" {
		t.Fatalf("unexpected capabilities: %+v", body)
	}
	caps, _ := body["capabilities"].(map[string]any)
	if _, ok := caps["jam"]; !ok {
		t.Fatal("jam capability missing")
	}
}

func TestFriendRequestAcceptFlow(t *testing.T) {
	srv, _ := newEnv(t)

	// alice requests bob.
	v := creds("alice")
	v.Set("username", "bob")
	if r := postForm(t, srv, "/friends/request", v); r["ok"] != true {
		t.Fatalf("request failed: %+v", r)
	}

	// bob sees the pending request.
	pending := postForm(t, srv, "/friends/pending", creds("bob"))
	list, _ := pending["pending"].([]any)
	if len(list) != 1 {
		t.Fatalf("bob should have 1 pending request, got %d", len(list))
	}

	// bob accepts.
	va := creds("bob")
	va.Set("username", "alice")
	if r := postForm(t, srv, "/friends/accept", va); r["ok"] != true {
		t.Fatalf("accept failed: %+v", r)
	}

	// Both now list each other as friends.
	for _, pair := range [][2]string{{"alice", "bob"}, {"bob", "alice"}} {
		r := postForm(t, srv, "/friends", creds(pair[0]))
		friends, _ := r["friends"].([]any)
		if len(friends) != 1 {
			t.Fatalf("%s should have 1 friend, got %d", pair[0], len(friends))
		}
	}
}

func TestJamSSEKeepsClientsSynced(t *testing.T) {
	srv, _ := newEnv(t)

	// alice creates a jam.
	created := postForm(t, srv, "/jam/create", creds("alice"))
	session, _ := created["session"].(map[string]any)
	sessionID, _ := session["id"].(string)
	if sessionID == "" {
		t.Fatalf("no session id: %+v", created)
	}

	// bob joins.
	jv := creds("bob")
	jv.Set("sessionId", sessionID)
	postForm(t, srv, "/jam/join", jv)

	// bob opens the SSE stream.
	streamURL := srv.URL + "/jam/events?" + jv.Encode()
	resp, err := http.Get(streamURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", ct)
	}
	reader := bufio.NewReader(resp.Body)
	// Drain the initial snapshot event.
	readSSEData(t, reader)

	// alice (host) updates playback.
	uv := creds("alice")
	uv.Set("sessionId", sessionID)
	uv.Set("currentTrackId", "track-2")
	uv.Set("position", "42000")
	uv.Set("state", "playing")
	postForm(t, srv, "/jam/update", uv)

	// bob receives the synchronized state via SSE.
	done := make(chan map[string]any, 1)
	go func() { done <- readSSEData(t, reader) }()
	select {
	case data := <-done:
		sess, _ := data["session"].(map[string]any)
		if sess["currentTrackId"] != "track-2" || sess["position"].(float64) != 42000 {
			t.Errorf("bob not synced via SSE: %+v", sess)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("bob did not receive SSE playback update")
	}
}

// readSSEData reads one SSE event and returns the parsed JSON "data:" payload.
func readSSEData(t *testing.T, reader *bufio.Reader) map[string]any {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read sse: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var out map[string]any
			if err := json.Unmarshal([]byte(payload), &out); err != nil {
				t.Fatalf("parse sse data: %v", err)
			}
			return out
		}
	}
}
