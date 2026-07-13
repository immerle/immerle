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

// playlistSelect joins users and computes song_count/duration via correlated
// subqueries — neither a JOIN nor a subquery column is expressible by melody, so
// every read built on it (Get, ListVisible, ListPublic*, FindFederated, Tracks)
// stays hand-written.
const playlistSelect = `
	SELECT p.id, p.name, p.owner_id, u.username, p.comment, p.public, p.collaborative, p.federated,
	       p.source_instance_id, p.source_external_id,
	       p.cover_art, p.created_at, p.updated_at,
	       (SELECT COUNT(*) FROM playlist_tracks pt WHERE pt.playlist_id = p.id) AS song_count,
	       (SELECT COALESCE(SUM(t.duration),0) FROM playlist_tracks pt JOIN tracks t ON t.id = pt.track_id WHERE pt.playlist_id = p.id) AS duration
	FROM playlists p JOIN users u ON u.id = p.owner_id`

func scanPlaylist(s rowScanner) (models.Playlist, error) {
	var p models.Playlist
	var public, collab, fed int
	var created, updated int64
	if err := s.Scan(&p.ID, &p.Name, &p.OwnerID, &p.OwnerName, &p.Comment, &public, &collab, &fed,
		&p.SourceInstanceID, &p.SourceExternalID,
		&p.CoverArt, &created, &updated, &p.SongCount, &p.Duration); err != nil {
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
	_, err := r.bexec(ctx, r.mel.NewInsert("playlists").
		Set("id", p.ID).Set("name", p.Name).Set("owner_id", p.OwnerID).Set("comment", p.Comment).
		Set("public", db.Bool(p.Public)).Set("collaborative", db.Bool(p.Collaborative)).Set("federated", db.Bool(p.Federated)).
		Set("source_instance_id", p.SourceInstanceID).Set("source_external_id", p.SourceExternalID).
		Set("created_at", db.Millis(p.CreatedAt)).Set("updated_at", db.Millis(p.UpdatedAt)))
	return err
}

// UpdateMeta updates name/comment/public/collaborative.
func (r *PlaylistRepo) UpdateMeta(ctx context.Context, p models.Playlist) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("playlists").
		Set("name", p.Name).Set("comment", p.Comment).Set("public", db.Bool(p.Public)).
		Set("collaborative", db.Bool(p.Collaborative)).Set("updated_at", db.Millis(time.Now())).Where("id", "=", p.ID))
	return err
}

// SetCover sets (or clears) a playlist's custom cover id.
func (r *PlaylistRepo) SetCover(ctx context.Context, id, coverID string) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("playlists").
		Set("cover_art", coverID).Set("updated_at", db.Millis(time.Now())).Where("id", "=", id))
	return err
}

