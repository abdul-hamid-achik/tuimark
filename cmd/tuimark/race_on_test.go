//go:build race

package main

// slowdown scales the wall-clock ceilings of the host protocol's pty tests
// (host_pty_test.go), as internal/host's race_on_test.go does for its own:
// the race detector makes the child 5-10x slower, and CI runs every
// package's race-instrumented tests at once on a two-core runner.
const slowdown = 5
