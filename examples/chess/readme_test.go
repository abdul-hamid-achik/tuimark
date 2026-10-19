package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
)

func readme(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Every flag is in the README, and the README's flags are real.
func TestReadmeNamesEveryFlag(t *testing.T) {
	text := readme(t)
	var o options
	fs := newFlagSet(&o)
	n := 0
	fs.VisitAll(func(f *flag.Flag) {
		n++
		if !strings.Contains(text, "`-"+f.Name) {
			t.Errorf("README does not mention the flag -%s", f.Name)
		}
	})
	if n < 13 {
		t.Errorf("only %d flags", n)
	}
	// And every flag the command table shows is real.
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, "go run ./examples/chess") {
			continue
		}
		for _, tok := range strings.Fields(strings.Trim(line, "|")) {
			if tok = strings.Trim(tok, "`\""); strings.HasPrefix(tok, "-") && len(tok) > 1 && tok[1] >= 'a' && tok[1] <= 'z' {
				if fs.Lookup(tok[1:]) == nil {
					t.Errorf("README shows %s, which is no flag", tok)
				}
			}
		}
	}
}

// The keys table covers every key of the board's keymap, and the dialog
// keys are named.
func TestReadmeNamesEveryKey(t *testing.T) {
	text := readme(t)
	for _, c := range keymapCases["#board"] {
		if !strings.Contains(text, "`"+c.key+"`") {
			t.Errorf("README does not list the key `%s`", c.key)
		}
	}
	for _, want := range []string{"`tab`", "`esc`", "`m s l c`", "`y`/`n`", "`q r b n`"} {
		if !strings.Contains(text, want) {
			t.Errorf("README lacks %s", want)
		}
	}
}

// The level table is the engine's.
func TestReadmeLevelsAreTheEngines(t *testing.T) {
	text := readme(t)
	for level := 1; level <= 6; level++ {
		lim := levelLimits[level]
		nodes := "none"
		if lim.Nodes > 0 {
			nodes = commas(lim.Nodes)
		}
		row := "| " + string(rune('0'+level)) + "     | " + string(rune('0'+lim.Depth)) + "     | " + nodes
		if !strings.Contains(text, row) {
			t.Errorf("README has no row %q", row)
		}
	}
}

// The perft table carries the counts the tests check, and the deep ones are
// marked deep.
func TestReadmePerftTable(t *testing.T) {
	text := readme(t)
	last := map[string]perftCase{}
	for _, c := range perftCases {
		if c.deep && !strings.Contains(text, commas(int64(c.nodes))) {
			t.Errorf("README lacks the deep count %s (%s depth %d)", commas(int64(c.nodes)), c.name, c.depth)
		}
		if !c.deep {
			last[c.name] = c
		}
	}
	for name, c := range last {
		if name == "position 4 mirrored" {
			continue
		}
		if !strings.Contains(text, commas(int64(c.nodes))) {
			t.Errorf("README lacks the count %s that is the last of %s run by default", commas(int64(c.nodes)), name)
		}
	}
}

func TestReadmeDemoClaims(t *testing.T) {
	text := readme(t)
	frames, _ := demoFrames(t, "", 100, 24)
	if !strings.Contains(text, "34 frames of about 280 ms") || len(frames) != 34 || demoPlyMs != 280 {
		t.Errorf("README: %d frames of %d ms", len(frames), demoPlyMs)
	}
	if !strings.Contains(text, "held for 1.3 seconds") || demoHoldMs != 1300 {
		t.Errorf("README says 1.3 seconds, the demo holds %d ms", demoHoldMs)
	}
	flat := strings.Join(strings.Fields(text), " ")
	if !strings.Contains(flat, "under ten seconds on its clock) thinks at most three plies") {
		t.Error("README lost the short-of-time claim")
	}
}

