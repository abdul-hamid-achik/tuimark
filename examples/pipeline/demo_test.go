package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type demoLine struct {
	MS   int `json:"ms"`
	Dump struct {
		Cols   int              `json:"cols"`
		Rows   int              `json:"rows"`
		OK     bool             `json:"ok"`
		Errors []map[string]any `json:"errors"`
		Grid   []string         `json:"grid"`
		Cells  []map[string]any `json:"cells"`
		Styles [][]any          `json:"styles"`
		Theme  string           `json:"theme"`
	} `json:"dump"`
}

func runDemo(t *testing.T, cols, rows int, theme string) ([]byte, []demoLine) {
	t.Helper()
	var out bytes.Buffer
	if err := demo(&out, ".", cols, rows, theme); err != nil {
		t.Fatal(err)
	}
	var lines []demoLine
	for _, raw := range bytes.Split(bytes.TrimRight(out.Bytes(), "\n"), []byte("\n")) {
		var l demoLine
		if err := json.Unmarshal(raw, &l); err != nil {
			t.Fatalf("not a JSON line: %v\n%.200s", err, raw)
		}
		lines = append(lines, l)
	}
	return out.Bytes(), lines
}

// The docs site turns the demo into an animated SVG: it must be the same
// bytes on every run, between 5 and 12 seconds, and every frame must be a
// clean dump.
func TestDemo(t *testing.T) {
	for _, theme := range []string{"", "light"} {
		first, lines := runDemo(t, 100, 24, theme)
		second, _ := runDemo(t, 100, 24, theme)
		if !bytes.Equal(first, second) {
			t.Fatalf("theme %q: two runs of the demo differ", theme)
		}
		if n := len(lines); n < 15 || n > 120 {
			t.Errorf("theme %q: %d frames, want 15 to 120", theme, n)
		}
		total := 0
		for i, l := range lines {
			total += l.MS
			if l.MS <= 0 {
				t.Errorf("frame %d shows for %dms", i, l.MS)
			}
			if !l.Dump.OK || len(l.Dump.Errors) != 0 {
				t.Fatalf("frame %d: ok=%v errors=%v", i, l.Dump.OK, l.Dump.Errors)
			}
			if l.Dump.Cols != 100 || l.Dump.Rows != 24 || len(l.Dump.Grid) != 24 {
				t.Fatalf("frame %d is %dx%d with %d rows", i, l.Dump.Cols, l.Dump.Rows, len(l.Dump.Grid))
			}
			if len(l.Dump.Cells) == 0 || len(l.Dump.Styles) == 0 {
				t.Fatalf("frame %d has no cells or styles", i)
			}
		}
		if total < 5000 || total > 12000 {
			t.Errorf("theme %q: total %dms, want 5000 to 12000", theme, total)
		}
		if last := lines[len(lines)-1]; last.MS < 1000 || last.MS > 1500 {
			t.Errorf("the last frame holds %dms, want about 1 to 1.5 s", last.MS)
		}
		want := "dark"
		if theme == "light" {
			want = "light"
		}
		if got := lines[0].Dump.Theme; got != want {
			t.Errorf("theme %q: frames are %q", theme, got)
		}
	}
}

// The script tells the story the README promises: the flaky test fails, r
// retries it, and the run ends green with deploy done.
func TestDemoTellsTheStory(t *testing.T) {
	_, lines := runDemo(t, 100, 24, "")
	var sawFail, sawRetry, sawSpin bool
	for _, l := range lines {
		g := strings.Join(l.Dump.Grid, "\n")
		for _, row := range l.Dump.Grid {
			sawFail = sawFail || (strings.Contains(row, "✗") && strings.Contains(row, "integration") && strings.Contains(row, "│ ✗"))
		}
		sawRetry = sawRetry || strings.Contains(g, "Retrying test/integration")
		sawSpin = sawSpin || strings.ContainsAny(g, strings.Join(spinner, ""))
	}
	if !sawFail || !sawRetry || !sawSpin {
		t.Errorf("fail=%v retry=%v spinner=%v", sawFail, sawRetry, sawSpin)
	}
	last := strings.Join(lines[len(lines)-1].Dump.Grid, "\n")
	for _, want := range []string{"✓ PASSED", "100%", "Run #1 passed"} {
		if !strings.Contains(last, want) {
			t.Errorf("the last frame has no %q:\n%s", want, last)
		}
	}
	first := strings.Join(lines[0].Dump.Grid, "\n")
	if !strings.Contains(first, "QUEUED") {
		t.Errorf("the first frame is not the queued run:\n%s", first)
	}
}

func TestDemoOtherSizes(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {120, 30}} {
		_, lines := runDemo(t, sz[0], sz[1], "")
		for i, l := range lines {
			if !l.Dump.OK || len(l.Dump.Errors) != 0 {
				t.Fatalf("%dx%d frame %d: %v", sz[0], sz[1], i, l.Dump.Errors)
			}
		}
	}
}
