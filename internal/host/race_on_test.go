//go:build race

package host

// slowdown scales the wall-clock ceilings in performance tests: the race
// detector makes this code 5-10x slower, and CI runs every package's
// race-instrumented tests at once on a two-core runner.
const slowdown = 5
