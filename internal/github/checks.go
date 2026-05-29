package github

type ChecksSummary struct {
	Passed       int
	Failed       int
	Pending      int
	FailedNames  []string
	PendingNames []string
}

func SummarizeChecks(checks []Check) ChecksSummary {
	var s ChecksSummary
	for _, c := range checks {
		switch {
		case c.Status != "COMPLETED":
			s.Pending++
			s.PendingNames = append(s.PendingNames, c.Name)
		case c.Conclusion == "SUCCESS":
			s.Passed++
		default:
			s.Failed++
			s.FailedNames = append(s.FailedNames, c.Name)
		}
	}
	return s
}
