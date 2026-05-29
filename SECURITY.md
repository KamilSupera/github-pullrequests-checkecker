# Security Policy

## Reporting a vulnerability

Please **do not** open a public issue for security problems.

Report privately via GitHub's [Security Advisories](https://github.com/KamilSupera/github-pullrequests-checkecker/security/advisories/new)
("Report a vulnerability" button), or email the maintainer.

Include: what you found, steps to reproduce, and impact. You'll get an
acknowledgement within a few days, and a fix or mitigation plan once the
report is triaged.

## Scope

`prcheck` stores no credentials of its own. It shells out to the `gh` and
`claude` CLIs, which own their respective authentication. Relevant areas:

- Handling of PR diffs, CI output, and Jira text passed to `claude`.
- Local files written under the cache dir (`PRCHECK_CACHE_DIR` / OS cache dir)
  and failed-review dumps in `$TMPDIR` (`prcheck-*.json`, mode `0600`).
- Construction of `gh`/`claude` subprocess arguments.

Issues in `gh`, `claude`, or the Atlassian MCP server should be reported to
their respective projects.

## Supported versions

Only the latest release receives security fixes.
