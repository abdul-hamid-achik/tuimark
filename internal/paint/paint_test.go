package paint

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// bx builds a box ("box#id" carries an id) styled by a style="" body.
func bx(kind, decls string, kids ...*layout.Box) *layout.Box {
	id := ""
	if i := strings.IndexByte(kind, '#'); i >= 0 {
		kind, id = kind[:i], kind[i+1:]
	}
	b := &layout.Box{Tag: kind, Kind: kind, ID: id, Follow: -1, Index: -1}
	if decls != "" {
		ds, err := css.ParseDecls(decls)
		if err != nil {
			panic(err)
		}
		b.Inline = ds
	}
	for _, k := range kids {
		k.Parent = b
		b.Children = append(b.Children, k)
	}
	return b
}

func tx(s, decls string) *layout.Box {
	b := bx("text", decls)
	b.Text = s
	return b
}

func styleAll(c *css.Cascade, b *layout.Box, parent *css.Style) {
	b.Style = c.Compute(b, parent, nil, b.Inline)
	for _, k := range b.Children {
		styleAll(c, k, &b.Style)
	}
}

// render styles, lays out, and paints root (+ modals) at cols×rows.
func render(t *testing.T, root *layout.Box, cols, rows int, modals ...*layout.Box) *Grid {
	t.Helper()
	c := css.NewCascade(nil, css.Env{Cols: cols, Rows: rows})
	styleAll(c, root, nil)
	for _, m := range modals {
		styleAll(c, m, nil)
	}
	if len(c.Diags) != 0 {
		t.Fatal(c.Diags)
	}
	(&layout.Engine{}).Layout(root, modals, cols, rows)
	return Paint(root, modals, cols, rows)
}

func lines(g *Grid) string { return strings.Join(g.Lines(), "\n") }

func TestNewGridAndLines(t *testing.T) {
	g := NewGrid(3, 2)
	if lines(g) != "   \n   " {
		t.Errorf("blank grid = %q", lines(g))
	}
	if g := NewGrid(-1, -1); g.W != 0 || g.H != 0 || len(g.Lines()) != 0 {
		t.Error("negative sizes clamp to 0")
	}
}

func TestBorderStyles(t *testing.T) {
	cases := map[string]string{
		"single":  "┌───┐\n│   │\n└───┘",
		"double":  "╔═══╗\n║   ║\n╚═══╝",
		"rounded": "╭───╮\n│   │\n╰───╯",
		"thick":   "┏━━━┓\n┃   ┃\n┗━━━┛",
		"none":    "     \n     \n     ",
	}
	for style, want := range cases {
		root := bx("col", "", bx("box", "width: 5; height: 3; border: "+style))
		if got := lines(render(t, root, 5, 3)); got != want {
			t.Errorf("border %s:\n%s\nwant:\n%s", style, got, want)
		}
	}
	// A box smaller than 2×2 cannot show a border.
	root := bx("col", "", bx("box", "width: 5; height: 1; border: single"))
	if got := lines(render(t, root, 5, 1)); got != "     " {
		t.Errorf("1-row bordered box = %q", got)
	}
}

func TestTitles(t *testing.T) {
	cases := []struct {
		width int
		title string
		want  string
	}{
		{12, "Inbox", "┌─ Inbox ──┐"},
		{11, "Inbox", "┌─ Inbox ─┐"}, // exact fit
		{9, "Inbox", "┌─ Inb…─┐"},    // one rule cell is kept before the corner
		{8, "Inbox long", "┌─ In…─┐"},
		{5, "abc", "┌─…─┐"},
		{4, "abc", "┌──┐"}, // too narrow for any title
	}
	for _, c := range cases {
		b := bx("box", "height: 3; border: single")
		b.Title = c.title
		root := bx("col", "width: "+strconv.Itoa(c.width), b)
		g := render(t, root, c.width, 3)
		if got := g.Lines()[0]; got != c.want {
			t.Errorf("width %d title %q: %q, want %q", c.width, c.title, got, c.want)
		}
	}
	// No border, no title.
	b := bx("box", "height: 1")
	b.Title = "hidden"
	if got := lines(render(t, bx("col", "", b), 10, 1)); strings.Contains(got, "hidden") {
		t.Errorf("title without a border: %q", got)
	}
	// title-color overrides the border color for the title only.
	b = bx("box", "height: 3; border: single; border-color: red; title-color: green")
	b.Title = "T"
	g := render(t, bx("col", "", b), 10, 3)
	if g.At(0, 0).FG.String() != "red" || g.At(3, 0).FG.String() != "green" || g.At(3, 0).Ch != 'T' {
		t.Errorf("colors: corner %v title %v %q", g.At(0, 0).FG, g.At(3, 0).FG, g.At(3, 0).Ch)
	}
}

