package claude

import "testing"

func TestParseReview_Valid(t *testing.T) {
	raw := `{
		"summary": "Looks good.",
		"comments": [
			{"path":"foo.go","line":12,"side":"RIGHT","body":"nit","severity":"nit"}
		]
	}`
	r, err := ParseReview([]byte(raw))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.Summary != "Looks good." {
		t.Errorf("Summary = %q", r.Summary)
	}
	if len(r.Comments) != 1 {
		t.Fatalf("Comments len = %d", len(r.Comments))
	}
	c := r.Comments[0]
	if c.Path != "foo.go" || c.Line != 12 || c.Side != "RIGHT" {
		t.Errorf("comment = %+v", c)
	}
}

func TestParseReview_ProseAroundJSON(t *testing.T) {
	raw := "Here is the review:\n" + `{"summary":"ok","comments":[]}` + "\nThanks!"
	r, err := ParseReview([]byte(raw))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.Summary != "ok" {
		t.Errorf("Summary = %q", r.Summary)
	}
}

func TestParseReview_InvalidSeverity(t *testing.T) {
	raw := `{"summary":"x","comments":[{"path":"a","line":1,"side":"RIGHT","body":"b","severity":"banana"}]}`
	_, err := ParseReview([]byte(raw))
	if err == nil {
		t.Fatal("expected error for invalid severity")
	}
}

func TestParseReview_MissingSummary(t *testing.T) {
	raw := `{"comments":[]}`
	_, err := ParseReview([]byte(raw))
	if err == nil {
		t.Fatal("expected error for missing summary")
	}
}

func TestParseReview_BadSide(t *testing.T) {
	raw := `{"summary":"x","comments":[{"path":"a","line":1,"side":"MIDDLE","body":"b","severity":"nit"}]}`
	_, err := ParseReview([]byte(raw))
	if err == nil {
		t.Fatal("expected error for bad side")
	}
}
