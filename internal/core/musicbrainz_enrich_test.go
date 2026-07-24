package core

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

// fakeMBIDLookup is a stub MBIDLookup: ISRC lookups keyed by ISRC, text
// searches keyed by title returning fixed candidates, and ISRC backfills
// keyed by mbid.
type fakeMBIDLookup struct {
	byISRC     map[string]string
	byTitle    map[string][]models.Track
	isrcByMBID map[string]string
}

func (f fakeMBIDLookup) LookupByISRC(_ context.Context, isrc string) (string, error) {
	return f.byISRC[isrc], nil
}

func (f fakeMBIDLookup) SearchRecording(_ context.Context, _, title string) ([]models.Track, error) {
	return f.byTitle[title], nil
}

func (f fakeMBIDLookup) LookupISRC(_ context.Context, mbid string) (string, error) {
	return f.isrcByMBID[mbid], nil
}

func TestMusicBrainzEnricherResolvesByISRC(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A"})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID})
	needsID, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Has ISRC", ArtistID: artistID, AlbumID: albumID, Path: "/a.mp3", ISRC: "USRC17607839",
	})

	lookup := fakeMBIDLookup{byISRC: map[string]string{"USRC17607839": "mbid-1"}}
	enr := NewMusicBrainzEnricher(store.Catalog, lookup, testutil.NewLogger())

	processed, resolved, err := enr.EnrichMissing(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 || resolved != 1 {
		t.Fatalf("expected processed=1 resolved=1, got %d/%d", processed, resolved)
	}
	tr, _ := store.Catalog.GetTrack(ctx, needsID)
	if tr.MBID != "mbid-1" {
		t.Fatalf("mbid not set, got %q", tr.MBID)
	}

	// Re-running finds nothing left to do.
	processed, _, _ = enr.EnrichMissing(ctx, 10)
	if processed != 0 {
		t.Fatalf("expected no remaining candidates, got %d", processed)
	}
}

func TestMusicBrainzEnricherFallsBackToTextSearchWithoutISRC(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Daft Punk"})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Discovery", ArtistID: artistID})
	id, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Get Lucky", ArtistID: artistID, AlbumID: albumID, Path: "/x.mp3",
	})

	// Candidates mirror the real-world ReccoBeats failure mode: a cover
	// (wrong artist) ranked alongside the correct recording.
	lookup := fakeMBIDLookup{byTitle: map[string][]models.Track{
		"Get Lucky": {
			{MBID: "mbid-cover", Title: "Get Lucky", ArtistName: "Jazz Cover Band", AlbumName: "Covers Vol. 1"},
			{MBID: "mbid-correct", Title: "Get Lucky", ArtistName: "Daft Punk", AlbumName: "Discovery"},
		},
	}}
	enr := NewMusicBrainzEnricher(store.Catalog, lookup, testutil.NewLogger())

	_, resolved, err := enr.EnrichMissing(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != 1 {
		t.Fatalf("expected 1 resolved via text search, got %d", resolved)
	}
	tr, _ := store.Catalog.GetTrack(ctx, id)
	if tr.MBID != "mbid-correct" {
		t.Fatalf("expected the matching-artist candidate to win, got %q", tr.MBID)
	}
}

func TestMusicBrainzEnricherBackfillsISRCAfterTextMatch(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Daft Punk"})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Discovery", ArtistID: artistID})
	id, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Get Lucky", ArtistID: artistID, AlbumID: albumID, Path: "/x.mp3", // no ISRC
	})

	lookup := fakeMBIDLookup{
		byTitle: map[string][]models.Track{
			"Get Lucky": {{MBID: "mbid-correct", Title: "Get Lucky", ArtistName: "Daft Punk", AlbumName: "Discovery"}},
		},
		isrcByMBID: map[string]string{"mbid-correct": "USRC17607839"},
	}
	enr := NewMusicBrainzEnricher(store.Catalog, lookup, testutil.NewLogger())

	if _, resolved, err := enr.EnrichMissing(ctx, 10); err != nil || resolved != 1 {
		t.Fatalf("resolved=%d err=%v", resolved, err)
	}
	tr, _ := store.Catalog.GetTrack(ctx, id)
	if tr.MBID != "mbid-correct" {
		t.Fatalf("mbid not set, got %q", tr.MBID)
	}
	if tr.ISRC != "USRC17607839" {
		t.Fatalf("isrc not backfilled, got %q", tr.ISRC)
	}
}

