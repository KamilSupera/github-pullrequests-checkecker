package claude

import "sync"

// Usage holds Claude token counts and cost. As a session total it is the
// sum across every claude CLI invocation (review + Jira fetch); as a
// single-call value it is what one envelope reported.
type Usage struct {
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	CostUSD             float64
	Calls               int
}

var (
	usageMu    sync.Mutex
	usageTotal Usage
)

// addUsage folds one call's usage into the session total. Calls is
// incremented by one regardless of the token fields.
func addUsage(u Usage) {
	usageMu.Lock()
	defer usageMu.Unlock()
	usageTotal.InputTokens += u.InputTokens
	usageTotal.OutputTokens += u.OutputTokens
	usageTotal.CacheCreationTokens += u.CacheCreationTokens
	usageTotal.CacheReadTokens += u.CacheReadTokens
	usageTotal.CostUSD += u.CostUSD
	usageTotal.Calls++
}

// SessionUsage returns a snapshot of the cumulative usage for this process.
// Safe to call concurrently with in-flight claude invocations.
func SessionUsage() Usage {
	usageMu.Lock()
	defer usageMu.Unlock()
	return usageTotal
}

// resetUsage clears the session total. Test hook.
func resetUsage() {
	usageMu.Lock()
	defer usageMu.Unlock()
	usageTotal = Usage{}
}
