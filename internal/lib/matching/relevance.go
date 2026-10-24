package matching

import "strings"

// Relevance scores how well s matches the query for search ordering: exact (0),
// prefix (1), substring (2), otherwise (3). Lower is better; ties keep input
// order (stable sort).
func Relevance(query, s string) int {
	q, x := strings.ToLower(strings.TrimSpace(query)), strings.ToLower(s)
	switch {
	case q == "" || x == q:
		return 0
	case strings.HasPrefix(x, q):
		return 1
	case strings.Contains(x, q):
		return 2
	default:
		return 3
	}
}

// TitleOverlap scores how well two titles match in either direction: one is
// often a decorated superstring of the other ("Get Lucky (Radio Edit - feat.
// ...)" vs "Get Lucky"), so it keeps the better of both Relevance directions.
func TitleOverlap(wanted, candidate string) int {
	return min(Relevance(wanted, candidate), Relevance(candidate, wanted))
}
