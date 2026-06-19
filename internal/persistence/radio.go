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

// RadioRepo persists internet radio stations.
type RadioRepo struct{ *base }

const radioCols = `id, name, stream_url, homepage_url, builtin, sort_order, created_at, updated_at`

func scanStation(s rowScanner) (models.RadioStation, error) {
	var st models.RadioStation
	var builtin int
	var created, updated int64
	if err := s.Scan(&st.ID, &st.Name, &st.StreamURL, &st.HomepageURL, &builtin, &st.SortOrder, &created, &updated); err != nil {
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
	_, err := r.exec(ctx, `INSERT INTO radio_stations (`+radioCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		st.ID, st.Name, st.StreamURL, st.HomepageURL, db.Bool(st.Builtin), st.SortOrder, db.Millis(st.CreatedAt), db.Millis(st.UpdatedAt))
	return err
}

// Update changes a station's name, stream and homepage.
func (r *RadioRepo) Update(ctx context.Context, st models.RadioStation) error {
	_, err := r.exec(ctx, `UPDATE radio_stations SET name=?, stream_url=?, homepage_url=?, updated_at=? WHERE id=?`,
		st.Name, st.StreamURL, st.HomepageURL, db.Millis(st.UpdatedAt), st.ID)
	return err
}

// Delete removes a station (callers must refuse built-ins).
func (r *RadioRepo) Delete(ctx context.Context, id string) error {
	_, err := r.exec(ctx, `DELETE FROM radio_stations WHERE id=? AND builtin=0`, id)
	return err
}

// builtinStations are the curated free streams seeded on first boot. They are
// well-known public radio streams that work out of the box.
// ponytail: a fixed seed list; admins curate the rest via the API.
var builtinStations = []models.RadioStation{
	// French general/music radios (verified reachable, https mp3).
	{ID: "builtin:nrj", Name: "NRJ", StreamURL: "https://scdn.nrjaudio.fm/audio1/fr/30001/mp3_128.mp3?origine=immerle", HomepageURL: "https://www.nrj.fr/"},
	{ID: "builtin:rtl", Name: "RTL", StreamURL: "https://streaming.radio.rtl.fr/rtl-1-44-128", HomepageURL: "https://www.rtl.fr/"},
	{ID: "builtin:rtl2", Name: "RTL2", StreamURL: "https://streaming.radio.rtl.fr/rtl2-1-44-128", HomepageURL: "https://www.rtl2.fr/"},
	{ID: "builtin:funradio", Name: "Fun Radio", StreamURL: "https://streaming.radio.funradio.fr/fun-1-44-128", HomepageURL: "https://www.funradio.fr/"},
	{ID: "builtin:skyrock", Name: "Skyrock", StreamURL: "https://icecast.skyrock.net/s/natio_mp3_128k", HomepageURL: "https://www.skyrock.fm/"},
	{ID: "builtin:europe1", Name: "Europe 1", StreamURL: "https://stream.europe1.fr/europe1.mp3", HomepageURL: "https://www.europe1.fr/"},
	// Radio France (public service).
	{ID: "builtin:franceinter", Name: "France Inter", StreamURL: "https://icecast.radiofrance.fr/franceinter-midfi.mp3", HomepageURL: "https://www.radiofrance.fr/franceinter"},
	{ID: "builtin:franceinfo", Name: "France Info", StreamURL: "https://icecast.radiofrance.fr/franceinfo-midfi.mp3", HomepageURL: "https://www.radiofrance.fr/franceinfo"},
	{ID: "builtin:franceculture", Name: "France Culture", StreamURL: "https://icecast.radiofrance.fr/franceculture-midfi.mp3", HomepageURL: "https://www.radiofrance.fr/franceculture"},
	{ID: "builtin:francemusique", Name: "France Musique", StreamURL: "https://icecast.radiofrance.fr/francemusique-midfi.mp3", HomepageURL: "https://www.radiofrance.fr/francemusique"},
	{ID: "builtin:fip", Name: "FIP", StreamURL: "https://icecast.radiofrance.fr/fip-midfi.mp3", HomepageURL: "https://www.fip.fr/"},
	// International (free / CC-friendly).
	{ID: "builtin:radioparadise-main", Name: "Radio Paradise — Main Mix", StreamURL: "https://stream.radioparadise.com/mp3-128", HomepageURL: "https://radioparadise.com/"},
	{ID: "builtin:somafm-groovesalad", Name: "SomaFM — Groove Salad", StreamURL: "https://ice.somafm.com/groovesalad-128-mp3", HomepageURL: "https://somafm.com/groovesalad/"},
	{ID: "builtin:somafm-dronezone", Name: "SomaFM — Drone Zone", StreamURL: "https://ice.somafm.com/dronezone-128-mp3", HomepageURL: "https://somafm.com/dronezone/"},
}

// EnsureBuiltins seeds the built-in stations once (idempotent). Existing rows
// (including admin edits) are left untouched; only missing built-ins are added.
func (r *RadioRepo) EnsureBuiltins(ctx context.Context) error {
	for i, s := range builtinStations {
		var exists int
		if err := r.queryRow(ctx, `SELECT COUNT(1) FROM radio_stations WHERE id=?`, s.ID).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
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
