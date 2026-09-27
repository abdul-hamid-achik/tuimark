package paint

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rivo/uniseg"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/uniwidth"
)

// checkGrid asserts the §12.1 cell invariants: a continuation always
// follows a wide lead and a wide lead is always followed by its
// continuation, with the same style and owner; no cell carries a control
// character or a width-0 cluster; and every row is exactly W columns wide
// (MUST 6), both summed per lead cell and as Lines() writes it.
func checkGrid(t *testing.T, g *Grid) {
	t.Helper()
	lines := g.Lines()
	for y := 0; y < g.H; y++ {
		cols := 0
		var row strings.Builder
		for x := 0; x < g.W; x++ {
			c := g.At(x, y)
			if c.Cont {
				if x == 0 || !g.At(x-1, y).Wide {
					t.Fatalf("(%d,%d): continuation without a wide lead", x, y)
				}
				l := g.At(x-1, y)
				if c.Ch != 0 || c.Ext != "" || c.Wide || !sameStyle(*c, *l) || c.Owner != l.Owner {
					t.Fatalf("(%d,%d): continuation %+v does not match its lead %+v", x, y, *c, *l)
				}
				continue
			}
			if c.Wide && (x+1 >= g.W || !g.At(x+1, y).Cont) {
				t.Fatalf("(%d,%d): wide lead %q without a continuation", x, y, c.Grapheme())
			}
			s := c.Grapheme()
			for _, r := range s {
				if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
					t.Fatalf("(%d,%d): control character %U in %q", x, y, r, s)
				}
			}
			w := uniwidth.ClusterWidth(s)
			if w != c.Width() || uniwidth.Count(s) != 1 {
				t.Fatalf("(%d,%d): cell %q has width %d, its cluster measures %d (%d clusters)", x, y, s, c.Width(), w, uniwidth.Count(s))
			}
			cols += w
			row.WriteString(s)
		}
		if cols != g.W {
			t.Fatalf("row %d is %d columns, want %d", y, cols, g.W)
		}
		if lines[y] != row.String() {
			t.Fatalf("Lines()[%d] = %q, want the lead cells concatenated %q", y, lines[y], row.String())
		}
		// Read as one string, the row segments into exactly its lead
		// cells' clusters, so it measures W columns (MUST 6, §12.1).
		if got, want := strings.Join(uniwidth.Split(lines[y]), "|"), strings.Join(leads(g, y), "|"); got != want {
			t.Fatalf("row %d %+q segments as %+q, its lead cells are %+q", y, lines[y], got, want)
		}
		if w := uniwidth.Width(lines[y]); w != g.W {
			t.Fatalf("row %d %+q measures %d columns as a string, want %d", y, lines[y], w, g.W)
		}
	}
}

// leads returns the clusters of row y's lead cells, in order.
func leads(g *Grid, y int) []string {
	var out []string
	for x := 0; x < g.W; x++ {
		if c := g.At(x, y); !c.Cont {
			out = append(out, c.Grapheme())
		}
	}
	return out
}

