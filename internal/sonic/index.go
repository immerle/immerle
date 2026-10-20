package sonic

import (
	"context"
	"math"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/immerle/immerle/internal/persistence"
)

const (
	// rebuildEvery throttles reloading the index while the worker keeps adding
	// tracks: a query rebuilds it at most once per period.
	rebuildEvery = time.Minute
	// maxPerArtist caps how many tracks of one artist a radio returns, so it
	// doesn't collapse into the seed artist's discography.
	maxPerArtist = 2
	// jitter is the noise added to each cosine score so two radios from the
	// same seed differ among close matches, without letting distant tracks in.
	jitter = 0.03
)

// scalarWeights weighs the 8 scalar features (see Analyze) against the timbre
// ones once z-scored. ponytail: hand-picked, tune by ear.
var scalarWeights = [8]float64{
	1,    // bpm
	1.5,  // energy
	0.5,  // loudness
	0.75, // dynamics
	1,    // brightness
	0.5,  // rolloff
	1,    // pulse clarity
	0.5,  // mode
}

func dimWeight(d int) float64 {
	switch {
	case d < len(scalarWeights):
		return scalarWeights[d]
	case d < len(scalarWeights)+numMFCC:
		return 0.6 // MFCC means: overall timbre
	default:
		return 0.4 // MFCC deviations: timbre movement
	}
}

// index holds every analyzed track's vector in memory, normalized so cosine
// similarity is a dot product.
// ponytail: brute-force scan, ~5ms per 100k tracks; move to an ANN index (HNSW)
// if libraries grow past ~500k tracks.
type index struct {
	repo  *persistence.SonicRepo
	dirty atomic.Bool

	mu        sync.Mutex
	built     time.Time
	ids       []string
	artists   []string
	durations []int
	pos       map[string]int
	vecs      []float32 // len(ids) rows of dim values
	dim       int
}

func newIndex(repo *persistence.SonicRepo) *index { return &index{repo: repo} }

// markDirty flags new analysis results, picked up by the next rebuild.
func (x *index) markDirty() { x.dirty.Store(true) }

// ensure (re)loads the index when never built, or stale and due. Callers hold x.mu.
func (x *index) ensure(ctx context.Context) error {
	if !x.built.IsZero() && (!x.dirty.Load() || time.Since(x.built) < rebuildEvery) {
		return nil
	}
	x.dirty.Store(false)
	rows, err := x.repo.Vectors(ctx, AlgoVersion)
	if err != nil {
		x.dirty.Store(true)
		return err
	}
	x.load(rows)
	x.built = time.Now()
	return nil
}

// load z-scores each dimension over the library (so BPM and MFCCs weigh in on
// the same scale), applies the dimension weights and L2-normalizes each row.
func (x *index) load(rows []persistence.SonicVector) {
	x.ids, x.artists, x.durations, x.vecs, x.dim = nil, nil, nil, nil, 0
	x.pos = make(map[string]int, len(rows))
	if len(rows) == 0 {
		return
	}
	dim := len(rows[0].Vector)
	rows = slices.DeleteFunc(rows, func(r persistence.SonicVector) bool { return len(r.Vector) != dim })
	mean, std := make([]float64, dim), make([]float64, dim)
	for _, r := range rows {
		for d, v := range r.Vector {
			mean[d] += float64(v)
		}
	}
	for d := range mean {
		mean[d] /= float64(len(rows))
	}
	for _, r := range rows {
		for d, v := range r.Vector {
			diff := float64(v) - mean[d]
			std[d] += diff * diff
		}
	}
	for d := range std {
		if std[d] = math.Sqrt(std[d] / float64(len(rows))); std[d] == 0 {
			std[d] = 1
		}
	}

	x.dim = dim
	x.vecs = make([]float32, 0, len(rows)*dim)
	row := make([]float64, dim)
	for _, r := range rows {
		var norm float64
		for d, v := range r.Vector {
			row[d] = (float64(v) - mean[d]) / std[d] * dimWeight(d)
			norm += row[d] * row[d]
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			norm = 1
		}
		for _, v := range row {
			x.vecs = append(x.vecs, float32(v/norm))
		}
		x.pos[r.TrackID] = len(x.ids)
		x.ids = append(x.ids, r.TrackID)
		x.artists = append(x.artists, r.ArtistID)
		x.durations = append(x.durations, r.Duration)
	}
}

func (x *index) row(i int) []float32 { return x.vecs[i*x.dim : (i+1)*x.dim] }

type candidate struct {
	i     int
	score float32
}

func (x *index) similar(ctx context.Context, seeds, exclude []string, count int) ([]string, bool, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if err := x.ensure(ctx); err != nil {
		return nil, false, err
	}
	if count <= 0 {
		return nil, true, nil
	}

	skip := make(map[string]bool, len(seeds)+len(exclude))
	centroid := make([]float32, x.dim)
	found := 0
	for _, id := range seeds {
		skip[id] = true
		if i, ok := x.pos[id]; ok {
			for d, v := range x.row(i) {
				centroid[d] += v
			}
			found++
		}
	}
	if found == 0 {
		return nil, false, nil
	}
	for _, id := range exclude {
		skip[id] = true
	}

	cands := make([]candidate, 0, len(x.ids))
	for i, id := range x.ids {
		if skip[id] {
			continue
		}
		var dot float32
		for d, v := range x.row(i) {
			dot += v * centroid[d]
		}
		cands = append(cands, candidate{i, dot/float32(found) + float32(rand.NormFloat64()*jitter)})
	}
	slices.SortFunc(cands, func(a, b candidate) int { return cmpDesc(a.score, b.score) })

	// Keep the closest tracks under the per-artist cap. A small library may
	// not have enough artists: top up with the capped-out tracks.
	perArtist := map[string]int{}
	pool := make([]candidate, 0, count)
	var overflow []candidate
	for _, c := range cands {
		if len(pool) == count {
			break
		}
		if a := x.artists[c.i]; perArtist[a] < maxPerArtist {
			perArtist[a]++
			pool = append(pool, c)
		} else if len(overflow) < count {
			overflow = append(overflow, c)
		}
	}
	if len(pool) < count {
		pool = append(pool, overflow[:min(len(overflow), count-len(pool))]...)
		slices.SortFunc(pool, func(a, b candidate) int { return cmpDesc(a.score, b.score) })
	}

	out := make([]string, len(pool))
	for i, c := range pool {
		out[i] = x.ids[c.i]
	}
	return out, true, nil
}

func cmpDesc(a, b float32) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	}
	return 0
}
