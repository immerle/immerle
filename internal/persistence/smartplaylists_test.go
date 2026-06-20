package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestSmartPlaylistEvaluate(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	user := models.User{ID: uuid.NewString(), Username: "u", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Artist", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Album", ArtistID: artistID, CreatedAt: now})
	mk := func(title, genre string, year int) string {
		id, err := store.Catalog.UpsertTrack(ctx, models.Track{
			ID: uuid.NewString(), Title: title, AlbumID: albumID, ArtistID: artistID,
			Genre: genre, Year: year, Duration: 200, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	house1 := mk("House One", "House", 2001)
	house2 := mk("House Two", "House", 2010)
	rock := mk("Rock Song", "Rock", 1995)

	// Make house2 the most played + starred.
	for i := 0; i < 5; i++ {
		_ = store.Annotations.IncrementPlay(ctx, user.ID, models.ItemTrack, house2, now)
	}
	_ = store.Annotations.IncrementPlay(ctx, user.ID, models.ItemTrack, house1, now)
	_ = store.Annotations.SetStarred(ctx, user.ID, models.ItemTrack, house1, true)

	eval := func(rules models.SmartRules) []models.Track {
		got, err := store.SmartPlaylists.Evaluate(ctx, rules, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	// Filter by genre.
	genre := eval(models.SmartRules{Conditions: []models.SmartCondition{{Field: "genre", Op: "is", Value: "House"}}})
	if len(genre) != 2 {
		t.Fatalf("genre filter returned %d, want 2", len(genre))
	}
	for _, tr := range genre {
		if tr.ID == rock {
			t.Fatal("rock leaked into a House-only filter")
		}
	}

	// Sort by playCount desc → house2 first.
	sorted := eval(models.SmartRules{Sort: "playCount", Order: "desc"})
	if len(sorted) == 0 || sorted[0].ID != house2 {
		t.Fatalf("playCount sort: first = %v, want house2", sorted[0].ID)
	}

	// starred = true → only house1.
	starred := eval(models.SmartRules{Conditions: []models.SmartCondition{{Field: "starred", Op: "is", Value: "true"}}})
	if len(starred) != 1 || starred[0].ID != house1 {
		t.Fatalf("starred filter = %+v, want [house1]", starred)
	}

	// match=any across genre Rock OR year>=2010 → rock + house2.
	any := eval(models.SmartRules{
		Match: "any",
		Conditions: []models.SmartCondition{
			{Field: "genre", Op: "is", Value: "Rock"},
			{Field: "year", Op: "gte", Value: "2010"},
		},
	})
	if len(any) != 2 {
		t.Fatalf("match=any returned %d, want 2", len(any))
	}

	// Limit is respected.
	limited := eval(models.SmartRules{Limit: 1})
	if len(limited) != 1 {
		t.Fatalf("limit=1 returned %d", len(limited))
	}

	// Injection safety: a malicious value is treated as a literal (no rows, table
	// intact). If it were interpolated, this would error or drop the table.
	inj := eval(models.SmartRules{Conditions: []models.SmartCondition{
		{Field: "genre", Op: "is", Value: "House'; DROP TABLE tracks;--"},
	}})
	if len(inj) != 0 {
		t.Fatalf("injection value matched %d rows, want 0", len(inj))
	}
	if all := eval(models.SmartRules{}); len(all) != 3 {
		t.Fatalf("tracks table altered after injection attempt: %d rows, want 3", len(all))
	}
}