// §12.1: clusters painted from different strings into adjacent cells never
// join when the row is read as one string. A lead whose cluster would
// join the one before it becomes a space (a width-2 lead two spaces),
// unless the join comes from a Prepend ending the lead before it, which
// joins whatever follows: then that lead becomes a space.
func TestAdjacentClustersNeverJoin(t *testing.T) {
	cases := []struct {
		name  string
		texts []string
		want  string
	}{
		{"lone regional indicators", []string{"🇺", "🇸", "|"}, "🇺  |   "},
		{"three regional indicators", []string{"🇺", "🇸", "🇫", "|"}, "🇺  🇫| "},
		{"Hangul L then V", []string{"ᄒ", "\u1161", "|"}, "ᄒ |    "},
		{"trailing ZWJ then a pictograph", []string{"👨\u200D", "👩", "|"}, "👨\u200D  |   "},
		{"wide then a spacing mark", []string{"微", "\u093F", "|"}, "微 |    "},
		{"a mark-led cluster", []string{"e", "\u0301\u093F", "|"}, "e |     "},
		{"Prepend before a wide cluster", []string{"\u0600", "微", "|"}, " 微|    "},
		{"Prepend before a mark-led cluster", []string{"\u0600", "\u0301\u093F", "|"}, "  |     "},
		{"Prepend before blank cells", []string{"x\u0600"}, "x       "},
		{"no join", []string{"微", "ab", "🇺🇸", "|"}, "微ab🇺🇸| "},
	}
	for _, c := range cases {
		var kids []*layout.Box
		for _, s := range c.texts {
			kids = append(kids, tx(s, ""))
		}
		g := render(t, bx("col", "", bx("row", "height: 1", kids...)), 8, 1)
		checkGrid(t, g)
		if got := g.Lines()[0]; got != c.want {
			t.Errorf("%s: row = %+q, want %+q", c.name, got, c.want)
		}
	}
	// The spaces keep the replaced cells' style and owner.
	g := render(t, bx("col", "", bx("row#r", "height: 1", tx("🇺", ""), bx("box#b", "width: 2; color: red", tx("🇸", "")))), 6, 1)
	checkGrid(t, g)
	for x := 2; x < 4; x++ {
		if c := g.At(x, 0); c.Grapheme() != " " || c.FG.String() != "red" || c.Owner != "b" {
			t.Errorf("replaced cell %d = %+v, want a red space owned by b", x, *c)
		}
	}
}

func cellsOf(g *Grid, y int) string {
	var parts []string
	for x := 0; x < g.W; x++ {
		c := g.At(x, y)
		switch {
		case c.Cont:
			parts = append(parts, "<")
		default:
			parts = append(parts, c.Grapheme())
		}
	}
	return strings.Join(parts, "|")
}

var all = layout.Rect{X: -1000, Y: -1000, W: 3000, H: 3000}

func TestWideText(t *testing.T) {
	g := render(t, bx("col", "", tx("微信ok", ""), tx("a👨\u200D👩\u200D👧b🇺🇸e\u0301", ""), tx("\u0301x\u200By", "")), 8, 3)
	checkGrid(t, g)
	want := []string{"微信ok  ", "a👨\u200D👩\u200D👧b🇺🇸e\u0301 ", "xy      "}
	for i, w := range want {
		if got := g.Lines()[i]; got != w {
			t.Errorf("row %d = %q, want %q", i, got, w)
		}
	}
	if got := cellsOf(g, 0); got != "微|<|信|<|o|k| | " {
		t.Errorf("cells row 0 = %s", got)
	}
	if got := cellsOf(g, 1); got != "a|👨\u200D👩\u200D👧|<|b|🇺🇸|<|e\u0301| " {
		t.Errorf("cells row 1 = %s", got)
	}
	if !g.At(0, 0).Complex() || !g.At(1, 1).Complex() || !g.At(6, 1).Complex() || g.At(4, 0).Complex() || !g.HasComplex() {
		t.Error("complex clusters: CJK, ZWJ sequences, flags, and combining sequences are complex; ASCII is not")
	}
	if NewGrid(3, 1).HasComplex() {
		t.Error("a blank grid has no complex cluster")
	}
	// A width-0 cluster at the start of a line occupies no column; so does
	// U+200B.
	if got := cellsOf(g, 2); got != "x|y| | | | | | " {
		t.Errorf("cells row 2 = %s", got)
	}
}

// Invalid UTF-8 never reaches the grid: each bad byte is one width-1
// cluster painted as U+FFFD.
func TestInvalidUTF8Paints(t *testing.T) {
	p := &painter{g: NewGrid(5, 1)}
	p.text(all, 0, 0, "a\xffb\xe4\xb8", cellStyle{})
	checkGrid(t, p.g)
	bad := string(utf8.RuneError)
	if got, want := p.g.Lines()[0], "a"+bad+"b"+bad+bad; got != want {
		t.Errorf("row = %q, want %q", got, want)
	}
}

