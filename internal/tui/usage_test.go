package tui

import (
	"testing"

	"github.com/KamilSupera/github-pullrequests-checkecker/internal/claude"
)

func TestFmtTokens(t *testing.T) {
	cases := map[int64]string{
		0:         "0",
		999:       "999",
		1000:      "1.0k",
		13802:     "13.8k",
		1_000_000: "1.0M",
		2_500_000: "2.5M",
	}
	for in, want := range cases {
		if got := fmtTokens(in); got != want {
			t.Errorf("fmtTokens(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderUsageLine(t *testing.T) {
	u := claude.Usage{InputTokens: 13802, OutputTokens: 4, CacheReadTokens: 17837, CacheCreationTokens: 2368, CostUSD: 0.0928, Calls: 1}
	got := renderUsageLine(u)
	for _, want := range []string{"13.8k in", "4 out", "20.2k cached", "$0.0928", "1 call"} {
		if !contains(got, want) {
			t.Errorf("usage line missing %q; got: %s", want, got)
		}
	}
	// plural
	u.Calls = 3
	if !contains(renderUsageLine(u), "3 calls") {
		t.Errorf("expected plural 'calls'")
	}
}
