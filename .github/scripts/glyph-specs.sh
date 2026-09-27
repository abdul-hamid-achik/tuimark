#!/usr/bin/env bash
# Runs the Glyphrun end-to-end specs for CI and release. ci.yml and
# release.yml both call this script, so the list of specs and the glyph
# invocation stay in one place.
#
# Unlike `task glyph`, which stops at the first failing spec, this runs every
# spec and then reports all the failures, so one CI run shows everything that
# broke.
#
# It builds nothing: the five binaries under bin/ must already exist (run
# `task build`, or the `go build` lines in README's Testing section).
#
# Usage:
#   .github/scripts/glyph-specs.sh                      # every specs/glyphrun/*.yml
#   .github/scripts/glyph-specs.sh specs/glyphrun/a.yml # only the named specs
#
# Env:
#   GLYPH  the glyph command to run (default: glyph)
#
# Written for bash 3.2 (the /bin/bash macOS ships), so no empty-array
# expansions under `set -u`.
set -u

# Sort the glob the same way on every machine, whatever the locale.
export LC_ALL=C

GLYPH="${GLYPH:-glyph}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root" || exit 1

if ! command -v "$GLYPH" >/dev/null 2>&1; then
  echo "glyph-specs: '$GLYPH' not found on PATH (go install github.com/abdul-hamid-achik/glyphrun/cmd/glyph@v0.20.0)" >&2
  exit 1
fi

for bin in tuimark inbox dashboard monitor agent; do
  if [ ! -x "bin/$bin" ]; then
    echo "glyph-specs: bin/$bin is missing; build the binaries first (task build)" >&2
    exit 1
  fi
done

if [ "$#" -eq 0 ]; then
  set -- specs/glyphrun/*.yml
fi

total=0
failed=0
failures=""
for spec in "$@"; do
  total=$((total + 1))
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    echo "::group::$spec"
  fi
  "$GLYPH" run "$spec" --format md
  status=$?
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    echo "::endgroup::"
  fi
  if [ "$status" -eq 0 ]; then
    echo "PASS $spec"
  else
    echo "FAIL $spec (glyph exited $status)"
    failed=$((failed + 1))
    failures="$failures $spec"
  fi
done

echo "glyph-specs: $total specs, $failed failed"
if [ "$failed" -gt 0 ]; then
  for spec in $failures; do
    if [ -n "${GITHUB_ACTIONS:-}" ]; then
      echo "::error title=Glyphrun spec failed::$spec"
    else
      echo "  failed: $spec"
    fi
  done
  exit 1
fi
