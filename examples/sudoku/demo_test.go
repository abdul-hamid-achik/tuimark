package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
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
		Nodes  []struct {
			ID      string   `json:"id"`
			Classes []string `json:"classes"`
		} `json:"nodes"`
	} `json:"dump"`
}

func (f demoFrame) text() string { return strings.Join(f.Dump.Grid, "\n") }

// cells lists the ids of the cell nodes that carry a class.
func (f demoFrame) withClass(prefix, class string) []string {
	var out []string
	for _, n := range f.Dump.Nodes {
		if !strings.HasPrefix(n.ID, prefix) {
			continue
		}
		for _, c := range n.Classes {
			if c == class {
				out = append(out, n.ID)
			}
		}
	}
	return out
}

func runDemo(t testing.TB, theme string, cols, rows int) ([]byte, []demoFrame) {
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

// demoOnce memoizes runDemo for the tests that only read the frames: the
// demo is a pure function of its arguments, and TestDemo is the one that
// runs it twice to prove so.
var demoMemo = map[string][]demoFrame{}

func demoOnce(t testing.TB, theme string, cols, rows int) []demoFrame {
	t.Helper()
	key := fmt.Sprintf("%s/%d/%d", theme, cols, rows)
	if f, ok := demoMemo[key]; ok {
		return f
	}
	_, f := runDemo(t, theme, cols, rows)
	demoMemo[key] = f
	return f
}

// The demo is the input of the docs site's animated SVG: the same bytes on
// every run, 15-120 frames, 5-12 seconds, every frame a clean dump.
func TestDemo(t *testing.T) {
	for _, theme := range []string{"", "light"} {
		a, frames := runDemo(t, theme, 90, 24)
		demoMemo[fmt.Sprintf("%s/%d/%d", theme, 90, 24)] = frames // for the story test
		if theme == "" {                                          // the second run proves the bytes; the light theme only recolours
			b, _ := runDemo(t, theme, 90, 24)
			if !bytes.Equal(a, b) {
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

var notesRE = regexp.MustCompile(`entering\s+notes`)
var filledRE = regexp.MustCompile(`filled\s+(\d+)/81`)

func filledOf(t testing.TB, f demoFrame) int {
	t.Helper()
	m := filledRE.FindStringSubmatch(f.text())
	if m == nil {
		t.Fatalf("no filled count in\n%s", f.text())
	}
	var n int
	fmt.Sscan(m[1], &n)
	return n
}

// The demo plays a real game: one pencil-marked cell, then every blank
// filled one at a time with the cursor moving, then the completion
// highlight, then the win modal.
func TestDemoStory(t *testing.T) {
	frames := demoOnce(t, "", 90, 24)
	first, last := frames[0], frames[len(frames)-1]
	if got := filledOf(t, first); got != demoClues {
		t.Errorf("the first frame has %d cells filled, want the puzzle's %d", got, demoClues)
	}
	if strings.Contains(first.text(), "solved in") {
		t.Error("the first frame shows the win")
	}
	// Digits go in one at a time and never come out.
	prev, jumps, notes, markers := demoClues, 0, 0, 0
	prevCur := ""
	for i, f := range frames[:len(frames)-2] {
		n := filledOf(t, f)
		if n < prev || n > prev+1 {
			t.Errorf("frame %d: %d cells filled after %d", i, n, prev)
		}
		prev = n
		if cur := f.withClass("c", "cur"); len(cur) == 1 {
			if prevCur != "" && cur[0] != prevCur {
				jumps++
			}
			prevCur = cur[0]
		}
		if notesRE.MatchString(f.text()) {
			notes++
		}
		markers += len(f.withClass("c", "note"))
	}
	if prev != 81 {
		t.Errorf("the board ends with %d cells filled", prev)
	}
	if jumps < 20 {
		t.Errorf("the cursor jumped %d times, want at least 20", jumps)
	}
	if notes < 2 || markers < 2 {
		t.Errorf("notes mode shows in %d frames, note markers in %d: the pencil marks are missing", notes, markers)
	}
	// The frame before the last shows the finished board with every cell
	// highlighted and no modal; the last shows the modal.
	before := frames[len(frames)-2]
	if got := len(before.withClass("c", "done")); got != 81 {
		t.Errorf("%d cells highlighted as done before the modal, want 81", got)
	}
	if strings.Contains(before.text(), "solved in") || !strings.Contains(before.text(), "SOLVED") {
		t.Errorf("the frame before the last:\n%s", before.text())
	}
	for _, want := range []string{"solved in 0:0", "hints used 0", "mistakes 0", "new game", "close"} {
		if !strings.Contains(last.text(), want) {
			t.Errorf("the last frame lacks %q:\n%s", want, last.text())
		}
	}
}

// The script is the logic solver's: every digit is the solution's, so the
// demo never needs a hint and never makes a mistake.
func TestDemoScriptIsRight(t *testing.T) {
	p := generateFloor(demoSeed, levelEasy, demoClues)
	steps, err := demoScript(p)
	if err != nil {
		t.Fatal(err)
	}
	placed := 0
	for _, st := range steps {
		if st.keys[0] == "n" {
			continue // pencil marks
		}
		if p.puz[st.cell] != 0 || len(st.keys) != 1 || st.keys[0] != fmt.Sprint(p.sol[st.cell]) {
			t.Errorf("cell %s gets %v, the solution has %d (given: %d)", cellID(st.cell), st.keys, p.sol[st.cell], p.puz[st.cell])
		}
		placed++
	}
	if placed != size-p.puz.clues() {
		t.Errorf("%d placements for %d blanks", placed, size-p.puz.clues())
	}
	if p.puz.clues() != demoClues || p.level != levelEasy {
		t.Errorf("demo puzzle: %d clues, level %d", p.puz.clues(), p.level)
	}
}

// Other sizes work too: the clicks find the cells wherever the layout
// puts them, and every frame is clean.
func TestDemoOtherSizes(t *testing.T) {
	for _, sz := range [][2]int{{120, 30}, {40, 14}} {
		_, frames := runDemo(t, "", sz[0], sz[1])
		for i, f := range frames {
			if !f.Dump.OK || len(f.Dump.Errors) != 0 {
				t.Fatalf("%dx%d frame %d: %v", sz[0], sz[1], i, f.Dump.Errors)
			}
		}
		if last := frames[len(frames)-1]; !strings.Contains(last.text(), "solved in") {
			t.Errorf("%dx%d: the last frame has no win modal", sz[0], sz[1])
		}
	}
}
