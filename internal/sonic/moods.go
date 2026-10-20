package sonic

import (
	"cmp"
	"context"
	"math"
	"slices"

	"github.com/immerle/immerle/internal/persistence"
)

// Mood is one mood playlist.
type Mood string

// The moods, each published as its own playlist.
const (
	MoodCalm       Mood = "calm"
	MoodEnergetic  Mood = "energetic"
	MoodParty      Mood = "party"
	MoodFocus      Mood = "focus"
	MoodMelancholy Mood = "melancholy"
	MoodWorkout    Mood = "workout"
)

// Moods lists every mood, in display order.
var Moods = []Mood{MoodCalm, MoodFocus, MoodMelancholy, MoodEnergetic, MoodParty, MoodWorkout}

// moodMaxPerArtist keeps one artist from filling a mood playlist.
const moodMaxPerArtist = 3

type moodRule struct {
	match func(persistence.SonicSummary) bool
	score func(persistence.SonicSummary) float64 // best first
}

// PickMoods sorts the analyzed library into mood playlists of up to size
// tracks. Energy and danceability thresholds are percentiles of the library
// itself rather than absolute values: "calm" in a metal library isn't "calm"
// in a jazz one. BPM is only used as a coarse band, since a tempo can read as
// its half or double.
func PickMoods(tracks []persistence.SonicSummary, size int) map[Mood][]string {
	energy := percentiles(tracks, func(t persistence.SonicSummary) float64 { return t.Energy })
	dance := percentiles(tracks, func(t persistence.SonicSummary) float64 { return t.Danceability })
	minor := func(t persistence.SonicSummary) bool { return t.Mode == "minor" }

	rules := map[Mood]moodRule{
		MoodCalm: {
			func(t persistence.SonicSummary) bool { return t.Energy <= energy(0.3) && t.Danceability <= dance(0.6) },
			func(t persistence.SonicSummary) float64 { return -t.Energy },
		},
		MoodFocus: {
			func(t persistence.SonicSummary) bool {
				return t.Energy >= energy(0.3) && t.Energy <= energy(0.6) && t.Danceability <= dance(0.5)
			},
			func(t persistence.SonicSummary) float64 { return -math.Abs(t.Energy - energy(0.45)) },
		},
		MoodMelancholy: {
			func(t persistence.SonicSummary) bool { return minor(t) && t.Energy <= energy(0.5) },
			func(t persistence.SonicSummary) float64 { return -t.Energy },
		},
		MoodEnergetic: {
			func(t persistence.SonicSummary) bool { return t.Energy >= energy(0.7) && t.BPM >= 115 },
			func(t persistence.SonicSummary) float64 { return t.Energy },
		},
		MoodParty: {
			func(t persistence.SonicSummary) bool {
				return t.Danceability >= dance(0.7) && t.Energy >= energy(0.5) && t.BPM >= 100 && t.BPM <= 135
			},
			func(t persistence.SonicSummary) float64 { return t.Danceability },
		},
		MoodWorkout: {
			func(t persistence.SonicSummary) bool { return t.BPM >= 120 && t.BPM <= 160 && t.Energy >= energy(0.6) },
			func(t persistence.SonicSummary) float64 { return t.Energy },
		},
	}

	out := make(map[Mood][]string, len(Moods))
	for _, m := range Moods {
		r := rules[m]
		var cands []persistence.SonicSummary
		for _, t := range tracks {
			if r.match(t) {
				cands = append(cands, t)
			}
		}
		slices.SortFunc(cands, func(a, b persistence.SonicSummary) int {
			return cmp.Or(cmp.Compare(r.score(b), r.score(a)), cmp.Compare(a.TrackID, b.TrackID))
		})
		perArtist := map[string]int{}
		var ids []string
		for _, t := range cands {
			if len(ids) == size {
				break
			}
			if perArtist[t.ArtistID] < moodMaxPerArtist {
				perArtist[t.ArtistID]++
				ids = append(ids, t.TrackID)
			}
		}
		out[m] = ids
	}
	return out
}

// percentiles returns a lookup of the q-th percentile (0..1) of a feature.
func percentiles(tracks []persistence.SonicSummary, feature func(persistence.SonicSummary) float64) func(q float64) float64 {
	vals := make([]float64, len(tracks))
	for i, t := range tracks {
		vals[i] = feature(t)
	}
	slices.Sort(vals)
	return func(q float64) float64 {
		if len(vals) == 0 {
			return 0
		}
		return vals[int(q*float64(len(vals)-1))]
	}
}

// MoodsEnabled reports whether mood playlists are switched on.
func (s *Service) MoodsEnabled() bool { return s.Enabled() && s.config().Moods }

// MoodPlaylists picks the tracks of every mood playlist from the analyzed library.
func (s *Service) MoodPlaylists(ctx context.Context, size int) (map[Mood][]string, error) {
	tracks, err := s.repo.Summaries(ctx, AlgoVersion)
	if err != nil {
		return nil, err
	}
	return PickMoods(tracks, size), nil
}
