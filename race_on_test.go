//go:build race

package tuimark_test

// slowdown scales the wall-clock ceilings of the expansion-limit tests, as
// internal/parse's race_on_test.go does for its own: the race detector
// makes loading a large document several times slower, and `go test -race
// ./...` runs every package's tests at once.
const slowdown = 5
