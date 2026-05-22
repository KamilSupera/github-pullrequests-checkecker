package github

import "testing"

func TestSummarizeChecks(t *testing.T) {
	checks := []Check{
		{Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS"},
		{Name: "test", Status: "COMPLETED", Conclusion: "FAILURE"},
		{Name: "lint", Status: "IN_PROGRESS", Conclusion: ""},
		{Name: "deploy", Status: "COMPLETED", Conclusion: "SUCCESS"},
	}
	s := SummarizeChecks(checks)
	if s.Passed != 2 {
		t.Errorf("Passed = %d", s.Passed)
	}
	if s.Failed != 1 {
		t.Errorf("Failed = %d", s.Failed)
	}
	if s.Pending != 1 {
		t.Errorf("Pending = %d", s.Pending)
	}
	if len(s.FailedNames) != 1 || s.FailedNames[0] != "test" {
		t.Errorf("FailedNames = %v", s.FailedNames)
	}
	if len(s.PendingNames) != 1 || s.PendingNames[0] != "lint" {
		t.Errorf("PendingNames = %v", s.PendingNames)
	}
}
