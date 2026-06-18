package persistence

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// maxMosaicCovers bounds how many leading track covers a playlist exposes for a
// mosaic thumbnail.
const maxMosaicCovers = 4

// PlaylistRepo persists playlists and their track ordering.
type PlaylistRepo struct{ *base }

const playlistSelect = `
	SELECT p.id, p.name, p.owner_id, u.username, p.comment, p.public, p.collaborative, p.federated,
	       p.created_at, p.updated_at,
	       (SELECT COUNT(*) FROM playlist_tracks pt WHERE pt.playlist_id = p.id) AS song_count,
	       (SELECT COALESCE(SUM(t.duration),0) FROM playlist_tracks pt JOIN tracks t ON t.id = pt.track_id WHERE pt.playlist_id = p.id) AS duration
	FROM playlists p JOIN users u ON u.id = p.owner_id`

func scanPlaylist(s rowScanner) (models.Playlist, error) {
	var p models.Playlist
	var public, collab, fed int
	var created, updated int64
	if err := s.Scan(&p.ID, &p.Name, &p.OwnerID, &p.OwnerName, &p.Comment, &public, &collab, &fed,
		&created, &updated, &p.SongCount, &p.Duration); err != nil {
		return p, err
	}
	p.Public = public != 0
	p.Collaborative = collab != 0
	p.Federated = fed != 0
	p.CreatedAt = db.FromMillis(created)
	p.UpdatedAt = db.FromMillis(updated)
	return p, nil
}

