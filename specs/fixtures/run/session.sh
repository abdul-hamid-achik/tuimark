#!/bin/sh
# POSIX sh helper for the run_* glyph specs (SPEC v0.4a §15.9, test 121).
#
# Copies the scenario NAME's files into a fresh temp dir and runs
# `tuimark run` on them in the foreground, on the spec's terminal:
#
#   NAME.tui          the document (app.tui)
#   NAME.json         --data (data.json), when it exists
#   NAME.mock.json    --mock (mock.json), when it exists
#   NAME.editN.tui    replaces app.tui once the session's --events file
#   NAME.editN.json   (or data.json) holds N records of the action "edit"
#
# Every file is replaced in one rename, so the 150 ms poll never reads half
# a file. Once run exits, the script prints its exit status, the --events
# file, and that file's mode, for the spec's outcomes to read.
#
# Usage: session.sh /path/to/tuimark NAME [run flags...]
set -eu

tuimark_bin=$1
name=$2
shift 2
fx=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
editor=
trap 'if [ -n "$editor" ]; then kill "$editor" 2>/dev/null || true; fi; rm -rf "$tmp"' EXIT

cp "$fx/$name.tui" "$tmp/app.tui"
set -- --events "$tmp/ev.ndjson" "$@"
if [ -f "$fx/$name.json" ]; then
  cp "$fx/$name.json" "$tmp/data.json"
  set -- --data "$tmp/data.json" "$@"
fi
if [ -f "$fx/$name.mock.json" ]; then
  cp "$fx/$name.mock.json" "$tmp/mock.json"
  set -- --mock "$tmp/mock.json" "$@"
fi

# edits prints how many "edit" records the --events file holds.
edits() {
  if [ -f "$tmp/ev.ndjson" ]; then
    grep -c '"action":"edit"' "$tmp/ev.ndjson" || true
  else
    echo 0
  fi
}

# replace copies $1 over $2 in one rename.
replace() {
  cp "$1" "$2.new"
  mv "$2.new" "$2"
}

if [ -f "$fx/$name.edit1.tui" ] || [ -f "$fx/$name.edit1.json" ]; then
  (
    n=1
    while [ -f "$fx/$name.edit$n.tui" ] || [ -f "$fx/$name.edit$n.json" ]; do
      until [ "$(edits)" -ge "$n" ]; do sleep 0.05; done
      if [ -f "$fx/$name.edit$n.tui" ]; then replace "$fx/$name.edit$n.tui" "$tmp/app.tui"; fi
      if [ -f "$fx/$name.edit$n.json" ]; then replace "$fx/$name.edit$n.json" "$tmp/data.json"; fi
      n=$((n + 1))
    done
  ) &
  editor=$!
fi

status=0
"$tuimark_bin" run "$tmp/app.tui" "$@" || status=$?
echo "run-exit=$status"
if [ -f "$tmp/ev.ndjson" ]; then
  cat "$tmp/ev.ndjson"
  echo "events-mode=$(ls -l "$tmp/ev.ndjson" | cut -c1-10)"
fi
echo "session-done"
