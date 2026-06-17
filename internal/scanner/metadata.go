package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/dhowden/tag"
)

// Metadata is the normalized set of fields extracted from an audio file.
type Metadata struct {
	Title       string
	Album       string
	Artist      string
	AlbumArtist string
	Genre       string
	Year        int
	TrackNo     int
	DiscNo      int
	Duration    int // seconds
	BitRate     int // kbps
	MBTrackID   string
	MBAlbumID   string
	MBArtistID  string
	Compilation bool
	HasPicture  bool
	Picture     []byte
	PictureMIME string
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
	ext := strings.ToLower(extOf(path))
	ct, ok := audioExtensions[ext]
	return ct, ok
}

func extOf(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[i:]
	}
	return ""
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
		md.Genre = m.Genre()
		md.Year = m.Year()
		md.TrackNo, _ = m.Track()
		md.DiscNo, _ = m.Disc()
		if pic := m.Picture(); pic != nil {
			md.HasPicture = true
			md.Picture = pic.Data
			md.PictureMIME = pic.MIMEType
		}
		extractMBIDs(m.Raw(), &md)
	}

	// Always consult ffprobe for technical fields (and as a fallback for tags).
	e.augmentWithFFprobe(ctx, path, &md)

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
	name := path
	if i := strings.LastIndexAny(name, "/\\"); i >= 0 {
		name = name[i+1:]
	}
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
		BitRate   string            `json:"bit_rate"`
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
	tags := mergeTags(probe.Format.Tags, probe.Streams)
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
	md.MBTrackID = firstNonEmpty(md.MBTrackID, tags["musicbrainz_trackid"], tags["musicbrainz_releasetrackid"])
	md.MBAlbumID = firstNonEmpty(md.MBAlbumID, tags["musicbrainz_albumid"])
	md.MBArtistID = firstNonEmpty(md.MBArtistID, tags["musicbrainz_artistid"])
}

func mergeTags(format map[string]string, streams []struct {
	CodecType string            `json:"codec_type"`
	BitRate   string            `json:"bit_rate"`
	Tags      map[string]string `json:"tags"`
}) map[string]string {
	out := make(map[string]string)
	for k, v := range format {
		out[strings.ToLower(k)] = v
	}
	for _, s := range streams {
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

func parseLeadingInt(s string) int {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