func TestTextAndClipping(t *testing.T) {
	root := bx("col", "",
		bx("box", "width: 6; height: 3; border: single", tx("abcdefgh", "")),
		tx("left", "height: 1"),
		tx("mid", "height: 1; content-align: center"),
		tx("end", "height: 1; content-align: end"),
		tx("a long line here", "width: 10; height: 1; wrap: truncate"),
		tx("aaa bbb ccc", "width: 7; height: 2; wrap: wrap"),
		tx("one\ntwo\nthree", "height: 2"),
	)
	got := render(t, root, 10, 12).Lines()
	want := []string{
		"┌────┐    ",
		"│abcd│    ", // clipped to the parent's content box
		"└────┘    ",
		"left      ",
		"   mid    ",
		"       end",
		"a long li…",
		"aaa bbb   ",
		"ccc       ",
		"one       ",
		"two       ", // "three" is cut by the height
		"          ",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestChildDoesNotPaintOverParentBorder(t *testing.T) {
	root := bx("col", "",
		bx("box", "width: 6; height: 3; border: single",
			bx("box", "width: 20; height: 5; background: red", tx("xxxxxxxxxx", "")),
		),
	)
	got := lines(render(t, root, 8, 3))
	want := "┌────┐  \n│xxxx│  \n└────┘  "
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestButtonProgressRule(t *testing.T) {
	btn := bx("button", "width: 10; content-align: center")
	btn.Text = "OK"
	bbtn := bx("button", "width: 6; height: 3; border: rounded")
	bbtn.Text = "OK"
	root := bx("col", "",
		btn,
		bbtn,
		bx("rule", ""),
	)
	g := render(t, root, 10, 5)
	if got := g.Lines()[0]; got != "  [ OK ]  " {
		t.Errorf("button = %q", got)
	}
	if got := g.Lines()[2]; got != "│OK  │    " {
		t.Errorf("bordered button = %q", got)
	}
	if got := g.Lines()[4]; got != "──────────" {
		t.Errorf("rule x = %q", got)
	}

	for _, c := range []struct {
		v    float64
		want string
	}{
		{0, "░░░░░░░░░░"},
		{50, "█████░░░░░"},
		{33, "███░░░░░░░"},
		{35, "████░░░░░░"}, // rounds to the nearest cell
		{100, "██████████"},
		{-5, "░░░░░░░░░░"},
		{250, "██████████"},
	} {
		p := bx("progress", "")
		p.Value = c.v
		g := render(t, bx("col", "", p), 10, 1)
		if got := g.Lines()[0]; got != c.want {
			t.Errorf("progress %v = %q, want %q", c.v, got, c.want)
		}
		if c.v == 50 && (g.At(0, 0).Attrs&Dim != 0 || g.At(9, 0).Attrs&Dim == 0) {
			t.Error("the empty part of a progress bar is dim")
		}
	}

	ry := bx("rule", "")
	ry.Axis = "y"
	g = render(t, bx("row", "height: 3", ry, tx("x", "")), 3, 3)
	if got := lines(g); got != "│x \n│  \n│  " {
		t.Errorf("rule y:\n%s", got)
	}
}

func TestInput(t *testing.T) {
	mk := func(text, ph string, secret, focused bool, cursor int) *Grid {
		in := bx("input#q", "width: 6")
		in.Text, in.Placeholder, in.Secret, in.Focused, in.Cursor = text, ph, secret, focused, cursor
		return render(t, bx("col", "", in), 8, 1)
	}
	g := mk("abc", "search", false, true, 3)
	if got := g.Lines()[0]; got != "abc     " || !g.CursorOn || g.CursorX != 3 || g.CursorY != 0 {
		t.Errorf("value: %q cursor %v (%d,%d)", got, g.CursorOn, g.CursorX, g.CursorY)
	}
	g = mk("", "search", false, true, 0)
	if got := g.Lines()[0]; got != "search  " || g.At(0, 0).Attrs&Dim == 0 || g.CursorX != 0 {
		t.Errorf("placeholder: %q dim=%v cursor %d", got, g.At(0, 0).Attrs&Dim != 0, g.CursorX)
	}
	g = mk("", "a very long placeholder", false, false, 0)
	if got := g.Lines()[0]; got != "a very  " || g.CursorOn {
		t.Errorf("long placeholder: %q cursor %v", got, g.CursorOn)
	}
	g = mk("hunter2", "", true, false, 7)
	if got := g.Lines()[0]; got != "•••••   " {
		t.Errorf("secret (tail kept visible) = %q", got)
	}
	g = mk("abcdefghij", "", false, true, 10)
	if got := g.Lines()[0]; got != "fghij   " || g.CursorX != 5 {
		t.Errorf("scrolled value = %q cursor %d", got, g.CursorX)
	}
	g = mk("abcdefghij", "", false, true, 2)
	if got := g.Lines()[0]; got != "abcdef  " || g.CursorX != 2 {
		t.Errorf("cursor near the start = %q cursor %d", got, g.CursorX)
	}
}

func TestModalClearsUnderneath(t *testing.T) {
	under := tx(strings.Repeat("x", 10)+"\n"+strings.Repeat("x", 10)+"\n"+strings.Repeat("x", 10)+"\n"+strings.Repeat("x", 10)+"\n"+strings.Repeat("x", 10), "height: 5")
	root := bx("col", "", under)
	m := bx("modal#m", "width: 6; height: 3", tx("hi", ""))
	got := lines(render(t, root, 10, 5, m))
	want := "xxxxxxxxxx\nxx┌────┐xx\nxx│hi  │xx\nxx└────┘xx\nxxxxxxxxxx"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestOwnersAndStyles(t *testing.T) {
	root := bx("col#root", "",
		bx("box#panel", "height: 3; border: single; color: cyan; background: #102030",
			tx("hi", "bold: true; underline: true"),
		),
		bx("box", "height: 1", tx("anon", "reverse: true; italic: true; dim: true")),
	)
	g := render(t, root, 6, 4)
	if o := g.At(0, 0).Owner; o != "panel" {
		t.Errorf("border owner = %q", o)
	}
	if o := g.At(1, 1).Owner; o != "panel" {
		t.Errorf("text inside #panel owned by %q", o)
	}
	if o := g.At(0, 3).Owner; o != "root" {
		t.Errorf("anonymous box cell owned by %q, want the nearest id", o)
	}
	c := g.At(1, 1)
	if c.Ch != 'h' || c.FG.String() != "cyan" || c.BG.String() != "#102030" || c.Attrs != Bold|Underline {
		t.Errorf("text cell = %+v", c)
	}
	if g.At(4, 1).BG.String() != "#102030" || g.At(4, 1).Ch != ' ' {
		t.Errorf("background fill = %+v", g.At(4, 1))
	}
	if a := g.At(0, 3).Attrs; a != Dim|Italic|Reverse {
		t.Errorf("attrs = %b", a)
	}
	// visibility: hidden paints nothing, not even children.
	hidden := bx("col", "", bx("box", "height: 1; visibility: hidden", tx("gone", "")))
	if got := lines(render(t, hidden, 4, 1)); got != "    " {
		t.Errorf("hidden = %q", got)
	}
}

func TestGridHasNoControlCharacters(t *testing.T) {
	b := bx("box", "height: 3; border: single")
	b.Title = "a\nb\x1b"
	btn := bx("button", "")
	btn.Text = "o\tk"
	in := bx("input#i", "width: 5")
	in.Placeholder = "p\nq"
	g := render(t, bx("col", "", b, btn, in), 12, 5)
	for y, l := range g.Lines() {
		for _, r := range l {
			if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
				t.Errorf("row %d has control rune %U: %q", y, r, l)
			}
		}
	}
	if got := g.Lines()[0]; got != "┌─ a b  ───┐" {
		t.Errorf("title = %q", got)
	}
}

func TestANSI(t *testing.T) {
	c := Cell{Ch: 'x', FG: css.Color{Kind: css.ColorANSI, Index: 1}, BG: css.Color{Kind: css.ColorRGB, R: 1, G: 2, B: 3}, Attrs: Bold | Reverse}
	if got := SGR(c); got != "\x1b[0;1;7;31;48;2;1;2;3m" {
		t.Errorf("SGR = %q", got)
	}
	bright := Cell{FG: css.Color{Kind: css.ColorANSI, Index: 9}, BG: css.Color{Kind: css.ColorANSI, Index: 12}, Attrs: Dim | Italic | Underline}
	if got := SGR(bright); got != "\x1b[0;2;3;4;91;104m" {
		t.Errorf("SGR bright = %q", got)
	}
	def := Cell{FG: css.Color{Kind: css.ColorDefault}, BG: css.Color{Kind: css.ColorRGB, R: 255}}
	if got := SGR(def); got != "\x1b[0;39;48;2;255;0;0m" {
		t.Errorf("SGR default = %q", got)
	}

	a := NewGrid(3, 2)
	full := Diff(nil, a)
	if !strings.HasPrefix(full, "\x1b[0m\x1b[2J") || !strings.Contains(full, "\x1b[1;1H") || !strings.HasSuffix(full, "\x1b[?25l") {
		t.Errorf("full repaint = %q", full)
	}
	b := NewGrid(3, 2)
	if got := Diff(a, b); got != "\x1b[0m\x1b[?25l" {
		t.Errorf("no change = %q", got)
	}
	b.At(1, 1).Ch = 'Z'
	b.At(2, 1).Ch = 'Y'
	if got := Diff(a, b); got != "\x1b[2;2H\x1b[0;39;49mZY\x1b[0m\x1b[?25l" {
		t.Errorf("one run = %q", got)
	}
	b.CursorOn, b.CursorX, b.CursorY = true, 2, 0
	if got := Diff(b, b); got != "\x1b[0m\x1b[1;3H\x1b[?25h" {
		t.Errorf("cursor = %q", got)
	}
	if got := Diff(NewGrid(2, 2), a); !strings.Contains(got, "\x1b[2J") {
		t.Errorf("resize repaints: %q", got)
	}
	g := NewGrid(2, 1)
	g.At(0, 0).Ch = 'a'
	g.At(1, 0).Ch = 'b'
	g.At(1, 0).Attrs = Bold
	if got := Full(g); got != "\x1b[0;39;49ma\x1b[0;1;39;49mb\x1b[0m\n" {
		t.Errorf("Full = %q", got)
	}
}

// A reversed box is a bar: its whole rect is reversed, not only the glyphs
// its children paint (reverse inherits, like bold and dim).
func TestReverseFillsTheBox(t *testing.T) {
	root := bx("col", "",
		bx("row", "height: 1; width: 6; reverse: true", tx("ab", ""), tx("c", "")),
		bx("row", "height: 1; width: 6", tx("de", "")),
	)
	g := render(t, root, 8, 2)
	if got := g.Lines()[0]; got != "abc     " {
		t.Fatalf("row 0 = %q", got)
	}
	for x := 0; x < 8; x++ {
		rev := g.At(x, 0).Attrs&Reverse != 0
		if rev != (x < 6) {
			t.Errorf("cell %d reversed=%v", x, rev)
		}
		if g.At(x, 1).Attrs&Reverse != 0 {
			t.Errorf("unreversed row cell %d is reversed", x)
		}
	}
}

// Findings 16/31: paint walks only the visible (clipped) cells, so a huge
// logical box costs no more than a small one and paints the same glyphs.
func TestHugeBoxesPaintOnlyVisibleCells(t *testing.T) {
	build := func(n string) *layout.Box {
		return bx("col", "",
			bx("row", "height: 3",
				bx("box#a", "width: "+n+"; border: single; background: red", tx("hi", "")),
			),
			bx("box#b", "width: 10; height: "+n),
			bx("progress", "width: "+n),
			bx("rule", "width: "+n),
		)
	}
	withRule := func(n string) *layout.Box {
		r := bx("rule", "height: "+n)
		r.Axis = "y"
		return bx("row", "", r, bx("box#c", "width: "+n+"; border: double"))
	}
	type result struct{ big, small string }
	done := make(chan result, 1)
	go func() {
		var r result
		r.big = lines(render(t, build("99999999999"), 20, 8)) + "\n" + lines(render(t, withRule("99999999999"), 20, 4))
		r.small = lines(render(t, build("60"), 20, 8)) + "\n" + lines(render(t, withRule("60"), 20, 4))
		done <- r
	}()
	select {
	case r := <-done:
		if r.big != r.small {
			t.Errorf("huge boxes paint differently from boxes past the edge:\n%s\n---\n%s", r.big, r.small)
		}
		if !strings.HasPrefix(r.big, "┌───") || !strings.Contains(r.big, "│hi") {
			t.Errorf("grid:\n%s", r.big)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("painting a 99999999999-cell box did not finish: paint walks the logical rect, not the visible one")
	}
}
