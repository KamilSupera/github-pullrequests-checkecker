package jira

import "testing"

func TestExtractKey(t *testing.T) {
	tests := []struct {
		name   string
		inputs []string
		want   string
	}{
		{"in body", []string{"Fixes ABC-123 thanks"}, "ABC-123"},
		{"in branch", []string{"feature/ABC-456-add-thing"}, "ABC-456"},
		{"first wins across inputs", []string{"FOO-1", "BAR-9"}, "FOO-1"},
		{"no key", []string{"no jira here", "main"}, ""},
		{"ignore lowercase", []string{"abc-123 fix"}, ""},
		{"multi digit project", []string{"ABCD-9999"}, "ABCD-9999"},
		{"hyphen-separated project rejected", []string{"AB-CD-1"}, "CD-1"},
		{"embedded in url", []string{"https://x/browse/ABC-12"}, "ABC-12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractKey(tt.inputs...)
			if got != tt.want {
				t.Errorf("ExtractKey(%v) = %q, want %q", tt.inputs, got, tt.want)
			}
		})
	}
}
