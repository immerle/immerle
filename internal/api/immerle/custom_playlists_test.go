package immerle

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/autoplaylists"
	"github.com/immerle/immerle/internal/models"
)

// TestCustomPlaylistsFindsOwnPrivatePlaylistsWithoutSubscribing covers the
// dedicated lookup GET /me/custom-playlists relies on: it must find the
// caller's own auto-generated (private, federated) playlists purely by
// ownership, with no subscription involved — and skip an empty one.
func TestCustomPlaylistsFindsOwnPrivatePlaylistsWithoutSubscribing(t *testing.T) {
	srv, store := newEnv(t)
	ctx := context.Background()
	aliceToken := login(t, srv, "alice")
	alice, err := store.Users.GetByUsername(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	topMonth := models.Playlist{
		ID: uuid.NewString(), Name: "Top du mois", OwnerID: alice.ID, Public: false, Federated: true,
		SourceInstanceID: autoplaylists.SourceTopMonth, SourceExternalID: alice.ID,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Playlists.Create(ctx, topMonth); err != nil {
		t.Fatal(err)
	}
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	trackID, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "T", AlbumID: albumID, ArtistID: artistID, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})
	if err := store.Playlists.ReplaceTracks(ctx, topMonth.ID, []string{trackID}, alice.ID); err != nil {
		t.Fatal(err)
	}

	// An empty personal playlist (e.g. never got a matching track) must be skipped.
	emptyOnRepeat := models.Playlist{
		ID: uuid.NewString(), Name: "On Repeat", OwnerID: alice.ID, Public: false, Federated: true,
		SourceInstanceID: autoplaylists.SourceOnRepeat, SourceExternalID: alice.ID,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Playlists.Create(ctx, emptyOnRepeat); err != nil {
		t.Fatal(err)
	}

	var out struct {
		Playlists []playlistView `json:"playlists"`
	}
	if st := getJSON(t, srv, aliceToken, "/me/custom-playlists", &out); st != http.StatusOK {
		t.Fatalf("status %d", st)
	}
	if len(out.Playlists) != 1 || out.Playlists[0].ID != topMonth.ID {
		t.Fatalf("expected only the non-empty top-month playlist, got %+v", out.Playlists)
	}
	if out.Playlists[0].AutoPlaylistKind != autoplaylists.SourceTopMonth {
		t.Fatalf("expected autoPlaylistKind %q so clients can render a translated label, got %q",
			autoplaylists.SourceTopMonth, out.Playlists[0].AutoPlaylistKind)
	}

	// Not subscribed: confirms the lookup doesn't depend on it.
	subscribed, err := store.Playlists.IsSubscribed(ctx, topMonth.ID, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if subscribed {
		t.Fatal("test setup should not have subscribed alice to her own playlist")
	}
}
