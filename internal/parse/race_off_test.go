//go:build !race

package parse_test

// slowdown scales the wall-clock ceilings in performance tests; see
// race_on_test.go.
const slowdown = 1
