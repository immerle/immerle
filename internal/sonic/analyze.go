// Package sonic computes a track's audio features (tempo, key, energy, timbre)
// from its decoded samples, in pure Go. They power the sonic features (track
// radio, mood playlists, Auto-DJ) without any external service.
package sonic

import (
	"errors"
	"math"
	"math/cmplx"
	"strconv"
	"sync"
)

const (
	// SampleRate is the mono sample rate the analysis expects (see Decode).
	SampleRate = 22050
	// AlgoVersion is bumped whenever the features change, so the worker
	// re-analyzes the whole library with the new algorithm.
	AlgoVersion = 1

	frameSize = 2048
	hopSize   = 256 // ~11.6ms: fine enough for a ±1 BPM tempo estimate
	numMel    = 26
	numMFCC   = 13
	silenceDB = -60.0 // frames quieter than this are ignored for spectral stats
	minBPM    = 60.0
	maxBPM    = 200.0
)

// Features is the analysis result for one track.
type Features struct {
	BPM          float64
	Key          string // pitch class of the tonic: "C", "C#", ... "B"
	Mode         string // "major" | "minor"
	Camelot      string // DJ wheel notation, e.g. "8B" (C major), "8A" (A minor)
	Energy       float64
	Loudness     float64 // mean frame RMS, dBFS
	Danceability float64 // pulse clarity, 0..1
	Brightness   float64 // spectral centroid / Nyquist, 0..1
	// Vector is the raw (not normalized) similarity vector: scalar features
	// then the MFCC means and standard deviations. Normalize it against the
	// library before comparing tracks.
	Vector []float32
}

var (
	errTooShort = errors.New("not enough audio to analyze")
	errSilent   = errors.New("audio is silent")
)

