# Contributing to prcheck

Thanks for your interest in improving `prcheck`. This doc covers how to get set up and what's expected in a pull request.

## Development setup

```bash
git clone https://github.com/KamilSupera/github-pullrequests-checkecker
cd github-pullrequests-checkecker
go build ./...
```

Requirements: Go 1.26+, plus `gh` and `claude` CLIs if you want to run the tool end-to-end (see [README](README.md)).

## Before you open a PR

Run the same checks CI runs:

```bash
go build ./...
go vet ./...
go test -race ./...
gofmt -l .          # must print nothing
```

All four must pass. CI runs `build`, `vet`, and `test -race` on every push and PR.

## Guidelines

- Keep changes focused — one logical change per PR.
- Add or update tests for any behavior change. The codebase favors table-driven tests with fake `gh`/`claude` subprocesses (see `internal/github/fake_gh_test.go`).
- Match the surrounding style; no new lint warnings from `go vet`.
- Don't commit the `prcheck` binary (it's gitignored).
- Conventional Commit prefixes are used in the changelog (`feat:`, `fix:`, etc.); `docs:`/`test:`/`chore:` are excluded from release notes.

## Releases

Maintainers tag `vX.Y.Z`; GoReleaser builds cross-platform binaries and publishes a GitHub Release automatically.
