package pipeline

import "fmt"

const DefaultDiffCap = 100_000

// CapDiff trims `s` to at most `cap` characters. Returns the (possibly
// trimmed) string and a boolean indicating whether truncation occurred.
// When trimmed, an explanatory marker is appended.
func CapDiff(s string, cap int) (string, bool) {
	if len(s) <= cap {
		return s, false
	}
	marker := fmt.Sprintf("\n\n... [diff truncated at %d chars; original was %d]\n", cap, len(s))
	return s[:cap] + marker, true
}
