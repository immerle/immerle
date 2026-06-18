package immerle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	chi "github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestLibraryStatsEndpoint(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if _, err := auth.CreateUser(ctx, "alice", "alicepw", "", "", false); err != nil {
		t.Fatal(err)
	}

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: time.Now()})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: time.Now()})
	_, _ = store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "T", AlbumID: albumID, ArtistID: artistID,
		Path: "/x.mp3", Size: 4242, Duration: 180, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	stats := core.NewLibraryStatsService(store.Catalog, testutil.NewLogger())
	if _, err := stats.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(Deps{Auth: auth, Users: store.Users, LibraryStats: stats, Logger: testutil.NewLogger()})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	alice := login(t, srv, "alice")

	status, s := doMap(t, srv, http.MethodGet, "/library/stats", alice, nil)
	if status != http.StatusOK {
		t.Fatalf("library stats failed: status %d %+v", status, s)
	}
	if s["totalSize"] != float64(4242) {
		t.Fatalf("expected totalSize 4242, got %v", s["totalSize"])
	}
	if s["tracks"] != float64(1) {
		t.Fatalf("expected tracks 1, got %v", s["tracks"])
	}
}
