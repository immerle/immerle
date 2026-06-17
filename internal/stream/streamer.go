package stream

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/models"
)

// Streamer serves audio files, transcoding on demand via ffmpeg. Transcoded
// outputs are cached on disk so repeated requests (and Range/seek requests) are
// served from a complete file.
type Streamer struct {
	ffmpegPath string
	cacheDir   string
	profiles   map[string]config.TranscodeProfile
	logger     *slog.Logger
	group      singleflight.Group
}

// NewStreamer builds a Streamer from transcode config.
func NewStreamer(cfg config.TranscodeConfig, logger *slog.Logger) *Streamer {
	profiles := make(map[string]config.TranscodeProfile)
	for _, p := range cfg.Profiles {
		profiles[strings.ToLower(p.Name)] = p
		profiles[strings.ToLower(p.Format)] = p
	}
	ffmpeg := cfg.FFmpegPath
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	return &Streamer{
		ffmpegPath: ffmpeg,
		cacheDir:   cfg.CacheDir,
		profiles:   profiles,
		logger:     logger,
	}
}

// Options control transcoding decisions for a stream request.
type Options struct {
	// MaxBitRate in kbps; 0 means no limit.
	MaxBitRate int
	// Format is the requested target format ("" = original, "raw" = no transcode).
	Format string
}

// Serve writes the track to w, honoring Range requests and transcoding per opts.
func (s *Streamer) Serve(w http.ResponseWriter, r *http.Request, track models.Track, opts Options) error {
	transcode, format, bitrate := s.decide(track, opts)
	if !transcode {
		return s.serveFile(w, r, track.Path, track.ContentType)
	}

	cachePath, err := s.transcodedFile(r.Context(), track, format, bitrate)
	if err != nil {
		return err
	}
	ct := contentTypeForFormat(format)
	return s.serveFile(w, r, cachePath, ct)
}

// serveFile serves a file with full Range/seek support via http.ServeContent.
func (s *Streamer) serveFile(w http.ResponseWriter, r *http.Request, path, contentType string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
	return nil
}

// decide determines whether to transcode and to what format/bitrate.
func (s *Streamer) decide(track models.Track, opts Options) (transcode bool, format string, bitrate int) {
	if strings.EqualFold(opts.Format, "raw") {
		return false, "", 0
	}
	targetFormat := strings.ToLower(opts.Format)
	needFormat := targetFormat != "" && targetFormat != strings.ToLower(track.Suffix)
	needBitrate := opts.MaxBitRate > 0 && (track.BitRate == 0 || track.BitRate > opts.MaxBitRate)

	if !needFormat && !needBitrate {
		return false, "", 0
	}

	format = targetFormat
	if format == "" {
		// A bitrate cap without an explicit format defaults to mp3.
		format = "mp3"
	}
	bitrate = opts.MaxBitRate
	if bitrate == 0 {
		if p, ok := s.profiles[format]; ok && p.BitRate > 0 {
			bitrate = p.BitRate
		} else {
			bitrate = 192
		}
	}
	return true, format, bitrate
}

// transcodedFile returns the path to a cached transcode, producing it if needed.
// Concurrent identical requests share a single ffmpeg run via singleflight.
func (s *Streamer) transcodedFile(ctx context.Context, track models.Track, format string, bitrate int) (string, error) {
	key := fmt.Sprintf("%s_%s_%d", track.ID, format, bitrate)
	cachePath := filepath.Join(s.cacheDir, key+"."+format)

	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}

	_, err, _ := s.group.Do(key, func() (any, error) {
		if _, err := os.Stat(cachePath); err == nil {
			return nil, nil
		}
		if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
			return nil, err
		}
		return nil, s.runFFmpeg(ctx, track.Path, cachePath, format, bitrate)
	})
	if err != nil {
		return "", err
	}
	return cachePath, nil
}

