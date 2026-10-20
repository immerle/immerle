package persistence

import (
	"context"
	"encoding/binary"
	"math"
	"strings"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// SonicRepo persists the locally computed audio features (track_sonic).
type SonicRepo struct{ *base }

// sonicScope restricts to local files: remote (not yet downloaded) tracks have
// nothing to decode.
const sonicScope = `FROM tracks t LEFT JOIN track_sonic s ON s.track_id = t.id WHERE t.remote = 0 AND t.path <> ''`

// sonicCurrent matches rows analyzed from the current file with the current algorithm.
const sonicCurrent = `s.track_id IS NOT NULL AND s.algo_version = ? AND s.file_hash = t.file_hash`

// PendingTracks returns local tracks never analyzed, analyzed by an older
// algorithm, or whose file changed since. Only ID, Path, Duration and
// FileHash are filled.
func (r *SonicRepo) PendingTracks(ctx context.Context, algoVersion, limit int) ([]models.Track, error) {
	rows, err := r.query(ctx, `SELECT t.id, t.path, t.duration, t.file_hash `+sonicScope+
		` AND NOT (`+sonicCurrent+`) LIMIT ?`, algoVersion, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Track
	for rows.Next() {
		var t models.Track
		if err := rows.Scan(&t.ID, &t.Path, &t.Duration, &t.FileHash); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Save inserts or replaces a track's features.
func (r *SonicRepo) Save(ctx context.Context, f models.TrackSonic) error {
	_, err := r.exec(ctx, `INSERT INTO track_sonic
		(track_id, file_hash, algo_version, bpm, musical_key, mode, camelot, energy, loudness, danceability, brightness, vector, error, analyzed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (track_id) DO UPDATE SET
		file_hash = excluded.file_hash, algo_version = excluded.algo_version, bpm = excluded.bpm,
		musical_key = excluded.musical_key, mode = excluded.mode, camelot = excluded.camelot,
		energy = excluded.energy, loudness = excluded.loudness, danceability = excluded.danceability,
		brightness = excluded.brightness, vector = excluded.vector, error = excluded.error,
		analyzed_at = excluded.analyzed_at`,
		f.TrackID, f.FileHash, f.AlgoVersion, f.BPM, f.Key, f.Mode, f.Camelot, f.Energy, f.Loudness,
		f.Danceability, f.Brightness, encodeVector(f.Vector), f.Error, db.Millis(f.AnalyzedAt))
	return err
}

// Get returns a track's stored features.
func (r *SonicRepo) Get(ctx context.Context, trackID string) (models.TrackSonic, error) {
	var f models.TrackSonic
	var vec []byte
	var analyzed int64
	err := r.queryRow(ctx, `SELECT track_id, file_hash, algo_version, bpm, musical_key, mode, camelot,
		energy, loudness, danceability, brightness, vector, error, analyzed_at FROM track_sonic WHERE track_id = ?`, trackID).
		Scan(&f.TrackID, &f.FileHash, &f.AlgoVersion, &f.BPM, &f.Key, &f.Mode, &f.Camelot,
			&f.Energy, &f.Loudness, &f.Danceability, &f.Brightness, &vec, &f.Error, &analyzed)
	f.Vector = decodeVector(vec)
	f.AnalyzedAt = db.FromMillis(analyzed)
	return f, err
}

// SonicVector is one analyzed track's similarity vector.
type SonicVector struct {
	TrackID  string
	ArtistID string
	Duration int // seconds
	Vector   []float32
}

// Vectors returns the vectors of every track analyzed successfully from its
// current file with the given algorithm.
func (r *SonicRepo) Vectors(ctx context.Context, algoVersion int) ([]SonicVector, error) {
	rows, err := r.query(ctx, `SELECT s.track_id, t.artist_id, t.duration, s.vector FROM track_sonic s
		JOIN tracks t ON t.id = s.track_id
		WHERE s.algo_version = ? AND s.error = '' AND s.file_hash = t.file_hash`, algoVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SonicVector
	for rows.Next() {
		var v SonicVector
		var raw []byte
		if err := rows.Scan(&v.TrackID, &v.ArtistID, &v.Duration, &raw); err != nil {
			return nil, err
		}
		v.Vector = decodeVector(raw)
		out = append(out, v)
	}
	return out, rows.Err()
}

// SonicSummary is one analyzed track's scalar features (mood playlists).
type SonicSummary struct {
	TrackID      string
	ArtistID     string
	BPM          float64
	Energy       float64
	Danceability float64
	Mode         string
	Camelot      string
}

// Summaries returns the scalar features of every track analyzed successfully
// from its current file with the given algorithm.
func (r *SonicRepo) Summaries(ctx context.Context, algoVersion int) ([]SonicSummary, error) {
	return r.summaries(ctx, "", algoVersion)
}

// SummariesFor is Summaries restricted to the given track ids.
func (r *SonicRepo) SummariesFor(ctx context.Context, ids []string, algoVersion int) ([]SonicSummary, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := []any{algoVersion}
	for _, id := range ids {
		args = append(args, id)
	}
	return r.summaries(ctx, " AND s.track_id IN (?"+strings.Repeat(", ?", len(ids)-1)+")", args...)
}

func (r *SonicRepo) summaries(ctx context.Context, filter string, args ...any) ([]SonicSummary, error) {
	rows, err := r.query(ctx, `SELECT s.track_id, t.artist_id, s.bpm, s.energy, s.danceability, s.mode, s.camelot FROM track_sonic s
		JOIN tracks t ON t.id = s.track_id
		WHERE s.algo_version = ? AND s.error = '' AND s.file_hash = t.file_hash`+filter, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SonicSummary
	for rows.Next() {
		var v SonicSummary
		if err := rows.Scan(&v.TrackID, &v.ArtistID, &v.BPM, &v.Energy, &v.Danceability, &v.Mode, &v.Camelot); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SonicStats is the analysis progress over the local library.
type SonicStats struct {
	Total    int
	Analyzed int
	Failed   int
}

// Stats counts local tracks, and those analyzed (or failed) with the current
// algorithm and file.
func (r *SonicRepo) Stats(ctx context.Context, algoVersion int) (SonicStats, error) {
	var st SonicStats
	err := r.queryRow(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN `+sonicCurrent+` AND s.error = '' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN `+sonicCurrent+` AND s.error <> '' THEN 1 ELSE 0 END), 0) `+sonicScope,
		algoVersion, algoVersion).Scan(&st.Total, &st.Analyzed, &st.Failed)
	return st, err
}

func encodeVector(v []float32) []byte {
	if len(v) == 0 {
		return nil
	}
	out := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(out[4*i:], math.Float32bits(x))
	}
	return out
}

func decodeVector(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return out
}
