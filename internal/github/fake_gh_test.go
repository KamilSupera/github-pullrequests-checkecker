package github

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeFakeGH writes a shell script named `gh` to a tempdir and prepends
// it to PATH for the duration of the test. The script echoes the content
// of FAKE_GH_OUT (set by each test to a fixture path).
func writeFakeGH(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-gh helper is POSIX shell only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	script := `#!/bin/sh
if [ -n "$FAKE_GH_EXIT" ]; then
  echo "$FAKE_GH_STDERR" >&2
  exit "$FAKE_GH_EXIT"
fi
cat "$FAKE_GH_OUT"
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}
