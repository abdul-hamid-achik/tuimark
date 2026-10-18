package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

type frameOut struct {
	Ms   int           `json:"ms"`
	Dump *tuimark.Dump `json:"dump"`
}

func demoBytes(t *testing.T, size, theme string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := runDemo(options{dir: ".", demo: size, theme: theme}, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func parseDemo(t *testing.T, raw []byte) []frameOut {
	t.Helper()
	var frames []frameOut
	for _, line := range bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n")) {
		var f frameOut
		if err := json.Unmarshal(line, &f); err != nil {
			t.Fatalf("a demo line is not JSON: %v", err)
		}
		frames = append(frames, f)
	}
	return frames
}

// cellLooks is each cell's character and style of a dump, flattened.
func cellLooks(d *tuimark.Dump) []string {
	out := make([]string, 0, d.Cols*d.Rows)
	for y := 0; y < d.Rows; y++ {
		row := []rune(d.Grid[y])
		x := 0
		for _, sp := range d.Styles[y] {
			for ; x < sp.X+sp.W && x < len(row); x++ {
				out = append(out, string(row[x])+"|"+sp.FG+"|"+sp.BG+"|"+strings.Join(sp.A, ","))
			}
		}
		for ; x < len(row); x++ {
			out = append(out, string(row[x]))
		}
	}
	return out
}

func TestDemo(t *testing.T) {
	for _, theme := range []string{"", "light"} {
		a := demoBytes(t, "90x24", theme)
		b := demoBytes(t, "90x24", theme)
		if !bytes.Equal(a, b) {
			t.Fatalf("theme %q: two runs differ", theme)
		}
		frames := parseDemo(t, a)
		if n := len(frames); n < 15 || n > 120 {
			t.Errorf("theme %q: %d frames, want 15-120", theme, n)
		}
		total := 0
		for i, f := range frames {
			total += f.Ms
			if f.Ms <= 0 {
				t.Errorf("frame %d shows for %d ms", i, f.Ms)
			}
			d := f.Dump
			if !d.OK || len(d.Errors) != 0 {
				t.Errorf("theme %q frame %d: ok=%v errors=%v", theme, i, d.OK, d.Errors)
			}
			if d.Cols != 90 || d.Rows != 24 || len(d.Cells) != 90*24 || len(d.Styles) != 24 {
				t.Errorf("frame %d: %dx%d with %d cells, %d style rows", i, d.Cols, d.Rows, len(d.Cells), len(d.Styles))
			}
		}
		if total < 5000 || total > 12000 {
			t.Errorf("theme %q: the demo lasts %d ms, want 5000-12000", theme, total)
		}
		if last := frames[len(frames)-1]; last.Ms < 1000 || last.Ms > 1500 {
			t.Errorf("the last frame holds %d ms, want 1000-1500", last.Ms)
		}
		if wantTheme := map[string]string{"": "dark", "light": "light"}[theme]; frames[0].Dump.Theme != wantTheme {
			t.Errorf("theme %q: the dump's theme is %q", theme, frames[0].Dump.Theme)
		}
	}
}

// The demo tells its story: the last seconds of a work phase, the done
// flash, then the short break counting.
func TestDemoStory(t *testing.T) {
	frames := parseDemo(t, demoBytes(t, "90x24", ""))
	grid := func(i int) string { return strings.Join(frames[i].Dump.Grid, "\n") }
	first, last := grid(0), grid(len(frames)-1)
	if !strings.Contains(first, "FOCUS") || !strings.Contains(first, "RUNNING") {
		t.Errorf("the first frame is not a running work phase:\n%s", first)
	}
	if !strings.Contains(last, "SHORT BREAK") || strings.Contains(last, "FOCUS") {
		t.Errorf("the last frame is not the short break:\n%s", last)
	}
	flashes := 0
	for i := range frames {
		if strings.Contains(grid(i), "DONE") {
			flashes++
		}
	}
	if flashes != demoFlash {
		t.Errorf("%d frames show DONE, want %d", flashes, demoFlash)
	}
	// The break's clock starts at 05:00 and moves.
	if !strings.Contains(grid(len(frames)-demoBreak), "▄████▄ ██████    ▄████▄ ▄████▄") {
		t.Errorf("the first break frame does not read 05:00:\n%s", grid(len(frames)-demoBreak))
	}
	// The colon blinks and the last seconds pulse: consecutive countdown
	// frames differ in both.
	prevColon := ""
	for i := 0; i < demoCountdown; i++ {
		g := strings.Split(grid(i), "\n")[4]
		c := string([]rune(g)[44:46]) // row 1 of the digits (screen row 4): the colon columns
		if i > 0 && c == prevColon {
			t.Errorf("frame %d: the colon did not blink", i)
		}
		prevColon = c
	}
}

// Per-frame changes stay modest: the docs site stores each changed run.
func TestDemoChangesAreModest(t *testing.T) {
	frames := parseDemo(t, demoBytes(t, "90x24", ""))
	max := 0
	for i := 1; i < len(frames); i++ {
		a, b := cellLooks(frames[i-1].Dump), cellLooks(frames[i].Dump)
		n := 0
		for j := range a {
			if a[j] != b[j] {
				n++
			}
		}
		if n > max {
			max = n
		}
	}
	t.Logf("most cells changed between two frames: %d of %d", max, 90*24)
	if max > 600 {
		t.Errorf("%d cells change between two frames, want a modest number", max)
	}
}

func TestDemoSizes(t *testing.T) {
	for _, sz := range []string{"80x24", "120x30", "60x20"} {
		frames := parseDemo(t, demoBytes(t, sz, ""))
		for i, f := range frames {
			if !f.Dump.OK || len(f.Dump.Errors) != 0 {
				t.Errorf("%s frame %d: %v", sz, i, f.Dump.Errors)
			}
		}
	}
}
