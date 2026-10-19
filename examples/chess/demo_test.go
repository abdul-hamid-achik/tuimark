package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type demoFrame struct {
	Ms   int `json:"ms"`
	Dump struct {
		OK     bool     `json:"ok"`
		Errors []any    `json:"errors"`
		Theme  string   `json:"theme"`
		Wide   bool     `json:"wide"`
		Grid   []string `json:"grid"`
	} `json:"dump"`
}

func demoFrames(t *testing.T, theme string, cols, rows int) ([]demoFrame, []byte) {
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
		frames, a := demoFrames(t, theme, 100, 24)
		_, b := demoFrames(t, theme, 100, 24)
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
			if f.Dump.Wide {
				t.Errorf("frame %d is wide", i)
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

// The demo plays the Opera Game: a frame for the start and one for each of
// the 33 plies, the move list and the board advancing, ending on Rd8# with
// checkmate and 1–0.
func TestDemoPlaysTheOperaGame(t *testing.T) {
	frames, _ := demoFrames(t, "", 100, 24)
	moves := operaMoves()
	if len(frames) != len(moves)+1 {
		t.Fatalf("%d frames, want %d", len(frames), len(moves)+1)
	}
	if first := strings.Join(frames[0].Dump.Grid, "\n"); !strings.Contains(first, "White to move") || strings.Contains(movesBox(frames[0]), "1.") {
		t.Errorf("the first frame is not the start:\n%s", first)
	}
	// Each frame's move list ends with the move just played, and the board
	// differs from the frame before.
	for i, san := range moves {
		g := strings.Join(frames[i+1].Dump.Grid, "\n")
		if !strings.Contains(movesBox(frames[i+1]), san) {
			t.Errorf("frame %d: the move list lacks %s:\n%s", i+1, san, g)
		}
		if g == strings.Join(frames[i].Dump.Grid, "\n") {
			t.Errorf("frame %d is the same as the one before", i+1)
		}
	}
	last := strings.Join(frames[len(frames)-1].Dump.Grid, "\n")
	for _, want := range []string{"Checkmate · 1–0", "White wins", "17.", "Rd8#", "Morphy", "Duke & Count", "eval", "1–0"} {
		if !strings.Contains(last, want) {
			t.Errorf("the last frame lacks %q:\n%s", want, last)
		}
	}
	// The evaluation bar moves: it is not the same in every frame.
	bars := map[string]bool{}
	for _, f := range frames {
		for _, l := range f.Dump.Grid {
			if i := strings.Index(l, "eval "); i >= 0 {
				bars[l[i:]] = true
			}
		}
	}
	if len(bars) < 8 {
		t.Errorf("the evaluation bar took only %d different states", len(bars))
	}
}

// movesBox is the text right of the board: the side panel.
func movesBox(f demoFrame) string {
	var b strings.Builder
	for _, l := range f.Dump.Grid {
		if r := []rune(l); len(r) > 45 {
			b.WriteString(string(r[45:]) + "\n")
		}
	}
	return b.String()
}

// The demo is clean at other sizes too (the docs site may pick one).
func TestDemoAtOtherSizes(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {120, 30}} {
		frames, _ := demoFrames(t, "", sz[0], sz[1])
		for i, f := range frames {
			if !f.Dump.OK || len(f.Dump.Errors) != 0 || f.Dump.Wide {
				t.Fatalf("%dx%d frame %d: ok=%v errors=%v wide=%v", sz[0], sz[1], i, f.Dump.OK, f.Dump.Errors, f.Dump.Wide)
			}
		}
	}
}
