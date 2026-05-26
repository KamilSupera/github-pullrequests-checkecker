package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ksupera/prcheck/internal/github"
	"github.com/ksupera/prcheck/internal/pipeline"
)

type stepRec struct {
	step   string
	status string
	note   string
	err    error
}

type loaderFn func(ctx context.Context, q github.Query) ([]github.PR, error)
type detailFn func(ctx context.Context, url string) (*github.PRDetail, error)
type diffFn func(ctx context.Context, url string) (string, error)
type runPipelineFn func(ctx context.Context, prURL string, emit func(pipeline.Event)) (*pipeline.Result, error)
type openFn func(url string) error

type Model struct {
	ctx        context.Context
	cancel     context.CancelFunc
	pipeCancel context.CancelFunc // cancel only the running pipeline

	loader   loaderFn
	detailFn detailFn
	diffFn   diffFn
	runPipe  runPipelineFn
	openURL  openFn

	tab      Tab
	prsByTab map[Tab][]github.PR
	loadErr  map[Tab]error
	cursor   int

	details map[string]*github.PRDetail
	diffs   map[string]string

	loadingDetail map[string]bool
	loadingDiff   map[string]bool

	running    bool
	steps      []stepRec // history of progress events
	lastReview *reviewDoneMsg

	viewingDiff bool
	diffVP      viewport.Model

	listH      int // visible height of the PR list pane
	listOffset int // index of first visible line in renderList

	commentsOffset int // first visible line in the comments box
	commentsH      int // visible height of the comments box

	detailOffset int // first visible line in the detail box
	detailH      int // visible height of the detail box

	termW int // last reported terminal width
	termH int // last reported terminal height

	spinner spinner.Model
	err     error
}

func NewModel(loader loaderFn, df detailFn, dfn diffFn, rp runPipelineFn, open openFn) Model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffb000"))

	ctx, cancel := context.WithCancel(context.Background())
	return Model{
		ctx:      ctx,
		cancel:   cancel,
		loader:   loader,
		detailFn: df,
		diffFn:   dfn,
		runPipe:  rp,
		openURL:  open,
		prsByTab: map[Tab][]github.PR{},
		loadErr:  map[Tab]error{},
		details:       map[string]*github.PRDetail{},
		diffs:         map[string]string{},
		loadingDetail: map[string]bool{},
		loadingDiff:   map[string]bool{},
		diffVP:   viewport.New(80, 20),
		listH:    20,
		termW:    80,
		termH:    24,
		spinner:  sp,
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
// two stay in lockstep.
func (m Model) flatGroupedPRs() []github.PR {
	var out []github.PR
	for _, g := range groupByRepo(m.prsByTab[m.tab]) {
		out = append(out, g.prs...)
	}
	return out
}
