package claude

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ReviewComment struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Side     string `json:"side"`
	Body     string `json:"body"`
	Severity string `json:"severity"`
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

// ParseReview parses Claude's JSON output. Tolerates leading/trailing
// prose by finding the first balanced JSON object in the input.
func ParseReview(raw []byte) (*Review, error) {
	doc := findJSONObject(string(raw))
	if doc == "" {
		return nil, fmt.Errorf("no JSON object found in claude output")
	}

	var r Review
	if err := json.Unmarshal([]byte(doc), &r); err != nil {
		return nil, fmt.Errorf("unmarshal review: %w", err)
	}

	if strings.TrimSpace(r.Summary) == "" {
		return nil, fmt.Errorf("review summary missing or empty")
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

func findJSONObject(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
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
