package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/matching"
	"github.com/immerle/immerle/internal/models"
)

// CatalogRepo persists artists, albums and tracks. It exposes idempotent upsert
// helpers used by the scanner and the on-demand catalog.
type CatalogRepo struct{ *base }

// ---- Artists ----

func scanArtist(s rowScanner) (models.Artist, error) {
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
		err := r.bqueryRow(ctx, r.mel.New("artists").Select("id").Where("mbid", "=", a.MBID)).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	var id string
	err := r.bqueryRow(ctx, r.mel.New("artists").Select("id").Where("name", "=", a.Name)).Scan(&id)
	if err == nil {
		// Backfill MBID if we learned it.
		if a.MBID != "" {
			if _, err := r.bexec(ctx, r.mel.NewUpdate("artists").Set("mbid", a.MBID).
				Where("id", "=", id).Where("mbid", "=", "")); err != nil {
				return "", err
			}
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_, err = r.bexec(ctx, r.mel.NewInsert("artists").
		Set("id", a.ID).Set("name", a.Name).Set("sort_name", a.SortName).Set("mbid", a.MBID).
		Set("cover_art", a.CoverArt).Set("created_at", db.Millis(a.CreatedAt)))
	if err != nil {
		return "", err
	}
	return a.ID, nil
}

// GetArtist returns one artist with its album count.
func (r *CatalogRepo) GetArtist(ctx context.Context, id string) (models.Artist, error) {
	row := r.bqueryRow(ctx, r.mel.New("artists").
		Select("id", "name", "sort_name", "mbid", "cover_art", "created_at").Where("id", "=", id))
	a, err := scanArtist(row)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	if err := r.bqueryRow(ctx, r.mel.New("albums").Select("COUNT(*)").Where("artist_id", "=", id)).Scan(&a.AlbumCount); err != nil {
		return a, err
	}
	return a, nil
}

// ListArtists returns all artists with album counts ordered by name.
func (r *CatalogRepo) ListArtists(ctx context.Context) ([]models.Artist, error) {
	// album_count is a correlated subquery column; melody passes raw select
	// columns through verbatim, so the generated SQL is identical.
	rows, err := r.bquery(ctx, r.mel.New("artists a").Select(
		"a.id", "a.name", "a.sort_name", "a.mbid", "a.cover_art", "a.created_at",
		"(SELECT COUNT(*) FROM albums al WHERE al.artist_id = a.id) AS album_count",
	).OrderBy("a.name", ""))
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

// albumSelect joins artists and computes song_count/duration via correlated
// subqueries — neither a JOIN nor a subquery column is expressible by melody, so
// every read built on it (GetAlbum, ListAlbumsByArtist, ListAlbums, Search)
// stays hand-written.
const albumSelect = `
	SELECT al.id, al.name, al.sort_name, al.artist_id, ar.name, al.mbid, al.year, al.genre,
	       COALESCE(NULLIF(al.cover_art,''),
	                (SELECT t.cover_art FROM tracks t
	                 WHERE t.album_id = al.id AND t.cover_art <> ''
	                 ORDER BY t.disc_no, t.track_no LIMIT 1), '') AS cover_art,
	       al.is_compilation, al.created_at,
	       (SELECT COUNT(*) FROM tracks t WHERE t.album_id = al.id) AS song_count,
	       (SELECT COALESCE(SUM(t.duration),0) FROM tracks t WHERE t.album_id = al.id) AS duration
	FROM albums al JOIN artists ar ON ar.id = al.artist_id`

func scanAlbum(s rowScanner) (models.Album, error) {
	var a models.Album
	var isComp int
	var createdAt int64
	if err := s.Scan(&a.ID, &a.Name, &a.SortName, &a.ArtistID, &a.ArtistName, &a.MBID, &a.Year, &a.Genre, &a.CoverArt, &isComp, &createdAt, &a.SongCount, &a.Duration); err != nil {
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
		err := r.bqueryRow(ctx, r.mel.New("albums").Select("id").Where("mbid", "=", a.MBID)).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	var id string
	err := r.bqueryRow(ctx, r.mel.New("albums").Select("id").
		Where("artist_id", "=", a.ArtistID).Where("name", "=", a.Name)).Scan(&id)
	if err == nil {
		// Conditional merge: keep the stored value unless the incoming one is
		// non-zero/non-empty (year, genre, mbid, cover) — only is_compilation is
		// overwritten outright.
		if _, err := r.bexec(ctx, r.mel.NewUpdate("albums").
			SetRaw("year", "COALESCE(NULLIF(?,0), year)", a.Year).
			SetRaw("sort_name", "COALESCE(NULLIF(?,''), sort_name)", a.SortName).
			SetRaw("genre", "COALESCE(NULLIF(?,''), genre)", a.Genre).
			SetRaw("cover_art", "CASE WHEN cover_art='' THEN ? ELSE cover_art END", a.CoverArt).
			Set("is_compilation", db.Bool(a.IsCompilation)).
			SetRaw("mbid", "CASE WHEN mbid='' THEN ? ELSE mbid END", a.MBID).
			Where("id", "=", id)); err != nil {
			return "", err
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_, err = r.bexec(ctx, r.mel.NewInsert("albums").
		Set("id", a.ID).Set("name", a.Name).Set("sort_name", a.SortName).Set("artist_id", a.ArtistID).Set("mbid", a.MBID).Set("year", a.Year).
		Set("genre", a.Genre).Set("cover_art", a.CoverArt).Set("is_compilation", db.Bool(a.IsCompilation)).
		Set("created_at", db.Millis(a.CreatedAt)))
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

// trackSelect joins albums and artists — a JOIN melody can't express, so every
// read built on it (GetTrack, ListTracksBy*, ListUploadedBy, ListAllTracks,
// RandomTracks, Search, and the smart-playlist evaluator) stays hand-written.
const trackSelect = `
	SELECT t.id, t.title, t.album_id, al.name, t.artist_id, ar.name, t.track_no, t.disc_no,
	       t.composer, t.genre, t.year, t.duration, t.bitrate, t.path, t.suffix, t.content_type, t.size,
	       t.title_sort, t.work, t.movement_name, t.movement_no, t.lyrics, t.participants,
	       t.mbid, t.file_hash, t.cover_art, t.bpm, t.replaygain_track, t.replaygain_album,
	       t.remote, t.provider, t.uploaded_by, t.created_at, t.updated_at
	FROM tracks t JOIN albums al ON al.id = t.album_id JOIN artists ar ON ar.id = t.artist_id`

func scanTrack(s rowScanner) (models.Track, error) {
	var t models.Track
	var remote int
	var createdAt, updatedAt int64
	var participants string
	if err := s.Scan(&t.ID, &t.Title, &t.AlbumID, &t.AlbumName, &t.ArtistID, &t.ArtistName, &t.TrackNo, &t.DiscNo,
		&t.Composer, &t.Genre, &t.Year, &t.Duration, &t.BitRate, &t.Path, &t.Suffix, &t.ContentType, &t.Size,
		&t.TitleSort, &t.Work, &t.MovementName, &t.MovementNo, &t.Lyrics, &participants,
		&t.MBID, &t.FileHash, &t.CoverArt, &t.BPM, &t.ReplayGainTrack, &t.ReplayGainAlbum,
		&remote, &t.Provider, &t.UploadedBy, &createdAt, &updatedAt); err != nil {
		return t, err
	}
	if participants != "" {
		_ = json.Unmarshal([]byte(participants), &t.Participants)
	}
	t.Remote = remote != 0
	t.CreatedAt = db.FromMillis(createdAt)
	t.UpdatedAt = db.FromMillis(updatedAt)
	return t, nil
}

// marshalParticipants serializes track participants for the JSON column ("" when
// none, so the column stays empty rather than holding "null").
func marshalParticipants(p []models.Participant) string {
	if len(p) == 0 {
		return ""
	}
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// UpsertTrack inserts or updates a track, matching by path, then MBID, then file
// hash so that renames preserve identity (and thus annotations).
func (r *CatalogRepo) UpsertTrack(ctx context.Context, t models.Track) (string, error) {
	existing, found, err := r.findTrackIdentity(ctx, t)
	if err != nil {
		return "", err
	}
	if found {
		_, err := r.bexec(ctx, r.mel.NewUpdate("tracks").
			Set("title", t.Title).Set("album_id", t.AlbumID).Set("artist_id", t.ArtistID).Set("track_no", t.TrackNo).
			Set("disc_no", t.DiscNo).Set("composer", t.Composer).Set("genre", t.Genre).Set("year", t.Year).Set("duration", t.Duration).
			Set("title_sort", t.TitleSort).Set("work", t.Work).Set("movement_name", t.MovementName).
			Set("movement_no", t.MovementNo).Set("lyrics", t.Lyrics).Set("participants", marshalParticipants(t.Participants)).
			Set("bitrate", t.BitRate).Set("path", t.Path).Set("suffix", t.Suffix).Set("content_type", t.ContentType).
			Set("size", t.Size).Set("mbid", t.MBID).Set("file_hash", t.FileHash).Set("cover_art", t.CoverArt).
			Set("bpm", t.BPM).Set("replaygain_track", t.ReplayGainTrack).Set("replaygain_album", t.ReplayGainAlbum).
			Set("remote", db.Bool(t.Remote)).Set("provider", t.Provider).Set("updated_at", db.Millis(t.UpdatedAt)).
			Where("id", "=", existing))
		return existing, err
	}
	_, err = r.bexec(ctx, r.mel.NewInsert("tracks").
		Set("id", t.ID).Set("title", t.Title).Set("album_id", t.AlbumID).Set("artist_id", t.ArtistID).
		Set("track_no", t.TrackNo).Set("disc_no", t.DiscNo).Set("composer", t.Composer).Set("genre", t.Genre).Set("year", t.Year).
		Set("title_sort", t.TitleSort).Set("work", t.Work).Set("movement_name", t.MovementName).
		Set("movement_no", t.MovementNo).Set("lyrics", t.Lyrics).Set("participants", marshalParticipants(t.Participants)).
		Set("duration", t.Duration).Set("bitrate", t.BitRate).Set("path", t.Path).Set("suffix", t.Suffix).
		Set("content_type", t.ContentType).Set("size", t.Size).Set("mbid", t.MBID).Set("file_hash", t.FileHash).
		Set("cover_art", t.CoverArt).Set("bpm", t.BPM).Set("replaygain_track", t.ReplayGainTrack).
		Set("replaygain_album", t.ReplayGainAlbum).Set("remote", db.Bool(t.Remote)).Set("provider", t.Provider).
		Set("created_at", db.Millis(t.CreatedAt)).Set("updated_at", db.Millis(t.UpdatedAt)))
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
		err := r.bqueryRow(ctx, r.mel.New("tracks").Select("id").Where("path", "=", t.Path)).Scan(&id)
		if err == nil {
			return id, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", false, err
		}
	}
	if t.MBID != "" {
		var id string
		err := r.bqueryRow(ctx, r.mel.New("tracks").Select("id").Where("mbid", "=", t.MBID)).Scan(&id)
		if err == nil {
			return id, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", false, err
		}
	}
	if t.FileHash != "" {
		var id string
		err := r.bqueryRow(ctx, r.mel.New("tracks").Select("id").Where("file_hash", "=", t.FileHash)).Scan(&id)
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
		err := r.bqueryRow(ctx, r.mel.New("tracks").Select("id").
			Where("mbid", "=", mbid).Where("remote", "=", 0)).Scan(&id)
		if err == nil {
			return id, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", false, err
		}
	}
	if hash != "" {
		var id string
		err := r.bqueryRow(ctx, r.mel.New("tracks").Select("id").
			Where("file_hash", "=", hash).Where("remote", "=", 0)).Scan(&id)
		if err == nil {
			return id, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", false, err
		}
	}
	return "", false, nil
}

// FindByArtistTitle looks up a local, non-remote track by exact
// (case-insensitive) artist+title match — the same key a federated entry
// falls back to when it carries no mbid, so a track already in the catalog
// (e.g. manually uploaded, tagged without an mbid) is found before resorting
// to a remote provider search.
func (r *CatalogRepo) FindByArtistTitle(ctx context.Context, artist, title string) (models.Track, bool, error) {
	candidates, err := r.listTracks(ctx, trackSelect+` WHERE t.remote=0 AND LOWER(ar.name)=LOWER(?) AND LOWER(t.title)=LOWER(?)`, artist, title)
	if err != nil {
		return models.Track{}, false, err
	}
	if len(candidates) == 0 {
		return models.Track{}, false, nil
	}
	// Same artist+title can legitimately exist more than once in a library
	// (an alternate version scanned alongside the original) — prefer whichever
	// isn't flagged as one by matching.VersionMarkerPenalty, the same
	// disambiguation core.ResolveBestRemoteMatch applies to remote candidates,
	// so a candidate can't dodge it just by already being local.
	best := candidates[0]
	bestPenalty := matching.VersionMarkerPenalty(title, best.Title, best.AlbumName)
	for _, c := range candidates[1:] {
		if p := matching.VersionMarkerPenalty(title, c.Title, c.AlbumName); p < bestPenalty {
			best, bestPenalty = c, p
		}
	}
	return best, true, nil
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

// ListUploadedBy returns the tracks a user uploaded ("local" library), newest
// first.
func (r *CatalogRepo) ListUploadedBy(ctx context.Context, userID string) ([]models.Track, error) {
	return r.listTracks(ctx, trackSelect+` WHERE t.uploaded_by=? ORDER BY t.created_at DESC`, userID)
}

// SetTrackOwner marks a track as uploaded by a user.
func (r *CatalogRepo) SetTrackOwner(ctx context.Context, trackID, userID string) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("tracks").Set("uploaded_by", userID).Where("id", "=", trackID))
	return err
}

// SetTrackTitle renames a track.
func (r *CatalogRepo) SetTrackTitle(ctx context.Context, trackID, title string) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("tracks").
		Set("title", title).Set("updated_at", db.Millis(time.Now())).Where("id", "=", trackID))
	return err
}

// SetTrackCover points a track at a custom cover id (a file under coversDir).
func (r *CatalogRepo) SetTrackCover(ctx context.Context, trackID, coverArt string) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("tracks").
		Set("cover_art", coverArt).Set("updated_at", db.Millis(time.Now())).Where("id", "=", trackID))
	return err
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

// ListTracksByYearRange returns tracks released in [fromYear, toYear) — powers
// the decade auto-playlists (internal/autoplaylists).
func (r *CatalogRepo) ListTracksByYearRange(ctx context.Context, fromYear, toYear, count, offset int) ([]models.Track, error) {
	if count <= 0 {
		count = 10
	}
	return r.listTracks(ctx, trackSelect+` WHERE t.year>=? AND t.year<? ORDER BY t.year, ar.name, al.name, t.disc_no, t.track_no LIMIT ? OFFSET ?`,
		fromYear, toYear, count, offset)
}

// TracksByIDs batch-fetches tracks by id, keyed by id (order not preserved —
// callers re-order by looking each id up). Public wrapper around the
// package-private tracksByIDs (see PlaylistRepo.Tracks/HallOfFameRepo.Entries),
// for callers outside persistence that already have an ordered id list (e.g. a
// "top tracks" or "forgotten favorites" personal list).
func (r *CatalogRepo) TracksByIDs(ctx context.Context, ids []string) (map[string]models.Track, error) {
	return tracksByIDs(ctx, r.base, ids)
}

// TrackListOptions controls the admin "all tracks" listing.
type TrackListOptions struct {
	Query  string // case-insensitive LIKE over title/artist/album
	Size   int
	Offset int
}

// ListAllTracks returns downloaded (local) tracks, newest first, with optional
// search and pagination — powers the admin library management screen. Remote
// (not-yet-downloaded) provider placeholders are excluded. Built on trackSelect
// (a JOIN) with a LOWER(col) LIKE filter, so it stays hand-written.
func (r *CatalogRepo) ListAllTracks(ctx context.Context, opt TrackListOptions) ([]models.Track, error) {
	if opt.Size <= 0 {
		opt.Size = 50
	}
	q := trackSelect + " WHERE t.remote=0"
	var args []any
	if opt.Query != "" {
		like := "%" + strings.ToLower(opt.Query) + "%"
		q += " AND (LOWER(t.title) LIKE ? OR LOWER(ar.name) LIKE ? OR LOWER(al.name) LIKE ?)"
		args = append(args, like, like, like)
	}
	q += " ORDER BY t.created_at DESC, t.title LIMIT ? OFFSET ?"
	args = append(args, opt.Size, opt.Offset)
	return r.listTracks(ctx, q, args...)
}

// CountTracks returns the number of downloaded tracks matching the same filter
// as ListAllTracks (for pagination totals). A JOIN + LOWER(col) LIKE filter melody
// can't express, so it stays hand-written.
func (r *CatalogRepo) CountTracks(ctx context.Context, query string) (int, error) {
	q := `SELECT COUNT(*) FROM tracks t JOIN albums al ON al.id=t.album_id JOIN artists ar ON ar.id=t.artist_id WHERE t.remote=0`
	var args []any
	if query != "" {
		like := "%" + strings.ToLower(query) + "%"
		q += " AND (LOWER(t.title) LIKE ? OR LOWER(ar.name) LIKE ? OR LOWER(al.name) LIKE ?)"
		args = append(args, like, like, like)
	}
	var n int
	if err := r.queryRow(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// UpdateTrackMetadata edits the directly-editable track fields (admin). Album
// and artist relationships are intentionally not touched here.
func (r *CatalogRepo) UpdateTrackMetadata(ctx context.Context, id string, title, genre string, year, trackNo, discNo int) error {
	res, err := r.bexec(ctx, r.mel.NewUpdate("tracks").
		Set("title", title).Set("genre", genre).Set("year", year).Set("track_no", trackNo).Set("disc_no", discNo).
		Set("updated_at", db.Millis(time.Now())).Where("id", "=", id))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTrackCascade removes a track and the rows that reference it but are not
// covered by an ON DELETE CASCADE foreign key (annotations, shares,
// activity_events, download_jobs). playlist_tracks and scrobbles cascade via FK.
// Stays hand-written: it runs inside a transaction (the builder helpers use the
// pool, not the tx).
func (r *CatalogRepo) DeleteTrackCascade(ctx context.Context, id string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM annotations WHERE item_type='track' AND item_id=?`,
			`DELETE FROM shares WHERE item_type='track' AND item_id=?`,
			`DELETE FROM activity_events WHERE item_type='track' AND item_id=?`,
			`DELETE FROM download_jobs WHERE track_id=?`,
			`DELETE FROM tracks WHERE id=?`,
		} {
			if _, err := tx.ExecContext(ctx, r.rebind(q), id); err != nil {
				return err
			}
		}
		return nil
	})
}

// RandomTracks returns up to count random tracks, optionally filtered by genre
// and/or year range (powers getRandomSongs). Built on trackSelect (a JOIN) and
// ordered by RANDOM(), neither expressible by melody, so it stays hand-written.
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
	rows, err := r.bquery(ctx, r.mel.New("artists").Select("id", "name", "mbid").
		Where("cover_art", "=", "").Where("image_checked", "=", 0).OrderBy("name", "").Limit(limit))
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
	_, err := r.bexec(ctx, r.mel.NewUpdate("artists").
		Set("cover_art", coverArt).Set("image_checked", 1).Where("id", "=", id))
	return err
}

// MarkArtistImageChecked flags an artist as checked (even when no image was
// found) so the enrichment loop does not retry it indefinitely.
func (r *CatalogRepo) MarkArtistImageChecked(ctx context.Context, id string) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("artists").Set("image_checked", 1).Where("id", "=", id))
	return err
}

// FindArtistByName returns an artist by exact name (powers getTopSongs).
func (r *CatalogRepo) FindArtistByName(ctx context.Context, name string) (models.Artist, error) {
	row := r.bqueryRow(ctx, r.mel.New("artists").
		Select("id", "name", "sort_name", "mbid", "cover_art", "created_at").Where("name", "=", name))
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
// Kept as one raw block: melody's WhereRaw could wrap each EXISTS, but the
// four-clause predicate reads better written out as SQL.
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
	_, err := r.bexec(ctx, r.mel.NewDelete("tracks").Where("id", "=", id))
	return err
}

// AllTrackPaths returns the set of known local track paths (for scan pruning).
func (r *CatalogRepo) AllTrackPaths(ctx context.Context) (map[string]string, error) {
	rows, err := r.bquery(ctx, r.mel.New("tracks").Select("id", "path").
		Where("path", "<>", "").Where("remote", "=", 0))
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

// Search performs a simple LIKE search across artists, albums and tracks. The
// LOWER(col) LIKE filters (plus JOIN/subquery columns) can't be expressed by
// melody, so these queries stay hand-written. A count of 0 skips that type's
// query entirely (rather than running it with LIMIT 0) — how a type filter
// (see internal/api/immerle/browse.go's searchCounts) avoids querying types
// the caller didn't ask for.
func (r *CatalogRepo) Search(ctx context.Context, q string, artistCount, albumCount, songCount int) ([]models.Artist, []models.Album, []models.Track, error) {
	like := "%" + strings.ToLower(q) + "%"

	var artists []models.Artist
	if artistCount > 0 {
		artRows, err := r.query(ctx, `
			SELECT a.id, a.name, a.sort_name, a.mbid, a.cover_art, a.created_at,
			       (SELECT COUNT(*) FROM albums al WHERE al.artist_id = a.id)
			FROM artists a WHERE LOWER(a.name) LIKE ? ORDER BY a.name LIMIT ?`, like, artistCount)
		if err != nil {
			return nil, nil, nil, err
		}
		for artRows.Next() {
			var a models.Artist
			var createdAt int64
			if err := artRows.Scan(&a.ID, &a.Name, &a.SortName, &a.MBID, &a.CoverArt, &createdAt, &a.AlbumCount); err != nil {
				artRows.Close()
				return nil, nil, nil, err
			}
			artists = append(artists, a)
		}
		if err := artRows.Err(); err != nil {
			artRows.Close()
			return nil, nil, nil, err
		}
		artRows.Close()
	}

	var albums []models.Album
	if albumCount > 0 {
		var err error
		albums, err = r.listAlbums(ctx, albumSelect+` WHERE LOWER(al.name) LIKE ? ORDER BY al.name LIMIT ?`, like, albumCount)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	var tracks []models.Track
	if songCount > 0 {
		var err error
		tracks, err = r.listTracks(ctx, trackSelect+` WHERE LOWER(t.title) LIKE ? ORDER BY t.title LIMIT ?`, like, songCount)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return artists, albums, tracks, nil
}

// Stats reports catalog cardinalities.
func (r *CatalogRepo) Stats(ctx context.Context) (artists, albums, tracks int, err error) {
	if err = r.bqueryRow(ctx, r.mel.New("artists").Select("COUNT(*)")).Scan(&artists); err != nil {
		return
	}
	if err = r.bqueryRow(ctx, r.mel.New("albums").Select("COUNT(*)")).Scan(&albums); err != nil {
		return
	}
	err = r.bqueryRow(ctx, r.mel.New("tracks").Select("COUNT(*)")).Scan(&tracks)
	return
}

// Totals reports the aggregate on-disk size (bytes) and duration (seconds) of the
// local library, summed from the indexed tracks.
func (r *CatalogRepo) Totals(ctx context.Context) (sizeBytes, durationSeconds int64, err error) {
	// The two aggregates are raw select expressions; melody passes them through
	// verbatim, so the generated SQL is identical.
	err = r.bqueryRow(ctx, r.mel.New("tracks").
		Select("COALESCE(SUM(size),0)", "COALESCE(SUM(duration),0)")).Scan(&sizeBytes, &durationSeconds)
	return
}
