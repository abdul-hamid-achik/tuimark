package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// The board is 64 static cells, each with its own id: an each template
// cannot take on:click, and a click must say which square it hit. The cells
// are named by screen position (v<row><column>, row 0 at the top), never by
// square, so flipping the board or resizing the terminal only changes data.
// That markup is long and regular, so a generator writes it between the
// board:begin and board:end comments of app.tui:
//
//	UPDATE_TUI=1 go test ./examples/chess -run TestBoardMarkupInSync
//
// and the same test fails when the file and the generator disagree.

const (
	boardBegin = "<!-- board:begin -->"
	boardEnd   = "<!-- board:end -->"
)

// boardMarkup is the board and its file labels, indented for a child of
// #boardbox.
func boardMarkup() string {
	var b strings.Builder
	w := func(indent int, format string, args ...any) {
		b.WriteString(strings.Repeat("  ", indent))
		fmt.Fprintf(&b, format, args...)
		b.WriteByte('\n')
	}
	w(5, `<col id="board" focusable="true">`)
	for r := 0; r < 8; r++ {
		w(6, `<row class="rank">`)
		w(7, `<text class="rl">{labels.r%d}</text>`, r)
		for c := 0; c < 8; c++ {
			shade := "dark"
			if (r+c)%2 == 0 {
				shade = "light" // a8 is a light square, and so is h1
			}
			id := cellID(r, c)
			var cls strings.Builder
			for _, g := range []struct{ class, field string }{
				{"white", "w"}, {"black", "b"}, {"cur", "cur"}, {"sel", "sel"}, {"tgt", "tgt"},
				{"cap", "cap"}, {"last", "last"}, {"chk", "chk"}, {"hint", "hint"},
			} {
				fmt.Fprintf(&cls, ` class:%s="cells.%s.%s"`, g.class, id, g.field)
			}
			w(7, `<text id="%s" class="sq %s"%s on:click="cell">{cells.%s.t}</text>`, id, shade, cls.String(), id)
		}
		w(6, `</row>`)
	}
	w(5, `</col>`)
	w(5, `<row id="files">`)
	w(6, `<text class="rl"/>`)
	for c := 0; c < 8; c++ {
		w(6, `<text class="fl">{labels.f%d}</text>`, c)
	}
	w(5, `</row>`)
	return b.String()
}

// withBoard returns doc with the region between the markers replaced.
func withBoard(t testing.TB, doc string) string {
	t.Helper()
	i := strings.Index(doc, boardBegin)
	j := strings.Index(doc, boardEnd)
	if i < 0 || j < i {
		t.Fatalf("app.tui has no %s … %s region", boardBegin, boardEnd)
	}
	i += len(boardBegin)
	// Keep the end marker's own indentation.
	k := strings.LastIndex(doc[:j], "\n") + 1
	return doc[:i] + "\n" + boardMarkup() + doc[k:]
}

func TestBoardMarkupInSync(t *testing.T) {
	raw, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	want := withBoard(t, string(raw))
	if os.Getenv("UPDATE_TUI") != "" {
		if err := os.WriteFile("app.tui", []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if want != string(raw) {
		t.Error("the board markup in app.tui differs from the generator's; run UPDATE_TUI=1 go test ./examples/chess -run TestBoardMarkupInSync")
	}
}

func TestBoardCellsAreWhatTheClicksNeed(t *testing.T) {
	raw, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			id := cellID(r, c)
			if n := strings.Count(doc, `id="`+id+`"`); n != 1 {
				t.Errorf("cell %s appears %d times", id, n)
			}
		}
	}
	if n := strings.Count(doc, `on:click="cell"`); n != 64 {
		t.Errorf("%d cells take clicks, want 64", n)
	}
	// Light and dark follow the screen position only, so a flip keeps the
	// pattern: the top-left cell is light.
	if !strings.Contains(doc, `id="v00" class="sq light"`) || !strings.Contains(doc, `id="v77" class="sq light"`) || !strings.Contains(doc, `id="v07" class="sq dark"`) {
		t.Error("the light and dark squares are not a chequerboard with a light top-left corner")
	}
}
