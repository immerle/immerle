package sonic

import (
	"context"
	"math"
	"strconv"
	"strings"

	"github.com/immerle/immerle/internal/persistence"
)

// maxTwoOptPasses bounds the 2-opt refinement of an Auto-DJ order.
// ponytail: nearest neighbour + 2-opt, each pass O(n²); fine for playlists of
// a few hundred tracks, move to Or-opt/simulated annealing if users DJ
// thousands at once.
const maxTwoOptPasses = 4

// transitionCost scores how rough going from a to b sounds: tempo jump (a
// half or double tempo counts as matching, they mix fine), key clash on the
// Camelot wheel, and energy jump.
func transitionCost(a, b persistence.SonicSummary) float64 {
	return tempoCost(a.BPM, b.BPM) + keyCost(a.Camelot, b.Camelot) + 2*math.Abs(a.Energy-b.Energy)
}

func tempoCost(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return 1
	}
	best := math.Inf(1)
	for _, m := range []float64{0.5, 1, 2} {
		best = math.Min(best, math.Abs(a-b*m)/a)
	}
	return math.Min(1, best*10) // 5% apart costs 0.5, 10% and more costs 1
}

// keyCost follows the Camelot wheel: same key, one step around it, or the
// relative major/minor mix harmonically (0); two steps is a stretch (0.5);
// anything else clashes (1).
func keyCost(a, b string) float64 {
	na, la, okA := parseCamelot(a)
	nb, lb, okB := parseCamelot(b)
	if !okA || !okB {
		return 0.5
	}
	steps := (na - nb + 12) % 12
	steps = min(steps, 12-steps)
	switch {
	case steps == 0 || (steps == 1 && la == lb):
		return 0
	case steps == 2 && la == lb:
		return 0.5
	default:
		return 1
	}
}

func parseCamelot(c string) (int, byte, bool) {
	if len(c) < 2 {
		return 0, 0, false
	}
	n, err := strconv.Atoi(c[:len(c)-1])
	letter := c[len(c)-1]
	if err != nil || n < 1 || n > 12 || !strings.ContainsRune("AB", rune(letter)) {
		return 0, 0, false
	}
	return n, letter, true
}

// OrderForDJ orders tracks for smooth transitions, starting from tracks[0]
// when keepFirst (the track playing now), else from the calmest one.
func OrderForDJ(tracks []persistence.SonicSummary, keepFirst bool) []string {
	n := len(tracks)
	if n == 0 {
		return nil
	}
	start := 0
	if !keepFirst {
		for i, t := range tracks {
			if t.Energy < tracks[start].Energy {
				start = i
			}
		}
	}

	// Nearest neighbour from the start.
	order := []int{start}
	used := make([]bool, n)
	used[start] = true
	for len(order) < n {
		last, best := order[len(order)-1], -1
		for j := range n {
			if !used[j] && (best == -1 || transitionCost(tracks[last], tracks[j]) < transitionCost(tracks[last], tracks[best])) {
				best = j
			}
		}
		used[best] = true
		order = append(order, best)
	}

	// 2-opt: reverse a segment (never the start) when that lowers the cost.
	// The cost is symmetric, so only the two edges at the segment ends change.
	cost := func(i, j int) float64 { return transitionCost(tracks[order[i]], tracks[order[j]]) }
	for range maxTwoOptPasses {
		improved := false
		for i := 1; i < n-1; i++ {
			for j := i + 1; j < n; j++ {
				before := cost(i-1, i)
				after := cost(i-1, j)
				if j+1 < n {
					before += cost(j, j+1)
					after += cost(i, j+1)
				}
				if after < before-1e-9 {
					for a, b := i, j; a < b; a, b = a+1, b-1 {
						order[a], order[b] = order[b], order[a]
					}
					improved = true
				}
			}
		}
		if !improved {
			break
		}
	}

	ids := make([]string, n)
	for i, k := range order {
		ids[i] = tracks[k].TrackID
	}
	return ids
}

// AutoDJEnabled reports whether Auto-DJ ordering is switched on.
func (s *Service) AutoDJEnabled() bool { return s.Enabled() && s.config().AutoDJ }

// AutoDJ orders track ids for smooth transitions, keeping every occurrence
// (a queue may hold a track twice). keepFirst pins ids[0], the track playing
// now. Tracks not analyzed yet can't be placed: they follow, in their
// original order.
func (s *Service) AutoDJ(ctx context.Context, ids []string, keepFirst bool) ([]string, error) {
	known, err := s.repo.SummariesFor(ctx, ids, AlgoVersion)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]persistence.SonicSummary, len(known))
	for _, k := range known {
		byID[k.TrackID] = k
	}
	var head []string
	if keepFirst && len(ids) > 0 {
		head, ids = ids[:1], ids[1:]
	}
	var analyzed []persistence.SonicSummary
	var rest []string
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			analyzed = append(analyzed, t)
		} else {
			rest = append(rest, id)
		}
	}
	// An analyzed pinned track leads the order; an unanalyzed one just stays first.
	if len(head) == 1 {
		if t, ok := byID[head[0]]; ok {
			return append(OrderForDJ(append([]persistence.SonicSummary{t}, analyzed...), true), rest...), nil
		}
	}
	return append(append(head, OrderForDJ(analyzed, false)...), rest...), nil
}