func TestWideOwnersAndStyle(t *testing.T) {
	root := bx("col#root", "",
		bx("box#panel", "height: 1; color: cyan; background: #102030", tx("微x", "bold: true")),
	)
	g := render(t, root, 4, 1)
	checkGrid(t, g)
	lead, cont := g.At(0, 0), g.At(1, 0)
	if !lead.Wide || !cont.Cont || lead.Owner != "panel" || cont.Owner != "panel" {
		t.Errorf("lead %+v cont %+v", *lead, *cont)
	}
	if cont.FG.String() != "cyan" || cont.BG.String() != "#102030" || cont.Attrs != Bold {
		t.Errorf("a continuation carries its lead's style: %+v", *cont)
	}
	// A child that claims ownership of the continuation column only (no
	// glyph written) does not split the cluster between owners.
	p := &painter{g: NewGrid(4, 1)}
	p.text(all, 0, 0, "微", cellStyle{owner: "a", clearOwner: true})
	p.g.At(1, 0).Owner = "b"
	p.settle()
	checkGrid(t, p.g)
	if p.g.At(1, 0).Owner != "a" {
		t.Errorf("continuation owner = %q, want its lead's", p.g.At(1, 0).Owner)
	}
}

// §21 test 19: a width-2 cluster whose second column is clipped paints a
// space with the node's style; overwriting either half of a wide cluster
// leaves no half cell.
func TestWideClip(t *testing.T) {
	red := css.Color{Kind: css.ColorANSI, Index: 1}
	st := cellStyle{fg: red, attrs: Bold, owner: "t", clearOwner: true}
	// Clip edge.
	p := &painter{g: NewGrid(6, 1)}
	p.text(layout.Rect{X: 0, Y: 0, W: 3, H: 1}, 0, 0, "微信", st)
	checkGrid(t, p.g)
	if got := cellsOf(p.g, 0); got != "微|<| | | | " {
		t.Errorf("clipped at column 3: %s", got)
	}
	if c := p.g.At(2, 0); c.FG != red || c.Attrs != Bold || c.Owner != "t" {
		t.Errorf("the replacement space has the node's style: %+v", *c)
	}
	// Grid edge.
	p = &painter{g: NewGrid(5, 1)}
	p.text(all, 0, 0, "ab微信", st)
	checkGrid(t, p.g)
	if got := cellsOf(p.g, 0); got != "a|b|微|<| " || p.g.At(4, 0).Owner != "t" {
		t.Errorf("grid edge: %s", got)
	}
	// First column outside the clip: nothing is painted.
	p = &painter{g: NewGrid(4, 1)}
	p.text(layout.Rect{X: 1, Y: 0, W: 3, H: 1}, 0, 0, "微ab", st)
	checkGrid(t, p.g)
	if got := cellsOf(p.g, 0); got != " | |a|b" {
		t.Errorf("left clip: %s", got)
	}
	// A text box one column wide holding a wide cluster (Wrap gives it a
	// line by itself) paints a space there.
	g := render(t, bx("col", "", bx("row", "height: 1", tx("微", "width: 1; wrap: wrap"), tx("|", ""))), 4, 1)
	checkGrid(t, g)
	if got := g.Lines()[0]; got != " |  " {
		t.Errorf("width-1 text = %q", got)
	}
	// A straddling cluster at the right edge of a nowrap text's content box.
	g = render(t, bx("col", "", bx("box", "width: 7; height: 3; border: single", tx("ab微信", ""))), 8, 3)
	checkGrid(t, g)
	if got := g.Lines()[1]; got != "│ab微 │ " {
		t.Errorf("straddle = %q", got)
	}
}

