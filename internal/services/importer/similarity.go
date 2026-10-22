package importer

import (
	"strings"
	"unicode"
)

// MatchThreshold is the minimum similarity (0..1) between a source track and a
// content-provider candidate for the match to be accepted automatically. Below
// it, the item is flagged "doubtful" for manual review rather than added.
const MatchThreshold = 0.90

// normalize lowercases, drops parenthetical/bracketed noise (e.g. "(feat. X)",
// "[Remastered]"), strips punctuation/diacritics to spaces and collapses runs of
// whitespace — so "Daft Punk - Da Funk (Radio Edit)" and "daft punk da funk"
// compare closely.
func normalize(s string) string {
	s = strings.ToLower(s)
	s = stripBracketed(s)
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			prevSpace = false
		default:
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// stripBracketed removes spans inside () or [] (commonly remaster/feat/edit
// annotations that wrongly penalise a match).
func stripBracketed(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// similarity returns a 0..1 score for two raw strings, comparing their
// normalized forms with a Levenshtein ratio. Empty-vs-empty is 1; empty-vs-
// non-empty is 0.
func similarity(a, b string) float64 {
	na, nb := normalize(a), normalize(b)
	if na == "" && nb == "" {
		return 1
	}
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1
	}
	ra, rb := []rune(na), []rune(nb)
	dist := levenshtein(ra, rb)
	maxLen := max(len(ra), len(rb))
	return 1 - float64(dist)/float64(maxLen)
}

// trackSimilarity scores a source (artist, title) against a candidate
// (artist, title), combining the two so neither field dominates.
func trackSimilarity(srcArtist, srcTitle, candArtist, candTitle string) float64 {
	return similarity(srcArtist+" "+srcTitle, candArtist+" "+candTitle)
}

// levenshtein computes the edit distance between two rune slices (two-row DP).
func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}
