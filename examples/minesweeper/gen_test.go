package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// The generator behind the parts of the document that repeat.
//
// On:click is not allowed in an each template, so a board you can click is
// one static element per cell. app.tui holds the rows and cells between two
// comments, and sizes.tcss the @media rules that need one block per board
// size; both are written here, from the constants the host uses (the
// grid, the glyph classes, the sizes a board can need).
//
//	UPDATE_TUI=1 go test ./examples/minesweeper -run TestGenerated
//
// rewrites them, and TestGenerated fails when either file differs from what
// this file would write.
//
// The cells are named by screen position ("c0312" is row 3, column 12 of
// the largest grid), not by board coordinate, so a new difficulty changes
// data, never the document. A cell outside the current grid has `if`
// false.

const (
	beginMark = "<!-- generated:"
	endMark   = "<!-- end generated -->"
)

// guardNames are the class guards of a cell, in document order.
var guardNames = []string{"hid", "n1", "n2", "n3", "n4", "n5", "n6", "n7", "n8", "flag", "q", "mine", "boom", "wrong", "cur"}

// genCells is the markup of the board: one row per grid row, one text per
// grid column. The indentation is what `tuimark fmt` writes inside the
// board's col (four levels down from the root).
func genCells() string {
	var b strings.Builder
	for r := 0; r < maxRows; r++ {
		fmt.Fprintf(&b, "          <row id=\"%s\" class=\"mrow\" if=\"rows.%s\">\n", rowID(r), rowID(r))
		for c := 0; c < maxCols; c++ {
			id := cellID(r, c)
			class := "cell"
			if (r+c)%2 == 1 {
				class = "cell alt" // a second cover shade, so covered cells have a grid
			}
			fmt.Fprintf(&b, "            <text id=\"%s\" class=\"%s\"", id, class)
			for _, g := range guardNames {
				fmt.Fprintf(&b, " class:%s=\"b.%s.%s\"", g, id, g)
			}
			fmt.Fprintf(&b, " on:click=\"cell\" if=\"b.%s.on\">{b.%s.t}</text>\n", id, id)
		}
		b.WriteString("          </row>\n")
	}
	return b.String()
}

// genSizes is sizes.tcss. For every width a board can need there is a block
// that puts the counters in the status line when the side panel would not
// fit beside the board, and one that swaps the board for a notice when the
// terminal is narrower than the board; for every height, a block that
// swaps it when the terminal is shorter. The host marks the one width and
// the one height of the current board with a class on #app.
func genSizes() string {
	var b strings.Builder
	b.WriteString("/* Written by gen_test.go (UPDATE_TUI=1 go test ./examples/minesweeper -run TestGenerated); do not edit by hand.\n")
	b.WriteString("   #app carries ncN for the columns the current board needs (the board in its frame, at least ")
	fmt.Fprintf(&b, "%d) and nhN for the rows (the board, the status line, the key hints). */\n", minTermCols)
	for _, v := range fitCols() {
		fmt.Fprintf(&b, "\n/* %d columns: the side panel (%d columns more) fits from %d. Below that the counters move into the status line, which drops the brand and, up to %d columns, the level label. */\n", v, sideRoom, v+sideRoom, levelHides)
		fmt.Fprintf(&b, "@media (max-cols: %d) {\n  #app.nc%d > #stage > #side { display: none; }\n  #app.nc%d > #top > #compact { display: flex; }\n  #app.nc%d > #top > .brand { display: none; }\n}\n", v+sideRoom-1, v, v, v)
		fmt.Fprintf(&b, "@media (max-cols: %d) {\n  #app.nc%d > #top > .level { display: none; }\n}\n", min(levelHides, v+sideRoom-1), v)
		fmt.Fprintf(&b, "@media (max-cols: %d) {\n  #app.nc%d > #stage { display: none; }\n  #app.nc%d > #toosmall { display: flex; }\n}\n", v-1, v, v)
		// The counters need the minimum 36 columns; below it only the notice shows.
		fmt.Fprintf(&b, "@media (max-cols: %d) {\n  #app.nc%d > #top > #compact { display: none; }\n}\n", minTermCols-1, v)
	}
	for _, v := range fitRows() {
		fmt.Fprintf(&b, "\n/* %d rows. */\n", v)
		fmt.Fprintf(&b, "@media (max-rows: %d) {\n  #app.nh%d > #stage { display: none; }\n  #app.nh%d > #toosmall { display: flex; }\n}\n", v-1, v, v)
	}
	return b.String()
}

// withGenerated replaces what lies between the begin and end comments of
// src with body.
func withGenerated(src, body string) (string, error) {
	lines := strings.SplitAfter(src, "\n")
	begin, end := -1, -1
	for i, l := range lines {
		switch {
		case strings.Contains(l, beginMark):
			begin = i
		case strings.Contains(l, endMark):
			end = i
		}
	}
	if begin < 0 || end < begin {
		return "", fmt.Errorf("app.tui has no %q ... %q region", beginMark, endMark)
	}
	return strings.Join(lines[:begin+1], "") + body + strings.Join(lines[end:], ""), nil
}

func TestGenerated(t *testing.T) {
	src, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	want, err := withGenerated(string(src), genCells())
	if err != nil {
		t.Fatal(err)
	}
	sizes := genSizes()
	if os.Getenv("UPDATE_TUI") != "" {
		if err := os.WriteFile("app.tui", []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("sizes.tcss", []byte(sizes), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if string(src) != want {
		t.Error("app.tui differs from the generator's board; run UPDATE_TUI=1 go test ./examples/minesweeper -run TestGenerated")
	}
	got, err := os.ReadFile("sizes.tcss")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sizes {
		t.Error("sizes.tcss differs from the generator's; run UPDATE_TUI=1 go test ./examples/minesweeper -run TestGenerated")
	}
}
