package immerle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/importer"
	"github.com/immerle/immerle/internal/testutil"
)

func TestImportEndpointsFlow(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if _, err := auth.CreateUser(ctx, "alice", "alicepw", "", "", false); err != nil {
		t.Fatal(err)
	}

	// No content resolver or hub: only exercises the API surface, not an actual fetch/resolve.
	cfg := func() map[string]map[string]string { return map[string]map[string]string{} }
	svc := importer.NewService(store.Imports, store.Playlists, nil, nil, cfg, testutil.NewLogger())

	h := NewHandler(Deps{Auth: auth, Users: store.Users, Imports: svc, Logger: testutil.NewLogger()})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	token := login(t, srv, "alice")

	// Spotify needs no credentials (fetches public playlists directly, see internal/spotifyweb).
	status, sources := doArr(t, srv, http.MethodGet, "/imports/sources", token, nil)
	if status != http.StatusOK {
		t.Fatalf("sources status %d", status)
	}
	if len(sources) == 0 {
		t.Fatalf("expected at least one source, got %+v", sources)
	}
	found := false
	for _, s := range sources {
		m := s.(map[string]any)
		if m["name"] == "spotify" {
			found = true
			if m["configured"] != true {
				t.Fatalf("spotify should be configured (no credentials needed), got %+v", m)
			}
		}
	}
	if !found {
		t.Fatalf("spotify source missing: %+v", sources)
	}

	if code := doStatus(t, srv, http.MethodPost, "/imports", token, map[string]any{"source": "not-a-real-source", "ref": "PL"}); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown source, got %d", code)
	}

	status, imports := doArr(t, srv, http.MethodGet, "/imports", token, nil)
	if status != http.StatusOK {
		t.Fatalf("imports status %d", status)
	}
	if len(imports) != 0 {
		t.Fatalf("expected no imports, got %+v", imports)
	}
}
