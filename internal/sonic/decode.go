package sonic

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

// windowSec is the length of each excerpt analyzed. Three excerpts (intro,
// middle, outro) are enough to characterize a track at a fraction of the cost
// of decoding all of it.
const windowSec = 30

type window struct{ start, length int }

// windows picks the excerpts to decode for a track of the given duration
// (seconds; 0 when unknown).
func windows(duration int) []window {
	if duration <= 3*windowSec {
		return []window{{0, 3 * windowSec}}
	}
	out := make([]window, 0, 3)
	for _, at := range []float64{0.2, 0.5, 0.8} {
		start := int(at*float64(duration)) - windowSec/2
		out = append(out, window{max(0, min(start, duration-windowSec)), windowSec})
	}
	return out
}

// Decode returns mono float32 samples at SampleRate for the excerpts of the
// file at path, decoded by ffmpeg.
func Decode(ctx context.Context, ffmpeg, path string, duration int) ([]float32, error) {
	var out []float32
	for _, w := range windows(duration) {
		cmd := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-v", "error",
			"-ss", strconv.Itoa(w.start), "-t", strconv.Itoa(w.length), "-i", path,
			"-vn", "-ac", "1", "-ar", strconv.Itoa(SampleRate), "-f", "f32le", "-")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		raw, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		for i := 0; i+4 <= len(raw); i += 4 {
			out = append(out, math.Float32frombits(binary.LittleEndian.Uint32(raw[i:])))
		}
	}
	return out, nil
}
