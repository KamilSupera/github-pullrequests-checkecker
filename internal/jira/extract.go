package jira

import "regexp"

// matches PROJECT-NUMBER where PROJECT is 2+ uppercase letters
// and is preceded by start-of-string or non-alphanumeric.
var keyRE = regexp.MustCompile(`(?:^|[^A-Z0-9])([A-Z]{2,}-\d+)`)

// ExtractKey returns the first Jira key found across the given inputs
// (checked in order). Returns "" if none found.
func ExtractKey(inputs ...string) string {
	for _, in := range inputs {
		m := keyRE.FindStringSubmatch(in)
		if len(m) >= 2 {
			return m[1]
		}
	}
	return ""
}
