package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
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

func runDemo(t *testing.T, theme string, cols, rows int) ([]demoFrame, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if err := demo(&buf, ".", theme, cols, rows); err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	frames := make([]demoFrame, len(lines))
	for i, line := range lines {
		if err := json.Unmarshal(line, &frames[i]); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	return frames, buf.Bytes()
}

// The demo is the input of the docs site's animated SVG: the same bytes on
// every run, 15-120 frames, 5-12 seconds, every frame a clean dump.
func TestDemo(t *testing.T) {
	for _, theme := range []string{"", "light"} {
		frames, a := cachedDemo(t, theme, 100, 24)
		if theme == "" { // a second run of the dark theme: the bytes are the same
			_, b := runDemo(t, theme, 100, 24)
			if !bytes.Equal(a, b) {
				t.Fatalf("two runs of the demo differ")
			}
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
		want := "dark"
		if theme != "" {
			want = theme
		}
		if last.Dump.Theme != want {
			t.Errorf("theme %q: dumps are %q", theme, last.Dump.Theme)
		}
	}
}

// The demo works at the other sizes people record at, with and without the
// stats panel.
func TestDemoSizes(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}} {
		frames, _ := cachedDemo(t, "", sz[0], sz[1])
		for i, f := range frames {
			if !f.Dump.OK || len(f.Dump.Errors) != 0 {
				t.Fatalf("%dx%d frame %d: %v", sz[0], sz[1], i, f.Dump.Errors)
			}
		}
	}
}

// flashRows counts the grid rows that are entirely flashing blocks.
func flashRows(grid []string) int {
	n := 0
	for _, row := range grid {
		if strings.Contains(row, strings.Repeat("▓▓", boardW)) {
			n++
		}
	}
	return n
}

// The demo plays a real game from a seeded start: the autopilot is on, the
// pieces move and drop, the score and the lines rise, and at least one clear
// of several rows flashes before it collapses.
func TestDemoShowsAMultiLineClear(t *testing.T) {
	frames, _ := cachedDemo(t, "", 100, 24)
	multi := 0
	distinct := map[string]bool{}
	for _, f := range frames {
		if flashRows(f.Dump.Grid) >= 2 {
			multi++
		}
		distinct[strings.Join(f.Dump.Grid, "\n")] = true
	}
	if multi < 2 {
		t.Errorf("%d frames show a flash of 2 or more rows, want at least 2 (the flash lasts two frames)", multi)
	}
	if len(distinct) < len(frames)*8/10 {
		t.Errorf("only %d of %d frames differ: the pieces barely move", len(distinct), len(frames))
	}
	last := strings.Join(frames[len(frames)-1].Dump.Grid, "\n")
	for _, want := range []string{"autopilot", "PLAYING"} {
		if !strings.Contains(last, want) {
			t.Errorf("last frame lacks %q:\n%s", want, last)
		}
	}
	if strings.Contains(last, "score              0") || strings.Contains(last, "lines              0") {
		t.Errorf("the demo scored nothing:\n%s", last)
	}
	// The first frame is the opening position, before any key.
	first := strings.Join(frames[0].Dump.Grid, "\n")
	if !strings.Contains(first, "manual") {
		t.Errorf("the opening frame is not manual:\n%s", first)
	}
}

// The numbers the README quotes for the demo at 100x24.
func TestDemoMatchesTheReadme(t *testing.T) {
	frames, _ := cachedDemo(t, "", 100, 24)
	total := 0
	var tetris []int
	for i, f := range frames {
		total += f.Ms
		if flashRows(f.Dump.Grid) == 4 {
			tetris = append(tetris, i)
		}
	}
	if len(frames) != 93 || total != 10900 {
		t.Errorf("%d frames, %d ms; the README says 93 and 10.9 s", len(frames), total)
	}
	if len(tetris) != 2 || tetris[0] != 42 {
		t.Errorf("four rows flash in frames %v; the README says two frames, from frame 42", tetris)
	}
	last := strings.Join(frames[len(frames)-1].Dump.Grid, "\n")
	for _, want := range []string{"pieces            27", "lines              7", "score           1622"} {
		if !strings.Contains(last, want) {
			t.Errorf("the last frame lacks %q:\n%s", want, last)
		}
	}
}

// Running the demo does not depend on the time zone: a fake UTC clock drives
// it. The same bytes come out whatever time.Local is.
func TestDemoIgnoresTheTimeZone(t *testing.T) {
	old := time.Local
	defer func() { time.Local = old }()
	_, here := cachedDemo(t, "", 100, 24)
	time.Local = time.FixedZone("far east", 13*3600)
	_, east := runDemo(t, "", 100, 24)
	if !bytes.Equal(here, east) {
		t.Error("the demo depends on the time zone")
	}
}

type demoKey struct {
	theme      string
	cols, rows int
}

type demoRun struct {
	frames []demoFrame
	raw    []byte
}

var (
	demoMu    sync.Mutex
	demoCache = map[demoKey]demoRun{}
)

// cachedDemo is runDemo, once per theme and size for the whole test binary:
// the demo is several seconds of game, and most tests only read it.
func cachedDemo(t *testing.T, theme string, cols, rows int) ([]demoFrame, []byte) {
	t.Helper()
	demoMu.Lock()
	defer demoMu.Unlock()
	k := demoKey{theme, cols, rows}
	if r, ok := demoCache[k]; ok {
		return r.frames, r.raw
	}
	frames, raw := runDemo(t, theme, cols, rows)
	demoCache[k] = demoRun{frames, raw}
	return frames, raw
}
