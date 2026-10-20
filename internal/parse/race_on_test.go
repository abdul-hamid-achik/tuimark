//go:build race

package parse_test

// slowdown scales the wall-clock ceilings in performance tests, as
// internal/host's race_on_test.go does for its own: the race detector
// makes the parser several times slower, and `go test -race ./...` runs
// every package's tests at once.
const slowdown = 5
