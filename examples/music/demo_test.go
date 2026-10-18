package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// runDemo plays the demo on a fresh app and returns the raw output.
func runDemo(t *testing.T, theme string, cols, rows int) []byte {
	t.Helper()
	a := newApp(t)
	if theme != "" {
		if err := a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := a.demo(&buf, cols, rows); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type frameOut struct {
	Ms   int           `json:"ms"`
	Dump *tuimark.Dump `json:"dump"`
}

func parseDemo(t *testing.T, out []byte) []frameOut {
	t.Helper()
	var frames []frameOut
	for i, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		var f frameOut
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		frames = append(frames, f)
	}
	return frames
}

// vizCells is the grid under the visualizer, as one string per frame.
func vizCells(t *testing.T, f frameOut) string {
	t.Helper()
	for _, n := range f.Dump.Nodes {
		if n.ID == "viz" {
			var rows []string
			for y := n.Y; y < n.Y+n.H; y++ {
				r := []rune(f.Dump.Grid[y])
				rows = append(rows, string(r[n.X:n.X+n.W]))
			}
			return strings.Join(rows, "\n")
		}
	}
	t.Fatal("no visualizer in the frame")
	return ""
}

func TestDemo(t *testing.T) {
	for _, theme := range []string{"", "light"} {
		first := runDemo(t, theme, 100, 24)
		second := runDemo(t, theme, 100, 24)
		if !bytes.Equal(first, second) {
			t.Fatalf("theme %q: two runs gave different bytes", theme)
		}
		frames := parseDemo(t, first)
		if n := len(frames); n < 15 || n > 120 {
			t.Errorf("%d frames, want 15 to 120", n)
		}
		total := 0
		for i, f := range frames {
			total += f.Ms
			if f.Ms <= 0 {
				t.Errorf("frame %d shows for %d ms", i, f.Ms)
			}
			d := f.Dump
			if !d.OK || len(d.Errors) != 0 {
				t.Fatalf("frame %d: ok=%v errors=%v", i, d.OK, d.Errors)
			}
			if d.Wide {
				t.Errorf("frame %d: a wide cluster in the grid", i)
			}
			if len(d.Cells) == 0 || len(d.Styles) != d.Rows {
				t.Fatalf("frame %d: no cells or styles", i)
			}
			if theme == "light" && d.Theme != "light" {
				t.Fatalf("frame %d: theme = %q", i, d.Theme)
			}
		}
		if total < 5000 || total > 12000 {
			t.Errorf("total %d ms, want 5000 to 12000", total)
		}
		if last := frames[len(frames)-1].Ms; last < 1000 || last > 1500 {
			t.Errorf("the last frame holds %d ms, want about 1000 to 1500", last)
		}
	}
}

// The story the demo tells: bars bounce, a skip changes the track, a pause
// lets the bars fall to nothing, a resume brings them back.
func TestDemoStory(t *testing.T) {
	frames := parseDemo(t, runDemo(t, "", 100, 24))
	grid := func(i int) string { return strings.Join(frames[i].Dump.Grid, "\n") }
	empty := func(s string) bool { return strings.Trim(s, " \n") == "" }

	if !strings.Contains(grid(0), "Tessellate") || !strings.Contains(grid(0), "▶") {
		t.Error("frame 0 is not the opening track playing")
	}
	if !strings.Contains(grid(8), "queue 3") {
		t.Error("the queue count did not go up after a")
	}
	if !strings.Contains(grid(30), "Window Seat Aurora") {
		t.Error("n did not skip to the queued track")
	}
	moved := map[string]bool{}
	for i := 10; i < 30; i++ {
		moved[vizCells(t, frames[i])] = true
	}
	if len(moved) < 15 {
		t.Errorf("the visualizer showed only %d distinct pictures in 20 frames", len(moved))
	}
	if empty(vizCells(t, frames[29])) {
		t.Error("no bars while playing")
	}
	if !strings.Contains(grid(53), "⏸") {
		t.Error("the pause icon is not shown after space")
	}
	if !empty(strings.ReplaceAll(vizCells(t, frames[65]), "▁", " ")) {
		t.Errorf("the bars did not fall by the end of the pause:\n%s", vizCells(t, frames[65]))
	}
	if empty(vizCells(t, frames[len(frames)-1])) || !strings.Contains(grid(len(frames)-1), "▶") {
		t.Error("the bars did not come back after resume")
	}
}

// Another size and the light theme render cleanly through the whole script.
func TestDemoOtherSizes(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {120, 30}, {60, 20}} {
		for _, f := range parseDemo(t, runDemo(t, "light", sz[0], sz[1])) {
			if !f.Dump.OK || len(f.Dump.Errors) != 0 {
				t.Fatalf("%dx%d: errors %v", sz[0], sz[1], f.Dump.Errors)
			}
		}
	}
}
