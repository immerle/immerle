package importer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

func init() {
	RegisterFactory("faketest", func(SourceDeps) (Source, error) { return fakeSource{}, nil })
}

// fakeSource yields a fixed three-track playlist exercising matched/doubtful/missing.
type fakeSource struct{}

func (fakeSource) Name() string { return "faketest" }
func (fakeSource) FetchPlaylist(_ context.Context, _ string) (Playlist, error) {
	return Playlist{Name: "My Mix", Tracks: []Track{
		{Title: "Da Funk", Artist: "Daft Punk"},    // → matched
		{Title: "Weird Title", Artist: "Artist X"}, // → doubtful (candidate too different)
		{Title: "Ghost", Artist: "Nobody"},         // → missing (no candidate)
	}}, nil
}

// fakeResolver mimics the content providers: search returns candidates by query;
// Resolve materialises a local track row (as a real download/ingest would).
type fakeResolver struct {
	catalog           *persistence.CatalogRepo
	artistID, albumID string
}

func (f *fakeResolver) SearchTracks(_ context.Context, query string, _ int) ([]models.Track, error) {
	switch {
	case strings.Contains(query, "Da Funk"):
		return []models.Track{{ID: "remote:fake:1", Title: "Da Funk", ArtistName: "Daft Punk"}}, nil
	case strings.Contains(query, "Weird Title"):
		return []models.Track{{ID: "remote:fake:2", Title: "Completely Other Song", ArtistName: "Other Band"}}, nil
	default:
		return nil, nil
	}
}

func (f *fakeResolver) Resolve(ctx context.Context, _, _ string) (string, error) {
	now := time.Now()
	return f.catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Da Funk", AlbumID: f.albumID, ArtistID: f.artistID,
		Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now,
	})
}

func TestImportEndToEnd(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "owner", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Daft Punk", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Homework", ArtistID: artistID, CreatedAt: now})

	resolver := &fakeResolver{catalog: store.Catalog, artistID: artistID, albumID: albumID}
	cfg := func() map[string]map[string]string { return map[string]map[string]string{"faketest": {}} }
	svc := NewService(store.Imports, store.Playlists, resolver, nil, cfg, testutil.NewLogger())

	im, err := svc.Start(ctx, owner.ID, "faketest", "ref-123")
	if err != nil {
		t.Fatal(err)
	}
	if im.Status != models.ImportQueued {
		t.Fatalf("expected queued, got %s", im.Status)
	}

	// Drive the worker once synchronously.
	claimed, err := store.Imports.ClaimNext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	svc.runImport(ctx, claimed)

	got, err := svc.Get(ctx, owner.ID, im.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.ImportCompleted {
		t.Fatalf("expected completed, got %s (%s)", got.Status, got.Error)
	}
	if got.SourcePlaylistName != "My Mix" || got.PlaylistID == "" {
		t.Fatalf("source name / playlist link wrong: %+v", got)
	}
	if got.Total != 3 || got.Matched != 1 || got.Doubtful != 1 || got.Missing != 1 {
		t.Fatalf("counts wrong: total=%d matched=%d doubtful=%d missing=%d", got.Total, got.Matched, got.Doubtful, got.Missing)
	}
	if len(got.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got.Items))
	}

	byStatus := map[models.ImportItemStatus]models.ImportItem{}
	for _, it := range got.Items {
		byStatus[it.Status] = it
	}
	if m := byStatus[models.ImportItemMatched]; m.SourceTitle != "Da Funk" || m.MatchedTrackID == "" || m.Confidence < MatchThreshold {
		t.Fatalf("matched item wrong: %+v", m)
	}
	d := byStatus[models.ImportItemDoubtful]
	if d.SourceTitle != "Weird Title" || d.MatchedTrackID != "" || d.Confidence >= MatchThreshold {
		t.Fatalf("doubtful item wrong: %+v", d)
	}
	// The doubtful candidate is exposed so the client can preview it.
	if d.CandidateID == "" {
		t.Fatalf("doubtful item should expose a candidate track id: %+v", d)
	}
	if _, ok := byStatus[models.ImportItemMissing]; !ok {
		t.Fatal("expected a missing item")
	}

	// The created playlist contains exactly the one matched track.
	tracks, err := store.Playlists.Tracks(ctx, got.PlaylistID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track in the imported playlist, got %d", len(tracks))
	}
}

func TestResolveItemValidateAndModify(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "owner", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Daft Punk", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Homework", ArtistID: artistID, CreatedAt: now})

	resolver := &fakeResolver{catalog: store.Catalog, artistID: artistID, albumID: albumID}
	cfg := func() map[string]map[string]string { return map[string]map[string]string{"faketest": {}} }
	svc := NewService(store.Imports, store.Playlists, resolver, nil, cfg, testutil.NewLogger())

	im, _ := svc.Start(ctx, owner.ID, "faketest", "ref")
	claimed, _ := store.Imports.ClaimNext(ctx)
	svc.runImport(ctx, claimed)

	got, _ := svc.Get(ctx, owner.ID, im.ID)
	var doubtful, missing models.ImportItem
	for _, it := range got.Items {
		switch it.Status {
		case models.ImportItemDoubtful:
			doubtful = it
		case models.ImportItemMissing:
			missing = it
		}
	}
	if doubtful.ID == "" || doubtful.CandidateID == "" {
		t.Fatalf("expected a doubtful item with a stored candidate: %+v", doubtful)
	}

	// Validate the doubtful item (no query → uses the stored candidate).
	resolved, err := svc.ResolveItem(ctx, owner.ID, doubtful.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != models.ImportItemMatched || resolved.MatchedTrackID == "" {
		t.Fatalf("validate did not match the item: %+v", resolved)
	}

	// Modify the missing item with a corrected query that now finds a candidate.
	resolved, err = svc.ResolveItem(ctx, owner.ID, missing.ID, "Daft Punk Da Funk")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != models.ImportItemMatched {
		t.Fatalf("modify did not match the item: %+v", resolved)
	}

	// Counters rebalanced and both tracks added to the playlist.
	after, _ := svc.Get(ctx, owner.ID, im.ID)
	if after.Matched != 3 || after.Doubtful != 0 || after.Missing != 0 {
		t.Fatalf("counts not rebalanced: matched=%d doubtful=%d missing=%d", after.Matched, after.Doubtful, after.Missing)
	}
	tracks, _ := store.Playlists.Tracks(ctx, after.PlaylistID)
	if len(tracks) != 3 {
		t.Fatalf("expected 3 tracks after validate+modify, got %d", len(tracks))
	}

	// Re-validating an already-matched item is rejected; foreign user is hidden.
	if _, err := svc.ResolveItem(ctx, owner.ID, doubtful.ID, ""); err == nil {
		t.Fatal("expected error re-validating a matched item")
	}
	if _, err := svc.ResolveItem(ctx, "someone-else", missing.ID, ""); err == nil {
		t.Fatal("expected not-found for a foreign user")
	}
}

func TestImportStartRejectsUnknownSource(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	cfg := func() map[string]map[string]string { return nil }
	svc := NewService(store.Imports, store.Playlists, &fakeResolver{catalog: store.Catalog}, nil, cfg, testutil.NewLogger())
	if _, err := svc.Start(ctx, "u1", "nope", "ref"); err == nil {
		t.Fatal("expected error for unknown source")
	}
	if _, err := svc.Start(ctx, "u1", "faketest", ""); err == nil {
		t.Fatal("expected error for empty ref")
	}
}
