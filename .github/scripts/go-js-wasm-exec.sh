#!/usr/bin/env bash
# `go test -exec` wrapper for GOOS=js GOARCH=wasm: Go's own go_js_wasm_exec
# with a minimal environment. wasm_exec.js aborts when argv plus the
# environment exceed its limit (8 KiB), which a developer shell or a CI
# runner easily does; a test run needs none of it.
#
# Use:  GOOS=js GOARCH=wasm go test -exec "$PWD/.github/scripts/go-js-wasm-exec.sh" ./internal/...
# (or task wasm-test)
set -euo pipefail
goroot="$(go env GOROOT)"
# Go 1.24 moved the helpers from misc/wasm to lib/wasm.
dir="$goroot/lib/wasm"
[ -d "$dir" ] || dir="$goroot/misc/wasm"
exec env -i PATH="$PATH" HOME="${HOME:-/tmp}" TMPDIR="${TMPDIR:-/tmp}" \
  node --stack-size=8192 "$dir/wasm_exec_node.js" "$@"
