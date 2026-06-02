package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/KamilSupera/github-pullrequests-checkecker/internal/claude"
	"github.com/KamilSupera/github-pullrequests-checkecker/internal/config"
	"github.com/KamilSupera/github-pullrequests-checkecker/internal/github"
	"github.com/KamilSupera/github-pullrequests-checkecker/internal/pipeline"
	"github.com/KamilSupera/github-pullrequests-checkecker/internal/tui"
)

// Set via -ldflags at release time by GoReleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	for _, a := range os.Args[1:] {
		if a == "--version" || a == "-v" || a == "version" {
			fmt.Printf("prcheck %s (commit %s, built %s)\n", version, commit, date)
			return
		}
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "prcheck:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := claude.SelectAgent(cfg.Agent); err != nil {
		return err
	}

	if err := checkBinary("gh"); err != nil {
		return err
	}
	if err := checkBinary(claude.AgentBinary()); err != nil {
		return err
	}
	if err := checkGHAuth(); err != nil {
		return err
	}

	jiraFetcher := claude.JiraMCPFetcher{}
	ghClient := github.CLIClient{}
	poster := github.Reviewer{}

	// We need a reference to the Program so the pipeline goroutine can
	// stream progress via Program.Send().
	var program *tea.Program

	runPipe := func(ctx context.Context, prURL string, emit func(pipeline.Event)) (*pipeline.Result, error) {
		wrappedEmit := func(e pipeline.Event) {
			emit(e)
			if program != nil {
				program.Send(tui.ProgressFromEvent(e))
			}
		}
		deps := pipeline.Deps{
			GH:     ghClient,
			Jira:   jiraFetcher,
			Claude: claudeAdapter{},
			Post:   poster,
			Focus:  cfg.Focus,
		}
		return pipeline.Run(ctx, deps, prURL, wrappedEmit)
	}

	loader := func(ctx context.Context, q github.Query) ([]github.PR, error) {
		return github.SearchPRs(ctx, q)
	}
	detailFn := func(ctx context.Context, url string) (*github.PRDetail, error) {
		return github.FetchPRDetail(ctx, url)
	}
	diffFn := func(ctx context.Context, url string) (string, error) {
		return github.FetchDiff(ctx, url)
	}
	checksFn := func(ctx context.Context, url string) (string, error) {
		return github.FetchChecks(ctx, url)
	}
	quickReviewFn := func(ctx context.Context, url, event, body string) (int64, error) {
		return github.PostQuickReview(ctx, url, event, body)
	}

	model := tui.NewModel(loader, detailFn, diffFn, checksFn, quickReviewFn, runPipe, openInBrowser)
	program = tea.NewProgram(model, tea.WithAltScreen())
	_, err = program.Run()
	return err
}

type claudeAdapter struct{}

func (claudeAdapter) Invoke(ctx context.Context, prompt string) (*claude.Review, error) {
	return claude.Invoke(ctx, prompt)
}

func checkBinary(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s not found on PATH", name)
	}
	return nil
}

func checkGHAuth() error {
	var stderr bytes.Buffer
	cmd := exec.Command("gh", "auth", "status")
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh not authenticated — run `gh auth login` (%s)", stderr.String())
	}
	return nil
}

func openInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}