func TestWideOverwrite(t *testing.T) {
	blue := css.Color{Kind: css.ColorANSI, Index: 4}
	base := cellStyle{fg: blue, owner: "a", clearOwner: true}
	over := cellStyle{owner: "b", clearOwner: true}
	cases := []struct {
		name  string
		paint func(p *painter)
		want  string
	}{
		{"narrow over a continuation", func(p *painter) { p.set(all, 2, 0, 'x', over) }, "a| |x|b|c|d"},
		{"narrow over a lead", func(p *painter) { p.set(all, 1, 0, 'x', over) }, "a|x| |b|c|d"},
		{"wide over a continuation", func(p *painter) { p.text(all, 2, 0, "信", over) }, "a| |信|<|c|d"},
		{"wide over a lead", func(p *painter) { p.text(all, 1, 0, "信", over) }, "a|信|<|b|c|d"},
		{"wide over the next cell", func(p *painter) { p.text(all, 0, 0, "信", over) }, "信|<| |b|c|d"},
		{"wide across two wide clusters", func(p *painter) {
			p.text(all, 3, 0, "文", base) // a|微|<|文|<|d
			p.text(all, 2, 0, "信", over)
		}, "a| |信|<| |d"},
	}
	for _, c := range cases {
		p := &painter{g: NewGrid(6, 1)}
		p.text(all, 0, 0, "a微bcd", base)
		c.paint(p)
		checkGrid(t, p.g)
		if got := cellsOf(p.g, 0); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
		// Cells turned into spaces keep their existing style.
		for x := 0; x < 6; x++ {
			if cl := p.g.At(x, 0); cl.Grapheme() == " " && cl.FG != blue {
				t.Errorf("%s: space at %d lost its style: %+v", c.name, x, *cl)
			}
		}
	}
	// A child box's background fill over half of a wide cluster.
	root := bx("col", "", bx("row", "height: 1", tx("微信", "width: 4"), bx("box", "width: 2; background: red")))
	g := render(t, root, 6, 1)
	checkGrid(t, g)
	// Borders over wide text inside a modal.
	under := tx(strings.Repeat("微", 5)+"\n"+strings.Repeat("微", 5)+"\n"+strings.Repeat("微", 5), "height: 3")
	m := bx("modal#m", "width: 5; height: 3", tx("x", ""))
	g = render(t, bx("col", "", under), 10, 3, m)
	checkGrid(t, g)
	if got := g.Lines()[1]; got != "微│x  │ 微" {
		t.Errorf("modal over wide text = %q", got)
	}
}

func TestWideBorderTitleAndButton(t *testing.T) {
	b := bx("box", "height: 3; border: single")
	b.Title = "微信 title"
	g := render(t, bx("col", "width: 12", b), 12, 3)
	checkGrid(t, g)
	if got := g.Lines()[0]; got != "┌─ 微信 t…─┐" {
		t.Errorf("title = %q", got)
	}
	b = bx("box", "height: 3; border: single")
	b.Title = "微信信"
	g = render(t, bx("col", "width: 9", b), 9, 3)
	checkGrid(t, g)
	if got := g.Lines()[0]; got != "┌─ 微…──┐" { // " 微信信 " in 5 columns: " 微" + "…", one column short
		t.Errorf("title = %q", got)
	}
	btn := bx("button", "width: 8; content-align: center")
	btn.Text = "确定"
	g = render(t, bx("col", "", btn), 10, 1)
	checkGrid(t, g)
	if got := g.Lines()[0]; got != "[ 确定 ]  " {
		t.Errorf("button = %q", got)
	}
}

