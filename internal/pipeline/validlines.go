package pipeline

import (
	"regexp"
	"strconv"
	"strings"
)

// hunkRE matches a unified-diff hunk header. Supports the optional
// length numbers (e.g. "@@ -10 +20,3 @@").
var hunkRE = regexp.MustCompile(`@@ -(\d+),?(\d*) \+(\d+),?(\d*) @@`)

// ValidLines tracks which (path, line, side) triples are addressable
// by inline review comments in a given diff. GitHub's POST review
// endpoint returns 422 when a comment points at a line outside any
// hunk of the diff, so we filter against this set first.
type ValidLines struct {
	right map[string]map[int]bool
	left  map[string]map[int]bool
}

func ParseValidLines(diff string) *ValidLines {
	v := &ValidLines{
		right: map[string]map[int]bool{},
		left:  map[string]map[int]bool{},
	}
	curPath := ""
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			p := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			p = strings.TrimPrefix(p, "b/")
			curPath = p

		case strings.HasPrefix(line, "@@"):
			if curPath == "" {
				continue
			}
			m := hunkRE.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			oldStart, _ := strconv.Atoi(m[1])
			oldLen := 1
			if m[2] != "" {
				oldLen, _ = strconv.Atoi(m[2])
			}
			newStart, _ := strconv.Atoi(m[3])
			newLen := 1
			if m[4] != "" {
				newLen, _ = strconv.Atoi(m[4])
			}
			if v.right[curPath] == nil {
				v.right[curPath] = map[int]bool{}
			}
			for i := 0; i < newLen; i++ {
				v.right[curPath][newStart+i] = true
			}
			if v.left[curPath] == nil {
				v.left[curPath] = map[int]bool{}
			}
			for i := 0; i < oldLen; i++ {
				v.left[curPath][oldStart+i] = true
			}
		}
	}
	return v
}

// Allows reports whether path/line/side is within some hunk of the diff.
// Side defaults to RIGHT when empty or unrecognized.
func (v *ValidLines) Allows(path string, line int, side string) bool {
	var m map[string]map[int]bool
	if side == "LEFT" {
		m = v.left
	} else {
		m = v.right
	}
	return m[path][line]
}
