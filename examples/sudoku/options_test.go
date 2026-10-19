package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestParseSize(t *testing.T) {
	for in, want := range map[string][2]int{"80x24": {80, 24}, "1x1": {1, 1}, "1000x1000": {1000, 1000}, "090x024": {90, 24}} {
		cols, rows, err := parseSize("dump", in)
		if err != nil || cols != want[0] || rows != want[1] {
			t.Errorf("parseSize(%q) = %d, %d, %v", in, cols, rows, err)
		}
	}
	for _, in := range []string{"", "80", "x24", "80x", "0x24", "80x0", "-80x24", "80x-24", "+80x24", "NaNx24", "80xNaN",
		"InfxInf", "80x24x1", "80X24", "80 x 24", " 80x24", "80x24 ", "1e2x24", "8.5x24", "1001x24", "80x1001",
		"99999999999999999999x24", "٨٠x24"} {
		if _, _, err := parseSize("dump", in); err == nil {
			t.Errorf("parseSize(%q) was accepted", in)
		} else if !strings.Contains(err.Error(), "-dump wants COLSxROWS") {
			t.Errorf("parseSize(%q): message %q", in, err)
		}
	}
}

func TestStartPuzzle(t *testing.T) {
	// A generated puzzle follows -seed and -difficulty.
	p, err := options{seed: 5, difficulty: "hard"}.startPuzzle()
	if err != nil || p != generateCached(5, levelHard) {
		t.Errorf("hard seed 5: %v, level %d", err, p.level)
	}
	p, err = options{seed: 2, difficulty: "easy", difficultySet: true}.startPuzzle()
	if err != nil || p.level != levelEasy {
		t.Errorf("easy: %v, level %d", err, p.level)
	}
	// A typed puzzle is accepted when it has one solution, and graded.
	p, err = options{puzzle: wikiPuzzle}.startPuzzle()
	if err != nil || p.sol.String() != wikiSolution || p.level != levelEasy {
		t.Errorf("classic puzzle: %v, level %d", err, p.level)
	}
	if _, err = (options{puzzle: " " + strings.Replace(wikiPuzzle, "0", ".", -1) + "\n"}).startPuzzle(); err != nil {
		t.Errorf("a puzzle with dots and white space: %v", err)
	}
	for name, o := range map[string]options{
		"unknown difficulty": {difficulty: "nightmare", difficultySet: true},
		"empty difficulty":   {difficulty: ""},
		"both":               {puzzle: wikiPuzzle, difficulty: "hard", difficultySet: true},
		"short":              {puzzle: wikiPuzzle[:80]},
		"bad character":      {puzzle: strings.Replace(wikiPuzzle, "5", "x", 1)},
		"two solutions":      {puzzle: twoSolutions(t)},
		"no solution":        {puzzle: strings.Repeat("1", 81)},
		"empty grid":         {puzzle: strings.Repeat("0", 81)},
	} {
		if _, err := o.startPuzzle(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := (options{puzzle: twoSolutions(t)}).startPuzzle(); err == nil || !strings.Contains(err.Error(), "-puzzle: the puzzle has more than one solution") {
		t.Errorf("two solutions: %v", err)
	}
}

func TestRunRejectsBadOptions(t *testing.T) {
	for name, o := range map[string]options{
		"theme":      {dir: ".", theme: "blue", difficulty: "medium", seed: 1},
		"difficulty": {dir: ".", difficulty: "x"},
		"dump size":  {dir: ".", difficulty: "medium", seed: 1, dump: "0x24"},
		"demo size":  {dir: ".", difficulty: "medium", seed: 1, demo: "80x"},
		"dir":        {dir: "no-such-dir", difficulty: "medium", seed: 1},
	} {
		if err := run(o); err == nil {
			t.Errorf("%s: run accepted %+v", name, o)
		}
	}
}

// -dump prints a clean Dump of the opening state, and -puzzle and -seed
// choose what it shows.
func TestRunDump(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	out := make(chan []byte)
	go func() { b, _ := io.ReadAll(r); out <- b }()
	err = run(options{dir: ".", difficulty: "medium", seed: 1, puzzle: wikiPuzzle, dump: "80x24"})
	w.Close()
	os.Stdout = old
	raw := <-out
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		OK     bool     `json:"ok"`
		Errors []any    `json:"errors"`
		Grid   []string `json:"grid"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if !d.OK || len(d.Errors) != 0 {
		t.Errorf("dump errors %v", d.Errors)
	}
	g := strings.Join(d.Grid, "\n")
	if !strings.Contains(g, "easy · 30 clues") || !strings.Contains(g, "5  3") {
		t.Errorf("the dump does not show the typed puzzle:\n%s", g)
	}
}

// run captures what run prints on stdout.
func runCapturing(t *testing.T, o options) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	out := make(chan []byte)
	go func() { b, _ := io.ReadAll(r); out <- b }()
	runErr := run(o)
	w.Close()
	os.Stdout = old
	return string(<-out), runErr
}

// -demo checks its size and its company before it prints a frame, and says
// so as a usage error: a size the board is not drawn on, or flags that would
// be silently ignored.
func TestDemoFlagsAreCheckedUpFront(t *testing.T) {
	base := options{dir: ".", difficulty: "medium", seed: 1}
	for name, o := range map[string]options{
		"too narrow":       {demo: "39x24"},
		"too short":        {demo: "90x13"},
		"tiny":             {demo: "1x1"},
		"with seed":        {demo: "90x24", seedSet: true, seed: 5},
		"with -puzzle":     {demo: "90x24", puzzle: wikiPuzzle},
		"with -difficulty": {demo: "90x24", difficultySet: true, difficulty: "hard"},
		"with dump":        {demo: "90x24", dump: "80x24"},
	} {
		o.dir = base.dir
		if o.difficulty == "" {
			o.difficulty = base.difficulty
		}
		out, err := runCapturing(t, o)
		if err == nil {
			t.Errorf("%s: run accepted %+v", name, o)
		}
		if out != "" {
			t.Errorf("%s: printed %d bytes before failing", name, len(out))
		}
	}
	// The smallest size, and the theme, are fine.
	out, err := runCapturing(t, options{dir: ".", difficulty: "medium", seed: 1, demo: "40x14", theme: "light"})
	if err != nil || !strings.HasPrefix(out, `{"ms":`) {
		t.Errorf("demo 40x14: %v, output starts %q", err, out[:min(len(out), 20)])
	}
}