// §21 test 21 (paint side): the cursor and the painted window work by
// cluster; a secret input shows one • per cluster.
func TestWideInput(t *testing.T) {
	mk := func(text string, secret bool, cursor, width int) *Grid {
		in := bx("input#q", fmt.Sprintf("width: %d", width))
		in.Text, in.Secret, in.Focused, in.Cursor = text, secret, true, cursor
		g := render(t, bx("col", "", in), width+2, 1)
		checkGrid(t, g)
		return g
	}
	thumbs := "👍🏽" // 2 code points, 1 cluster, width 2
	for _, c := range []struct {
		text      string
		secret    bool
		cursor, w int
		want      string
		cx        int
	}{
		{"a" + thumbs + "b", false, 3, 6, "a" + thumbs + "b    ", 3}, // cursor after the thumbs: 1 + 2 columns
		{"a" + thumbs + "b", false, 2, 6, "a" + thumbs + "b    ", 3}, // inside the cluster: stands after it
		{"a" + thumbs + "b", false, 4, 6, "a" + thumbs + "b    ", 4}, // at the end
		{"a" + thumbs + "b", true, 4, 6, "•••     ", 3},              // one • per cluster
		{"微信微信", false, 4, 5, "微信   ", 4},                            // window: the clusters up to the cursor fit in W-1
		{"微信微信", false, 3, 5, "信微   ", 4},                            // the window starts at the second cluster
		{"微信微信", false, 0, 5, "微信   ", 0},                            // from the start: 信 would not fit
		{"微信微信", false, 1, 5, "微信   ", 2},                            // p = 2 < W
		{"ab微", false, 3, 3, "微   ", 2},                              // p = 4 >= W: the window starts at 微
		{"微", false, 1, 1, "   ", 0},                                 // nothing fits in one column
		{"e\u0301e\u0301", false, 2, 4, "e\u0301e\u0301    ", 1},     // combining sequences are one cluster
	} {
		g := mk(c.text, c.secret, c.cursor, c.w)
		if got := g.Lines()[0]; got != c.want || g.CursorX != c.cx || !g.CursorOn {
			t.Errorf("input %q secret=%v cursor %d width %d: %q cursor %d, want %q cursor %d", c.text, c.secret, c.cursor, c.w, got, g.CursorX, c.want, c.cx)
		}
	}
	// A wide placeholder is cut at a cluster boundary.
	in := bx("input#q", "width: 5")
	in.Placeholder = "微信搜索"
	g := render(t, bx("col", "", in), 6, 1)
	checkGrid(t, g)
	if got := g.Lines()[0]; got != "微信  " || g.At(0, 0).Attrs&Dim == 0 {
		t.Errorf("placeholder = %q", got)
	}
}

// §21 test 17: paint does not change when another package sets uniseg's
// process-wide ambiguous width to 2.
func TestPaintIgnoresUnisegAmbiguousWidth(t *testing.T) {
	build := func() string {
		b := bx("box", "height: 3; border: rounded")
		b.Title = "…─ title ±"
		root := bx("col", "", b, tx("• ─ … ° ± § ¶ ① α", ""), tx("微信ok", ""))
		g := render(t, root, 16, 5)
		checkGrid(t, g)
		return strings.Join(g.Lines(), "\n") + Full(g)
	}
	before := build()
	saved := uniseg.EastAsianAmbiguousWidth
	uniseg.EastAsianAmbiguousWidth = 2
	defer func() { uniseg.EastAsianAmbiguousWidth = saved }()
	if after := build(); after != before {
		t.Errorf("paint depends on uniseg.EastAsianAmbiguousWidth:\n%s\n---\n%s", before, after)
	}
}

