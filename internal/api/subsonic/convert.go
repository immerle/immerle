package subsonic

import (
	"path/filepath"
	"time"

	"github.com/immerle/immerle/internal/models"
)

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func starredStr(a *models.Annotation) string {
	if a != nil && a.Starred != nil {
		return formatTime(*a.Starred)
	}
	return ""
}

// toChild converts a track to a Subsonic Child, enriching with annotation state.
func toChild(t models.Track, ann *models.Annotation) Child {
	c := Child{
		ID:             t.ID,
		Parent:         t.AlbumID,
		IsDir:          false,
		Title:          t.Title,
		Album:          t.AlbumName,
		Artist:         t.ArtistName,
		Track:          t.TrackNo,
		Year:           t.Year,
		Genre:          t.Genre,
		CoverArt:       coverOrAlbum(t),
		Size:           t.Size,
		ContentType:    t.ContentType,
		Suffix:         t.Suffix,
		Duration:       t.Duration,
		BitRate:        t.BitRate,
		Path:           basePath(t.Path),
		IsVideo:        false,
		DiscNumber:     t.DiscNo,
		Created:        formatTime(t.CreatedAt),
		AlbumID:        t.AlbumID,
		ArtistID:       t.ArtistID,
		Type:           "music",
		MusicBrainzID:  t.MBID,
		Composer:       t.Composer,
		BPM:            t.BPM,
		Work:           t.Work,
		MovementName:   t.MovementName,
		MovementNumber: t.MovementNo,
	}
	if t.ReplayGainTrack != 0 || t.ReplayGainAlbum != 0 {
		c.ReplayGain = &ReplayGain{TrackGain: t.ReplayGainTrack, AlbumGain: t.ReplayGainAlbum}
	}
	for _, p := range t.Participants {
		c.Contributors = append(c.Contributors, Contributor{Role: p.Role, Artist: ContributorArtist{Name: p.Name}})
	}
	if ann != nil {
		c.PlayCount = ann.PlayCount
		c.UserRating = ann.Rating
		c.Starred = starredStr(ann)
	}
	return c
}

// basePath returns just the file name of a stored track path. Tracks are stored
// with absolute filesystem paths, which must not be disclosed to clients, so we
// expose only the leaf name rather than the server's directory layout.
func basePath(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Base(p)
}

func coverOrAlbum(t models.Track) string {
	if t.CoverArt != "" {
		return t.CoverArt
	}
	return t.AlbumID
}

// toAlbumID3 converts an album, optionally including its songs.
func toAlbumID3(a models.Album, ann *models.Annotation, songs []Child) AlbumID3 {
	out := AlbumID3{
		ID:        a.ID,
		Name:      a.Name,
		Artist:    a.ArtistName,
		ArtistID:  a.ArtistID,
		CoverArt:  coverIDForAlbum(a),
		SongCount: a.SongCount,
		Duration:  a.Duration,
		Created:   formatTime(a.CreatedAt),
		Year:      a.Year,
		Genre:     a.Genre,
		Song:      songs,
	}
	if ann != nil {
		out.Starred = starredStr(ann)
	}
	return out
}

func coverIDForAlbum(a models.Album) string {
	if a.CoverArt != "" {
		return a.CoverArt
	}
	return a.ID
}

// toArtistID3 converts an artist, optionally including its albums.
func toArtistID3(a models.Artist, ann *models.Annotation, albums []AlbumID3) ArtistID3 {
	out := ArtistID3{
		ID:         a.ID,
		Name:       a.Name,
		CoverArt:   a.CoverArt,
		AlbumCount: a.AlbumCount,
		Album:      albums,
	}
	if ann != nil {
		out.Starred = starredStr(ann)
	}
	return out
}