func TestMusicBrainzEnricherDoesNotBackfillISRCWhenAlreadyKnown(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A"})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID})
	id, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Has ISRC", ArtistID: artistID, AlbumID: albumID, Path: "/a.mp3", ISRC: "USRC17607839",
	})

	// LookupByISRC misses (e.g. MB has no recording indexed under this exact
	// ISRC) so resolve falls through to text search -- but the track already
	// has an ISRC, so no backfill should be attempted. isrcByMBID is set to a
	// deliberately different value so a wrongly-attempted backfill fails loudly.
	lookup := fakeMBIDLookup{
		byTitle: map[string][]models.Track{
			"Has ISRC": {{MBID: "mbid-1", Title: "Has ISRC", ArtistName: "A"}},
		},
		isrcByMBID: map[string]string{"mbid-1": "WRONG00000000"},
	}
	enr := NewMusicBrainzEnricher(store.Catalog, lookup, testutil.NewLogger())

	if _, resolved, err := enr.EnrichMissing(ctx, 10); err != nil || resolved != 1 {
		t.Fatalf("resolved=%d err=%v", resolved, err)
	}
	tr, _ := store.Catalog.GetTrack(ctx, id)
	if tr.ISRC != "USRC17607839" {
		t.Fatalf("existing isrc must be left untouched, got %q", tr.ISRC)
	}
}

func TestMusicBrainzEnricherRejectsUnrelatedTextSearchResult(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Some Artist"})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID})
	id, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "My Obscure Track", ArtistID: artistID, AlbumID: albumID, Path: "/y.mp3",
	})

	// Nothing in the candidates shares the wanted title -- must be rejected,
	// not just weakly ranked.
	lookup := fakeMBIDLookup{byTitle: map[string][]models.Track{
		"My Obscure Track": {{MBID: "mbid-wrong", Title: "Completely Different Song", ArtistName: "Other Artist"}},
	}}
	enr := NewMusicBrainzEnricher(store.Catalog, lookup, testutil.NewLogger())

	_, resolved, _ := enr.EnrichMissing(ctx, 10)
	if resolved != 0 {
		t.Fatal("an unrelated title must not resolve a mbid")
	}
	tr, _ := store.Catalog.GetTrack(ctx, id)
	if tr.MBID != "" {
		t.Fatalf("mbid should remain empty, got %q", tr.MBID)
	}
}

func TestMusicBrainzEnricherNoMatchMarksChecked(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A"})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID})
	id, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Unknown", ArtistID: artistID, AlbumID: albumID, Path: "/c.mp3", ISRC: "ZZXXX0000000",
	})

	enr := NewMusicBrainzEnricher(store.Catalog, fakeMBIDLookup{}, testutil.NewLogger())

	_, resolved, _ := enr.EnrichMissing(ctx, 10)
	if resolved != 0 {
		t.Fatal("no MusicBrainz match should not resolve a mbid")
	}
	if tr, _ := store.Catalog.GetTrack(ctx, id); tr.MBID != "" {
		t.Fatalf("mbid should remain empty, got %q", tr.MBID)
	}

	// Marked checked so it isn't retried forever.
	processed, _, _ := enr.EnrichMissing(ctx, 10)
	if processed != 0 {
		t.Fatal("unmatched track should be marked checked (not retried)")
	}
}

func TestMusicBrainzEnricherNilLookupIsNoop(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A"})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID})
	_, _ = store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "T", ArtistID: artistID, AlbumID: albumID, Path: "/d.mp3", ISRC: "USRC17607839",
	})

	enr := NewMusicBrainzEnricher(store.Catalog, nil, testutil.NewLogger())
	processed, resolved, err := enr.EnrichMissing(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 0 || resolved != 0 {
		t.Fatalf("expected no work with a nil lookup, got %d/%d", processed, resolved)
	}
}