// Delete removes a playlist.
func (r *PlaylistRepo) Delete(ctx context.Context, id string) error {
	_, err := r.bexec(ctx, r.mel.NewDelete("playlists").Where("id", "=", id))
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

// ListVisible returns the playlists that appear in a user's library: their
// own, ones they collaborate on, and ones they have subscribed to (which
// includes federated ones — they're just public playlists sourced from the
// hub). A federated playlist's "owner" is only an internal attribution (the
// nominal local admin the sync process had to pick for the row's owner_id FK)
// — it never grants that admin real ownership, so federated rows are excluded
// from the owner/collaborator clauses and only ever surface via subscription,
// the same as for any other user. Public/federated playlists are otherwise NOT
// shown wholesale — a user opts in by subscribing, from /playlists/public (see
// ListPublic).
func (r *PlaylistRepo) ListVisible(ctx context.Context, userID string) ([]models.Playlist, error) {
	rows, err := r.query(ctx, playlistSelect+`
		WHERE (p.federated=0 AND p.owner_id=?)
		   OR (p.federated=0 AND p.id IN (SELECT playlist_id FROM playlist_collaborators WHERE user_id=?))
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

// ListPublic returns public playlists not owned by the given user, including
// federated ones (the hub's editorial catalog and subscribed instances' public
// playlists) — for discovery / subscribing. Federated rows are never excluded
// by the owner_id<>? clause: their "owner" is only an internal attribution
// (see ListVisible), so the nominal owner must still be able to discover and
// subscribe to them like anyone else.
func (r *PlaylistRepo) ListPublic(ctx context.Context, excludeUserID string) ([]models.Playlist, error) {
	rows, err := r.query(ctx, playlistSelect+`
		WHERE p.public=1 AND (p.federated=1 OR p.owner_id<>?) ORDER BY p.name`, excludeUserID)
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
	_, err := r.bexec(ctx, r.mel.NewInsert("playlist_subscriptions").
		Set("playlist_id", playlistID).Set("user_id", userID).Set("created_at", db.Millis(time.Now())).
		OnConflict("playlist_id", "user_id").OnConflictDoNothing())
	return err
}

// Unsubscribe removes a user's subscription. Returns whether a row matched.
func (r *PlaylistRepo) Unsubscribe(ctx context.Context, playlistID, userID string) (bool, error) {
	res, err := r.bexec(ctx, r.mel.NewDelete("playlist_subscriptions").
		Where("playlist_id", "=", playlistID).Where("user_id", "=", userID))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// IsSubscribed reports whether a user is subscribed to a playlist.
func (r *PlaylistRepo) IsSubscribed(ctx context.Context, playlistID, userID string) (bool, error) {
	var n int
	err := r.bqueryRow(ctx, r.mel.New("playlist_subscriptions").Select("COUNT(*)").
		Where("playlist_id", "=", playlistID).Where("user_id", "=", userID)).Scan(&n)
	return n > 0, err
}

// Tracks returns a playlist's tracks in order. A federated entry not yet
// resolved to a local track (or whose local track was since removed, nulling
// track_id via ON DELETE SET NULL) comes back as an unresolved stub carrying
// its portable title/artist/album/mbid — see ResolveFederatedTrack.
func (r *PlaylistRepo) Tracks(ctx context.Context, playlistID string) ([]models.Track, error) {
	rows, err := r.query(ctx, `SELECT track_id, mbid, artist, title, album FROM playlist_tracks WHERE playlist_id=? ORDER BY position`, playlistID)
	if err != nil {
		return nil, err
	}
	type ref struct {
		trackID                    sql.NullString
		mbid, artist, title, album string
	}
	var refs []ref
	for rows.Next() {
		var e ref
		if err := rows.Scan(&e.trackID, &e.mbid, &e.artist, &e.title, &e.album); err != nil {
			rows.Close()
			return nil, err
		}
		refs = append(refs, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	var ids []string
	for _, e := range refs {
		if e.trackID.Valid {
			ids = append(ids, e.trackID.String)
		}
	}
	byID, err := r.tracksByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]models.Track, 0, len(refs))
	for _, e := range refs {
		if e.trackID.Valid {
			if t, ok := byID[e.trackID.String]; ok {
				out = append(out, t)
				continue
			}
		}
		out = append(out, models.Track{
			Title: e.title, ArtistName: e.artist, AlbumName: e.album, MBID: e.mbid,
			Unresolved: true,
		})
	}
	return out, nil
}

// tracksByIDs batch-fetches tracks by id, keyed by id (order not preserved —
// callers re-order by looking each id up).
func (r *PlaylistRepo) tracksByIDs(ctx context.Context, ids []string) (map[string]models.Track, error) {
	out := map[string]models.Track{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	q := trackSelect + ` WHERE t.id IN (` + strings.TrimRight(strings.Repeat("?,", len(ids)), ",") + `)`
	rows, err := r.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		out[t.ID] = t
	}
	return out, rows.Err()
}

// FederatedTrackRef is one track inside a federated playlist as synced from
// the hub: TrackID is set once resolved to a local track; until then, the
// portable MBID/Artist/Title/Album identify it for lazy resolution at play
// time (see PlaylistRepo.TrackRef / ResolveFederatedTrack).
type FederatedTrackRef struct {
	TrackID                    string
	MBID, Artist, Title, Album string
}

// ReplaceFederatedTracks atomically sets a federated playlist's full ordered
// track list, resolved and unresolved entries alike. An incoming entry with no
// TrackID (no mbid match at sync time) inherits whatever track_id the same
// track was already resolved to before this sync — e.g. by an on-demand
// resolve of a provider hit with no mbid of its own — identified by mbid when
// present, else by artist+title, so that resolve isn't lost on the next sync.
func (r *PlaylistRepo) ReplaceFederatedTracks(ctx context.Context, playlistID string, entries []FederatedTrackRef) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		resolved, err := r.resolvedFederatedTrackIDs(ctx, tx, playlistID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, r.rebind(`DELETE FROM playlist_tracks WHERE playlist_id=?`), playlistID); err != nil {
			return err
		}
		now := db.Millis(time.Now())
		for i, e := range entries {
			id := e.TrackID
			if id == "" {
				id = resolved[federatedTrackKey(e.MBID, e.Artist, e.Title)]
			}
			var trackID any
			if id != "" {
				trackID = id
			}
			if _, err := tx.ExecContext(ctx, r.rebind(`INSERT INTO playlist_tracks (playlist_id, track_id, position, added_by, added_at, mbid, artist, title, album)
				VALUES (?, ?, ?, '', ?, ?, ?, ?, ?)`), playlistID, trackID, i, now, e.MBID, e.Artist, e.Title, e.Album); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, r.rebind(`UPDATE playlists SET updated_at=? WHERE id=?`), now, playlistID)
		return err
	})
}

// resolvedFederatedTrackIDs maps every currently-resolved entry of a federated
// playlist to its track_id, keyed by federatedTrackKey, for ReplaceFederatedTracks
// to carry forward across a resync.
func (r *PlaylistRepo) resolvedFederatedTrackIDs(ctx context.Context, tx *sql.Tx, playlistID string) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, r.rebind(`SELECT track_id, mbid, artist, title FROM playlist_tracks WHERE playlist_id=? AND track_id IS NOT NULL`), playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var trackID, mbid, artist, title string
		if err := rows.Scan(&trackID, &mbid, &artist, &title); err != nil {
			return nil, err
		}
		out[federatedTrackKey(mbid, artist, title)] = trackID
	}
	return out, rows.Err()
}

// federatedTrackKey identifies a federated track for carrying a resolved
// track_id across a resync: by mbid when present (most precise), else by a
// case-insensitive artist+title pair.
func federatedTrackKey(mbid, artist, title string) string {
	if mbid != "" {
		return "mbid:" + mbid
	}
	return "at:" + strings.ToLower(artist) + "|" + strings.ToLower(title)
}

// TrackRef returns the raw (possibly unresolved) entry at a playlist position.
func (r *PlaylistRepo) TrackRef(ctx context.Context, playlistID string, position int) (FederatedTrackRef, error) {
	var e FederatedTrackRef
	var trackID sql.NullString
	err := r.queryRow(ctx, `SELECT track_id, mbid, artist, title, album FROM playlist_tracks WHERE playlist_id=? AND position=?`,
		playlistID, position).Scan(&trackID, &e.MBID, &e.Artist, &e.Title, &e.Album)
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	if err != nil {
		return e, err
	}
	e.TrackID = trackID.String
	return e, nil
}

// ResolveFederatedTrack records the local track a playlist position resolved
// to, so future reads/plays skip resolution.
func (r *PlaylistRepo) ResolveFederatedTrack(ctx context.Context, playlistID string, position int, trackID string) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("playlist_tracks").Set("track_id", trackID).
		Where("playlist_id", "=", playlistID).Where("position", "=", position))
	return err
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
// compacts the remaining ordering. It deletes only the targeted rows and
// renumbers the survivors in place, so kept rows retain their identity
// (including unresolved federated stubs, which carry no track_id) and their
// added_by/added_at.
func (r *PlaylistRepo) RemoveIndexes(ctx context.Context, playlistID string, indexes []int) error {
	if len(indexes) == 0 {
		return nil
	}
	return r.withTx(ctx, func(tx *sql.Tx) error {
		placeholders := make([]string, len(indexes))
		args := make([]any, 0, len(indexes)+1)
		args = append(args, playlistID)
		for i, idx := range indexes {
			placeholders[i] = "?"
			args = append(args, idx)
		}
		if _, err := tx.ExecContext(ctx, r.rebind(
			`DELETE FROM playlist_tracks WHERE playlist_id=? AND position IN (`+strings.Join(placeholders, ",")+`)`), args...); err != nil {
			return err
		}
		// Renumber survivors to a contiguous 0-based sequence. Positions only ever
		// decrease and are reassigned in ascending order, so no interim UPDATE
		// collides with an existing (playlist_id, position) row.
		rows, err := tx.QueryContext(ctx, r.rebind(`SELECT position FROM playlist_tracks WHERE playlist_id=? ORDER BY position`), playlistID)
		if err != nil {
			return err
		}
		var positions []int
		for rows.Next() {
			var p int
			if err := rows.Scan(&p); err != nil {
				rows.Close()
				return err
			}
			positions = append(positions, p)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for newPos, oldPos := range positions {
			if newPos == oldPos {
				continue
			}
			if _, err := tx.ExecContext(ctx, r.rebind(`UPDATE playlist_tracks SET position=? WHERE playlist_id=? AND position=?`), newPos, playlistID, oldPos); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, r.rebind(`UPDATE playlists SET updated_at=? WHERE id=?`), db.Millis(time.Now()), playlistID)
		return err
	})
}

// IsCollaborator reports whether a user may edit a collaborative playlist.
func (r *PlaylistRepo) IsCollaborator(ctx context.Context, playlistID, userID string) (bool, error) {
	var n int
	err := r.bqueryRow(ctx, r.mel.New("playlist_collaborators").Select("COUNT(*)").
		Where("playlist_id", "=", playlistID).Where("user_id", "=", userID)).Scan(&n)
	return n > 0, err
}

// AddCollaborator grants edit rights on a collaborative playlist.
func (r *PlaylistRepo) AddCollaborator(ctx context.Context, playlistID, userID string) error {
	_, err := r.bexec(ctx, r.mel.NewInsert("playlist_collaborators").
		Set("playlist_id", playlistID).Set("user_id", userID).
		OnConflict("playlist_id", "user_id").OnConflictDoNothing())
	return err
}

// DeleteUnsubscribedFederated removes every federated playlist sourced from
// instanceID that no user has subscribed to (kept in their library). Called
// when unfollowing an instance: playlists nobody chose to keep go away with
// it, but a subscription is a deliberate keep and survives the unfollow.
func (r *PlaylistRepo) DeleteUnsubscribedFederated(ctx context.Context, instanceID string) error {
	_, err := r.exec(ctx, `DELETE FROM playlists WHERE federated=1 AND source_instance_id=?
		AND id NOT IN (SELECT playlist_id FROM playlist_subscriptions)`, instanceID)
	return err
}

// FindFederated returns the federated playlist sourced from (instanceID,
// externalID), if any — instanceID is "" for the hub's own editorial catalog.
// This (not name) is the dedupe key, so same-named playlists from different
// sources don't collapse into one.
func (r *PlaylistRepo) FindFederated(ctx context.Context, instanceID, externalID string) (models.Playlist, error) {
	row := r.queryRow(ctx, playlistSelect+` WHERE p.federated=1 AND p.source_instance_id=? AND p.source_external_id=?`, instanceID, externalID)
	p, err := scanPlaylist(row)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}
