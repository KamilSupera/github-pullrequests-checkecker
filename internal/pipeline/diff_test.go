package pipeline

import (
	"strings"
	"testing"
)

func TestCapDiff_UnderCap(t *testing.T) {
	in := "diff line\n"
	out, trunc := CapDiff(in, 100)
	if out != in {
		t.Errorf("out = %q, want %q", out, in)
	}
	if trunc {
		t.Error("trunc = true, want false")
	}
}

func TestCapDiff_OverCap(t *testing.T) {
	in := strings.Repeat("x", 200)
	out, trunc := CapDiff(in, 100)
	if !trunc {
		t.Error("trunc = false, want true")
	}
	if len(out) > 200 { // out includes a truncation marker
		t.Errorf("out too long: %d", len(out))
	}
	if !strings.Contains(out, "truncated") {
		t.Error("expected truncation marker")
	}
}