var pitchNames = [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// Krumhansl-Kessler key profiles, indexed from the tonic.
var (
	majorProfile = [12]float64{6.35, 2.23, 3.48, 2.33, 4.38, 4.09, 2.52, 5.19, 2.39, 3.66, 2.29, 2.88}
	minorProfile = [12]float64{6.33, 2.68, 3.52, 5.38, 2.60, 3.53, 2.54, 4.75, 3.98, 2.69, 3.34, 3.17}
)

// Analyze extracts the features of mono samples at SampleRate.
func Analyze(samples []float32) (Features, error) {
	if len(samples) < SampleRate*5 {
		return Features{}, errTooShort
	}
	tb := getTables()
	nBins := frameSize/2 + 1
	nFrames := 1 + (len(samples)-frameSize)/hopSize

	buf := make([]complex128, frameSize)
	mag := make([]float64, nBins)
	prevLog := make([]float64, nBins)
	melE := make([]float64, numMel)
	onset := make([]float64, nFrames)
	var (
		chroma                              [12]float64
		mfccSum, mfccSq                     [numMFCC]float64
		centroidSum, rolloffSum, dbSum, db2 float64
		voiced                              int
	)

	for f := range nFrames {
		off := f * hopSize
		var power float64
		for i := range frameSize {
			s := float64(samples[off+i])
			power += s * s
			buf[i] = complex(s*tb.window[i], 0)
		}
		fft(buf)
		var flux, total, weighted float64
		for k := range nBins {
			m := cmplx.Abs(buf[k])
			mag[k] = m
			l := math.Log1p(m)
			if d := l - prevLog[k]; d > 0 {
				flux += d
			}
			prevLog[k] = l
			total += m
			weighted += m * float64(k)
		}
		onset[f] = flux

		db := 10 * math.Log10(power/frameSize+1e-18)
		if db < silenceDB || total == 0 {
			continue
		}
		voiced++
		dbSum += db
		db2 += db * db
		centroidSum += weighted / total / float64(nBins-1)
		target, acc := 0.85*total, 0.0
		for k := range nBins {
			if acc += mag[k]; acc >= target {
				rolloffSum += float64(k) / float64(nBins-1)
				break
			}
		}
		for k := tb.chromaLo; k <= tb.chromaHi; k++ {
			chroma[tb.pitchClass[k]] += mag[k] * mag[k]
		}
		for b, filt := range tb.mel {
			var e float64
			for _, w := range filt {
				e += w.w * mag[w.k] * mag[w.k]
			}
			melE[b] = math.Log(e + 1e-10)
		}
		for c := range numMFCC {
			var v float64
			for b, e := range melE {
				v += e * tb.dct[c][b]
			}
			mfccSum[c] += v
			mfccSq[c] += v * v
		}
	}
	if voiced == 0 {
		return Features{}, errSilent
	}

	n := float64(voiced)
	loudness := dbSum / n
	dynamics := math.Sqrt(math.Max(0, db2/n-loudness*loudness))
	brightness := centroidSum / n
	rolloff := rolloffSum / n
	bpm, clarity := tempo(onset)
	tonic, minor := detectKey(chroma)

	// ponytail: heuristic energy (loudness, nudged by brightness); calibrate
	// against real libraries if mood playlists feel off.
	energy := clamp01(0.75*clamp01((loudness+30)/24) + 0.25*clamp01(brightness*4))

	mode := "major"
	if minor {
		mode = "minor"
	}
	vec := []float32{
		float32(bpm / maxBPM), float32(energy), float32(loudness), float32(dynamics),
		float32(brightness), float32(rolloff), float32(clarity), float32(boolTo01(minor)),
	}
	for c := range numMFCC {
		vec = append(vec, float32(mfccSum[c]/n))
	}
	for c := range numMFCC {
		mean := mfccSum[c] / n
		vec = append(vec, float32(math.Sqrt(math.Max(0, mfccSq[c]/n-mean*mean))))
	}

	return Features{
		BPM:          math.Round(bpm*10) / 10,
		Key:          pitchNames[tonic],
		Mode:         mode,
		Camelot:      camelot(tonic, minor),
		Energy:       energy,
		Loudness:     loudness,
		Danceability: clarity,
		Brightness:   brightness,
		Vector:       vec,
	}, nil
}

// tempo estimates the BPM from the onset-strength envelope by autocorrelation,
// weighted toward ~120 BPM to settle the half/double tempo ambiguity. clarity
// is the normalized autocorrelation at the chosen period (0..1).
func tempo(onset []float64) (bpm, clarity float64) {
	fps := float64(SampleRate) / hopSize
	env := detrend(onset, int(fps))
	var acf0 float64
	for _, v := range env {
		acf0 += v * v
	}
	minLag, maxLag := int(60*fps/maxBPM), int(math.Ceil(60*fps/minBPM))
	if acf0 == 0 || len(env) <= maxLag+1 {
		return 0, 0
	}
	acf := make([]float64, maxLag+2)
	for lag := minLag - 1; lag <= maxLag+1; lag++ {
		var s float64
		for i := lag; i < len(env); i++ {
			s += env[i] * env[i-lag]
		}
		acf[lag] = s / acf0
	}
	best, bestScore := 0, math.Inf(-1)
	for lag := minLag; lag <= maxLag; lag++ {
		octaves := math.Log2(60 * fps / float64(lag) / 120)
		if score := acf[lag] * math.Exp(-0.5*octaves*octaves/0.81); score > bestScore {
			best, bestScore = lag, score
		}
	}
	// Parabolic interpolation around the peak for sub-frame precision.
	period := float64(best)
	if y0, y1, y2 := acf[best-1], acf[best], acf[best+1]; y0-2*y1+y2 != 0 {
		if d := 0.5 * (y0 - y2) / (y0 - 2*y1 + y2); math.Abs(d) < 1 {
			period += d
		}
	}
	return 60 * fps / period, clamp01(acf[best])
}

// detrend subtracts a moving average of width w and keeps the positive part,
// so the autocorrelation sees onsets rather than loudness swells.
func detrend(x []float64, w int) []float64 {
	prefix := make([]float64, len(x)+1)
	for i, v := range x {
		prefix[i+1] = prefix[i] + v
	}
	out := make([]float64, len(x))
	for i, v := range x {
		lo, hi := max(0, i-w/2), min(len(x), i+w/2+1)
		if d := v - (prefix[hi]-prefix[lo])/float64(hi-lo); d > 0 {
			out[i] = d
		}
	}
	return out
}

// detectKey correlates the chroma against the 24 rotated key profiles.
func detectKey(chroma [12]float64) (tonic int, minor bool) {
	best := math.Inf(-1)
	for pc := range 12 {
		for _, isMinor := range []bool{false, true} {
			profile := majorProfile
			if isMinor {
				profile = minorProfile
			}
			var rotated [12]float64
			for i := range 12 {
				rotated[i] = profile[(i-pc+12)%12]
			}
			if r := pearson(rotated, chroma); r > best {
				best, tonic, minor = r, pc, isMinor
			}
		}
	}
	return tonic, minor
}

// camelot maps a key to the Camelot wheel: C major is 8B, each fifth up adds
// one, and a minor key shares the number of its relative major.
func camelot(tonic int, minor bool) string {
	letter := "B"
	if minor {
		tonic = (tonic + 3) % 12
		letter = "A"
	}
	return strconv.Itoa((7*tonic+7)%12+1) + letter
}

func pearson(a, b [12]float64) float64 {
	var ma, mb float64
	for i := range 12 {
		ma += a[i]
		mb += b[i]
	}
	ma, mb = ma/12, mb/12
	var num, da, db float64
	for i := range 12 {
		x, y := a[i]-ma, b[i]-mb
		num += x * y
		da += x * x
		db += y * y
	}
	if da == 0 || db == 0 {
		return 0
	}
	return num / math.Sqrt(da*db)
}

type melWeight struct {
	k int
	w float64
}

// tables holds the per-frame constants, computed once.
type tables struct {
	window             []float64
	mel                [][]melWeight
	dct                [numMFCC][numMel]float64
	pitchClass         []int
	chromaLo, chromaHi int
}

var getTables = sync.OnceValue(func() *tables {
	nBins := frameSize/2 + 1
	binHz := float64(SampleRate) / frameSize
	t := &tables{window: make([]float64, frameSize), pitchClass: make([]int, nBins)}
	for i := range frameSize {
		t.window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/frameSize)
	}

	// Chroma: below ~130Hz a bin spans more than a semitone, above 2kHz it's
	// mostly harmonics and noise.
	t.chromaLo, t.chromaHi = int(math.Ceil(130/binHz)), int(2000/binHz)
	for k := t.chromaLo; k <= t.chromaHi; k++ {
		midi := 12*math.Log2(float64(k)*binHz/440) + 69
		t.pitchClass[k] = (int(math.Round(midi))%12 + 12) % 12
	}

	// Triangular mel filters between 20Hz and 8kHz.
	toMel := func(hz float64) float64 { return 2595 * math.Log10(1+hz/700) }
	fromMel := func(m float64) float64 { return 700 * (math.Pow(10, m/2595) - 1) }
	lo, hi := toMel(20), toMel(8000)
	edges := make([]float64, numMel+2)
	for i := range edges {
		edges[i] = fromMel(lo+(hi-lo)*float64(i)/float64(numMel+1)) / binHz
	}
	t.mel = make([][]melWeight, numMel)
	for b := range numMel {
		left, center, right := edges[b], edges[b+1], edges[b+2]
		for k := int(math.Ceil(left)); float64(k) < right && k < nBins; k++ {
			w := (float64(k) - left) / (center - left)
			if float64(k) > center {
				w = (right - float64(k)) / (right - center)
			}
			if w > 0 {
				t.mel[b] = append(t.mel[b], melWeight{k, w})
			}
		}
	}
	for c := range numMFCC {
		for b := range numMel {
			t.dct[c][b] = math.Cos(math.Pi * float64(c) * (float64(b) + 0.5) / numMel)
		}
	}
	return t
})

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func boolTo01(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