func TestWideANSI(t *testing.T) {
	p := &painter{g: NewGrid(6, 1)}
	p.text(all, 0, 0, "微e\u0301ok", cellStyle{})
	// Full: CHA after each complex cluster that does not end its row.
	if got := Full(p.g); got != AutowrapOff+"\x1b[0;39;49m微\x1b[3Ge\u0301\x1b[4Gok \x1b[0m\n"+AutowrapOn {
		t.Errorf("Full = %q", got)
	}
	if got := FullWith(p.g, Options{NoCHA: true}); got != AutowrapOff+"\x1b[0;39;49m微e\u0301ok \x1b[0m\n"+AutowrapOn {
		t.Errorf("Full without CHA = %q", got)
	}
	// No CHA after a complex cluster in the last column.
	q := &painter{g: NewGrid(3, 1)}
	q.text(all, 0, 0, "a微", cellStyle{})
	if got := Full(q.g); got != AutowrapOff+"\x1b[0;39;49ma微\x1b[0m\n"+AutowrapOn {
		t.Errorf("Full at the row end = %q", got)
	}
	// Diff: the first frame writes each wide cluster once, with CHA.
	full := Diff(nil, p.g)
	if want := "\x1b[0m\x1b[2J\x1b[1;1H\x1b[0;39;49m微\x1b[3Ge\u0301\x1b[4Gok \x1b[0m\x1b[?25l"; full != want {
		t.Errorf("Diff(nil) = %q, want %q", full, want)
	}
	if got := DiffWith(nil, p.g, Options{NoCHA: true}); strings.Contains(got, "G") {
		t.Errorf("NoCHA wrote CHA: %q", got)
	}
	// Changing only the continuation's half (a narrow cell over it) turns
	// the lead into a space: both columns are rewritten, lead first.
	next := &painter{g: cloneGrid(p.g)}
	next.set(all, 1, 0, 'x', cellStyle{})
	checkGrid(t, next.g)
	if got := Diff(p.g, next.g); got != "\x1b[1;1H\x1b[0;39;49m x\x1b[0m\x1b[?25l" {
		t.Errorf("Diff over a continuation = %q", got)
	}
	// Replacing a narrow pair by a wide cluster writes the lead once and
	// never the continuation column on its own. Its columns are erased
	// first (ECH), and the unchanged d after the CHA is written again
	// (§26.5).
	a := &painter{g: NewGrid(4, 1)}
	a.text(all, 0, 0, "abcd", cellStyle{})
	b := &painter{g: cloneGrid(a.g)}
	b.text(all, 1, 0, "信", cellStyle{})
	if got := Diff(a.g, b.g); got != "\x1b[1;2H\x1b[0;39;49m\x1b[2X信\x1b[4Gd\x1b[0m\x1b[?25l" {
		t.Errorf("Diff narrow to wide = %q", got)
	}
	// A style change on a wide cluster rewrites it whole.
	c := &painter{g: cloneGrid(b.g)}
	c.text(all, 1, 0, "信", cellStyle{attrs: Bold})
	if got := Diff(b.g, c.g); got != "\x1b[1;2H\x1b[0;1;39;49m\x1b[2X信\x1b[4G\x1b[0;39;49md\x1b[0m\x1b[?25l" {
		t.Errorf("Diff restyled wide = %q", got)
	}
	if got := Diff(c.g, c.g); got != "\x1b[0m\x1b[?25l" {
		t.Errorf("no change = %q", got)
	}
}

func cloneGrid(g *Grid) *Grid {
	c := *g
	c.Cells = append([]Cell(nil), g.Cells...)
	return &c
}

// FuzzGridColumns paints overlapping strings, wide ones included, at
// random places and clips, and checks the §12.1 invariants and MUST 6 after
// every write, then that Diff from a blank frame and between two frames
// never splits a wide cluster.
func FuzzGridColumns(f *testing.F) {
	f.Add("微信ok", "👨\u200D👩\u200D👧 e\u0301 🇺🇸", uint8(1), uint8(0), uint8(9), uint8(3))
	f.Add("ab\tc\x1b[1m", "한국어\u0301\u200B", uint8(7), uint8(1), uint8(2), uint8(2))
	f.Add("❤\uFE0F❤", "⸺⸻…", uint8(0), uint8(2), uint8(0), uint8(1))
	f.Fuzz(func(t *testing.T, s1, s2 string, x1, x2, cx, cw uint8) {
		if len(s1) > 64 || len(s2) > 64 {
			return
		}
		g := NewGrid(10, 3)
		p := &painter{g: g}
		clip := layout.Rect{X: int(cx % 12), Y: 0, W: int(cw % 12), H: 3}
		p.text(all, int(x1%12)-1, 0, s1, cellStyle{owner: "a", clearOwner: true})
		p.text(all, int(x2%12)-1, 0, s2, cellStyle{owner: "b", clearOwner: true, attrs: Bold})
		p.text(clip, int(x2%12)-2, 1, s1+s2, cellStyle{owner: "c", clearOwner: true})
		p.text(all, int(x1%12)-1, 2, s2, cellStyle{})
		p.set(all, int(cx%10), 0, 'x', cellStyle{})
		p.set(clip, int(cw%10), 1, 'y', cellStyle{})
		p.settle()
		p.unjoin()
		checkGrid(t, g)
		prev := NewGrid(10, 3)
		pp := &painter{g: prev}
		pp.text(all, 0, 0, s2+s1, cellStyle{})
		pp.unjoin()
		checkGrid(t, prev)
		for _, out := range []string{Diff(nil, g), Diff(prev, g), FullWith(g, Options{NoCHA: true})} {
			if !utf8.ValidString(out) {
				t.Fatalf("invalid UTF-8 in %q", out)
			}
		}
		// A blank frame diffed against g repaints it exactly: replaying the
		// written clusters column by column lands every one on its lead.
		replay(t, g, Diff(nil, g))
		replay(t, g, Diff(prev, g), prev)
		// §26.5: a terminal that measures every complex cluster differently (a
		// width-1 one 2 wide, a width-2 one 1 wide), with autowrap off as Run
		// sets it, damages only those clusters' own cells, in the first
		// frame and in a diff.
		disputed := map[string]int{}
		for _, gr := range []*Grid{prev, g} {
			for _, c := range gr.Cells {
				if c.Complex() {
					disputed[c.Grapheme()] = 3 - c.Width()
				}
			}
		}
		term := newFakeTerm(g.W, g.H, disputed)
		term.write(t, AutowrapOff+Diff(nil, prev))
		if bad := term.damage(prev); len(bad) != 0 {
			t.Fatalf("first frame damaged %v: %q", bad, term.cells)
		}
		term.write(t, Diff(prev, g))
		if bad := term.damage(g); len(bad) != 0 {
			t.Fatalf("diff frame %q damaged %v: %q", Diff(prev, g), bad, term.cells)
		}
	})
}

