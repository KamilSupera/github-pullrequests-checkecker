#!/bin/sh
# Run prcheck against canned mock PRs for screenshots/recordings.
# Prepends the fake gh/claude in demo/bin to PATH and isolates the cache,
# so nothing touches GitHub, Anthropic, or your real prcheck cache.
set -e

DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH= cd -- "$DIR/.." && pwd)

# Build prcheck into the demo dir if missing or stale.
BIN="$DIR/prcheck"
echo "building prcheck..."
go build -o "$BIN" "$ROOT/cmd/prcheck"

# Isolated cache so demo bookmarks/history/seen don't mix with real data.
CACHE="$DIR/.cache"
rm -rf "$CACHE"
mkdir -p "$CACHE"

echo "launching demo (fake gh + fake claude on PATH)..."
PATH="$DIR/bin:$PATH" \
  PRCHECK_CACHE_DIR="$CACHE" \
  exec "$BIN"
