package cache

import (
	"testing"
	"time"

	"github.com/KamilSupera/github-pullrequests-checkecker/internal/github"
)

func TestSnapshotRoundTrip(t *testing.T) {
	t.Setenv("PRCHECK_CACHE_DIR", t.TempDir())

	if got := Load("mine"); got != nil {
		t.Fatalf("Load on empty cache = %v, want nil", got)
	}

	prs := []github.PR{
		{Number: 1, Title: "first", URL: "https://github.com/o/r/pull/1"},
		{Number: 2, Title: "second", URL: "https://github.com/o/r/pull/2"},
	}
	Save("mine", prs)

	got := Load("mine")
	if got == nil {
		t.Fatal("Load after Save = nil")
	}
	if len(got.PRs) != 2 || got.PRs[0].Number != 1 || got.PRs[1].Title != "second" {
		t.Fatalf("round-tripped PRs = %+v", got.PRs)
	}
	if got.FetchedAt.IsZero() {
		t.Error("FetchedAt not set")
	}
}

func TestSnapshotTabsIsolated(t *testing.T) {
	t.Setenv("PRCHECK_CACHE_DIR", t.TempDir())

	Save("mine", []github.PR{{Number: 1}})
	Save("review", []github.PR{{Number: 2}, {Number: 3}})

	if got := Load("mine"); got == nil || len(got.PRs) != 1 {
		t.Fatalf("mine tab = %+v", got)
	}
	if got := Load("review"); got == nil || len(got.PRs) != 2 {
		t.Fatalf("review tab = %+v", got)
	}
}

func TestSeenStore(t *testing.T) {
	t.Setenv("PRCHECK_CACHE_DIR", t.TempDir())

	s := LoadSeen()
	if s == nil || s.URLs == nil {
		t.Fatal("LoadSeen returned nil map")
	}

	url := "https://github.com/o/r/pull/1"
	base := time.Now()

	if !s.IsNew(url, base) {
		t.Error("never-seen URL should be new")
	}

	s.Mark(url)
	if s.IsNew(url, base.Add(-time.Hour)) {
		t.Error("older update should not be new after Mark")
	}
	if !s.IsNew(url, time.Now().Add(time.Hour)) {
		t.Error("newer update should be new")
	}

	s.Save()
	if reloaded := LoadSeen(); reloaded.IsNew(url, base.Add(-time.Hour)) {
		t.Error("seen state did not persist")
	}
}

func TestBookmarkStore(t *testing.T) {
	t.Setenv("PRCHECK_CACHE_DIR", t.TempDir())

	s := LoadBookmarks()
	url := "https://github.com/o/r/pull/9"
	if s.Has(url) {
		t.Error("empty store should not have url")
	}

	if added := s.Toggle(url); !added {
		t.Error("first Toggle should add")
	}
	if !s.Has(url) {
		t.Error("Has should be true after Toggle add")
	}
	if removed := s.Toggle(url); removed {
		t.Error("second Toggle should remove")
	}

	s.Toggle(url)
	s.Save()
	if !LoadBookmarks().Has(url) {
		t.Error("bookmark did not persist")
	}
}

func TestHistory(t *testing.T) {
	t.Setenv("PRCHECK_CACHE_DIR", t.TempDir())

	if got := LoadHistory(0); got != nil {
		t.Fatalf("empty history = %v", got)
	}

	for i := 1; i <= 3; i++ {
		AppendHistory(HistoryEntry{PRURL: "u", ReviewID: int64(i), Comments: i})
	}

	all := LoadHistory(0)
	if len(all) != 3 || all[0].ReviewID != 1 || all[2].ReviewID != 3 {
		t.Fatalf("LoadHistory(0) = %+v", all)
	}

	last2 := LoadHistory(2)
	if len(last2) != 2 || last2[0].ReviewID != 2 {
		t.Fatalf("LoadHistory(2) = %+v", last2)
	}
}

func TestDetectChanges(t *testing.T) {
	if got := DetectChanges(nil, []github.PR{{URL: "a"}}); got != nil {
		t.Errorf("nil old snapshot = %v, want nil", got)
	}

	old := &Snapshot{PRs: []github.PR{
		{URL: "a", UpdatedAt: "t1"},
		{URL: "b", UpdatedAt: "t1"},
	}}
	fresh := []github.PR{
		{URL: "a", UpdatedAt: "t1"},
		{URL: "b", UpdatedAt: "t2"},
		{URL: "c", UpdatedAt: "t1"},
	}

	got := DetectChanges(old, fresh)
	want := map[string]bool{"b": true, "c": true}
	if len(got) != 2 {
		t.Fatalf("DetectChanges = %v, want 2 entries", got)
	}
	for _, u := range got {
		if !want[u] {
			t.Errorf("unexpected changed URL %q", u)
		}
	}
}

func TestSeenOpened(t *testing.T) {
	s := &SeenStore{URLs: map[string]time.Time{}}
	if s.Opened("x") {
		t.Error("unknown url should not report opened")
	}
	s.Mark("x")
	if !s.Opened("x") {
		t.Error("marked url should report opened")
	}
}
