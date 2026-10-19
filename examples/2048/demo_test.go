package main

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

type demoFrame struct {
	Ms   int `json:"ms"`
	Dump struct {
		OK     bool     `json:"ok"`
		Errors []any    `json:"errors"`
		Theme  string   `json:"theme"`
		Grid   []string `json:"grid"`
	} `json:"dump"`
}

func runDemo(t *testing.T, theme string, cols, rows int) ([]byte, []demoFrame) {
	t.Helper()
	var buf bytes.Buffer
	if err := demo(&buf, ".", theme, cols, rows); err != nil {
		t.Fatal(err)
	}
	var frames []demoFrame
	for i, line := range bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n")) {
		var f demoFrame
		if err := json.Unmarshal(line, &f); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		frames = append(frames, f)
	}
	return buf.Bytes(), frames
}

// The demo is the input of the docs site's animated SVG: the same bytes on
// every run, 15-120 frames, 5-12 seconds, every frame a clean dump.
func TestDemo(t *testing.T) {
	for _, theme := range []string{"", "light"} {
		a, frames := runDemo(t, theme, 90, 24)
		b, _ := runDemo(t, theme, 90, 24)
		if !bytes.Equal(a, b) {
			t.Fatalf("theme %q: two runs of the demo differ", theme)
		}
		if len(frames) < 15 || len(frames) > 120 {
			t.Errorf("theme %q: %d frames, want 15 to 120", theme, len(frames))
		}
		total := 0
		for i, f := range frames {
			if !f.Dump.OK || len(f.Dump.Errors) != 0 {
				t.Errorf("frame %d: ok=%v errors=%v", i, f.Dump.OK, f.Dump.Errors)
			}
			if f.Ms <= 0 {
				t.Errorf("frame %d shows for %d ms", i, f.Ms)
			}
			total += f.Ms
		}
		if total < 5000 || total > 12000 {
			t.Errorf("theme %q: %d ms in total, want 5000 to 12000", theme, total)
		}
		last := frames[len(frames)-1]
		if last.Ms < 1000 || last.Ms > 1500 {
			t.Errorf("theme %q: the last frame is held %d ms, want 1000 to 1500", theme, last.Ms)
		}
		wantTheme := "dark"
		if theme != "" {
			wantTheme = theme
		}
		if last.Dump.Theme != wantTheme {
			t.Errorf("theme %q: dumps are %q", theme, last.Dump.Theme)
		}
	}
}

// panelInt reads the number after label in the side panel of a frame.
func panelInt(f demoFrame, label string) int {
	re := regexp.MustCompile(label + `\s+(\d+)\s+│`)
	for _, line := range f.Dump.Grid {
		if m := re.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			return n
		}
	}
	return -1
}

// The demo plays a real game: the score only climbs from frame to frame
// and ends well above 0, a tile of 256 or more is reached, the autopilot
// plays in the middle, and the last frame is the board handed back.
func TestDemoPlaysAGame(t *testing.T) {
	_, frames := runDemo(t, "", 90, 24)
	prev := 0
	for i, f := range frames {
		s := panelInt(f, "score")
		if s < prev {
			t.Fatalf("frame %d: the score fell from %d to %d", i, prev, s)
		}
		prev = s
	}
	first, last := frames[0], frames[len(frames)-1]
	if panelInt(first, "score") != 0 || panelInt(first, "moves") != 0 {
		t.Error("the first frame is not the opening board")
	}
	if got := panelInt(last, "largest tile"); got < 256 {
		t.Errorf("the largest tile at the end is %d, want 256 or more", got)
	}
	if got := panelInt(last, "score"); got < 1000 {
		t.Errorf("the final score is %d, want 1000 or more", got)
	}
	if got := panelInt(last, "moves"); got < 100 {
		t.Errorf("the demo played %d moves, want 100 or more", got)
	}
	g := strings.Join(last.Dump.Grid, "\n")
	if !strings.Contains(g, "manual") || !strings.Contains(g, "PLAYING") {
		t.Errorf("the last frame is not the board handed back:\n%s", g)
	}
	if mid := strings.Join(frames[len(frames)/2].Dump.Grid, "\n"); !strings.Contains(mid, "autopilot") {
		t.Errorf("the middle of the demo is not the autopilot playing:\n%s", mid)
	}
}

// The demo shows the pop and new-tile highlights: the cells it paints with
// them (reverse for a merged tile, the new-tile colour) are in the frames.
func TestDemoShowsHighlights(t *testing.T) {
	var buf bytes.Buffer
	if err := demo(&buf, ".", "", 90, 24); err != nil {
		t.Fatal(err)
	}
	type styled struct {
		Dump struct {
			Styles [][]struct {
				BG string   `json:"bg"`
				A  []string `json:"a"`
			} `json:"styles"`
		} `json:"dump"`
	}
	reversed, fresh, frames := 0, 0, 0
	for _, line := range bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n")) {
		var f styled
		if err := json.Unmarshal(line, &f); err != nil {
			t.Fatal(err)
		}
		frames++
		hasRev, hasFresh := false, false
		for _, row := range f.Dump.Styles {
			for _, sp := range row {
				if sp.BG == "#58a6ff" {
					hasFresh = true
				}
				for _, a := range sp.A {
					if a == "reverse" {
						hasRev = true
					}
				}
			}
		}
		if hasRev {
			reversed++
		}
		if hasFresh {
			fresh++
		}
	}
	if fresh < frames/2 {
		t.Errorf("a new tile is highlighted in %d of %d frames, want most", fresh, frames)
	}
	if reversed < frames/3 {
		t.Errorf("a merged tile is highlighted in %d of %d frames, want a third", reversed, frames)
	}
}

// 80x24 and 60x20 play the same demo cleanly too.
func TestDemoIsCleanAtSmallerSizes(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {60, 20}, {100, 30}} {
		_, frames := runDemo(t, "", sz[0], sz[1])
		for i, f := range frames {
			if !f.Dump.OK || len(f.Dump.Errors) != 0 {
				t.Fatalf("%dx%d frame %d: %v", sz[0], sz[1], i, f.Dump.Errors)
			}
		}
	}
}
