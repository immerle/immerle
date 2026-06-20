package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dhowden/tag"

	"github.com/immerle/immerle/internal/models"
)

// Metadata is the normalized set of fields extracted from an audio file.
type Metadata struct {
	Title           string
	TitleSort       string
	Album           string
	AlbumSort       string
	Artist          string
	ArtistSort      string
	AlbumArtist     string
	AlbumArtistSort string
	Composer        string
	Genre           string
	Year            int
	TrackNo         int
	DiscNo          int
	BPM             int
	Work            string
	MovementName    string
	MovementNo      int
	Lyrics          string
	Participants    []models.Participant
	Duration        int     // seconds
	BitRate         int     // kbps
	ReplayGainTrack float64 // dB
	ReplayGainAlbum float64 // dB
	MBTrackID       string
	MBAlbumID       string
	MBArtistID      string
	Compilation     bool
	HasPicture      bool
	Picture         []byte
}

// audioExtensions are the file suffixes treated as audio.
var audioExtensions = map[string]string{
	".mp3":  "audio/mpeg",
	".flac": "audio/flac",
	".ogg":  "audio/ogg",
	".oga":  "audio/ogg",
	".opus": "audio/opus",
	".m4a":  "audio/mp4",
	".m4b":  "audio/mp4",
	".aac":  "audio/aac",
	".wav":  "audio/wav",
	".wma":  "audio/x-ms-wma",
	".aiff": "audio/aiff",
	".aif":  "audio/aiff",
	".ape":  "audio/x-ape",
	".wv":   "audio/x-wavpack",
}

// IsAudioFile reports whether the path has a recognized audio extension and
// returns its content type.
func IsAudioFile(path string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	ct, ok := audioExtensions[ext]
	return ct, ok
}

// Extractor reads tags from audio files, falling back to ffprobe for fields the
// tag library cannot provide (duration, bitrate) and for formats it cannot read.
type Extractor struct {
	ffprobePath string
}

// NewExtractor builds an Extractor using the given ffprobe binary.
func NewExtractor(ffprobePath string) *Extractor {
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	return &Extractor{ffprobePath: ffprobePath}
}

// Extract reads metadata from the file at path.
func (e *Extractor) Extract(ctx context.Context, path string) (Metadata, error) {
	md := Metadata{}

	f, err := os.Open(path)
	if err != nil {
		return md, err
	}
	defer func() { _ = f.Close() }()

	m, tagErr := tag.ReadFrom(f)
	if tagErr == nil {
		md.Title = m.Title()
		md.Album = m.Album()
		md.Artist = m.Artist()
		md.AlbumArtist = m.AlbumArtist()
		md.Composer = m.Composer()
		md.Genre = m.Genre()
		md.Year = m.Year()
		md.TrackNo, _ = m.Track()
		md.DiscNo, _ = m.Disc()
		md.Lyrics = m.Lyrics()
		if pic := m.Picture(); pic != nil {
			md.HasPicture = true
			md.Picture = pic.Data
		}
		extractMBIDs(m.Raw(), &md)
	}

	// Always consult ffprobe for technical fields (and as a fallback for tags).
	e.augmentWithFFprobe(ctx, path, &md)

	// A sidecar "<name>.lrc" wins over embedded lyrics (it carries synced
	// timestamps). Fixes the common case where lyrics live next to the file.
	if lrc := readSidecarLyrics(path); lrc != "" {
		md.Lyrics = lrc
	}

	if md.Title == "" {
		md.Title = baseNameNoExt(path)
	}
	if md.Artist == "" {
		md.Artist = "Unknown Artist"
	}
	if md.Album == "" {
		md.Album = "Unknown Album"
	}
	return md, nil
}

func baseNameNoExt(path string) string {
	name := filepath.Base(path)
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	return name
}

