package main

import "testing"

// newApp loads the sample and installs the handlers and derived values,
// as run does.
func newApp(t *testing.T) *app {
	t.Helper()
	a, err := load(".")
	if err != nil {
		t.Fatal(err)
	}
	a.register()
	return a
}
