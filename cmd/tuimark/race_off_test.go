//go:build !race

package main

// slowdown scales the wall-clock ceilings in the pty tests; see
// race_on_test.go.
const slowdown = 1
