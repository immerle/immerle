// Package strutil holds tiny string helpers shared across packages.
package strutil

import "strings"

// FirstNonEmpty returns the first argument that is non-empty after trimming
// surrounding whitespace, returned trimmed; "" if none qualify.
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}