// Create inserts a playlist (without tracks).
func (r *PlaylistRepo) Create(ctx context.Context, p models.Playlist) error {
	_, err := r.exec(ctx, `INSERT INTO playlists (id, name, owner_id, comment, public, collaborative, federated, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.OwnerID, p.Comment, db.Bool(p.Public), db.Bool(p.Collaborative), db.Bool(p.Federated),
		db.Millis(p.CreatedAt), db.Millis(p.UpdatedAt))
	return err
}

// UpdateMeta updates name/comment/public/collaborative.
func (r *PlaylistRepo) UpdateMeta(ctx context.Context, p models.Playlist) error {
	_, err := r.exec(ctx, `UPDATE playlists SET name=?, comment=?, public=?, collaborative=?, updated_at=? WHERE id=?`,
		p.Name, p.Comment, db.Bool(p.Public), db.Bool(p.Collaborative), db.Millis(time.Now()), p.ID)
	return err
}

// Delete removes a playlist.
func (r *PlaylistRepo) Delete(ctx context.Context, id string) error {
	_, err := r.exec(ctx, `DELETE FROM playlists WHERE id=?`, id)
	return err
}

// Get returns one playlist (metadata only, plus its mosaic cover arts).
func (r *PlaylistRepo) Get(ctx context.Context, id string) (models.Playlist, error) {
	row := r.queryRow(ctx, playlistSelect+` WHERE p.id=?`, id)
	p, err := scanPlaylist(row)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	covers, err := r.coverArtsByPlaylist(ctx, []string{p.ID})
	if err != nil {
		return p, err
	}
	p.CoverArts = covers[p.ID]
	return p, nil
}

// coverArtsByPlaylist returns, per playlist id, the cover-art ids of its first
// up-to-maxMosaicCovers tracks (by position) — a single set-based query (no
// N+1). A track's effective cover is its own cover_art, falling back to its
// album id (mirrors the streaming layer's coverOrAlbum).
func (r *PlaylistRepo) coverArtsByPlaylist(ctx context.Context, ids []string) (map[string][]string, error) {
	out := make(map[string][]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := `SELECT playlist_id, cover FROM (
		SELECT pt.playlist_id AS playlist_id,
		       COALESCE(NULLIF(t.cover_art, ''), t.album_id) AS cover,
		       ROW_NUMBER() OVER (PARTITION BY pt.playlist_id ORDER BY pt.position) AS rn
		FROM playlist_tracks pt JOIN tracks t ON t.id = pt.track_id
		WHERE pt.playlist_id IN (` + strings.Join(placeholders, ",") + `)
	) ranked
	WHERE rn <= ?
	ORDER BY playlist_id, rn`
	args = append(args, maxMosaicCovers)
	rows, err := r.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pid, cover string
		if err := rows.Scan(&pid, &cover); err != nil {
			return nil, err
		}
		if cover != "" {
			out[pid] = append(out[pid], cover)
		}
	}
	return out, rows.Err()
}

// attachCoverArts fills CoverArts on each playlist in the slice using one query.
func (r *PlaylistRepo) attachCoverArts(ctx context.Context, lists []models.Playlist) error {
	if len(lists) == 0 {
		return nil
	}
	ids := make([]string, len(lists))
	for i := range lists {
		ids[i] = lists[i].ID
	}
	covers, err := r.coverArtsByPlaylist(ctx, ids)
	if err != nil {
		return err
	}
	for i := range lists {
		lists[i].CoverArts = covers[lists[i].ID]
	}
	return nil
}

// ListVisible returns the playlists that appear in a user's library: their own,
// ones they collaborate on, ones they have subscribed to, and federated
// (read-only, shown to everyone). Public playlists are NOT shown wholesale — a
// user opts in by subscribing.
func (r *PlaylistRepo) ListVisible(ctx context.Context, userID string) ([]models.Playlist, error) {
	rows, err := r.query(ctx, playlistSelect+`
		WHERE p.owner_id=?
		   OR p.federated=1
		   OR p.id IN (SELECT playlist_id FROM playlist_collaborators WHERE user_id=?)
		   OR p.id IN (SELECT playlist_id FROM playlist_subscriptions WHERE user_id=?)
		ORDER BY p.name`, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Playlist
	for rows.Next() {
		p, err := scanPlaylist(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, r.attachCoverArts(ctx, out)
}

// ListPublic returns public, non-federated playlists not owned by the given user
// (for discovery / subscribing).
func (r *PlaylistRepo) ListPublic(ctx context.Context, excludeUserID string) ([]models.Playlist, error) {
	rows, err := r.query(ctx, playlistSelect+`
		WHERE p.public=1 AND p.federated=0 AND p.owner_id<>? ORDER BY p.name`, excludeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Playlist
	for rows.Next() {
		p, err := scanPlaylist(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, r.attachCoverArts(ctx, out)
}

// ListPublicByOwner returns the public, non-federated playlists owned by a given
// user (for their public profile).
func (r *PlaylistRepo) ListPublicByOwner(ctx context.Context, ownerID string) ([]models.Playlist, error) {
	rows, err := r.query(ctx, playlistSelect+`
		WHERE p.owner_id=? AND p.public=1 AND p.federated=0 ORDER BY p.name`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Playlist
	for rows.Next() {
		p, err := scanPlaylist(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, r.attachCoverArts(ctx, out)
}

// Subscribe adds a user's subscription to a playlist (idempotent).
func (r *PlaylistRepo) Subscribe(ctx context.Context, playlistID, userID string) error {
	_, err := r.exec(ctx, `INSERT INTO playlist_subscriptions (playlist_id, user_id, created_at) VALUES (?, ?, ?)
		ON CONFLICT(playlist_id, user_id) DO NOTHING`, playlistID, userID, db.Millis(time.Now()))
	return err
}

// Unsubscribe removes a user's subscription. Returns whether a row matched.
func (r *PlaylistRepo) Unsubscribe(ctx context.Context, playlistID, userID string) (bool, error) {
	res, err := r.exec(ctx, `DELETE FROM playlist_subscriptions WHERE playlist_id=? AND user_id=?`, playlistID, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// IsSubscribed reports whether a user is subscribed to a playlist.
func (r *PlaylistRepo) IsSubscribed(ctx context.Context, playlistID, userID string) (bool, error) {
	var n int
	err := r.queryRow(ctx, `SELECT COUNT(*) FROM playlist_subscriptions WHERE playlist_id=? AND user_id=?`, playlistID, userID).Scan(&n)
	return n > 0, err
}

// Tracks returns a playlist's tracks in order.
func (r *PlaylistRepo) Tracks(ctx context.Context, playlistID string) ([]models.Track, error) {
	q := trackSelect + ` JOIN playlist_tracks pt ON pt.track_id = t.id WHERE pt.playlist_id=? ORDER BY pt.position`
	rows, err := r.query(ctx, q, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Track
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ReplaceTracks atomically sets the full ordered track list of a playlist.
func (r *PlaylistRepo) ReplaceTracks(ctx context.Context, playlistID string, trackIDs []string, addedBy string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, r.rebind(`DELETE FROM playlist_tracks WHERE playlist_id=?`), playlistID); err != nil {
			return err
		}
		now := db.Millis(time.Now())
		for i, tid := range trackIDs {
			if _, err := tx.ExecContext(ctx, r.rebind(`INSERT INTO playlist_tracks (playlist_id, track_id, position, added_by, added_at)
				VALUES (?, ?, ?, ?, ?)`), playlistID, tid, i, addedBy, now); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, r.rebind(`UPDATE playlists SET updated_at=? WHERE id=?`), now, playlistID)
		return err
	})
}

// AppendTracks adds tracks to the end of a playlist. Used by collaborative edits;
// it appends at the current max position so concurrent appends do not collide.
func (r *PlaylistRepo) AppendTracks(ctx context.Context, playlistID string, trackIDs []string, addedBy string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		// Serialize concurrent appends to the same playlist so two transactions
		// don't read the same MAX(position) and collide on PK(playlist_id,
		// position). SQLite's single-writer lock already serializes writers;
		// Postgres (READ COMMITTED) needs an explicit lock on the parent row.
		if r.db.Dialect == "postgres" {
			if _, err := tx.ExecContext(ctx, r.rebind(`SELECT 1 FROM playlists WHERE id=? FOR UPDATE`), playlistID); err != nil {
				return err
			}
		}
		var maxPos sql.NullInt64
		if err := tx.QueryRowContext(ctx, r.rebind(`SELECT MAX(position) FROM playlist_tracks WHERE playlist_id=?`), playlistID).Scan(&maxPos); err != nil {
			return err
		}
		next := 0
		if maxPos.Valid {
			next = int(maxPos.Int64) + 1
		}
		now := db.Millis(time.Now())
		for _, tid := range trackIDs {
			if _, err := tx.ExecContext(ctx, r.rebind(`INSERT INTO playlist_tracks (playlist_id, track_id, position, added_by, added_at)
				VALUES (?, ?, ?, ?, ?)`), playlistID, tid, next, addedBy, now); err != nil {
				return err
			}
			next++
		}
		_, err := tx.ExecContext(ctx, r.rebind(`UPDATE playlists SET updated_at=? WHERE id=?`), now, playlistID)
		return err
	})
}

// RemoveIndexes removes the tracks at the given zero-based positions and
// compacts the remaining ordering.
func (r *PlaylistRepo) RemoveIndexes(ctx context.Context, playlistID string, indexes []int) error {
	remove := make(map[int]bool, len(indexes))
	for _, i := range indexes {
		remove[i] = true
	}
	current, err := r.Tracks(ctx, playlistID)
	if err != nil {
		return err
	}
	var kept []string
	for i, t := range current {
		if !remove[i] {
			kept = append(kept, t.ID)
		}
	}
	return r.ReplaceTracks(ctx, playlistID, kept, "")
}

// IsCollaborator reports whether a user may edit a collaborative playlist.
func (r *PlaylistRepo) IsCollaborator(ctx context.Context, playlistID, userID string) (bool, error) {
	var n int
	err := r.queryRow(ctx, `SELECT COUNT(*) FROM playlist_collaborators WHERE playlist_id=? AND user_id=?`, playlistID, userID).Scan(&n)
	return n > 0, err
}

// AddCollaborator grants edit rights on a collaborative playlist.
func (r *PlaylistRepo) AddCollaborator(ctx context.Context, playlistID, userID string) error {
	_, err := r.exec(ctx, `INSERT INTO playlist_collaborators (playlist_id, user_id) VALUES (?, ?)
		ON CONFLICT(playlist_id, user_id) DO NOTHING`, playlistID, userID)
	return err
}

// FindFederated returns the federated playlist with the given name, if any.
func (r *PlaylistRepo) FindFederated(ctx context.Context, name string) (models.Playlist, error) {
	row := r.queryRow(ctx, playlistSelect+` WHERE p.federated=1 AND p.name=?`, name)
	p, err := scanPlaylist(row)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}