// extractMBIDs pulls MusicBrainz identifiers and compilation flags from the raw
// tag map (keys vary by format/tagger; we check the common variants).
func extractMBIDs(raw map[string]interface{}, md *Metadata) {
	if raw == nil {
		return
	}
	get := func(keys ...string) string {
		for k, v := range raw {
			lk := strings.ToLower(k)
			for _, want := range keys {
				if lk == want {
					return fmt.Sprint(v)
				}
			}
		}
		return ""
	}
	md.MBTrackID = firstNonEmpty(md.MBTrackID, get("musicbrainz_trackid", "musicbrainz track id", "----:com.apple.itunes:musicbrainz track id"))
	md.MBAlbumID = firstNonEmpty(md.MBAlbumID, get("musicbrainz_albumid", "musicbrainz album id"))
	md.MBArtistID = firstNonEmpty(md.MBArtistID, get("musicbrainz_artistid", "musicbrainz artist id"))
	if comp := get("compilation", "tcmp", "itunescompilation"); comp == "1" || strings.EqualFold(comp, "true") {
		md.Compilation = true
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ffprobeOutput is the subset of ffprobe JSON we consume.
type ffprobeOutput struct {
	Format struct {
		Duration string            `json:"duration"`
		BitRate  string            `json:"bit_rate"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
	Streams []struct {
		CodecType string            `json:"codec_type"`
		Tags      map[string]string `json:"tags"`
	} `json:"streams"`
}

func (e *Extractor) augmentWithFFprobe(ctx context.Context, path string, md *Metadata) {
	cmd := exec.CommandContext(ctx, e.ffprobePath,
		"-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", path)
	out, err := cmd.Output()
	if err != nil {
		return
	}
	var probe ffprobeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return
	}
	if md.Duration == 0 {
		if d, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
			md.Duration = int(d + 0.5)
		}
	}
	if md.BitRate == 0 {
		if br, err := strconv.Atoi(probe.Format.BitRate); err == nil {
			md.BitRate = br / 1000
		}
	}

	// Fall back to ffprobe tags for fields tag.ReadFrom could not provide.
	tags := mergeTags(probe)
	if md.Title == "" {
		md.Title = tags["title"]
	}
	if md.Album == "" {
		md.Album = tags["album"]
	}
	if md.Artist == "" {
		md.Artist = tags["artist"]
	}
	if md.AlbumArtist == "" {
		md.AlbumArtist = tags["album_artist"]
	}
	if md.Genre == "" {
		md.Genre = tags["genre"]
	}
	if md.Year == 0 {
		md.Year = parseYear(firstNonEmpty(tags["date"], tags["year"], tags["originalyear"]))
	}
	if md.TrackNo == 0 {
		md.TrackNo = parseLeadingInt(tags["track"])
	}
	if md.DiscNo == 0 {
		md.DiscNo = parseLeadingInt(tags["disc"])
	}
	if md.Composer == "" {
		md.Composer = tags["composer"]
	}
	if md.BPM == 0 {
		md.BPM = parseLeadingInt(firstNonEmpty(tags["bpm"], tags["tbpm"]))
	}
	if md.TitleSort == "" {
		md.TitleSort = firstNonEmpty(tags["title-sort"], tags["sort_name"], tags["titlesort"])
	}
	if md.ArtistSort == "" {
		md.ArtistSort = firstNonEmpty(tags["artist-sort"], tags["sort_artist"], tags["artistsort"])
	}
	if md.AlbumSort == "" {
		md.AlbumSort = firstNonEmpty(tags["album-sort"], tags["sort_album"], tags["albumsort"])
	}
	if md.AlbumArtistSort == "" {
		md.AlbumArtistSort = firstNonEmpty(tags["album_artist-sort"], tags["sort_album_artist"], tags["albumartistsort"])
	}
	if md.Work == "" {
		md.Work = firstNonEmpty(tags["work"], tags["content_group"], tags["grouping"])
	}
	if md.MovementName == "" {
		md.MovementName = firstNonEmpty(tags["movementname"], tags["movement_name"])
	}
	if md.MovementNo == 0 {
		md.MovementNo = parseLeadingInt(firstNonEmpty(tags["movement"], tags["movement_index"], tags["movementnumber"]))
	}
	if md.Lyrics == "" {
		md.Lyrics = firstNonEmpty(tags["lyrics"], tags["unsyncedlyrics"], tags["unsynced lyrics"])
	}
	if md.ReplayGainTrack == 0 {
		md.ReplayGainTrack = firstGainDB(tags["replaygain_track_gain"], tags["r128_track_gain"])
	}
	if md.ReplayGainAlbum == 0 {
		md.ReplayGainAlbum = firstGainDB(tags["replaygain_album_gain"], tags["r128_album_gain"])
	}
	if md.Participants == nil {
		md.Participants = extractParticipants(tags)
	}
	md.MBTrackID = firstNonEmpty(md.MBTrackID, tags["musicbrainz_trackid"], tags["musicbrainz_releasetrackid"])
	md.MBAlbumID = firstNonEmpty(md.MBAlbumID, tags["musicbrainz_albumid"])
	md.MBArtistID = firstNonEmpty(md.MBArtistID, tags["musicbrainz_artistid"])
}

func mergeTags(probe ffprobeOutput) map[string]string {
	out := make(map[string]string)
	for k, v := range probe.Format.Tags {
		out[strings.ToLower(k)] = v
	}
	for _, s := range probe.Streams {
		if s.CodecType != "audio" {
			continue
		}
		for k, v := range s.Tags {
			lk := strings.ToLower(k)
			if _, ok := out[lk]; !ok {
				out[lk] = v
			}
		}
	}
	return out
}

func parseYear(s string) int {
	s = strings.TrimSpace(s)
	if len(s) >= 4 {
		if y, err := strconv.Atoi(s[:4]); err == nil {
			return y
		}
	}
	return 0
}

// parseGainDB parses a ReplayGain value like "-6.50 dB" into a float of dB.
func parseGainDB(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "dB"))
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// firstGainDB returns the first usable gain from a standard ReplayGain tag (dB)
// or, failing that, an R128 gain tag. R128_*_GAIN (Opus/Vorbis) is an integer in
// Q7.8 fixed point referenced to -23 LUFS; ReplayGain references -18 LUFS, so the
// equivalent dB is value/256 + 5.
func firstGainDB(replayGain, r128 string) float64 {
	if g := parseGainDB(replayGain); g != 0 {
		return g
	}
	if r128 = strings.TrimSpace(r128); r128 != "" {
		if n, err := strconv.Atoi(r128); err == nil {
			return float64(n)/256.0 + 5.0
		}
	}
	return 0
}

// participantRoles maps known tag keys (as ffprobe lowercases them) to the role
// label exposed in the API. Main artist/album-artist are deliberately excluded —
// they have their own fields.
var participantRoles = map[string]string{
	"composer":  "composer",
	"performer": "performer",
	"producer":  "producer",
	"lyricist":  "lyricist",
	"writer":    "writer",
	"arranger":  "arranger",
	"conductor": "conductor",
	"engineer":  "engineer",
	"mixer":     "mixer",
	"remixer":   "remixer",
	"djmixer":   "dj-mixer",
	"director":  "director",
}

// extractParticipants pulls contributor roles from the merged tag map, splitting
// multi-value tags on the usual separators.
func extractParticipants(tags map[string]string) []models.Participant {
	var out []models.Participant
	for key, role := range participantRoles {
		for _, name := range splitNames(tags[key]) {
			out = append(out, models.Participant{Role: role, Name: name})
		}
	}
	return out
}

// splitNames splits a multi-artist tag value on "/", ";", ",", and "&".
func splitNames(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '/' || r == ';' || r == ',' || r == '&'
	})
	var out []string
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// readSidecarLyrics returns the contents of "<path-without-ext>.lrc" if present.
func readSidecarLyrics(path string) string {
	lrc := strings.TrimSuffix(path, filepath.Ext(path)) + ".lrc"
	data, err := os.ReadFile(lrc)
	if err != nil {
		return ""
	}
	return string(data)
}

func parseLeadingInt(s string) int {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
