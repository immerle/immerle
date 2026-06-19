package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/radio"
)

// RadioRepo persists internet radio stations.
type RadioRepo struct{ *base }

const radioCols = `id, name, stream_url, homepage_url, country, cover_art, builtin, sort_order, created_at, updated_at`

func scanStation(s rowScanner) (models.RadioStation, error) {
	var st models.RadioStation
	var builtin int
	var created, updated int64
	if err := s.Scan(&st.ID, &st.Name, &st.StreamURL, &st.HomepageURL, &st.Country, &st.CoverArt, &builtin, &st.SortOrder, &created, &updated); err != nil {
		return st, err
	}
	st.Builtin = builtin != 0
	st.CreatedAt = db.FromMillis(created)
	st.UpdatedAt = db.FromMillis(updated)
	return st, nil
}

// List returns all stations ordered by sort_order then name.
func (r *RadioRepo) List(ctx context.Context) ([]models.RadioStation, error) {
	rows, err := r.query(ctx, `SELECT `+radioCols+` FROM radio_stations ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RadioStation{}
	for rows.Next() {
		st, err := scanStation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// ListForUser returns all stations with each station's Liked flag set for userID.
func (r *RadioRepo) ListForUser(ctx context.Context, userID string) ([]models.RadioStation, error) {
	stations, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	liked, err := r.LikedIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range stations {
		stations[i].Liked = liked[stations[i].ID]
	}
	return stations, nil
}

// Get returns a station by id, or ErrNotFound.
func (r *RadioRepo) Get(ctx context.Context, id string) (models.RadioStation, error) {
	st, err := scanStation(r.queryRow(ctx, `SELECT `+radioCols+` FROM radio_stations WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return st, ErrNotFound
	}
	return st, err
}

// Create inserts a station.
func (r *RadioRepo) Create(ctx context.Context, st models.RadioStation) error {
	_, err := r.exec(ctx, `INSERT INTO radio_stations (`+radioCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		st.ID, st.Name, st.StreamURL, st.HomepageURL, st.Country, st.CoverArt, db.Bool(st.Builtin), st.SortOrder, db.Millis(st.CreatedAt), db.Millis(st.UpdatedAt))
	return err
}

// Update changes a station's name, stream, homepage and cover.
func (r *RadioRepo) Update(ctx context.Context, st models.RadioStation) error {
	_, err := r.exec(ctx, `UPDATE radio_stations SET name=?, stream_url=?, homepage_url=?, cover_art=?, updated_at=? WHERE id=?`,
		st.Name, st.StreamURL, st.HomepageURL, st.CoverArt, db.Millis(st.UpdatedAt), st.ID)
	return err
}

// Delete removes a station (callers must refuse built-ins).
func (r *RadioRepo) Delete(ctx context.Context, id string) error {
	_, err := r.exec(ctx, `DELETE FROM radio_stations WHERE id=? AND builtin=0`, id)
	return err
}

// SetLiked marks (or clears) a station as a favorite for the user. Likes are
// stored as a 'radio' annotation, a separate item_type from track stars — so
// liked radios never surface in the (track-based) "liked songs" view.
func (r *RadioRepo) SetLiked(ctx context.Context, userID, stationID string, liked bool) error {
	if liked {
		_, err := r.exec(ctx, `INSERT INTO annotations (user_id, item_type, item_id, starred_at)
			VALUES (?, 'radio', ?, ?)
			ON CONFLICT(user_id, item_type, item_id) DO UPDATE SET starred_at=excluded.starred_at`,
			userID, stationID, db.Millis(time.Now()))
		return err
	}
	_, err := r.exec(ctx, `UPDATE annotations SET starred_at=NULL WHERE user_id=? AND item_type='radio' AND item_id=?`, userID, stationID)
	return err
}

// LikedIDs returns the set of station ids the user has liked.
func (r *RadioRepo) LikedIDs(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := r.query(ctx, `SELECT item_id FROM annotations WHERE user_id=? AND item_type='radio' AND starred_at IS NOT NULL`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// EnsureBuiltins seeds the built-in stations (idempotent). New stations are
// inserted; for stations that already exist it backfills the country grouping
// and, only when the row has no logo yet, the embedded cover — without clobbering
// an admin's custom edits. The curated list lives in the embedded per-country
// radio/<cc>/stations.json (see radio.Builtins).
func (r *RadioRepo) EnsureBuiltins(ctx context.Context) error {
	for i, s := range radio.Builtins() {
		var exists int
		if err := r.queryRow(ctx, `SELECT COUNT(1) FROM radio_stations WHERE id=?`, s.ID).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			if _, err := r.exec(ctx, `UPDATE radio_stations SET country=? WHERE id=? AND builtin=1`, s.Country, s.ID); err != nil {
				return err
			}
			if _, err := r.exec(ctx, `UPDATE radio_stations SET cover_art=? WHERE id=? AND builtin=1 AND cover_art=''`, s.CoverArt, s.ID); err != nil {
				return err
			}
			continue
		}
		now := time.Now()
		s.Builtin = true
		s.SortOrder = i
		s.CreatedAt = now
		s.UpdatedAt = now
		if err := r.Create(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// NewStationID returns a fresh id for a custom (non-builtin) station.
func NewStationID() string { return uuid.NewString() }
