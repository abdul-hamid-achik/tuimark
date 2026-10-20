//go:build !race

package tuimark_test

// slowdown is 1 without the race detector (see race_on_test.go).
const slowdown = 1