// replay interprets the escape sequences Diff writes (CUP, CHA, ECH, SGR,
// cursor visibility) on a cell model of the terminal, starting from base (or a
// blank screen), advancing by each cluster's width, and checks the result
// equals g cluster by cluster.
func replay(t *testing.T, g *Grid, out string, base ...*Grid) {
	t.Helper()
	screen := make([]string, g.W*g.H)
	for i := range screen {
		screen[i] = " "
	}
	if len(base) > 0 {
		for i, c := range base[0].Cells {
			screen[i] = c.Grapheme()
			if c.Cont {
				screen[i] = "<"
			}
		}
	}
	x, y := 0, 0
	for out != "" {
		if strings.HasPrefix(out, "\x1b[") {
			end := strings.IndexFunc(out[2:], func(r rune) bool { return r >= 0x40 && r <= 0x7e })
			if end < 0 {
				t.Fatalf("unterminated CSI in %q", out)
			}
			params, final := out[2:2+end], out[2+end]
			out = out[3+end:]
			switch final {
			case 'H':
				var row, col int
				fmt.Sscanf(params, "%d;%d", &row, &col)
				y, x = row-1, col-1
			case 'G':
				var col int
				fmt.Sscanf(params, "%d", &col)
				x = col - 1
			case 'J':
				for i := range screen {
					screen[i] = " "
				}
			case 'X':
				var n int
				fmt.Sscanf(params, "%d", &n)
				for i := x; i < min(x+max(n, 1), g.W); i++ {
					screen[y*g.W+i] = " "
				}
			}
			continue
		}
		// Text: the stream must hold, at the cursor, exactly the cluster of
		// the lead cell there. The replay follows the grid rather than
		// segmenting the stream: a diff writes runs of cells that are not
		// adjacent in the row.
		if y < 0 || y >= g.H || x < 0 || x >= g.W || g.At(x, y).Cont {
			t.Fatalf("text %q written at (%d,%d), not at a lead cell", out, x, y)
		}
		c, w := g.At(x, y).Grapheme(), g.At(x, y).Width()
		if !strings.HasPrefix(out, c) {
			t.Fatalf("at (%d,%d) Diff wrote %q, want the cell's cluster %q", x, y, out, c)
		}
		out = out[len(c):]
		screen[y*g.W+x] = c
		if w == 2 {
			screen[y*g.W+x+1] = "<"
		}
		x += w
	}
	for y := 0; y < g.H; y++ {
		for x := 0; x < g.W; x++ {
			want := g.At(x, y).Grapheme()
			if g.At(x, y).Cont {
				want = "<"
			}
			if got := screen[y*g.W+x]; got != want {
				t.Fatalf("after replay (%d,%d) = %q, want %q", x, y, got, want)
			}
		}
	}
}
