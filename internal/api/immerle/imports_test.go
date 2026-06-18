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

	// No content resolver or hub: spotify is therefore unconfigured, so we only
	// exercise the API surface (sources listing, validation, list).
	cfg := func() map[string]map[string]string { return map[string]map[string]string{} }
	svc := importer.NewService(store.Imports, store.Playlists, nil, nil, cfg, testutil.NewLogger())

	h := NewHandler(Deps{Auth: auth, Users: store.Users, Imports: svc, Logger: testutil.NewLogger()})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	token := login(t, srv, "alice")

	// Sources lists spotify, reported as not configured (no credentials).
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
			if m["configured"] != false {
				t.Fatalf("spotify should be unconfigured, got %+v", m)
			}
		}
	}
	if !found {
		t.Fatalf("spotify source missing: %+v", sources)
	}

	// Starting an import for the unconfigured source fails with 400.
	if code := doStatus(t, srv, http.MethodPost, "/imports", token, map[string]any{"source": "spotify", "ref": "PL"}); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unconfigured source, got %d", code)
	}

	// Empty imports list for the caller.
	status, imports := doArr(t, srv, http.MethodGet, "/imports", token, nil)
	if status != http.StatusOK {
		t.Fatalf("imports status %d", status)
	}
	if len(imports) != 0 {
		t.Fatalf("expected no imports, got %+v", imports)
	}
}
