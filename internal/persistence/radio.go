package persistence

import (
	"context"
	"database/sql"
	"errors"
	"strings"
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
	rows, err := r.bquery(ctx, r.mel.New("radio_stations").Select(radioCols).
		OrderBy("sort_order", "").OrderBy("name", ""))
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
	st, err := scanStation(r.bqueryRow(ctx, r.mel.New("radio_stations").Select(radioCols).Where("id", "=", id)))
	if errors.Is(err, sql.ErrNoRows) {
		return st, ErrNotFound
	}
	return st, err
}

// Create inserts a station.
func (r *RadioRepo) Create(ctx context.Context, st models.RadioStation) error {
	_, err := r.bexec(ctx, r.mel.NewInsert("radio_stations").
		Set("id", st.ID).Set("name", st.Name).Set("stream_url", st.StreamURL).Set("homepage_url", st.HomepageURL).
		Set("country", st.Country).Set("cover_art", st.CoverArt).Set("builtin", db.Bool(st.Builtin)).
		Set("sort_order", st.SortOrder).Set("created_at", db.Millis(st.CreatedAt)).Set("updated_at", db.Millis(st.UpdatedAt)))
	return err
}

// Update changes a station's name, stream, homepage and cover.
func (r *RadioRepo) Update(ctx context.Context, st models.RadioStation) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("radio_stations").
		Set("name", st.Name).Set("stream_url", st.StreamURL).Set("homepage_url", st.HomepageURL).
		Set("cover_art", st.CoverArt).Set("updated_at", db.Millis(st.UpdatedAt)).Where("id", "=", st.ID))
	return err
}

// Delete removes a station (callers must refuse built-ins).
func (r *RadioRepo) Delete(ctx context.Context, id string) error {
	_, err := r.bexec(ctx, r.mel.NewDelete("radio_stations").Where("id", "=", id).Where("builtin", "=", 0))
	return err
}

// SetLiked marks (or clears) a station as a favorite for the user. Likes are
// stored as a 'radio' annotation, a separate item_type from track stars — so
// liked radios never surface in the (track-based) "liked songs" view.
func (r *RadioRepo) SetLiked(ctx context.Context, userID, stationID string, liked bool) error {
	if liked {
		_, err := r.bexec(ctx, r.mel.NewInsert("annotations").
			Set("user_id", userID).Set("item_type", "radio").Set("item_id", stationID).
			Set("starred_at", db.Millis(time.Now())).UpdateDuplicateKey().
			OnConflict("user_id", "item_type", "item_id"))
		return err
	}
	_, err := r.bexec(ctx, r.mel.NewUpdate("annotations").Set("starred_at", nil).
		Where("user_id", "=", userID).Where("item_type", "=", "radio").Where("item_id", "=", stationID))
	return err
}

// LikedIDs returns the set of station ids the user has liked.
func (r *RadioRepo) LikedIDs(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := r.bquery(ctx, r.mel.New("annotations").Select("item_id").
		Where("user_id", "=", userID).Where("item_type", "=", "radio").WhereNotNull("starred_at"))
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

// EnsureBuiltins seeds the built-in stations from the embedded per-country
// radio/<cc>/stations.json (see radio.Builtins). New stations are inserted; for
// stations that already exist it refreshes the curated fields (name, stream URL,
// homepage, country) so fixes ship on upgrade — the embedded JSON is the source
// of truth for built-ins. The cover is only set when the row has none, so a
// custom admin logo survives. (Admins wanting a permanently custom station
// should add one rather than edit a built-in.)
func (r *RadioRepo) EnsureBuiltins(ctx context.Context) error {
	builtins := radio.Builtins()
	// Prune built-ins that are no longer in the embedded list (e.g. a station
	// removed from stations.json), so removals reach existing installs. The
	// NOT IN over a runtime-sized id list stays hand-written.
	ids := make([]string, 0, len(builtins))
	for _, s := range builtins {
		ids = append(ids, s.ID)
	}
	if len(ids) > 0 {
		ph := make([]string, len(ids))
		args := make([]any, len(ids))
		for i, id := range ids {
			ph[i] = "?"
			args[i] = id
		}
		if _, err := r.exec(ctx, `DELETE FROM radio_stations WHERE builtin=1 AND id NOT IN (`+strings.Join(ph, ",")+`)`, args...); err != nil {
			return err
		}
	}
	for i, s := range builtins {
		var exists int
		if err := r.bqueryRow(ctx, r.mel.New("radio_stations").Select("COUNT(1)").Where("id", "=", s.ID)).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			if _, err := r.bexec(ctx, r.mel.NewUpdate("radio_stations").
				Set("name", s.Name).Set("stream_url", s.StreamURL).Set("homepage_url", s.HomepageURL).
				Set("country", s.Country).Set("updated_at", db.Millis(time.Now())).
				Where("id", "=", s.ID).Where("builtin", "=", 1)); err != nil {
				return err
			}
			if _, err := r.bexec(ctx, r.mel.NewUpdate("radio_stations").Set("cover_art", s.CoverArt).
				Where("id", "=", s.ID).Where("builtin", "=", 1).Where("cover_art", "=", "")); err != nil {
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
