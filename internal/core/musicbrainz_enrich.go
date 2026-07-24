package core

import (
	"context"
	"log/slog"
	"time"

	"github.com/immerle/immerle/internal/matching"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// MBIDLookup resolves MusicBrainz recording candidates for a track.
// Implemented by *musicbrainz.Client (kept as an interface here so core
// doesn't import it).
type MBIDLookup interface {
	// LookupByISRC returns the MBID matching isrc, or "" if none.
	LookupByISRC(ctx context.Context, isrc string) (string, error)
	// SearchRecording returns candidate recordings for an artist/title text
	// search (only MBID/Title/ArtistName/AlbumName are populated), for the
	// caller to disambiguate -- used when there's no ISRC to look up by.
	SearchRecording(ctx context.Context, artist, title string) ([]models.Track, error)
	// LookupISRC returns an ISRC MusicBrainz has on file for mbid, or "" if
	// none -- backfills a track matched by text search rather than by ISRC.
	LookupISRC(ctx context.Context, mbid string) (string, error)
}

// minSearchTitleScore rejects a text-search candidate sharing nothing with
// the wanted title, same cutoff and reasoning as ResolveBestRemoteMatch.
const minSearchTitleScore = 3

// MusicBrainzEnricher fills in tracks' MusicBrainz recording id (MBID): by
// ISRC when the track has one (precise, no disambiguation needed), otherwise
// by an artist/title text search re-ranked with the same
// title-overlap/artist-relevance/version-marker scoring
// ResolveBestRemoteMatch uses for remote provider results -- so a track
// scanned or downloaded with no MusicBrainz tag still gets one, and
// ListenBrainz scrobbles link precisely instead of relying on its own fuzzy
// matching. Failures mark the track as checked so the loop does not retry
// forever.
type MusicBrainzEnricher struct {
	catalog *persistence.CatalogRepo
	lookup  MBIDLookup
	logger  *slog.Logger
	wake    chan struct{}
}

// NewMusicBrainzEnricher builds an enricher. lookup may be nil to disable it.
func NewMusicBrainzEnricher(catalog *persistence.CatalogRepo, lookup MBIDLookup, logger *slog.Logger) *MusicBrainzEnricher {
	return &MusicBrainzEnricher{catalog: catalog, lookup: lookup, logger: logger, wake: make(chan struct{}, 1)}
}

// Wake nudges the background Run loop to re-check for tracks needing a MBID
// (e.g. right after a scan or an on-demand download adds new tracks).
func (e *MusicBrainzEnricher) Wake() {
	if e == nil {
		return
	}
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// EnrichMissing processes up to limit tracks lacking a MBID. It returns the
// number of candidates processed and the number of MBIDs resolved.
func (e *MusicBrainzEnricher) EnrichMissing(ctx context.Context, limit int) (processed, resolved int, err error) {
	if e.lookup == nil {
		return 0, 0, nil
	}
	tracks, err := e.catalog.ListTracksNeedingMBID(ctx, limit)
	if err != nil {
		return 0, 0, err
	}
	for _, t := range tracks {
		if err := ctx.Err(); err != nil {
			return processed, resolved, err
		}
		processed++
		if e.enrichOne(ctx, t) {
			resolved++
		}
	}
	return processed, resolved, nil
}

// Run continuously enriches tracks missing a MBID: it drains the backlog in
// batches, then idles until new tracks appear (e.g. after a scan).
func (e *MusicBrainzEnricher) Run(ctx context.Context, idle time.Duration) {
	if idle <= 0 {
		idle = 30 * time.Minute
	}
	for {
		processed, resolved, err := e.EnrichMissing(ctx, 50)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			e.logger.Warn("musicbrainz enrichment error", "error", err)
		}
		if processed > 0 {
			e.logger.Info("track MusicBrainz ids enriched", "processed", processed, "resolved", resolved)
			continue // more backlog may remain; keep draining
		}
		select {
		case <-ctx.Done():
			return
		case <-e.wake:
		case <-time.After(idle):
		}
	}
}

func (e *MusicBrainzEnricher) enrichOne(ctx context.Context, t models.Track) bool {
	mbid, viaTextSearch, err := e.resolve(ctx, t)
	if err != nil {
		e.logger.Debug("musicbrainz lookup failed", "track", t.ID, "error", err)
		return false // transient -- retry next round, don't mark as permanently checked
	}
	if mbid == "" {
		// MusicBrainz has nothing for this track -- don't retry it next time.
		_ = e.catalog.MarkTrackMBIDChecked(ctx, t.ID)
		return false
	}
	if err := e.catalog.SetTrackMBID(ctx, t.ID, mbid); err != nil {
		e.logger.Warn("could not set track mbid", "track", t.ID, "error", err)
		return false
	}
	// Matched by text search rather than by ISRC, so the track still has none
	// -- backfill it from the matched recording, best-effort (a miss here
	// doesn't undo the mbid match above).
	if viaTextSearch && t.ISRC == "" {
		if isrc, err := e.lookup.LookupISRC(ctx, mbid); err != nil {
			e.logger.Debug("musicbrainz isrc backfill failed", "track", t.ID, "error", err)
		} else if isrc != "" {
			if err := e.catalog.SetTrackISRC(ctx, t.ID, isrc); err != nil {
				e.logger.Warn("could not set track isrc", "track", t.ID, "error", err)
			}
		}
	}
	return true
}

// resolve looks up t's MBID: by ISRC when available (exact, no ranking
// needed), falling back to a re-ranked artist/title text search when there's
// no ISRC or MusicBrainz has no recording indexed under it. viaTextSearch
// reports which path resolved it, so the caller knows whether an ISRC
// backfill is worth attempting.
func (e *MusicBrainzEnricher) resolve(ctx context.Context, t models.Track) (mbid string, viaTextSearch bool, err error) {
	if t.ISRC != "" {
		mbid, err = e.lookup.LookupByISRC(ctx, t.ISRC)
		if err != nil || mbid != "" {
			return mbid, false, err
		}
	}
	candidates, err := e.lookup.SearchRecording(ctx, t.ArtistName, t.Title)
	if err != nil {
		return "", false, err
	}
	return bestRecordingMatch(t, candidates), true, nil
}

// bestRecordingMatch re-ranks raw MusicBrainz search candidates with the same
// scoring ResolveBestRemoteMatch applies to provider search results: reject
// anything sharing nothing with the wanted title, then rank by title overlap,
// artist relevance and an alternate-version penalty (remix/live/cover/...).
// Returns "" if no candidate clears the title-overlap bar.
func bestRecordingMatch(wanted models.Track, candidates []models.Track) string {
	best := ""
	bestScore := -1
	for _, c := range candidates {
		titleScore := titleOverlap(wanted.Title, c.Title)
		if titleScore >= minSearchTitleScore {
			continue
		}
		score := titleScore*10 + Relevance(wanted.ArtistName, c.ArtistName) + matching.VersionMarkerPenalty(wanted.Title, c.Title, c.AlbumName)*100
		if bestScore == -1 || score < bestScore {
			best, bestScore = c.MBID, score
		}
	}
	return best
}
