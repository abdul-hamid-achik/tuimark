package layout

import (
	"reflect"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// runCurrent is run for the current language: its built-in sheet (the
// textarea's rules included) and its layout defaults.
func runCurrent(t *testing.T, root *Box, cols, rows int) *Engine {
	t.Helper()
	c := css.NewCascadeLang(nil, css.Env{Cols: cols, Rows: rows}, ir.LangCurrent)
	styleTree(c, root, nil)
	if len(c.Diags) != 0 {
		t.Fatalf("cascade: %v", c.Diags)
	}
	e := &Engine{File: "t.tui", Lang: ir.LangCurrent}
	e.Layout(root, nil, cols, rows)
	return e
}

func ta(id, text string, cursor int, decls string) *Box {
	b := n("textarea#"+id, decls)
	b.Text, b.Cursor = text, cursor
	return b
}

// SPEC 0.4d test 289 (decision 230): the visual lines are one PreWrap of
// the whole value at max(1, W-1) columns; an empty value has one empty
// line and "a\n" two.
func TestTextareaLines(t *testing.T) {
	for _, c := range []struct {
		text string
		w    int
		want []TextLine
	}{
		{"", 10, []TextLine{{0, 0, false}}},
		{"a\n", 10, []TextLine{{0, 1, true}, {2, 2, false}}},
		{"abcdefghijkl", 10, []TextLine{{0, 9, false}, {9, 12, false}}},
		{"abcdefghi", 10, []TextLine{{0, 9, false}}},
		{"ab         cd", 10, []TextLine{{0, 11, false}, {11, 13, false}}},
		{"a\n\nb", 10, []TextLine{{0, 1, true}, {2, 2, true}, {3, 4, false}}},
		{"abc", 0, []TextLine{{0, 1, false}, {1, 2, false}, {2, 3, false}}}, // W - 1 is at least 1
		{"abc", 1, []TextLine{{0, 1, false}, {1, 2, false}, {2, 3, false}}},
	} {
		got := TextareaLines(c.text, c.w)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("TextareaLines(%q, %d) = %v, want %v", c.text, c.w, got, c.want)
		}
		if n := TextareaLineCount(c.text, c.w); n != len(c.want) {
			t.Errorf("TextareaLineCount(%q, %d) = %d, want %d", c.text, c.w, n, len(c.want))
		}
	}
}

// SPEC 0.4d test 289: where the caret stands.
func TestLocateCaret(t *testing.T) {
	for _, c := range []struct {
		name  string
		text  string
		w     int
		caret int // a rune index (ASCII here)
		line  int
		col   int
	}{
		{"end of a wrapped value", "abcdefghijkl", 10, 12, 1, 3},
		{"the spare column after a full line, end of the value", "abcdefghi", 10, 9, 0, 9},
		{"hanging spaces, after the eighth space", "ab         cd", 10, 10, 0, 10},
		{"hanging spaces, after the ninth: the next line", "ab         cd", 10, 11, 1, 0},
		{"the end of a hard line stays on it", "a\nb", 10, 1, 0, 1},
		{"after the line break", "a\nb", 10, 2, 1, 0},
		{"an empty last line", "a\n", 10, 2, 1, 0},
		{"a soft break before the end of the value", "abcdefghijkl", 10, 9, 1, 0},
		{"empty", "", 10, 0, 0, 0},
	} {
		lines := TextareaLines(c.text, c.w)
		line, col := LocateCaret(c.text, lines, ByteOffset(c.text, c.caret))
		if line != c.line || col != c.col {
			t.Errorf("%s: caret %d at (%d, %d), want (%d, %d)", c.name, c.caret, line, col, c.line, c.col)
		}
	}
	if l, col := LocateHard("one two\nthree", ByteOffset("one two\nthree", 13)); l != 1 || col != 5 {
		t.Errorf("LocateHard = (%d, %d)", l, col)
	}
	// A wide cluster counts its columns, and the offsets are bytes.
	text := "微信x"
	if l, col := LocateCaret(text, TextareaLines(text, 10), ByteOffset(text, 2)); l != 0 || col != 4 {
		t.Errorf("wide: (%d, %d)", l, col)
	}
	if RuneIndex(text, ByteOffset(text, 3)) != 3 || ByteOffset(text, 99) != len(text) || ByteOffset(text, -1) != 0 {
		t.Error("ByteOffset and RuneIndex disagree")
	}
}

