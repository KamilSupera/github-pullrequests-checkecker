package claude

import (
	"strconv"
	"testing"
)

// envelope wraps an inner result string in the JSON envelope shape the
// real `claude --output-format json` emits, with usage + cost set.
func envelope(result string, in, out, cacheCreate, cacheRead int64, cost float64) string {
	itoa := func(n int64) string { return strconv.FormatInt(n, 10) }
	// result is embedded as a JSON string; the test inputs avoid quotes.
	return `{"type":"result","subtype":"success","is_error":false,` +
		`"total_cost_usd":` + strconv.FormatFloat(cost, 'f', -1, 64) + `,` +
		`"result":"` + result + `",` +
		`"usage":{"input_tokens":` + itoa(in) +
		`,"output_tokens":` + itoa(out) +
		`,"cache_creation_input_tokens":` + itoa(cacheCreate) +
		`,"cache_read_input_tokens":` + itoa(cacheRead) + `}}`
}

func TestInvoke_RecordsUsage(t *testing.T) {
	resetUsage()
	// result holds a review JSON whose quotes are escaped for the envelope.
	review := `{\"summary\":\"ok\",\"comments\":[]}`
	writeFakeClaude(t, envelope(review, 1000, 50, 200, 800, 0.12))

	r, err := Invoke(t.Context(), "p")
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if r.Summary != "ok" {
		t.Errorf("Summary = %q, want ok", r.Summary)
	}
	u := SessionUsage()
	if u.InputTokens != 1000 || u.OutputTokens != 50 {
		t.Errorf("tokens = %d in / %d out, want 1000/50", u.InputTokens, u.OutputTokens)
	}
	if u.CacheCreationTokens != 200 || u.CacheReadTokens != 800 {
		t.Errorf("cache = %d create / %d read, want 200/800", u.CacheCreationTokens, u.CacheReadTokens)
	}
	if u.CostUSD != 0.12 {
		t.Errorf("cost = %v, want 0.12", u.CostUSD)
	}
	if u.Calls != 1 {
		t.Errorf("calls = %d, want 1", u.Calls)
	}
}

func TestInvoke_FallbackBareJSONNoUsage(t *testing.T) {
	resetUsage()
	// Legacy/stub output: bare review JSON, not an envelope.
	writeFakeClaude(t, `{"summary":"ok","comments":[]}`)

	r, err := Invoke(t.Context(), "p")
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if r.Summary != "ok" {
		t.Errorf("Summary = %q", r.Summary)
	}
	if u := SessionUsage(); u.Calls != 0 || u.InputTokens != 0 {
		t.Errorf("expected no usage recorded for bare JSON, got %+v", u)
	}
}

func TestSessionUsage_Accumulates(t *testing.T) {
	resetUsage()
	addUsage(Usage{InputTokens: 100, OutputTokens: 10, CostUSD: 0.01})
	addUsage(Usage{InputTokens: 200, OutputTokens: 20, CostUSD: 0.02})
	u := SessionUsage()
	if u.InputTokens != 300 || u.OutputTokens != 30 {
		t.Errorf("tokens = %d/%d, want 300/30", u.InputTokens, u.OutputTokens)
	}
	if u.Calls != 2 {
		t.Errorf("calls = %d, want 2", u.Calls)
	}
	if u.CostUSD < 0.0299 || u.CostUSD > 0.0301 {
		t.Errorf("cost = %v, want ~0.03", u.CostUSD)
	}
}
