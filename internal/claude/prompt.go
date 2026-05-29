package claude

import (
	"fmt"
	"strings"
)

type PromptInput struct {
	Title         string
	HeadRef       string
	BaseRef       string
	Author        string
	JiraKey       string
	JiraSummary   string
	JiraDesc      string
	FailedChecks  []string
	PendingChecks []string
	Diff          string
	DiffTruncated bool
	// Focus aspects (security, performance, ...) to emphasize. Empty
	// means balanced review across all aspects.
	Focus []string
}

const schemaBlock = `Review the diff across these aspects (report each in "aspects" below):
- correctness        — logic bugs, edge cases, off-by-ones, nil handling
- performance        — algorithmic complexity, allocations, hot-path cost
- security           — input validation, secrets, injection, auth
- error_handling     — error wrapping, propagation, recovery
- tests              — coverage of new behavior, test quality
- complexity         — readability, dead code, unnecessary abstraction
- requirements_match — does the diff implement the Jira issue (when present)

Only include "comments" at severity "blocker" or "major". Do NOT
include nit, minor, style, or formatting suggestions.

Each "body" must be plain prose. Do NOT prefix it with severity tags
like "[blocker]" or "**MAJOR**:", emoji or icons (🔴 ⚠️ ✗ etc.), or
line markers like "L28:".

Every comment MUST include a "suggestion" — a concrete fix proposal
(1-3 sentences or a short code snippet in fenced backticks). The
suggestion explains WHAT to change, not just that something is wrong.
If you genuinely cannot propose a fix, set "suggestion" to a short
explanation of what investigation is needed.

Return ONLY JSON matching this schema (no prose before or after):
{
  "summary": "string (overall review summary, 2-5 sentences)",
  "aspects": [
    {
      "name": "correctness | performance | security | error_handling | tests | complexity | requirements_match",
      "status": "ok" | "issue" | "missing" | "n/a",
      "note": "one-line explanation (required unless status=ok)"
    }
  ],
  "comments": [
    {
      "path": "file path from diff",
      "line": <int, line number in NEW file>,
      "side": "RIGHT" | "LEFT",
      "body": "review comment (the problem)",
      "severity": "blocker" | "major",
      "suggestion": "concrete fix proposal (1-3 sentences or short code)"
    }
  ]
}`

func BuildPrompt(in PromptInput) string {
	var b strings.Builder
	b.WriteString("Use skill caveman:caveman-review.\n\n")
	fmt.Fprintf(&b, "PR: %s\n", in.Title)
	fmt.Fprintf(&b, "Branch: %s -> %s\n", in.HeadRef, in.BaseRef)
	fmt.Fprintf(&b, "Author: %s\n\n", in.Author)

	if in.JiraKey != "" {
		b.WriteString("Jira issue:\n")
		fmt.Fprintf(&b, "%s: %s\n", in.JiraKey, in.JiraSummary)
		if in.JiraDesc != "" {
			b.WriteString(in.JiraDesc)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("CI checks:\n")
	if len(in.FailedChecks) > 0 {
		fmt.Fprintf(&b, "- failed: %s\n", strings.Join(in.FailedChecks, ", "))
	} else {
		b.WriteString("- failed: (none)\n")
	}
	if len(in.PendingChecks) > 0 {
		fmt.Fprintf(&b, "- pending: %s\n", strings.Join(in.PendingChecks, ", "))
	} else {
		b.WriteString("- pending: (none)\n")
	}
	b.WriteString("\n")

	if in.DiffTruncated {
		b.WriteString("Diff (TRUNCATED — review may be incomplete):\n")
	} else {
		b.WriteString("Diff:\n")
	}
	b.WriteString(in.Diff)
	if !strings.HasSuffix(in.Diff, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if len(in.Focus) > 0 {
		fmt.Fprintf(&b, "FOCUS: emphasize these aspects above all others: %s.\n\n",
			strings.Join(in.Focus, ", "))
	}

	b.WriteString(schemaBlock)
	b.WriteString("\n")
	return b.String()
}
