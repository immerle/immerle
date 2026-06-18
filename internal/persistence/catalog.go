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

// CatalogRepo persists artists, albums and tracks. It exposes idempotent upsert
// helpers used by the scanner and the on-demand catalog.
type CatalogRepo struct{ *base }

// ---- Artists ----

func scanArtist(s interface{ Scan(...any) error }) (models.Artist, error) {
	var a models.Artist
	var createdAt int64
	if err := s.Scan(&a.ID, &a.Name, &a.SortName, &a.MBID, &a.CoverArt, &createdAt); err != nil {
		return a, err
	}
	a.CreatedAt = db.FromMillis(createdAt)
	return a, nil
}

// UpsertArtist inserts the artist or returns the id of an existing one matched by
// MBID (preferred) or name. It is idempotent.
func (r *CatalogRepo) UpsertArtist(ctx context.Context, a models.Artist) (string, error) {
	if a.MBID != "" {
		var id string
		err := r.queryRow(ctx, `SELECT id FROM artists WHERE mbid=?`, a.MBID).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	var id string
	err := r.queryRow(ctx, `SELECT id FROM artists WHERE name=?`, a.Name).Scan(&id)
	if err == nil {
		// Backfill MBID if we learned it.
		if a.MBID != "" {
			if _, err := r.exec(ctx, `UPDATE artists SET mbid=? WHERE id=? AND mbid=''`, a.MBID, id); err != nil {
				return "", err
			}
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_, err = r.exec(ctx, `INSERT INTO artists (id, name, sort_name, mbid, cover_art, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.SortName, a.MBID, a.CoverArt, db.Millis(a.CreatedAt))
	if err != nil {
		return "", err
	}
	return a.ID, nil
}

// GetArtist returns one artist with its album count.
func (r *CatalogRepo) GetArtist(ctx context.Context, id string) (models.Artist, error) {
	row := r.queryRow(ctx, `SELECT id, name, sort_name, mbid, cover_art, created_at FROM artists WHERE id=?`, id)
	a, err := scanArtist(row)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	if err := r.queryRow(ctx, `SELECT COUNT(*) FROM albums WHERE artist_id=?`, id).Scan(&a.AlbumCount); err != nil {
		return a, err
	}
	return a, nil
}

// ListArtists returns all artists with album counts ordered by name.
func (r *CatalogRepo) ListArtists(ctx context.Context) ([]models.Artist, error) {
	rows, err := r.query(ctx, `
		SELECT a.id, a.name, a.sort_name, a.mbid, a.cover_art, a.created_at,
		       (SELECT COUNT(*) FROM albums al WHERE al.artist_id = a.id) AS album_count
		FROM artists a ORDER BY a.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Artist
	for rows.Next() {
		var a models.Artist
		var createdAt int64
		if err := rows.Scan(&a.ID, &a.Name, &a.SortName, &a.MBID, &a.CoverArt, &createdAt, &a.AlbumCount); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- Albums ----

const albumSelect = `
	SELECT al.id, al.name, al.artist_id, ar.name, al.mbid, al.year, al.genre, al.cover_art,
	       al.is_compilation, al.created_at,
	       (SELECT COUNT(*) FROM tracks t WHERE t.album_id = al.id) AS song_count,
	       (SELECT COALESCE(SUM(t.duration),0) FROM tracks t WHERE t.album_id = al.id) AS duration
	FROM albums al JOIN artists ar ON ar.id = al.artist_id`

func scanAlbum(s interface{ Scan(...any) error }) (models.Album, error) {
	var a models.Album
	var isComp int
	var createdAt int64
	if err := s.Scan(&a.ID, &a.Name, &a.ArtistID, &a.ArtistName, &a.MBID, &a.Year, &a.Genre, &a.CoverArt, &isComp, &createdAt, &a.SongCount, &a.Duration); err != nil {
		return a, err
	}
	a.IsCompilation = isComp != 0
	a.CreatedAt = db.FromMillis(createdAt)
	return a, nil
}

// UpsertAlbum inserts or finds an album by (MBID) or (artist_id, name).
func (r *CatalogRepo) UpsertAlbum(ctx context.Context, a models.Album) (string, error) {
	if a.MBID != "" {
		var id string
		err := r.queryRow(ctx, `SELECT id FROM albums WHERE mbid=?`, a.MBID).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	var id string
	err := r.queryRow(ctx, `SELECT id FROM albums WHERE artist_id=? AND name=?`, a.ArtistID, a.Name).Scan(&id)
	if err == nil {
		if _, err := r.exec(ctx, `UPDATE albums SET year=COALESCE(NULLIF(?,0), year), genre=COALESCE(NULLIF(?,''), genre),
			cover_art=CASE WHEN cover_art='' THEN ? ELSE cover_art END, is_compilation=?, mbid=CASE WHEN mbid='' THEN ? ELSE mbid END
			WHERE id=?`, a.Year, a.Genre, a.CoverArt, db.Bool(a.IsCompilation), a.MBID, id); err != nil {
			return "", err
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_, err = r.exec(ctx, `INSERT INTO albums (id, name, artist_id, mbid, year, genre, cover_art, is_compilation, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.ArtistID, a.MBID, a.Year, a.Genre, a.CoverArt, db.Bool(a.IsCompilation), db.Millis(a.CreatedAt))
	if err != nil {
		return "", err
	}
	return a.ID, nil
}

// GetAlbum returns one album.
func (r *CatalogRepo) GetAlbum(ctx context.Context, id string) (models.Album, error) {
	row := r.queryRow(ctx, albumSelect+` WHERE al.id=?`, id)
	a, err := scanAlbum(row)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// ListAlbumsByArtist returns the albums of an artist.
func (r *CatalogRepo) ListAlbumsByArtist(ctx context.Context, artistID string) ([]models.Album, error) {
	return r.listAlbums(ctx, albumSelect+` WHERE al.artist_id=? ORDER BY al.year, al.name`, artistID)
}

// AlbumListOptions controls getAlbumList2-style queries.
type AlbumListOptions struct {
	Type     string // newest, recent, frequent, random, alphabeticalByName, byGenre, byYear, starred
	Size     int
	Offset   int
	Genre    string
	FromYear int
	ToYear   int
	UserID   string // for starred/frequent/recent
}

// ListAlbums returns albums according to options (powers getAlbumList2).
func (r *CatalogRepo) ListAlbums(ctx context.Context, opt AlbumListOptions) ([]models.Album, error) {
	if opt.Size <= 0 {
		opt.Size = 10
	}
	order := "al.name"
	var join string
	var where []string
	var joinArgs []any
	var args []any

	// annotationJoin wires the per-user album annotations (play stats, rating,
	// starred) so the stat-based list types can sort/filter by them.
	annotationJoin := func(condition, ordering string) {
		join = ` JOIN annotations an ON an.item_type='album' AND an.item_id=al.id AND an.user_id=?`
		joinArgs = append(joinArgs, opt.UserID)
		if condition != "" {
			where = append(where, condition)
		}
		order = ordering
	}

	switch opt.Type {
	case "newest":
		order = "al.created_at DESC"
	case "alphabeticalByName", "":
		order = "al.name"
	case "alphabeticalByArtist":
		order = "ar.name, al.name"
	case "byYear":
		if opt.FromYear <= opt.ToYear {
			where = append(where, "al.year >= ? AND al.year <= ?")
			args = append(args, opt.FromYear, opt.ToYear)
			order = "al.year"
		} else {
			where = append(where, "al.year <= ? AND al.year >= ?")
			args = append(args, opt.FromYear, opt.ToYear)
			order = "al.year DESC"
		}
	case "byGenre":
		where = append(where, "al.genre = ?")
		args = append(args, opt.Genre)
	case "random":
		order = "RANDOM()"
	case "starred":
		annotationJoin("an.starred_at IS NOT NULL", "an.starred_at DESC")
	case "frequent":
		annotationJoin("an.play_count > 0", "an.play_count DESC, an.last_played DESC")
	case "recent":
		annotationJoin("an.last_played IS NOT NULL", "an.last_played DESC")
	case "highest":
		annotationJoin("an.rating > 0", "an.rating DESC, an.play_count DESC")
	}

	q := albumSelect + join
	// Join args precede WHERE/limit args in the placeholder order.
	args = append(joinArgs, args...)
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY " + order + " LIMIT ? OFFSET ?"
	args = append(args, opt.Size, opt.Offset)
	return r.listAlbums(ctx, q, args...)
}

func (r *CatalogRepo) listAlbums(ctx context.Context, q string, args ...any) ([]models.Album, error) {
	rows, err := r.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Album
	for rows.Next() {
		a, err := scanAlbum(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- Tracks ----

const trackSelect = `
	SELECT t.id, t.title, t.album_id, al.name, t.artist_id, ar.name, t.track_no, t.disc_no,
	       t.genre, t.year, t.duration, t.bitrate, t.path, t.suffix, t.content_type, t.size,
	       t.mbid, t.file_hash, t.cover_art, t.bpm, t.replaygain_track, t.replaygain_album,
	       t.remote, t.provider, t.created_at, t.updated_at
	FROM tracks t JOIN albums al ON al.id = t.album_id JOIN artists ar ON ar.id = t.artist_id`

func scanTrack(s interface{ Scan(...any) error }) (models.Track, error) {
	var t models.Track
	var remote int
	var createdAt, updatedAt int64
	if err := s.Scan(&t.ID, &t.Title, &t.AlbumID, &t.AlbumName, &t.ArtistID, &t.ArtistName, &t.TrackNo, &t.DiscNo,
		&t.Genre, &t.Year, &t.Duration, &t.BitRate, &t.Path, &t.Suffix, &t.ContentType, &t.Size,
		&t.MBID, &t.FileHash, &t.CoverArt, &t.BPM, &t.ReplayGainTrack, &t.ReplayGainAlbum,
		&remote, &t.Provider, &createdAt, &updatedAt); err != nil {
		return t, err
	}
	t.Remote = remote != 0
	t.CreatedAt = db.FromMillis(createdAt)
	t.UpdatedAt = db.FromMillis(updatedAt)
	return t, nil
}

// UpsertTrack inserts or updates a track, matching by path, then MBID, then file
// hash so that renames preserve identity (and thus annotations).
func (r *CatalogRepo) UpsertTrack(ctx context.Context, t models.Track) (string, error) {
	existing, found, err := r.findTrackIdentity(ctx, t)
	if err != nil {
		return "", err
	}
	if found {
		_, err := r.exec(ctx, `UPDATE tracks SET title=?, album_id=?, artist_id=?, track_no=?, disc_no=?, genre=?,
			year=?, duration=?, bitrate=?, path=?, suffix=?, content_type=?, size=?, mbid=?, file_hash=?,
			cover_art=?, bpm=?, replaygain_track=?, replaygain_album=?, remote=?, provider=?, updated_at=?
			WHERE id=?`,
			t.Title, t.AlbumID, t.ArtistID, t.TrackNo, t.DiscNo, t.Genre, t.Year, t.Duration, t.BitRate, t.Path,
			t.Suffix, t.ContentType, t.Size, t.MBID, t.FileHash, t.CoverArt, t.BPM, t.ReplayGainTrack, t.ReplayGainAlbum,
			db.Bool(t.Remote), t.Provider, db.Millis(t.UpdatedAt), existing)
		return existing, err
	}
	_, err = r.exec(ctx, `INSERT INTO tracks (id, title, album_id, artist_id, track_no, disc_no, genre, year, duration,
		bitrate, path, suffix, content_type, size, mbid, file_hash, cover_art, bpm, replaygain_track, replaygain_album,
		remote, provider, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Title, t.AlbumID, t.ArtistID, t.TrackNo, t.DiscNo, t.Genre, t.Year, t.Duration, t.BitRate, t.Path,
		t.Suffix, t.ContentType, t.Size, t.MBID, t.FileHash, t.CoverArt, t.BPM, t.ReplayGainTrack, t.ReplayGainAlbum,
		db.Bool(t.Remote), t.Provider, db.Millis(t.CreatedAt), db.Millis(t.UpdatedAt))
	if err != nil {
		return "", err
	}
	return t.ID, nil
}

// findTrackIdentity locates an existing track that should be considered the same
// as t, preserving its id (and annotations). Order: exact path, MBID, file hash.
func (r *CatalogRepo) findTrackIdentity(ctx context.Context, t models.Track) (string, bool, error) {
	if t.Path != "" {
		var id string
		err := r.queryRow(ctx, `SELECT id FROM tracks WHERE path=?`, t.Path).Scan(&id)
		if err == nil {
			return id, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", false, err
		}
	}
	if t.MBID != "" {
		var id string
		err := r.queryRow(ctx, `SELECT id FROM tracks WHERE mbid=?`, t.MBID).Scan(&id)
		if err == nil {
			return id, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", false, err
		}
	}
	if t.FileHash != "" {
		var id string
		err := r.queryRow(ctx, `SELECT id FROM tracks WHERE file_hash=?`, t.FileHash).Scan(&id)
		if err == nil {
			return id, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", false, err
		}
	}
	return "", false, nil
}

// TrackExistsByMBIDOrHash reports whether a non-remote track with the given MBID
// or file hash already exists — used for strict on-demand dedup.
func (r *CatalogRepo) TrackExistsByMBIDOrHash(ctx context.Context, mbid, hash string) (string, bool, error) {
	if mbid != "" {
		var id string
		err := r.queryRow(ctx, `SELECT id FROM tracks WHERE mbid=? AND remote=0`, mbid).Scan(&id)
		if err == nil {
			return id, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", false, err
		}
	}
	if hash != "" {
		var id string
		err := r.queryRow(ctx, `SELECT id FROM tracks WHERE file_hash=? AND remote=0`, hash).Scan(&id)
		if err == nil {
			return id, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", false, err
		}
	}
	return "", false, nil
}

// GetTrack returns one track.
func (r *CatalogRepo) GetTrack(ctx context.Context, id string) (models.Track, error) {
	row := r.queryRow(ctx, trackSelect+` WHERE t.id=?`, id)
	t, err := scanTrack(row)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// GetTracks returns multiple tracks preserving the requested order.
func (r *CatalogRepo) GetTracks(ctx context.Context, ids []string) ([]models.Track, error) {
	out := make([]models.Track, 0, len(ids))
	for _, id := range ids {
		t, err := r.GetTrack(ctx, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// ListTracksByAlbum returns an album's tracks ordered by disc/track number.
func (r *CatalogRepo) ListTracksByAlbum(ctx context.Context, albumID string) ([]models.Track, error) {
	return r.listTracks(ctx, trackSelect+` WHERE t.album_id=? ORDER BY t.disc_no, t.track_no, t.title`, albumID)
}

// ListTracksByArtist returns an artist's tracks (powers getTopSongs/getMusicDirectory).
func (r *CatalogRepo) ListTracksByArtist(ctx context.Context, artistID string, limit int) ([]models.Track, error) {
	if limit <= 0 {
		limit = 50
	}
	return r.listTracks(ctx, trackSelect+` WHERE t.artist_id=? ORDER BY al.year, al.name, t.disc_no, t.track_no LIMIT ?`, artistID, limit)
}

// ListTracksByGenre returns tracks in a genre (powers getSongsByGenre).
func (r *CatalogRepo) ListTracksByGenre(ctx context.Context, genre string, count, offset int) ([]models.Track, error) {
	if count <= 0 {
		count = 10
	}
	return r.listTracks(ctx, trackSelect+` WHERE t.genre=? ORDER BY ar.name, al.name, t.disc_no, t.track_no LIMIT ? OFFSET ?`, genre, count, offset)
}

// RandomTracks returns up to count random tracks, optionally filtered by genre
// and/or year range (powers getRandomSongs).
func (r *CatalogRepo) RandomTracks(ctx context.Context, count int, genre string, fromYear, toYear int) ([]models.Track, error) {
	if count <= 0 {
		count = 10
	}
	q := trackSelect
	var where []string
	var args []any
	if genre != "" {
		where = append(where, "t.genre=?")
		args = append(args, genre)
	}
	if fromYear > 0 {
		where = append(where, "t.year >= ?")
		args = append(args, fromYear)
	}
	if toYear > 0 {
		where = append(where, "t.year <= ?")
		args = append(args, toYear)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY RANDOM() LIMIT ?"
	args = append(args, count)
	return r.listTracks(ctx, q, args...)
}

// ListArtistsNeedingImage returns artists without cover art that have not yet
// been checked for an avatar (powers the artist-image enrichment loop).
func (r *CatalogRepo) ListArtistsNeedingImage(ctx context.Context, limit int) ([]models.Artist, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.query(ctx, `SELECT id, name, mbid FROM artists WHERE cover_art='' AND image_checked=0 ORDER BY name LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Artist
	for rows.Next() {
		var a models.Artist
		if err := rows.Scan(&a.ID, &a.Name, &a.MBID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetArtistCover sets an artist's cover art id and marks it image-checked.
func (r *CatalogRepo) SetArtistCover(ctx context.Context, id, coverArt string) error {
	_, err := r.exec(ctx, `UPDATE artists SET cover_art=?, image_checked=1 WHERE id=?`, coverArt, id)
	return err
}

// MarkArtistImageChecked flags an artist as checked (even when no image was
// found) so the enrichment loop does not retry it indefinitely.
func (r *CatalogRepo) MarkArtistImageChecked(ctx context.Context, id string) error {
	_, err := r.exec(ctx, `UPDATE artists SET image_checked=1 WHERE id=?`, id)
	return err
}

// FindArtistByName returns an artist by exact name (powers getTopSongs).
func (r *CatalogRepo) FindArtistByName(ctx context.Context, name string) (models.Artist, error) {
	row := r.queryRow(ctx, `SELECT id, name, sort_name, mbid, cover_art, created_at FROM artists WHERE name=?`, name)
	a, err := scanArtist(row)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func (r *CatalogRepo) listTracks(ctx context.Context, q string, args ...any) ([]models.Track, error) {
	rows, err := r.query(ctx, q, args...)
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

// ProviderTracksToEvict returns provider-downloaded tracks eligible for cleanup:
// a track that is the result of a completed download job AND has no reason to be
// kept — not starred (by anyone), not played since `playedSince`, and not in any
// playlist. Manually-scanned tracks (no download job) are never returned.
func (r *CatalogRepo) ProviderTracksToEvict(ctx context.Context, playedSince time.Time) ([]models.Track, error) {
	rows, err := r.query(ctx, `
		SELECT t.id, t.path FROM tracks t
		WHERE t.remote=0
		  AND EXISTS (SELECT 1 FROM download_jobs dj WHERE dj.track_id=t.id AND dj.status='completed')
		  AND NOT EXISTS (SELECT 1 FROM annotations a WHERE a.item_type='track' AND a.item_id=t.id AND a.starred_at IS NOT NULL)
		  AND NOT EXISTS (SELECT 1 FROM annotations a WHERE a.item_type='track' AND a.item_id=t.id AND a.last_played IS NOT NULL AND a.last_played >= ?)
		  AND NOT EXISTS (SELECT 1 FROM playlist_tracks pt WHERE pt.track_id=t.id)`,
		db.Millis(playedSince))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Track
	for rows.Next() {
		var t models.Track
		if err := rows.Scan(&t.ID, &t.Path); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTrack removes a track by id.
func (r *CatalogRepo) DeleteTrack(ctx context.Context, id string) error {
	_, err := r.exec(ctx, `DELETE FROM tracks WHERE id=?`, id)
	return err
}

// AllTrackPaths returns the set of known local track paths (for scan pruning).
func (r *CatalogRepo) AllTrackPaths(ctx context.Context) (map[string]string, error) {
	rows, err := r.query(ctx, `SELECT id, path FROM tracks WHERE path <> '' AND remote=0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		out[path] = id
	}
	return out, rows.Err()
}

// Search performs a simple LIKE search across artists, albums and tracks.
func (r *CatalogRepo) Search(ctx context.Context, q string, artistCount, albumCount, songCount int) ([]models.Artist, []models.Album, []models.Track, error) {
	like := "%" + strings.ToLower(q) + "%"

	artRows, err := r.query(ctx, `
		SELECT a.id, a.name, a.sort_name, a.mbid, a.cover_art, a.created_at,
		       (SELECT COUNT(*) FROM albums al WHERE al.artist_id = a.id)
		FROM artists a WHERE LOWER(a.name) LIKE ? ORDER BY a.name LIMIT ?`, like, artistCount)
	if err != nil {
		return nil, nil, nil, err
	}
	var artists []models.Artist
	for artRows.Next() {
		var a models.Artist
		var createdAt int64
		if err := artRows.Scan(&a.ID, &a.Name, &a.SortName, &a.MBID, &a.CoverArt, &createdAt, &a.AlbumCount); err != nil {
			artRows.Close()
			return nil, nil, nil, err
		}
		artists = append(artists, a)
	}
	artRows.Close()

	albums, err := r.listAlbums(ctx, albumSelect+` WHERE LOWER(al.name) LIKE ? ORDER BY al.name LIMIT ?`, like, albumCount)
	if err != nil {
		return nil, nil, nil, err
	}

	tracks, err := r.listTracks(ctx, trackSelect+` WHERE LOWER(t.title) LIKE ? ORDER BY t.title LIMIT ?`, like, songCount)
	if err != nil {
		return nil, nil, nil, err
	}
	return artists, albums, tracks, nil
}

// Stats reports catalog cardinalities.
func (r *CatalogRepo) Stats(ctx context.Context) (artists, albums, tracks int, err error) {
	if err = r.queryRow(ctx, `SELECT COUNT(*) FROM artists`).Scan(&artists); err != nil {
		return
	}
	if err = r.queryRow(ctx, `SELECT COUNT(*) FROM albums`).Scan(&albums); err != nil {
		return
	}
	err = r.queryRow(ctx, `SELECT COUNT(*) FROM tracks`).Scan(&tracks)
	return
}

// Totals reports the aggregate on-disk size (bytes) and duration (seconds) of the
// local library, summed from the indexed tracks.
func (r *CatalogRepo) Totals(ctx context.Context) (sizeBytes, durationSeconds int64, err error) {
	err = r.queryRow(ctx, `SELECT COALESCE(SUM(size),0), COALESCE(SUM(duration),0) FROM tracks`).Scan(&sizeBytes, &durationSeconds)
	return
}
