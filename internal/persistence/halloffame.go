package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// HallOfFameRepo persists each user's personal top-tracks ranking — its own
// dedicated tables (hall_of_fame / hall_of_fame_entries, see migration 00038),
// not a flag on playlists.
type HallOfFameRepo struct{ *base }

func scanHallOfFame(s rowScanner) (models.HallOfFame, error) {
	var h models.HallOfFame
	var created, updated int64
	if err := s.Scan(&h.ID, &h.OwnerID, &created, &updated); err != nil {
		return h, err
	}
	h.CreatedAt = db.FromMillis(created)
	h.UpdatedAt = db.FromMillis(updated)
	return h, nil
}

// GetOrCreate returns a user's Hall of Fame, creating an empty one on first access.
func (r *HallOfFameRepo) GetOrCreate(ctx context.Context, ownerID string) (models.HallOfFame, error) {
	row := r.queryRow(ctx, `SELECT id, owner_id, created_at, updated_at FROM hall_of_fame WHERE owner_id=?`, ownerID)
	h, err := scanHallOfFame(row)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now()
		h = models.HallOfFame{ID: uuid.NewString(), OwnerID: ownerID, CreatedAt: now, UpdatedAt: now}
		_, insertErr := r.bexec(ctx, r.mel.NewInsert("hall_of_fame").
			Set("id", h.ID).Set("owner_id", h.OwnerID).
			Set("created_at", db.Millis(now)).Set("updated_at", db.Millis(now)))
		return h, insertErr
	}
	return h, err
}

// Entries returns a Hall of Fame's ranked tracks in position order, each with
// its personal nostalgia note.
func (r *HallOfFameRepo) Entries(ctx context.Context, id string) ([]models.HallOfFameEntry, error) {
	rows, err := r.query(ctx, `SELECT track_id, comment FROM hall_of_fame_entries WHERE hall_of_fame_id=? ORDER BY position`, id)
	if err != nil {
		return nil, err
	}
	type ref struct{ trackID, comment string }
	var refs []ref
	for rows.Next() {
		var e ref
		if err := rows.Scan(&e.trackID, &e.comment); err != nil {
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

	ids := make([]string, len(refs))
	for i, e := range refs {
		ids[i] = e.trackID
	}
	byID, err := tracksByIDs(ctx, r.base, ids)
	if err != nil {
		return nil, err
	}

	out := make([]models.HallOfFameEntry, 0, len(refs))
	for _, e := range refs {
		if t, ok := byID[e.trackID]; ok {
			out = append(out, models.HallOfFameEntry{Track: t, Comment: e.comment})
		}
	}
	return out, nil
}

// ReplaceEntries atomically sets a Hall of Fame's full ranked track list —
// used for reordering, adding and removing alike (the caller computes the
// desired full order). Duplicate track ids collapse to their first occurrence.
// Existing comments are carried forward by track id, so they survive the
// delete+reinsert this does under the hood.
func (r *HallOfFameRepo) ReplaceEntries(ctx context.Context, id string, trackIDs []string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, r.rebind(`SELECT track_id, comment FROM hall_of_fame_entries WHERE hall_of_fame_id=?`), id)
		if err != nil {
			return err
		}
		comments := map[string]string{}
		for rows.Next() {
			var trackID, comment string
			if err := rows.Scan(&trackID, &comment); err != nil {
				rows.Close()
				return err
			}
			comments[trackID] = comment
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		if _, err := tx.ExecContext(ctx, r.rebind(`DELETE FROM hall_of_fame_entries WHERE hall_of_fame_id=?`), id); err != nil {
			return err
		}
		now := db.Millis(time.Now())
		seen := make(map[string]bool, len(trackIDs))
		pos := 0
		for _, tid := range trackIDs {
			if seen[tid] {
				continue
			}
			seen[tid] = true
			if _, err := tx.ExecContext(ctx, r.rebind(`INSERT INTO hall_of_fame_entries (hall_of_fame_id, track_id, position, comment, added_at)
				VALUES (?, ?, ?, ?, ?)`), id, tid, pos, comments[tid], now); err != nil {
				return err
			}
			pos++
		}
		_, err = tx.ExecContext(ctx, r.rebind(`UPDATE hall_of_fame SET updated_at=? WHERE id=?`), now, id)
		return err
	})
}

// SetNote sets (or, given an empty comment, clears) a track's nostalgia note.
func (r *HallOfFameRepo) SetNote(ctx context.Context, id, trackID, comment string) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("hall_of_fame_entries").
		Set("comment", comment).Where("hall_of_fame_id", "=", id).Where("track_id", "=", trackID))
	return err
}
