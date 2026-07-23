package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/KamilSupera/github-pullrequests-checkecker/internal/cache"
	"github.com/KamilSupera/github-pullrequests-checkecker/internal/github"
	"github.com/KamilSupera/github-pullrequests-checkecker/internal/pipeline"
)

func tabCacheKey(t Tab) string {
	switch t {
	case TabMine:
		return "mine"
	case TabReview:
		return "review"
	case TabMentioned:
		return "mentioned"
	}
	return "unknown"
}

type stepRec struct {
	step   string
	status string
	note   string
	err    error
}

// focusArea identifies which on-screen box currently has focus. The
// focused box gets the bright border and j/k act on it. Zero value is
// focusList so existing behavior is preserved.
type focusArea int

const (
	focusList focusArea = iota
	focusDetail
	focusComments
)

type loaderFn func(ctx context.Context, q github.Query) ([]github.PR, error)
type detailFn func(ctx context.Context, url string) (*github.PRDetail, error)
type diffFn func(ctx context.Context, url string) (string, error)
type checksFn func(ctx context.Context, url string) (string, error)
type quickReviewFn func(ctx context.Context, prURL, event, body string) (int64, error)
type runPipelineFn func(ctx context.Context, prURL string, emit func(pipeline.Event)) (*pipeline.Result, error)
type openFn func(url string) error

type Model struct {
	ctx        context.Context
	cancel     context.CancelFunc
	pipeCancel context.CancelFunc // cancel only the running pipeline

	loader      loaderFn
	detailFn    detailFn
	diffFn      diffFn
	checksFn    checksFn
	quickReview quickReviewFn
	runPipe     runPipelineFn
	openURL     openFn

	statusMsg string // ephemeral status line (e.g. "Approved #1234")

	tab      Tab
	prsByTab map[Tab][]github.PR
	loadErr  map[Tab]error
	cursor   int

	focus focusArea // which box (list/detail/comments) has focus

	details map[string]*github.PRDetail
	diffs   map[string]string

	loadingDetail map[string]bool
	loadingDiff   map[string]bool

	running    bool
	steps      []stepRec // history of progress events
	lastReview *reviewDoneMsg

	viewingDiff bool
	diffVP      viewport.Model

	viewingChecks bool
	checksVP      viewport.Model
	checks        map[string]string

	viewingStats bool
	statsVP      viewport.Model

	selectingAgent bool // agent-picker overlay is open
	agentChoice    int  // highlighted row in the agent picker

	selectingModel bool   // model-picker overlay is open (pre-review)
	modelChoice    int    // highlighted row in the model picker
	pendingReview  string // PR URL to review once a model is picked

	listH      int // visible height of the PR list pane
	listOffset int // index of first visible line in renderList

	commentsOffset int // first visible line in the comments box
	commentsH      int // visible height of the comments box

	detailOffset int // first visible line in the detail box
	detailH      int // visible height of the detail box

	resultOffset int // first visible line in the review-result view
	resultH      int // visible height of the result pane

	filtering bool   // is the filter input active
	filter    string // current filter substring (case-insensitive)

	seen      *cache.SeenStore
	bookmarks *cache.BookmarkStore
	reviewed  map[string]bool // PR URLs the user has posted a review on

	termW int // last reported terminal width
	termH int // last reported terminal height

	spinner spinner.Model
	err     error
}

func NewModel(loader loaderFn, df detailFn, dfn diffFn, cf checksFn, qr quickReviewFn, rp runPipelineFn, open openFn) Model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffb000"))

	ctx, cancel := context.WithCancel(context.Background())
	prsByTab := map[Tab][]github.PR{}
	// Preload from disk cache so the first frame shows immediately
	// while the background fetches run.
	for _, t := range []Tab{TabMine, TabReview, TabMentioned} {
		if s := cache.Load(tabCacheKey(t)); s != nil {
			prsByTab[t] = s.PRs
		}
	}
	return Model{
		ctx:           ctx,
		cancel:        cancel,
		loader:        loader,
		detailFn:      df,
		diffFn:        dfn,
		checksFn:      cf,
		quickReview:   qr,
		runPipe:       rp,
		openURL:       open,
		prsByTab:      prsByTab,
		loadErr:       map[Tab]error{},
		seen:          cache.LoadSeen(),
		bookmarks:     cache.LoadBookmarks(),
		reviewed:      reviewedFromHistory(),
		details:       map[string]*github.PRDetail{},
		diffs:         map[string]string{},
		loadingDetail: map[string]bool{},
		loadingDiff:   map[string]bool{},
		diffVP:        viewport.New(80, 20),
		checksVP:      viewport.New(80, 20),
		statsVP:       viewport.New(80, 20),
		checks:        map[string]string{},
		listH:         20,
		termW:         80,
		termH:         24,
		spinner:       sp,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.loadTab(TabMine),
		m.loadTab(TabReview),
		m.loadTab(TabMentioned),
	)
}

func (m Model) loadTab(t Tab) tea.Cmd {
	return func() tea.Msg {
		var q github.Query
		switch t {
		case TabMine:
			q = github.QueryAuthored
		case TabReview:
			q = github.QueryReviewRequested
		case TabMentioned:
			q = github.QueryMentioned
		}
		prs, err := m.loader(m.ctx, q)
		return prsLoadedMsg{tab: t, prs: prs, err: err}
	}
}

// currentPR returns the PR under the cursor. Cursor indexes the
// display-order list (PRs flattened in group iteration order), so
// keystrokes and rendering stay in sync.
func (m Model) currentPR() (github.PR, bool) {
	flat := m.flatGroupedPRs()
	if len(flat) == 0 || m.cursor < 0 || m.cursor >= len(flat) {
		return github.PR{}, false
	}
	return flat[m.cursor], true
}

// flatGroupedPRs returns the PRs of the current tab in the exact order
// they appear in the rendered list — derived from groupByRepo so the
// two stay in lockstep. Filter is applied when set.
func (m Model) flatGroupedPRs() []github.PR {
	src := m.filteredPRs()
	var out []github.PR
	for _, g := range groupByRepo(src) {
		out = append(out, g.prs...)
	}
	return out
}

// filteredPRs returns the current tab's PRs filtered by m.filter
// (case-insensitive substring match against title, repo, and branch).
func (m Model) filteredPRs() []github.PR {
	prs := m.prsByTab[m.tab]
	if m.filter == "" {
		return prs
	}
	q := strings.ToLower(m.filter)
	var out []github.PR
	for _, pr := range prs {
		if strings.Contains(strings.ToLower(pr.Title), q) ||
			strings.Contains(strings.ToLower(pr.Repo), q) ||
			strings.Contains(strings.ToLower(pr.HeadRefName), q) ||
			strings.Contains(strings.ToLower(pr.Author), q) {
			out = append(out, pr)
		}
	}
	return out
}

// reviewedFromHistory builds the set of PR URLs the user has posted a
// review on, from the persisted review history. Always non-nil.
func reviewedFromHistory() map[string]bool {
	set := map[string]bool{}
	for _, e := range cache.LoadHistory(0) {
		if e.PRURL != "" {
			set[e.PRURL] = true
		}
	}
	return set
}
