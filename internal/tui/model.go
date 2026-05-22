package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ksupera/prcheck/internal/github"
	"github.com/ksupera/prcheck/internal/pipeline"
)

type loaderFn func(ctx context.Context, q github.Query) ([]github.PR, error)
type detailFn func(ctx context.Context, url string) (*github.PRDetail, error)
type diffFn func(ctx context.Context, url string) (string, error)
type runPipelineFn func(ctx context.Context, prURL string, emit func(pipeline.Event)) (int64, error)
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

	running    bool
	steps      []string // history of progress events
	lastReview *reviewDoneMsg

	viewingDiff bool
	diffVP      viewport.Model

	spinner spinner.Model
	err     error
}

func NewModel(loader loaderFn, df detailFn, dfn diffFn, rp runPipelineFn, open openFn) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

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
		details:  map[string]*github.PRDetail{},
		diffs:    map[string]string{},
		diffVP:   viewport.New(80, 20),
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

func (m Model) currentPR() (github.PR, bool) {
	prs := m.prsByTab[m.tab]
	if len(prs) == 0 || m.cursor < 0 || m.cursor >= len(prs) {
		return github.PR{}, false
	}
	return prs[m.cursor], true
}
