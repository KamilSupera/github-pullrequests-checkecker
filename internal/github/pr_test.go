package github

import (
	"path/filepath"
	"testing"
)

func TestSearchPRs(t *testing.T) {
	writeFakeGH(t)
	abs, _ := filepath.Abs("../../testdata/gh/search-mine.json")
	t.Setenv("FAKE_GH_OUT", abs)

	prs, err := SearchPRs(t.Context(), QueryAuthored)
	if err != nil {
		t.Fatalf("SearchPRs err: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("got %d PRs, want 2", len(prs))
	}
	if prs[0].Number != 1234 {
		t.Errorf("prs[0].Number = %d", prs[0].Number)
	}
	if prs[0].Title != "Add caching layer" {
		t.Errorf("prs[0].Title = %q", prs[0].Title)
	}
	if prs[0].Author != "kamil" {
		t.Errorf("prs[0].Author = %q", prs[0].Author)
	}
}

func TestFetchPRDetail(t *testing.T) {
	writeFakeGH(t)
	abs, _ := filepath.Abs("../../testdata/gh/pr-view.json")
	t.Setenv("FAKE_GH_OUT", abs)

	d, err := FetchPRDetail(t.Context(), "https://github.com/org/repo/pull/1234")
	if err != nil {
		t.Fatalf("FetchPRDetail err: %v", err)
	}
	if d.Body == "" {
		t.Error("Body empty")
	}
	if len(d.Checks) != 2 {
		t.Fatalf("Checks count = %d", len(d.Checks))
	}
	if d.Checks[1].Conclusion != "FAILURE" {
		t.Errorf("Checks[1].Conclusion = %q", d.Checks[1].Conclusion)
	}
}