// runFFmpeg transcodes src to a temp file then atomically renames it into place,
// so partial outputs are never served. The command is bound to ctx so a
// cancelled/closed request terminates ffmpeg (no leaked processes).
func (s *Streamer) runFFmpeg(ctx context.Context, src, dst, format string, bitrate int) error {
	tmp := dst + ".tmp"
	defer os.Remove(tmp)

	args := s.ffmpegArgs(src, tmp, format, bitrate)
	cmd := exec.CommandContext(ctx, s.ffmpegPath, args...)
	// Ensure a hard kill on context cancellation.
	cmd.Cancel = func() error { return cmd.Process.Kill() }
	cmd.WaitDelay = 5 * time.Second

	start := time.Now()
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.logger.Warn("ffmpeg transcode failed", "src", src, "format", format, "error", err, "output", truncate(string(out), 500))
		return fmt.Errorf("transcode: %w", err)
	}
	s.logger.Debug("transcoded", "src", src, "format", format, "bitrate", bitrate, "elapsed", time.Since(start))

	return os.Rename(tmp, dst)
}

// ffmpegArgs builds the ffmpeg argument list for a profile.
func (s *Streamer) ffmpegArgs(src, dst, format string, bitrate int) []string {
	base := []string{"-v", "error", "-nostdin", "-i", src, "-map", "0:a", "-vn"}

	if p, ok := s.profiles[format]; ok && p.FFmpegArgs != "" {
		// Custom args from config; %b is replaced with the bitrate in kbps.
		custom := strings.ReplaceAll(p.FFmpegArgs, "%b", fmt.Sprintf("%d", bitrate))
		// Whitelist the flags so a stray/hostile config value can't inject an
		// extra input/output and turn the transcode into an arbitrary file
		// read/write (it's argv, not a shell, but ffmpeg flags are still powerful).
		if fields, ok := safeFFmpegArgs(custom); ok {
			return append(append(base, fields...), dst)
		}
		s.logger.Warn("ignoring ffmpeg profile args: disallowed flag", "format", format, "args", custom)
		// fall through to the built-in defaults below
	}

	switch format {
	case "opus":
		base = append(base, "-c:a", "libopus", "-b:a", fmt.Sprintf("%dk", bitrate), "-f", "opus")
	case "ogg", "vorbis":
		base = append(base, "-c:a", "libvorbis", "-b:a", fmt.Sprintf("%dk", bitrate), "-f", "ogg")
	case "aac", "m4a":
		base = append(base, "-c:a", "aac", "-b:a", fmt.Sprintf("%dk", bitrate), "-f", "adts")
	default: // mp3
		base = append(base, "-c:a", "libmp3lame", "-b:a", fmt.Sprintf("%dk", bitrate), "-f", "mp3")
	}
	return append(base, dst)
}

// allowedFFmpegFlags is the set of ffmpeg flags an admin transcode profile may
// use. Anything outside it (e.g. -i, -y, a second output) is rejected so the
// profile can't redirect ffmpeg's I/O.
var allowedFFmpegFlags = map[string]bool{
	"-c:a": true, "-codec:a": true, "-acodec": true,
	"-b:a": true, "-q:a": true, "-vbr": true, "-profile:a": true,
	"-ar": true, "-ac": true, "-af": true, "-filter:a": true,
	"-compression_level": true, "-application": true, "-cutoff": true,
	"-frame_duration": true, "-f": true, "-movflags": true,
}

// safeFFmpegArgs splits a profile's custom args and accepts them only if every
// flag token (one starting with "-") is in the whitelist. Value tokens that
// follow a flag are allowed as-is.
func safeFFmpegArgs(custom string) ([]string, bool) {
	fields := strings.Fields(custom)
	for _, f := range fields {
		if strings.HasPrefix(f, "-") && !allowedFFmpegFlags[f] {
			return nil, false
		}
	}
	return fields, true
}

func contentTypeForFormat(format string) string {
	switch format {
	case "opus":
		return "audio/opus"
	case "ogg", "vorbis":
		return "audio/ogg"
	case "aac", "m4a":
		return "audio/aac"
	case "mp3":
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
