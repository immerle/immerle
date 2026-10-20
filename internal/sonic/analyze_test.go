package sonic

import (
	"context"
	"math"
	"math/cmplx"
	"math/rand/v2"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFFTMatchesNaiveDFT(t *testing.T) {
	const n = 64
	x := make([]complex128, n)
	for i := range x {
		x[i] = complex(rand.Float64(), 0)
	}
	want := make([]complex128, n)
	for k := range n {
		for i := range n {
			want[k] += x[i] * cmplx.Exp(complex(0, -2*math.Pi*float64(k*i)/n))
		}
	}
	fft(x)
	for k := range n {
		if cmplx.Abs(x[k]-want[k]) > 1e-9 {
			t.Fatalf("bin %d: got %v, want %v", k, x[k], want[k])
		}
	}
}

// clickTrack is a short noise burst every beat, plus a low tone so frames
// aren't silent.
func clickTrack(bpm float64, seconds int) []float32 {
	out := make([]float32, SampleRate*seconds)
	period := int(60 / bpm * SampleRate)
	for i := range out {
		out[i] = float32(0.05 * math.Sin(2*math.Pi*220*float64(i)/SampleRate))
		if i%period < 200 {
			out[i] += float32(rand.Float64()*1.6 - 0.8)
		}
	}
	return out
}

func TestAnalyzeTempo(t *testing.T) {
	for _, bpm := range []float64{90, 120, 128, 150} {
		f, err := Analyze(clickTrack(bpm, 30))
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(f.BPM-bpm) > 1.5 {
			t.Errorf("click track at %v BPM: got %v", bpm, f.BPM)
		}
		if f.Danceability < 0.5 {
			t.Errorf("a metronome should have a clear pulse, got %v", f.Danceability)
		}
	}
}

// chord sums harmonic-rich tones for the given MIDI notes.
func chord(seconds int, notes ...int) []float32 {
	out := make([]float32, SampleRate*seconds)
	for _, n := range notes {
		hz := 440 * math.Pow(2, float64(n-69)/12)
		for h := 1.0; h <= 4; h++ {
			for i := range out {
				out[i] += float32(0.1 / h * math.Sin(2*math.Pi*hz*h*float64(i)/SampleRate))
			}
		}
	}
	return out
}

func TestAnalyzeKey(t *testing.T) {
	cases := []struct {
		name    string
		notes   []int
		key     string
		mode    string
		camelot string
	}{
		{"C major", []int{60, 64, 67, 72}, "C", "major", "8B"},
		{"A minor", []int{57, 60, 64, 69}, "A", "minor", "8A"},
		{"G major", []int{55, 59, 62, 67}, "G", "major", "9B"},
	}
	for _, c := range cases {
		f, err := Analyze(chord(10, c.notes...))
		if err != nil {
			t.Fatal(err)
		}
		if f.Key != c.key || f.Mode != c.mode || f.Camelot != c.camelot {
			t.Errorf("%s: got %s %s (%s)", c.name, f.Key, f.Mode, f.Camelot)
		}
	}
}

func TestAnalyzeRejectsSilenceAndShortAudio(t *testing.T) {
	if _, err := Analyze(make([]float32, SampleRate*10)); err != errSilent {
		t.Errorf("silence: got %v", err)
	}
	if _, err := Analyze(make([]float32, SampleRate)); err != errTooShort {
		t.Errorf("short audio: got %v", err)
	}
}

func TestWindowsStayInsideTheTrack(t *testing.T) {
	if w := windows(0); len(w) != 1 || w[0].start != 0 {
		t.Errorf("unknown duration: %+v", w)
	}
	for _, d := range []int{91, 200, 3600} {
		for _, w := range windows(d) {
			if w.start < 0 || w.start+w.length > d {
				t.Errorf("duration %d: window %+v out of bounds", d, w)
			}
		}
	}
}

func TestDecodeAndAnalyzeFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	path := filepath.Join(t.TempDir(), "tone.flac")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=12", path).CombinedOutput(); err != nil {
		t.Fatalf("generate fixture: %v: %s", err, out)
	}
	samples, err := Decode(context.Background(), "ffmpeg", path, 12)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(samples); got < SampleRate*11 || got > SampleRate*13 {
		t.Fatalf("decoded %d samples, want ~%d", got, SampleRate*12)
	}
	f, err := Analyze(samples)
	if err != nil {
		t.Fatal(err)
	}
	if f.Key != "A" {
		t.Errorf("a 440Hz tone should read as A, got %s %s", f.Key, f.Mode)
	}
	if len(f.Vector) != 8+2*numMFCC {
		t.Errorf("vector has %d dims", len(f.Vector))
	}
}