// The claim in the README: below ten seconds a side thinks at most three
// plies.
func TestShortOfTimeThinksFast(t *testing.T) {
	a := newTestApp(t, func(o *options) { o.level = 6; o.clock = "0.1+0" })
	a.moves(t, "e4")
	if d := a.get(t, "engine").(map[string]any)["depth"].(float64); d < 1 || d > 3 {
		t.Errorf("a computer with six seconds left searched to depth %v", d)
	}
	b := newTestApp(t, func(o *options) { o.level = 4 })
	b.moves(t, "e4")
	if d := b.get(t, "engine").(map[string]any)["depth"].(float64); d != 4 {
		t.Errorf("level 4 searched to depth %v", d)
	}
}

// The board's size follows the terminal as the README says: 5x2 cells from
// 80 columns and 20 rows, 3x1 below, and no side panel under 60 columns.
func TestBoardSizeFollowsTheTerminal(t *testing.T) {
	a := newTestApp(t, twoPlayers)
	cell := func(cols, rows int) (w, h int, side bool) {
		d, err := a.ui.Dump(cols, rows)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range d.Nodes {
			switch n.ID {
			case "v00":
				w, h = n.W, n.H
			case "side":
				side = n.W > 0 && n.H > 0
			}
		}
		return
	}
	for _, c := range []struct {
		cols, rows int
		w, h       int
		side       bool
	}{
		{80, 24, 5, 2, true}, {100, 20, 5, 2, true}, {120, 30, 5, 2, true},
		{79, 24, 3, 1, true}, {80, 19, 3, 1, true}, {60, 20, 3, 1, true},
		{59, 20, 3, 1, false}, {40, 14, 3, 1, false}, {100, 14, 3, 1, true},
	} {
		w, h, side := cell(c.cols, c.rows)
		if w != c.w || h != c.h || side != c.side {
			t.Errorf("%dx%d: cell %dx%d side=%v, want %dx%d side=%v", c.cols, c.rows, w, h, side, c.w, c.h, c.side)
		}
	}
	// Without the panel the top line carries the status.
	res := a.playAt(t, 40, 14)
	if !strings.Contains(res.Dump.Grid[0], "White to move") {
		t.Errorf("no status in the top line at 40x14:\n%s", grid(res))
	}
}

func TestCommandLineParsing(t *testing.T) {
	parse := func(args ...string) (options, error) {
		var o options
		fs := newFlagSet(&o)
		fs.Init("chess", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		err := fs.Parse(args)
		return o, err
	}
	o, err := parse("-level", "5", "-clock", "3+2", "-side", "black", "-mode", "cvc", "-letters", "-seed", "42")
	if err != nil || o.level != 5 || o.clock != "3+2" || o.side != "black" || o.mode != modeCvC || !o.letters || o.seed != 42 {
		t.Errorf("parsed %+v, %v", o, err)
	}
	for _, args := range [][]string{
		{"-level", "NaN"}, {"-level", "2.5"}, {"-level", ""}, {"-level", "Inf"}, {"-seed", "NaN"}, {"-seed", "1e3"},
		{"-no-such-flag"}, {"-letters=maybe"},
	} {
		if _, err := parse(args...); err == nil {
			t.Errorf("%v parsed", args)
		}
	}
}

// -dump prints the frame as JSON.
func TestDumpFlag(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	o := defaults()
	o.dump = "100x24"
	o.mode = mode2P
	runErr := run(o)
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	if runErr != nil {
		t.Fatal(runErr)
	}
	var d struct {
		OK   bool     `json:"ok"`
		Grid []string `json:"grid"`
	}
	if err := json.Unmarshal(buf.Bytes(), &d); err != nil {
		t.Fatalf("%v\n%s", err, buf.String())
	}
	if !d.OK || len(d.Grid) != 24 || !strings.Contains(strings.Join(d.Grid, "\n"), "two players") {
		t.Errorf("dump = %+v", d)
	}
	// And the theme is applied before it.
	o.theme = "light"
	o.dump = "80x24"
	r2, w2, _ := os.Pipe()
	os.Stdout = w2
	runErr = run(o)
	_ = w2.Close()
	os.Stdout = old
	buf.Reset()
	_, _ = io.Copy(&buf, r2)
	if runErr != nil || !strings.Contains(buf.String(), `"ok": true`) {
		t.Errorf("light dump: %v\n%s", runErr, buf.String())
	}
}
