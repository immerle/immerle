package stream

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

	ct := contentTypeForFormat(format)
	var live http.ResponseWriter
	if progressiveRequest(r) {
		live = w
	}
	cachePath, streamed, err := s.transcodedFile(r.Context(), track, format, bitrate, live, ct)
	if streamed || err != nil {
		return err
	}
	return s.serveFile(w, r, cachePath, ct)
}

// progressiveRequest reports whether a not-yet-cached transcode can be streamed
// to this request as ffmpeg produces it: a GET from the first byte. Any other
// range (a seek, or AVPlayer's bytes=0-1 probe) waits for the cached file.
func progressiveRequest(r *http.Request) bool {
	rg := r.Header.Get("Range")
	return r.Method == http.MethodGet && (rg == "" || rg == "bytes=0-")
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
// With live set, the caller that runs ffmpeg also streams its output to live as
// it is produced (streamed=true), so the first byte doesn't wait for the whole
// transcode; the other callers wait for the cached file as before.
func (s *Streamer) transcodedFile(ctx context.Context, track models.Track, format string, bitrate int, live http.ResponseWriter, contentType string) (cachePath string, streamed bool, err error) {
	key := fmt.Sprintf("%s_%s_%d", track.ID, format, bitrate)
	cachePath = filepath.Join(s.cacheDir, key+"."+format)

	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, false, nil
	}

	_, err, _ = s.group.Do(key, func() (any, error) {
		if _, err := os.Stat(cachePath); err == nil {
			return nil, nil
		}
		if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
			return nil, err
		}
		// Detach from the first caller: a coalesced transcode must survive that
		// client disconnecting. Keep a deadline so an orphaned ffmpeg still dies.
		// ponytail: fixed 10m ceiling; make configurable if long transcodes appear.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Minute)
		defer cancel()
		if live == nil {
			return nil, s.runFFmpeg(fctx, track.Path, cachePath, format, bitrate, nil)
		}
		streamed = true
		live.Header().Set("Content-Type", contentType)
		live.Header().Set("Accept-Ranges", "none")
		live.WriteHeader(http.StatusOK)
		return nil, s.runFFmpeg(fctx, track.Path, cachePath, format, bitrate, &flushWriter{w: live})
	})
	if err != nil {
		return "", streamed, err
	}
	return cachePath, streamed, nil
}

// flushWriter pushes each chunk to the client right away and swallows write
// errors (client gone), so ffmpeg keeps filling the cache regardless.
type flushWriter struct {
	w   http.ResponseWriter
	err error
}

func (f *flushWriter) Write(p []byte) (int, error) {
	if f.err == nil {
		if _, f.err = f.w.Write(p); f.err == nil {
			f.err = http.NewResponseController(f.w).Flush()
		}
	}
	return len(p), nil
}

// runFFmpeg transcodes src to a temp file then atomically renames it into place,
// so partial outputs are never served from the cache. With live set, ffmpeg
// writes to a pipe teed to both the temp file and live instead. The command is
// bound to ctx so a cancelled/closed request terminates ffmpeg (no leaked processes).
func (s *Streamer) runFFmpeg(ctx context.Context, src, dst, format string, bitrate int, live io.Writer) error {
	tmp := dst + ".tmp"
	defer os.Remove(tmp)

	var out bytes.Buffer
	var cmd *exec.Cmd
	var pipeFile *os.File
	if live == nil {
		cmd = exec.CommandContext(ctx, s.ffmpegPath, s.ffmpegArgs(src, tmp, format, bitrate)...)
		cmd.Stdout = &out
	} else {
		f, err := os.Create(tmp)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		pipeFile = f
		cmd = exec.CommandContext(ctx, s.ffmpegPath, s.ffmpegArgs(src, "pipe:1", format, bitrate)...)
		cmd.Stdout = io.MultiWriter(f, live)
	}
	cmd.Stderr = &out
	// Ensure a hard kill on context cancellation.
	cmd.Cancel = func() error { return cmd.Process.Kill() }
	cmd.WaitDelay = 5 * time.Second

	start := time.Now()
	if err := cmd.Run(); err != nil {
		out := out.Bytes()
		s.logger.Warn("ffmpeg transcode failed", "src", src, "format", format, "error", err, "output", string(out[:min(len(out), 500)]))
		return fmt.Errorf("transcode: %w", err)
	}
	s.logger.Debug("transcoded", "src", src, "format", format, "bitrate", bitrate, "elapsed", time.Since(start))

	if pipeFile != nil {
		if err := pipeFile.Close(); err != nil {
			return err
		}
	}
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
			if !slices.Contains(fields, "-f") {
				// A pipe output has no extension to infer the muxer from.
				fields = append(fields, "-f", muxerFor(format))
			}
			return append(append(base, fields...), dst)
		}
		s.logger.Warn("ignoring ffmpeg profile args: disallowed flag", "format", format, "args", custom)
		// fall through to the built-in defaults below
	}

	codec := "libmp3lame"
	switch format {
	case "opus":
		codec = "libopus"
	case "ogg", "vorbis":
		codec = "libvorbis"
	case "aac", "m4a":
		codec = "aac"
	}
	return append(base, "-c:a", codec, "-b:a", fmt.Sprintf("%dk", bitrate), "-f", muxerFor(format), dst)
}

func muxerFor(format string) string {
	switch format {
	case "opus":
		return "opus"
	case "ogg", "vorbis":
		return "ogg"
	case "aac", "m4a":
		return "adts"
	default:
		return "mp3"
	}
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
