package sonic

import (
	"cmp"
	"context"
	"slices"
)

const (
	// duplicateSimilarity is the cosine above which two tracks count as the
	// same recording. Calibrated on a real library: a re-encode (MP3 96k)
	// scores 0.997, a lossless copy 1.0, while distinct tracks of a similar
	// length topped out at 0.80.
	duplicateSimilarity = 0.98
	// duplicateMaxGap is the largest duration gap (seconds) between copies.
	duplicateMaxGap = 3
)

// DuplicatesEnabled reports whether sonic duplicate detection is switched on.
func (s *Service) DuplicatesEnabled() bool { return s.Enabled() && s.config().Duplicates }

// Duplicates groups analyzed tracks that sound identical: the same recording
// under other tags, or in another format.
func (s *Service) Duplicates(ctx context.Context) ([][]string, error) {
	return s.index.duplicates(ctx)
}

// ponytail: compares each track with those of a similar length only (sorted
// by duration), a few seconds over 100k tracks; fine for an on-demand admin
// check, cache it if it ever runs on every page load.
func (x *index) duplicates(ctx context.Context) ([][]string, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if err := x.ensure(ctx); err != nil {
		return nil, err
	}
	var order []int
	for i, d := range x.durations {
		if d > 0 {
			order = append(order, i)
		}
	}
	slices.SortFunc(order, func(a, b int) int { return cmp.Compare(x.durations[a], x.durations[b]) })

	parent := make([]int, len(x.ids))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	for a, i := range order {
		for _, j := range order[a+1:] {
			if x.durations[j]-x.durations[i] > duplicateMaxGap {
				break
			}
			var dot float32
			for d, v := range x.row(i) {
				dot += v * x.row(j)[d]
			}
			if dot >= duplicateSimilarity {
				parent[find(j)] = find(i)
			}
		}
	}

	byRoot := map[int][]string{}
	for _, i := range order {
		byRoot[find(i)] = append(byRoot[find(i)], x.ids[i])
	}
	var groups [][]string
	for _, g := range byRoot {
		if len(g) > 1 {
			slices.Sort(g)
			groups = append(groups, g)
		}
	}
	slices.SortFunc(groups, func(a, b []string) int { return cmp.Compare(a[0], b[0]) })
	return groups, nil
}
