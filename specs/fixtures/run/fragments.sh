#!/bin/sh
# POSIX sh helper for run_reload_fragments (SPEC v0.4b §15.9, test 151).
#
# Copies fragments/tree (test 134's fixture without its two errors, with
# ui/parts/body.tui also including deep.tui, and a ctrl+r → edit keymap row
# for the harness) into a fresh temp dir and runs
# `tuimark run ui/app.tui --data d.json` there, on the spec's terminal.
# Each time the session's --events file gains a record of the action
# "edit", it makes the next edit, every file replaced in one rename so the
# 150 ms poll never reads half a file:
#
#   1. ui/parts/body.tui with omega in place of alpha;
#   2. ui/parts/deep.tui with abyss in place of deep;
#   3. ui/parts/deep.tui as <fragment><text> (not well-formed);
#   4. ui/parts/deep.tui restored (abyss).
#
# Once run exits, the script prints its exit status.
#
# Usage: fragments.sh /path/to/tuimark [run flags...]
set -eu

tuimark_bin=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
shift
fx=$(cd "$(dirname "$0")" && pwd)/fragments
tmp=$(mktemp -d)
editor=
trap 'if [ -n "$editor" ]; then kill "$editor" 2>/dev/null || true; fi; rm -rf "$tmp"' EXIT

cp -R "$fx/tree/." "$tmp/"

# edits prints how many "edit" records the --events file holds.
edits() {
  if [ -f "$tmp/ev.ndjson" ]; then
    grep -c '"action":"edit"' "$tmp/ev.ndjson" || true
  else
    echo 0
  fi
}

# step N SRC DST waits for the Nth edit record, then copies SRC over DST
# (relative to the session's directory) in one rename.
step() {
  until [ "$(edits)" -ge "$1" ]; do sleep 0.05; done
  cp "$2" "$tmp/$3.new"
  mv "$tmp/$3.new" "$tmp/$3"
}

(
  step 1 "$fx/body.omega.tui" ui/parts/body.tui
  step 2 "$fx/deep.abyss.tui" ui/parts/deep.tui
  step 3 "$fx/deep.broken.txt" ui/parts/deep.tui
  step 4 "$fx/deep.abyss.tui" ui/parts/deep.tui
) &
editor=$!

status=0
(cd "$tmp" && "$tuimark_bin" run ui/app.tui --data d.json --events ev.ndjson "$@") || status=$?
echo "run-exit=$status"
echo "session-done"
