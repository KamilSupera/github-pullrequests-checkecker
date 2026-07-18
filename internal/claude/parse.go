package claude

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ReviewComment struct {
	Path       string `json:"path"`
	Line       int    `json:"line"`
	Side       string `json:"side"`
	Body       string `json:"body"`
	Severity   string `json:"severity"`
	Suggestion string `json:"suggestion"`
}

type Review struct {
	Summary  string          `json:"summary"`
	Aspects  []Aspect        `json:"aspects"`
	Comments []ReviewComment `json:"comments"`
}

// Aspect is one review dimension Claude checked.
type Aspect struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "ok" | "issue" | "missing" | "n/a"
	Note   string `json:"note"`
}

var validSeverity = map[string]bool{
	"blocker": true, "major": true, "minor": true, "nit": true,
}

var validAspectStatus = map[string]bool{
	"ok": true, "issue": true, "missing": true, "n/a": true,
}

// ParseReview parses Claude's JSON output. Tolerates prose around the
// JSON — including stray braces inside that prose — by trying every
// balanced {...} candidate in order and accepting the first one that
// unmarshals as a Review with a non-empty summary (issue #12).
func ParseReview(raw []byte) (*Review, error) {
	s := string(raw)
	var r Review
	found := decodeFirstObject(s, func(doc string) bool {
		var cand Review
		if err := json.Unmarshal([]byte(doc), &cand); err != nil {
			return false
		}
		if strings.TrimSpace(cand.Summary) == "" {
			return false
		}
		r = cand
		return true
	})
	if !found {
		return nil, fmt.Errorf("no parseable review JSON in %s output (tail: %q)", AgentName(), tail(s, 300))
	}
	for i, a := range r.Aspects {
		if a.Name == "" {
			return nil, fmt.Errorf("aspects[%d] missing name", i)
		}
		if !validAspectStatus[a.Status] {
			return nil, fmt.Errorf("aspects[%d] invalid status %q", i, a.Status)
		}
	}
	for i, c := range r.Comments {
		if c.Path == "" || c.Body == "" {
			return nil, fmt.Errorf("comment[%d] missing path or body", i)
		}
		if c.Side != "LEFT" && c.Side != "RIGHT" {
			return nil, fmt.Errorf("comment[%d] invalid side %q", i, c.Side)
		}
		if !validSeverity[c.Severity] {
			return nil, fmt.Errorf("comment[%d] invalid severity %q", i, c.Severity)
		}
	}
	return &r, nil
}

// decodeFirstObject scans s for balanced {...} candidates in order and
// returns true once decode accepts one. Skipping rejected candidates is
// what makes stray braces in prose before the real JSON harmless.
func decodeFirstObject(s string, decode func(doc string) bool) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '{' {
			continue
		}
		doc := balancedObjectAt(s, i)
		if doc == "" {
			continue
		}
		if decode(doc) {
			return true
		}
	}
	return false
}

// balancedObjectAt returns the balanced {...} substring starting at
// s[start] (which must be '{'), or "" if it never closes.
func balancedObjectAt(s string, start int) string {
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		switch {
		case esc:
			esc = false
		case ch == '\\' && inStr:
			esc = true
		case ch == '"':
			inStr = !inStr
		case !inStr && ch == '{':
			depth++
		case !inStr && ch == '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
