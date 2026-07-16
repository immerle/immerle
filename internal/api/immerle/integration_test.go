package immerle

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

// apiBase is the REST API mount point; test paths are given relative to it.
const apiBase = "/api/v1"

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
		Auth:        auth,
		Users:       store.Users,
		Activity:    core.NewActivityService(store.Activity),
		Playlists:   store.Playlists,
		Annotations: store.Annotations,
		PlayQueues:  store.PlayQueues,
		Jam:         core.NewJamService(store.Jam),
		Logger:      testutil.NewLogger(),
	})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, store
}

// login authenticates a seeded user (password = username+"pw") and returns a
// Bearer device token.
func login(t *testing.T, srv *httptest.Server, username string) string {
	t.Helper()
	status, body := doMap(t, srv, http.MethodPost, "/auth/sessions", "", map[string]any{
		"username": username,
		"password": username + "pw",
		"device":   "test",
	})
	if status != http.StatusCreated {
		t.Fatalf("login %s: status %d body %+v", username, status, body)
	}
	tok, _ := body["token"].(string)
	if tok == "" {
		t.Fatalf("login %s: no token in %+v", username, body)
	}
	return tok
}

// do performs an API request. path is relative to /api/v1; token is a Bearer
// token (empty for none); body is JSON-encoded when non-nil. The caller closes
// the returned response body.
func do(t *testing.T, srv *httptest.Server, method, path, token string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+apiBase+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// doMap performs a request and decodes a JSON object response.
func doMap(t *testing.T, srv *httptest.Server, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	resp := do(t, srv, method, path, token, body)
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// doArr performs a request and decodes a JSON array response.
func doArr(t *testing.T, srv *httptest.Server, method, path, token string, body any) (int, []any) {
	t.Helper()
	resp := do(t, srv, method, path, token, body)
	defer resp.Body.Close()
	var out []any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// doStatus performs a request and returns just the status code (body discarded).
func doStatus(t *testing.T, srv *httptest.Server, method, path, token string, body any) int {
	t.Helper()
	resp := do(t, srv, method, path, token, body)
	resp.Body.Close()
	return resp.StatusCode
}

func TestCapabilitiesUnauthenticated(t *testing.T) {
	srv, _ := newEnv(t)
	status, body := doMap(t, srv, http.MethodGet, "/capabilities", "", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if body["server"] != "immerle" {
		t.Fatalf("unexpected capabilities: %+v", body)
	}
	caps, _ := body["capabilities"].(map[string]any)
	if _, ok := caps["jam"]; !ok {
		t.Fatal("jam capability missing")
	}
}

func TestJamSSEKeepsClientsSynced(t *testing.T) {
	srv, _ := newEnv(t)
	alice := login(t, srv, "alice")
	bob := login(t, srv, "bob")

	_, created := doMap(t, srv, http.MethodPost, "/jam", alice, map[string]any{"name": "test"})
	session, _ := created["session"].(map[string]any)
	sessionID, _ := session["id"].(string)
	if sessionID == "" {
		t.Fatalf("no session id: %+v", created)
	}

	doStatus(t, srv, http.MethodPost, "/jam/"+sessionID+"/participants", bob, nil)

	resp := do(t, srv, http.MethodGet, "/jam/"+sessionID+"/events", bob, nil)
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", ct)
	}
	reader := bufio.NewReader(resp.Body)
	// Drain the initial snapshot event.
	readSSEData(t, reader)

	// alice, the host, updates playback.
	doStatus(t, srv, http.MethodPatch, "/jam/"+sessionID, alice, map[string]any{
		"currentTrackId": "track-2",
		"position":       42000,
		"state":          "playing",
	})

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
