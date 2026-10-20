package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

// Spotify mirrors are on demand: hidden from public discovery, and only the
// ones last synced before the cutoff come back as stale, flagged when kept.
func TestSpotifyMirrorsHiddenAndStale(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	admin := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", CreatedAt: now}
	me := models.User{ID: uuid.NewString(), Username: "me", PasswordHash: "x", CreatedAt: now}
	for _, u := range []models.User{admin, me} {
		if err := store.Users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
	}

	mirror := func(externalID string, updated time.Time) string {
		t.Helper()
		p := models.Playlist{ID: uuid.NewString(), Name: "Daft Punk " + externalID, OwnerID: admin.ID, Public: true, Federated: true,
			SourceInstanceID: persistence.SpotifySourceID, SourceExternalID: externalID, CreatedAt: updated, UpdatedAt: updated}
		if err := store.Playlists.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	old := now.Add(-8 * 24 * time.Hour)
	kept := mirror("kept", old)
	mirror("dropped", old)
	mirror("fresh", now)
	if err := store.Playlists.Subscribe(ctx, kept, me.ID); err != nil {
		t.Fatal(err)
	}

	if got, err := store.Playlists.SearchPublic(ctx, me.ID, "daft punk", 10); err != nil || len(got) != 0 {
		t.Fatalf("SearchPublic = %+v, %v; want no Spotify mirrors", got, err)
	}
	if got, err := store.Playlists.ListPublic(ctx, me.ID); err != nil || len(got) != 0 {
		t.Fatalf("ListPublic = %+v, %v; want no Spotify mirrors", got, err)
	}

	stale, err := store.Playlists.StaleFederated(ctx, persistence.SpotifySourceID, now.Add(-7*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, s := range stale {
		got[s.ExternalID] = s.Subscribed
	}
	if len(got) != 2 || !got["kept"] || got["dropped"] {
		t.Fatalf("StaleFederated = %+v, want kept (subscribed) and dropped (not)", stale)
	}
}