// SPEC 0.4d §6.19.4, decisions 231 and 232: a textarea is as high as its
// visual lines, at least 1, up to max-height: 8; its scroll extent is the
// larger of its rows and its lines; it is not a viewport for sizing.
func TestTextareaSizeAndExtent(t *testing.T) {
	lines := func(k int) string {
		s := ""
		for i := 0; i < k; i++ {
			if i > 0 {
				s += "\n"
			}
			s += "x"
		}
		return s
	}
	for _, c := range []struct {
		name         string
		text         string
		decls        string
		h, extent, y int
	}{
		{"empty", "", "", 1, 1, 0},
		{"three lines", lines(3), "", 3, 3, 0},
		{"capped at eight, the caret at the end", lines(10), "", 8, 10, 2},
		{"a height wins", "x", "height: 5", 5, 5, 0},
		{"a max-height", lines(5), "max-height: 3", 3, 5, 2},
	} {
		tb := ta("t", c.text, len([]rune(c.text)), c.decls)
		tb.Follow = 0
		root := n("screen", "", tb)
		runCurrent(t, root, 30, 20)
		if tb.H != c.h || tb.ContentH != c.extent || tb.ScrollY != c.y {
			t.Errorf("%s: h %d extent %d offset %d, want %d %d %d", c.name, tb.H, tb.ContentH, tb.ScrollY, c.h, c.extent, c.y)
		}
	}

	// Next to a list it does not share the space; the list takes the rest.
	l := n("list#l", "")
	for i := 0; i < 50; i++ {
		l.Children = append(l.Children, n("item", "", txt("row", "")))
		l.Children[i].Parent = l
	}
	tb := ta("t", lines(3), 5, "")
	root := n("screen", "", n("col", "", l, tb))
	runCurrent(t, root, 30, 12)
	if tb.H != 3 || l.H != 9 {
		t.Errorf("list %d, textarea %d", l.H, tb.H)
	}
	// An fr sibling: the textarea stays auto too.
	sp := n("box#b", "height: 1fr")
	tb = ta("t", lines(3), 5, "")
	root = n("screen", "", n("col", "", sp, tb))
	runCurrent(t, root, 30, 12)
	if tb.H != 3 || sp.H != 9 {
		t.Errorf("fr sibling %d, textarea %d", sp.H, tb.H)
	}
	// In a row it takes the rest of the width.
	tb = ta("t", "x", 1, "")
	b := n("button#b", "width: 8")
	b.Text = "send"
	root = n("screen", "", n("row", "height: 1", tb, b))
	runCurrent(t, root, 30, 3)
	if tb.W != 22 || b.W != 8 {
		t.Errorf("row: textarea %d, button %d", tb.W, b.W)
	}
	// The intrinsic width is an input's, whatever the value.
	tb = ta("t", "a long long long long long long long value", 3, "width: auto")
	b = n("box#b", "")
	root = n("screen", "", n("row", "height: 1", n("box", "", tb), b))
	runCurrent(t, root, 60, 3)
	if tb.W != 20 {
		t.Errorf("auto width %d, want 20", tb.W)
	}
	tb = ta("t", "", 0, "width: auto")
	tb.Placeholder = "a placeholder of 27 columns!!"
	root = n("screen", "", n("row", "height: 1", n("box", "", tb)))
	runCurrent(t, root, 60, 3)
	if tb.W != 30 {
		t.Errorf("auto width with a placeholder %d, want 30", tb.W)
	}
}

// SPEC 0.4d §6.19.4, decision 233: the offset follows the caret by the
// least change, and stays where the wheel left it otherwise.
func TestTextareaOffsetFollowsCaret(t *testing.T) {
	text := "x"
	for i := 1; i < 20; i++ {
		text += "\nx"
	}
	at := func(line int) int { return 2 * line } // the rune index of the start of a line
	for _, c := range []struct {
		name   string
		in     int // the incoming offset
		follow bool
		line   int
		want   int
	}{
		{"no offset yet: the least that shows the caret at the end", 0, true, 19, 12},
		{"a caret above the box scrolls up to it", 9, true, 3, 3},
		{"a caret below scrolls down to it", 0, true, 10, 3},
		{"a caret inside leaves the offset", 3, true, 5, 3},
		{"the wheel's offset stays when not following", 4, false, 0, 4},
		{"clamped to the extent", 40, false, 0, 12},
	} {
		tb := ta("t", text, at(c.line), "max-height: 8")
		tb.ScrollY = c.in
		if c.follow {
			tb.Follow = 0
		}
		root := n("screen", "", tb)
		runCurrent(t, root, 30, 12)
		if tb.ScrollY != c.want {
			t.Errorf("%s: offset %d, want %d", c.name, tb.ScrollY, c.want)
		}
	}
}

// SPEC 0.4d §6.19.5, decision 237: the caret's cell is its column in its
// visual line from the content origin, clamped to the last column; it has
// none when its line is outside the content box.
func TestCaretCell(t *testing.T) {
	tb := ta("t", "abcdefghijkl", 12, "")
	tb.Follow = 0
	root := n("screen", "", n("text", "", &Box{Tag: "text", Kind: "text", Style: &css.Style{}}), tb)
	runCurrent(t, root, 10, 6)
	c := CaretOf(tb)
	if !c.Shown || c.X != 3 || c.Y != tb.Content.Y+1 || c.Line != 0 || c.Col != 12 || c.VLine != 1 || c.VCol != 3 {
		t.Errorf("caret %+v (content %+v)", c, tb.Content)
	}
	// Hanging spaces: the cell is clamped to the last column.
	tb = ta("t", "ab         cd", 10, "")
	tb.Follow = 0
	runCurrent(t, n("screen", "", tb), 10, 6)
	if c := CaretOf(tb); !c.Shown || c.X != 9 || c.Y != 0 || c.Col != 10 {
		t.Errorf("clamped caret %+v", c)
	}
	// Scrolled away by the wheel: the line is outside the box.
	tb = ta("t", "1\n2\n3\n4\n5\n6\n7\n8\n9\n10", 0, "max-height: 3")
	tb.ScrollY = 4
	runCurrent(t, n("screen", "", tb), 10, 6)
	if c := CaretOf(tb); c.Shown || c.Line != 0 || c.Col != 0 {
		t.Errorf("hidden caret %+v", c)
	}
}

// L006 applies to a textarea as to any viewport (decision 232).
func TestTextareaL006(t *testing.T) {
	tb := ta("t", "x\ny", 1, "height: 0")
	e := runCurrent(t, n("screen", "", tb), 10, 6)
	if got := codesOf(e); got != "L006" {
		t.Errorf("codes %q: %v", got, e.Diags)
	}
	e = runCurrent(t, n("screen", "", ta("t", "x\ny", 1, "")), 10, 6)
	if got := codesOf(e); got != "" {
		t.Errorf("a normal textarea: %q", got)
	}
}
