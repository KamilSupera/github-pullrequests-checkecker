package pipeline

import "testing"

func TestStripDecorations(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"[blocker] L28: 🔴 fix this nil deref", "fix this nil deref"},
		{"**MAJOR**: missing error check", "missing error check"},
		{"⚠️  unsafe cast", "unsafe cast"},
		{"L42: rename for clarity", "rename for clarity"},
		{"plain comment with no decoration", "plain comment with no decoration"},
		{"  [major]  duplicated logic", "duplicated logic"},
		{"(blocker) race in cache", "race in cache"},
		{"🔴 use context.Done()", "use context.Done()"},
	}
	for _, c := range cases {
		if got := stripDecorations(c.in); got != c.want {
			t.Errorf("stripDecorations(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
