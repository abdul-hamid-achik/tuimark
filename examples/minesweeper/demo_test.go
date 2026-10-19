package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

type demoFrame struct {
	Ms   int `json:"ms"`
	Dump struct {
		OK     bool   `json:"ok"`
		Errors []any  `json:"errors"`
		Theme  string `json:"theme"`
		Grid   []string
	} `json:"dump"`
}

type demoKey struct {
	theme      string
	cols, rows int
}

var demoCache = map[demoKey]struct {
	out    []byte
	frames []demoFrame
}{}

// runDemo is the demo's output at a size, from the first run of it; the
// tests that only read the frames share one.
func runDemo(t *testing.T, theme string, cols, rows int) ([]byte, []demoFrame) {
	t.Helper()
	k := demoKey{theme, cols, rows}
	if c, ok := demoCache[k]; ok {
		return c.out, c.frames
	}
	out, frames := runDemoFresh(t, theme, cols, rows)
	demoCache[k] = struct {
		out    []byte
		frames []demoFrame
	}{out, frames}
	return out, frames
}

func runDemoFresh(t *testing.T, theme string, cols, rows int) ([]byte, []demoFrame) {
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
		if theme == "" { // two runs are compared once; the light theme is the same code
			if b, _ := runDemoFresh(t, theme, 90, 24); !bytes.Equal(a, b) {
				t.Fatalf("theme %q: two runs of the demo differ", theme)
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
		last := frames[len(frames)-1]
		if total < 5000 || total > 12000 {
			t.Errorf("theme %q: %d ms in total, want 5000 to 12000", theme, total)
		}
		if last.Ms < 1000 || last.Ms > 1500 {
			t.Errorf("theme %q: the last frame is held %d ms, want 1000 to 1500", theme, last.Ms)
		}
		want := "dark"
		if theme != "" {
			want = theme
		}
		for i, f := range frames {
			if f.Dump.Theme != want {
				t.Fatalf("frame %d is %q, want %q", i, f.Dump.Theme, want)
			}
		}
	}
}

// The demo plays a real game and ends on the win, with the time, and the
// frames before it show what the solver does: a flag mode, flags, chords.
func TestDemoPlaysAndWins(t *testing.T) {
	_, frames := runDemo(t, "", 90, 24)
	first := strings.Join(frames[0].Dump.Grid, "\n")
	for _, want := range []string{"ready", "0:00", "intermediate 16x16, 40 mines"} {
		if !strings.Contains(first, want) {
			t.Errorf("the first frame lacks %q:\n%s", want, first)
		}
	}
	if strings.Count(first, glyphHidden) != 16*16 {
		t.Errorf("the first frame does not show a covered board")
	}
	lastGrid := strings.Join(frames[len(frames)-1].Dump.Grid, "\n")
	m := regexp.MustCompile(`B\) cleared[\s\S]*?mines left\s+000[\s\S]*?time\s+(\d+:\d\d)`).FindStringSubmatch(lastGrid)
	if m == nil {
		t.Fatalf("the last frame is not the win:\n%s", lastGrid)
	}
	if strings.Contains(lastGrid, glyphHidden) {
		t.Errorf("covered cells are left in the won game:\n%s", lastGrid)
	}
	// The win takes one clock second per move after the first click, and the
	// best time of the level now shows it.
	if !strings.Contains(lastGrid, "intermediate        "+m[1]) && !regexp.MustCompile(`intermediate\s+`+m[1]).MatchString(lastGrid) {
		t.Errorf("the best time %s is not in the panel:\n%s", m[1], lastGrid)
	}
	var sawFlagMode, sawReveal, sawFlag, sawSomeNumbers bool
	for _, f := range frames {
		g := strings.Join(f.Dump.Grid, "\n")
		sawFlagMode = sawFlagMode || regexp.MustCompile(`a click\s+flag`).MatchString(g)
		sawReveal = sawReveal || regexp.MustCompile(`a click\s+reveal`).MatchString(g)
		sawFlag = sawFlag || strings.Contains(g, glyphFlag)
		sawSomeNumbers = sawSomeNumbers || strings.Contains(g, " 3")
	}
	if !sawFlagMode || !sawReveal || !sawFlag || !sawSomeNumbers {
		t.Errorf("the demo does not show flag mode (%v), reveal mode (%v), flags (%v), and numbers (%v)", sawFlagMode, sawReveal, sawFlag, sawSomeNumbers)
	}
	// The timer moves frame by frame.
	times := map[string]bool{}
	for _, f := range frames {
		if m := regexp.MustCompile(`time\s+(\d+:\d\d)`).FindStringSubmatch(strings.Join(f.Dump.Grid, "\n")); m != nil {
			times[m[1]] = true
		}
	}
	if len(times) < 20 {
		t.Errorf("the timer showed only %d different times", len(times))
	}
}

// The seed the demo uses is one on which the solver wins; if a rules change
// moves the mines, DEMO_SEARCH=1 go test -run TestDemoSeedSearch lists the
// seeds that win.
func TestDemoSeedWins(t *testing.T) {
	r := newRigOn(t, demoBoard, demoSeed)
	err := demoSteps(r.a, func(d time.Duration) { r.now = r.now.Add(d) }, tuimark.PlayOptions{Cols: 90, Rows: 24}, func(*tuimark.PlayResult) error { return nil })
	if err != nil || !r.a.g.won {
		t.Fatalf("seed %d: err=%v won=%v", demoSeed, err, r.a.g.won)
	}
}

func TestDemoSeedSearch(t *testing.T) {
	if os.Getenv("DEMO_SEARCH") == "" {
		t.Skip("set DEMO_SEARCH=1 to list the seeds the solver wins")
	}
	for seed := int64(1); seed <= 60; seed++ {
		r := newRigOn(t, demoBoard, seed)
		n := 0
		err := demoSteps(r.a, func(d time.Duration) { r.now = r.now.Add(d) }, tuimark.PlayOptions{Cols: 90, Rows: 24}, func(*tuimark.PlayResult) error { n++; return nil })
		fmt.Printf("seed %d: moves %d won %v err %v\n", seed, n, r.a.g.won, err)
	}
}

// Other terminal sizes work too: the demo needs the board on the screen.
func TestDemoAtOtherSizes(t *testing.T) {
	// The smallest terminal the demo accepts (stacked layout), not the 90x24 of the docs.
	for _, sz := range [][2]int{{needCols(demoBoard.cols), needRows(demoBoard.rows)}} {
		_, frames := runDemo(t, "", sz[0], sz[1])
		for i, f := range frames {
			if !f.Dump.OK || len(f.Dump.Errors) != 0 {
				t.Fatalf("%dx%d frame %d: %v", sz[0], sz[1], i, f.Dump.Errors)
			}
		}
	}
}
