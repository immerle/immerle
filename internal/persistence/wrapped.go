package persistence

import (
	"context"
	"time"

	"github.com/immerle/immerle/internal/models"
)

// WrappedRepo computes a user's year-in-review from the scrobble history.
type WrappedRepo struct{ *base }

// chartLimit bounds each top-N chart.
const chartLimit = 10

// Wrapped aggregates the user's plays in the given calendar year (UTC). It runs
// a handful of GROUP BY queries over scrobbles joined to tracks; all portable
// across sqlite and postgres (no DB date functions — month bucketing is done in
// Go from the unix-millis played_at).
func (r *WrappedRepo) Wrapped(ctx context.Context, userID string, year int) (models.Wrapped, error) {
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	out := models.Wrapped{Year: year}

	// Totals + per-month histogram from a single scan of (played_at, duration).
	rows, err := r.query(ctx, `SELECT s.played_at, t.duration
		FROM scrobbles s JOIN tracks t ON t.id = s.track_id
		WHERE s.user_id=? AND s.submitted=1 AND s.played_at>=? AND s.played_at<?`,
		userID, start, end)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var playedAt, duration int64
		if err := rows.Scan(&playedAt, &duration); err != nil {
			return out, err
		}
		out.TotalPlays++
		out.TotalSeconds += duration
		out.ByMonth[time.UnixMilli(playedAt).UTC().Month()-1]++
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	if out.TopTracks, err = r.topTracks(ctx, userID, start, end); err != nil {
		return out, err
	}
	if out.TopArtists, err = r.topArtists(ctx, userID, start, end); err != nil {
		return out, err
	}
	if out.TopGenres, err = r.topGenres(ctx, userID, start, end); err != nil {
		return out, err
	}
	return out, nil
}

func (r *WrappedRepo) topTracks(ctx context.Context, userID string, start, end int64) ([]models.WrappedTrack, error) {
	rows, err := r.query(ctx, `SELECT t.id, t.title, ar.name, COUNT(*) AS plays
		FROM scrobbles s
		JOIN tracks t ON t.id = s.track_id
		JOIN artists ar ON ar.id = t.artist_id
		WHERE s.user_id=? AND s.submitted=1 AND s.played_at>=? AND s.played_at<?
		GROUP BY t.id, t.title, ar.name
		ORDER BY plays DESC, t.title ASC
		LIMIT ?`, userID, start, end, chartLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.WrappedTrack
	for rows.Next() {
		var t models.WrappedTrack
		if err := rows.Scan(&t.ID, &t.Title, &t.Artist, &t.Plays); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// topByLabel powers the artist and genre charts (same shape, different column).
func (r *WrappedRepo) topByLabel(ctx context.Context, query, userID string, start, end int64) ([]models.WrappedCount, error) {
	rows, err := r.query(ctx, query, userID, start, end, chartLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.WrappedCount
	for rows.Next() {
		var c models.WrappedCount
		if err := rows.Scan(&c.Name, &c.Plays); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *WrappedRepo) topArtists(ctx context.Context, userID string, start, end int64) ([]models.WrappedCount, error) {
	return r.topByLabel(ctx, `SELECT ar.name, COUNT(*) AS plays
		FROM scrobbles s
		JOIN tracks t ON t.id = s.track_id
		JOIN artists ar ON ar.id = t.artist_id
		WHERE s.user_id=? AND s.submitted=1 AND s.played_at>=? AND s.played_at<?
		GROUP BY ar.name
		ORDER BY plays DESC, ar.name ASC
		LIMIT ?`, userID, start, end)
}

func (r *WrappedRepo) topGenres(ctx context.Context, userID string, start, end int64) ([]models.WrappedCount, error) {
	return r.topByLabel(ctx, `SELECT t.genre, COUNT(*) AS plays
		FROM scrobbles s
		JOIN tracks t ON t.id = s.track_id
		WHERE s.user_id=? AND s.submitted=1 AND s.played_at>=? AND s.played_at<? AND t.genre<>''
		GROUP BY t.genre
		ORDER BY plays DESC, t.genre ASC
		LIMIT ?`, userID, start, end)
}
