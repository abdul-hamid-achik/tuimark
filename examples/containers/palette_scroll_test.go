package main

import (
	"strings"
	"testing"
)

// The palette has fifteen commands and room for ten rows: moving the cursor
// down must scroll the list so the last command can be reached and run.
func TestPaletteListScrollsToTheCursor(t *testing.T) {
	r := newRig(t)
	r.advance(sampleSecs)
	steps := []string{"ctrl+k"}
	for i := 0; i < 14; i++ {
		steps = append(steps, "down")
	}
	res := r.play(100, 24, steps...)
	if r.str("palette.cur") != "help" {
		t.Fatalf("palette.cur = %q after 14 downs, want help", r.str("palette.cur"))
	}
	if !strings.Contains(gridOf(res), "Show the keys") {
		t.Errorf("the last command should have scrolled into view:\n%s", gridOf(res))
	}
	res = r.play(100, 24, "enter")
	if !r.boolean("help.open") || !strings.Contains(gridOf(res), "close help") {
		t.Errorf("enter on the last command should open the help; help.open=%v", r.boolean("help.open"))
	}
}
