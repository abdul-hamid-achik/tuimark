#!/bin/sh
# POSIX sh helper for the run_usage_errors glyph spec (SPEC v0.4a §15.9,
# test 121): a --mock naming an unknown action, and an --events path that
# is the spec's own terminal, are usage errors before the terminal is
# touched. Each run's stdout goes to a file, whose size must be 0 (the
# session's first bytes switch to the alternate screen), and its stderr is
# printed after its exit status, with the fixture directory and the
# terminal's path taken out.
#
# Usage: usage.sh /path/to/tuimark
set -eu

tuimark_bin=$1
fx=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

status=0
"$tuimark_bin" run "$fx/mock.tui" --mock "$fx/bad.mock.json" >"$tmp/out1" 2>"$tmp/err1" || status=$?
echo "mock-exit=$status stdout-bytes=$(wc -c <"$tmp/out1" | tr -d ' ')"
# The message names the mock by its path: drop the directory, so that the
# line fits the spec's 120 columns wherever the repository is checked out.
sed "s|$fx/||" "$tmp/err1"

tty_path=$(tty)
status=0
"$tuimark_bin" run "$fx/mock.tui" --events "$tty_path" >"$tmp/out2" 2>"$tmp/err2" || status=$?
echo "tty-exit=$status stdout-bytes=$(wc -c <"$tmp/out2" | tr -d ' ')"
sed "s|$tty_path|TTY|" "$tmp/err2"
echo "usage-done"
