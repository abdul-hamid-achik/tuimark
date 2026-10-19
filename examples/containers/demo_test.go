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
// bytes on every run, between 5 and 16 seconds, and every frame must be a
// clean dump.
func TestDemo(t *testing.T) {
	for _, theme := range []string{"", "light"} {
		first, lines := runDemo(t, 100, 24, theme)
		second, _ := runDemo(t, 100, 24, theme)
		if !bytes.Equal(first, second) {
			t.Fatalf("theme %q: two runs of the demo differ", theme)
		}
		if n := len(lines); n < 30 || n > demoMaxFrames {
			t.Errorf("theme %q: %d frames, want 30 to %d", theme, n, demoMaxFrames)
		}
		total := 0
		for i, l := range lines {
			total += l.MS
			if l.MS <= 0 {
				t.Errorf("frame %d shows for %dms", i, l.MS)
			}
			if !l.Dump.OK || len(l.Dump.Errors) != 0 {
				t.Errorf("theme %q frame %d: ok=%v errors=%v", theme, i, l.Dump.OK, l.Dump.Errors)
			}
			if l.Dump.Cols != 100 || l.Dump.Rows != 24 || len(l.Dump.Cells) == 0 || len(l.Dump.Styles) == 0 {
				t.Errorf("frame %d: not a full 100x24 frame with cells and styles", i)
			}
		}
		if total < 5000 || total > 16000 {
			t.Errorf("theme %q: the demo shows for %dms, want 5s to 16s", theme, total)
		}
	}
}

// The story the docs tell is really on screen: the filter, the dialogs, the
// detail, the crash, and the help.
func TestDemoTellsTheStory(t *testing.T) {
	_, lines := runDemo(t, 100, 24, "")
	var seen []string
	for _, l := range lines {
		seen = append(seen, strings.Join(l.Dump.Grid, "\n"))
	}
	all := strings.Join(seen, "\n--\n")
	for _, want := range []string{
		"Containers · 2 of 12",      // the filter narrowed the table
		"2 selected",                // two marked rows
		"Stop 2 containers?",        // the confirm dialog
		"Stopped api-gateway and b", // its toast
		"Started api-gateway and b", // started again
		"sort cpu",                  // the sort
		"api-gateway · log",         // the detail's log
		"NODE_ENV",                  // the env tab
		"Published ports",           // the ports tab
		"Command palette",           // the palette
		"Restart api-gateway?",      // the dialog it reaches
		"die worker-email",          // the crash in the events
		"DEGRADED",                  // the status bar while it is down
		"close help",                // the help overlay
	} {
		if !strings.Contains(all, want) {
			t.Errorf("no frame shows %q", want)
		}
	}
	last := seen[len(seen)-1]
	if !strings.Contains(last, "Containers · 12 of 12") {
		t.Errorf("the demo should end on the full table:\n%s", last)
	}
}
