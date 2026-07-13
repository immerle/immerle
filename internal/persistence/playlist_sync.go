package persistence

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/db"
)

// PlaylistSyncRepo tracks the last content hash synced per playlist, so an
// unchanged playlist can be skipped without any hub call.
type PlaylistSyncRepo struct{ *base }

// Hash returns the last synced content hash for a playlist ("" if never synced).
func (r *PlaylistSyncRepo) Hash(ctx context.Context, playlistID string) (string, error) {
	var h string
	err := r.queryRow(ctx, `SELECT last_payload_hash FROM playlist_sync WHERE playlist_id = ?`, playlistID).Scan(&h)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return h, nil
}

// Set records the content hash just synced for a playlist.
func (r *PlaylistSyncRepo) Set(ctx context.Context, playlistID, hash string) error {
	_, err := r.exec(ctx,
		`INSERT INTO playlist_sync (playlist_id, last_payload_hash, last_synced_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT (playlist_id) DO UPDATE SET last_payload_hash = excluded.last_payload_hash, last_synced_at = excluded.last_synced_at`,
		playlistID, hash, db.Millis(time.Now()))
	return err
}

// Delete forgets a playlist's sync state (after it is deleted/unpublished).
func (r *PlaylistSyncRepo) Delete(ctx context.Context, playlistID string) error {
	_, err := r.exec(ctx, `DELETE FROM playlist_sync WHERE playlist_id = ?`, playlistID)
	return err
}

// SetPayload records the resolved payload (covers already rewritten to hub
// URLs) and its version for a playlist, so a replay.request can be answered
// without recomputing anything.
func (r *PlaylistSyncRepo) SetPayload(ctx context.Context, playlistID, payload, version string) error {
	_, err := r.exec(ctx,
		`INSERT INTO playlist_sync (playlist_id, last_payload, last_version, last_synced_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (playlist_id) DO UPDATE SET last_payload = excluded.last_payload, last_version = excluded.last_version, last_synced_at = excluded.last_synced_at`,
		playlistID, payload, version, db.Millis(time.Now()))
	return err
}

// LastPayload returns the last resolved payload and its version for a
// playlist ("", "" if never synced).
func (r *PlaylistSyncRepo) LastPayload(ctx context.Context, playlistID string) (payload, version string, err error) {
	err = r.queryRow(ctx, `SELECT last_payload, last_version FROM playlist_sync WHERE playlist_id = ?`, playlistID).Scan(&payload, &version)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return payload, version, err
}

// IDs returns the playlist ids currently synced to the hub.
func (r *PlaylistSyncRepo) IDs(ctx context.Context) ([]string, error) {
	rows, err := r.query(ctx, `SELECT playlist_id FROM playlist_sync`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CoverUploadRepo caches the sha256 of covers confirmed present on the hub.
type CoverUploadRepo struct{ *base }

// Unknown returns the subset of hashes NOT yet confirmed present on the hub.
func (r *CoverUploadRepo) Unknown(ctx context.Context, hashes []string) ([]string, error) {
	if len(hashes) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(hashes)), ",")
	args := make([]any, len(hashes))
	for i, h := range hashes {
		args[i] = h
	}
	rows, err := r.query(ctx, `SELECT sha256 FROM cover_uploads WHERE sha256 IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[string]bool{}
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		known[h] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []string
	for _, h := range hashes {
		if !known[h] {
			out = append(out, h)
		}
	}
	return out, nil
}

// Mark records hashes confirmed present on the hub.
func (r *CoverUploadRepo) Mark(ctx context.Context, hashes ...string) error {
	now := db.Millis(time.Now())
	for _, h := range hashes {
		if _, err := r.exec(ctx,
			`INSERT INTO cover_uploads (sha256, created_at) VALUES (?, ?) ON CONFLICT (sha256) DO NOTHING`,
			h, now); err != nil {
			return err
		}
	}
	return nil
}

// FeedCursorRepo tracks, per followed source instance, the version of the
// last federation feed item applied locally, so a socket reconnect can resume
// instead of re-pulling the whole feed (RFC-socket-federation-client.md §4).
type FeedCursorRepo struct{ *base }

// Get returns the last applied version for a source instance ("" if none,
// meaning a full catch-up is needed).
func (r *FeedCursorRepo) Get(ctx context.Context, sourceInstanceID string) (string, error) {
	var v string
	err := r.queryRow(ctx, `SELECT last_version FROM federation_feed_cursor WHERE source_instance_id = ?`, sourceInstanceID).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// Set records the version of the last feed item applied for a source instance.
func (r *FeedCursorRepo) Set(ctx context.Context, sourceInstanceID, version string) error {
	_, err := r.exec(ctx,
		`INSERT INTO federation_feed_cursor (source_instance_id, last_version, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT (source_instance_id) DO UPDATE SET last_version = excluded.last_version, updated_at = excluded.updated_at`,
		sourceInstanceID, version, db.Millis(time.Now()))
	return err
}

// Delete forgets the cursor for a source instance, so a later resubscribe
// resumes from empty (a full catch-up) instead of picking up mid-feed from a
// stale version.
func (r *FeedCursorRepo) Delete(ctx context.Context, sourceInstanceID string) error {
	_, err := r.exec(ctx, `DELETE FROM federation_feed_cursor WHERE source_instance_id = ?`, sourceInstanceID)
	return err
}
