package jira

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchIssue_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/ABC-123" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key": "ABC-123",
			"fields": map[string]any{
				"summary":     "Add caching",
				"description": "We need a cache",
			},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "user@example.com", "tok")
	iss, err := c.FetchIssue(t.Context(), "ABC-123")
	if err != nil {
		t.Fatalf("FetchIssue err: %v", err)
	}
	if iss.Key != "ABC-123" {
		t.Errorf("Key = %q", iss.Key)
	}
	if iss.Summary != "Add caching" {
		t.Errorf("Summary = %q", iss.Summary)
	}
	if !strings.Contains(iss.Description, "cache") {
		t.Errorf("Description = %q", iss.Description)
	}
}

func TestFetchIssue_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "u", "t")
	_, err := c.FetchIssue(t.Context(), "ABC-1")
	if err == nil {
		t.Fatal("expected error on 404")
	}
}
