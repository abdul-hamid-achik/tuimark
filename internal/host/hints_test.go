package host

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// SPEC v0.2b §6.12 (hints, label, keycap), §6.11 (sparkline), and §11.7
// (layout: grid) through the whole frame. §21 tests 56, 57, 58.

// hintText is the items of hints id in f, laid out or not, as
// "keycap label" joined by " | ", and how many are laid out.
func hintText(f *Frame, id string) (string, int) {
	var out []string
	laid := 0
	for _, it := range f.ByID[id].Children {
		out = append(out, it.Children[0].Text+" "+it.Children[1].Text)
		if it.Laid {
			laid++
		}
	}
	return strings.Join(out, " | "), laid
}

const hintsDoc = `<tui version="2">
<keymap>
  <bind keys="q,ctrl+q" action="quit" label="quit"/>
  <bind keys="q" action="shadowed" label="never"/>
  <bind keys="ctrl+i,ctrl+j" action="unreachable" label="nope"/>
  <bind keys="/" action="focus" to="#filter" label="filter"/>
  <bind keys="esc" action="cancel" when="#filter" label="cancel"/>
  <bind keys="esc" action="close" label="close"/>
  <bind keys="j" action="move-next" to="#empty" label="next-empty"/>
  <bind keys="k" action="move-next" to="#items" keycap="K" label="next"/>
  <bind keys="2" action="switch-to" to="#two" label="two"/>
  <bind keys="o" action="open_modal" label="open"/>
</keymap>
<screen id="main" focus="#items">
  <input id="filter" bind="query"/>
  <list id="items" each="items as it" key="it"><item><text>{it}</text></item></list>
  <list id="empty" each="none as n" key="n"><item><text>{n}</text></item></list>
  <tabs id="t"><tab id="one" label="1"><text>one</text></tab><tab id="two" label="2"><text>two</text></tab></tabs>
  <hints id="keys"/>
  <hints id="all" scope="all" style="layout: column"/>
  <modal id="m" open="show" on:escape="esc_modal"><button id="ok" label="ok"/></modal>
</screen>
</tui>`

func hintsApp(t *testing.T) *App {
	t.Helper()
	a := doc(t, hintsDoc)
	bindJSON(t, a, `{"query": "", "items": ["a", "b"], "none": [], "show": false}`)
	return a
}

// 57. scope="active" shows the hint rows one of whose keys would fire now,
// through the key dispatch: a row shadowed by an earlier row with the same
// key is hidden, ctrl+i/ctrl+j never arrive, a built-in whose target is
// incompatible (an empty list) is hidden, a switch-to an inactive tab is
// shown, keycap replaces the key text, and the input consumes printable
// keys; esc rows are hidden while the top modal has on:escape.
// scope="all" lists every labeled row with its keycap or first key.
func TestHintItems(t *testing.T) {
	a := hintsApp(t)
	f := a.Frame(200, 30)
	got, _ := hintText(f, "keys")
	if want := "q quit | / filter | esc close | K next | 2 two | o open"; got != want {
		t.Errorf("list focused:\n got %s\nwant %s", got, want)
	}
	all, _ := hintText(f, "all")
	if want := "q quit | q never | ctrl+i nope | / filter | esc cancel | esc close | j next-empty | K next | 2 two | o open"; all != want {
		t.Errorf("scope=all:\n got %s\nwant %s", all, want)
	}
	_ = a.Set("@focus", "#filter")
	f = a.Frame(200, 30)
	if got, _ := hintText(f, "keys"); got != "ctrl+q quit | esc cancel" {
		t.Errorf("input focused: %s", got)
	}
	_ = a.Set("show", true)
	f = a.Frame(200, 30)
	if got, _ := hintText(f, "keys"); got != "q quit | / filter | K next | 2 two | o open" {
		t.Errorf("modal with on:escape: %s", got)
	}
	// The items are generated nodes: a row and two texts, classes
	// hint-key and hint-label, in the frame's hints.
	it := f.ByID["keys"].Children[0]
	if it.Tag != "row" || it.Role != layout.RoleHintItem || it.Children[0].Classes[0] != "hint-key" || it.Children[1].Classes[0] != "hint-label" {
		t.Errorf("item %+v", it)
	}
}

// 57. Layout: items one after another along the main axis, separated by
// the hints' gap, each keycap + the row's gap + label wide; the first
// item that does not fit in the hints' content box, and all after it, are
// not laid out, painted, or dumped; under layout: column one row per
// item. The intrinsic size counts every item, and a hints never shrinks.
func TestHintLayout(t *testing.T) {
	const keys = `<keymap><bind keys="a" action="x" label="one"/><bind keys="b" action="y" label="two"/><bind keys="c" action="z" keycap="C!" label="three"/></keymap>`
	a := doc(t, `<tui version="2"><style>#h > row { gap: 2; } #v { layout: column; gap: 1; }</style>`+keys+`
<screen id="s"><row id="r" height="1"><hints id="h"/><text id="after">|</text></row><hints id="v"/></screen></tui>`)
	f := a.Frame(40, 8)
	h := f.ByID["h"]
	// Items: "a  one" 6, "b  two" 6, "C!  three" 9, gap 2: 6+2+6+2+9 = 25.
	if h.W != 25 || h.H != 1 || f.ByID["after"].X != 25 {
		t.Errorf("hints %dx%d, after at %d", h.W, h.H, f.ByID["after"].X)
	}
	// Column: 3 rows and 2 gaps; items at their own width ("C! three").
	v := f.ByID["v"]
	if v.W != 40 || v.H != 5 || v.Children[2].Y != v.Y+4 || v.Children[2].W != 8 {
		t.Errorf("column hints %dx%d, third item at y %d w %d", v.W, v.H, v.Children[2].Y, v.Children[2].W)
	}
	// Even at 20 columns the row keeps the hints' full width (it never
	// shrinks): its content box holds all three items, clipped.
	if f = a.Frame(20, 8); f.ByID["h"].W != 25 {
		t.Errorf("hints shrank to %d", f.ByID["h"].W)
	}

	b := doc(t, `<tui version="2"><style>#h > row { gap: 2; } #v { layout: column; gap: 1; height: 4; }</style>`+keys+`
<screen id="s"><hints id="h"/><hints id="v"/></screen></tui>`)
	f = b.Frame(20, 8)
	if _, laid := hintText(f, "h"); laid != 2 {
		t.Errorf("row hints at 20 columns: %d items laid out, want 2", laid)
	}
	if _, laid := hintText(f, "v"); laid != 2 {
		t.Errorf("column hints 4 rows high: %d items laid out, want 2", laid)
	}
	if strings.TrimRight(f.Grid.Lines()[0], " ") != "a  one  b  two" {
		t.Errorf("row 0 %q", f.Grid.Lines()[0])
	}
	d := b.Dump(20, 8, false)
	rows := 0
	for _, n := range d.Nodes {
		if n.Tag == "row" {
			rows++
		}
	}
	if rows != 4 {
		t.Errorf("dumped %d hint rows, want the 4 that fit", rows)
	}
}

// 57 (property). For every state of the fixture, pressing the first key
// that would fire each shown item's row fires that row (its host action
// is the event), and no key of a hidden labeled row fires it.
func TestHintsMatchDispatch(t *testing.T) {
	type state struct {
		name  string
		setup func(a *App)
	}
	states := []state{
		{"list", func(a *App) {}},
		{"input", func(a *App) { _ = a.Set("@focus", "#filter") }},
		{"modal", func(a *App) { _ = a.Set("show", true) }},
		{"tab two", func(a *App) { _ = a.Set("@focus", "#filter"); a.Frame(200, 30); a.HandleKey(Key{Name: "2", Rune: '2'}) }},
	}
	hostRow := func(action string) bool {
		switch action {
		case "quit", "shadowed", "unreachable", "cancel", "close", "open_modal":
			return true
		}
		return false
	}
	for _, s := range states {
		a := hintsApp(t)
		s.setup(a)
		f := a.Frame(200, 30)
		shown := map[string]bool{}
		for _, it := range f.ByID["keys"].Children {
			shown[it.Children[1].Text] = true
		}
		for i, kb := range a.doc.Keymap {
			if !hostRow(kb.Action) {
				continue
			}
			for _, tok := range kb.Keys {
				if neverArrives(tok) {
					continue
				}
				b := hintsApp(t)
				s.setup(b)
				b.Frame(200, 30)
				evs := b.HandleKey(keyOf(tok))
				fired := len(evs) == 1 && evs[0].Action == kb.Action
				if fired && !shown[kb.Label] {
					t.Errorf("%s: key %s fires row %d (%s), which is hidden", s.name, tok, i, kb.Label)
				}
				if shown[kb.Label] && tok == firstFiring(b, i) && !fired {
					t.Errorf("%s: key %s of shown row %d (%s) fires %v", s.name, tok, i, kb.Label, evs)
				}
			}
		}
	}
}

// firstFiring is the first key of keymap row i that the dispatch would
// send to it on a's live frame, or "".
func firstFiring(a *App, i int) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, tok := range a.doc.Keymap[i].Keys {
		if neverArrives(tok) {
			continue
		}
		if p := a.planKey(a.liveView(), keyOf(tok)); p.step == stepKeymap && p.row == i {
			return tok
		}
	}
	return ""
}

// 56. Sparkline diagnostics: a string element is B009 (a gap), a value
// that is not an array is B009 and paints nothing, a missing path is B003;
// an empty array paints nothing; the intrinsic width is the array's
// length.
func TestSparklineDiagnostics(t *testing.T) {
	src := `<tui version="2"><screen id="s"><row height="1"><sparkline id="sp" bind="h" min="0" max="8"/><text id="end">|</text></row></screen></tui>`
	for _, c := range []struct {
		data, code, row string
	}{
		{`{"h": [0, 1, "x", 8]}`, "B009", " ▁ █|"},
		{`{"h": "x"}`, "B009", "|"},
		{`{"h": {"a": 1}}`, "B009", "|"},
		{`{}`, "B003", "|"},
		{`{"h": []}`, "", "|"},
		{`{"h": [null, 4]}`, "", " ▄|"},
	} {
		a := doc(t, src)
		bindJSON(t, a, c.data)
		f := a.Frame(8, 1)
		var codes []string
		for _, d := range f.Diags {
			codes = append(codes, d.Code)
		}
		if got := strings.TrimRight(f.Grid.Lines()[0], " "); strings.Join(codes, " ") != c.code || got != c.row {
			t.Errorf("%s: codes %v row %q, want %q %q", c.data, codes, got, c.code, c.row)
		}
	}
}

// 92. scale="NAME" groups are formed after layout, over the laid-out
// sparklines of the frame (SPEC v0.3b §6.11): a visibility: hidden member
// is laid out, so it belongs to its group and its values count; an
// inactive tab's sparkline is never inflated at all, so it cannot join
// one; a group of one gets the group fields too (with its own shown
// min/max, so painting it is unaffected either way).
func TestSparklineScaleGrouping(t *testing.T) {
	src := `<tui version="3"><screen id="s">
    <sparkline id="a" bind="a" scale="t" width="1" height="1"/>
    <sparkline id="hidden" bind="hd" scale="t" width="1" height="1" style="visibility: hidden"/>
    <tabs id="nav">
      <tab id="x" label="x"><spacer/></tab>
      <tab id="y" label="y"><sparkline id="inactive" bind="ia" scale="t" width="1" height="1"/></tab>
    </tabs>
    <sparkline id="solo" bind="so" scale="solo" width="5" height="1"/>
  </screen></tui>`
	a := doc(t, src)
	bindJSON(t, a, `{"a": [0], "hd": [100], "ia": [9999], "so": [3, 7, 1, 9, 4]}`)
	f := a.Frame(10, 4)
	for _, id := range []string{"a", "hidden"} {
		b := f.ByID[id]
		if b == nil || !b.HasGroupLo || !b.HasGroupHi || b.GroupLo != 0 || b.GroupHi != 100 {
			t.Errorf("%s: group %+v", id, b)
		}
	}
	if f.ByID["inactive"] != nil {
		t.Errorf("inactive tab's sparkline was inflated: %+v", f.ByID["inactive"])
	}
	solo := f.ByID["solo"]
	if solo == nil || !solo.HasGroupLo || !solo.HasGroupHi || solo.GroupLo != 1 || solo.GroupHi != 9 {
		t.Errorf("solo: group %+v", solo)
	}
}

// 58. The cores example: inside a panel with a 1-cell border, with
// grid-columns 4, grid-min-width 22, gap 1, the grid has 1, 3, and 4
// columns at 40, 80, and 120.
func TestGridCoresColumns(t *testing.T) {
	a := doc(t, `<tui version="2"><style>
#panel { border: single; }
#cores { layout: grid; grid-columns: 4; grid-min-width: 22; gap: 1; }
</style><screen id="s"><box id="panel"><box id="cores" each="cores as c" key="c"><text>{c}</text></box></box></screen></tui>`)
	bindJSON(t, a, `{"cores": [0,1,2,3,4,5,6,7,8,9,10,11]}`)
	for cols, want := range map[int]int{40: 1, 80: 3, 120: 4} {
		f := a.Frame(cols, 24)
		xs := map[int]bool{}
		for _, c := range f.ByID["cores"].Children {
			xs[c.X] = true
		}
		if len(xs) != want {
			t.Errorf("%d cols: %d columns, want %d", cols, len(xs), want)
		}
	}
}

// 58. align places a shorter child in its row; L007 reports an author
// width and an author fr height on a grid child, but not a plain col's
// built-in 1fr; a grid inside a <scroll> keeps its full height and
// scrolls; a <scroll> or overflow: scroll grid is shrinkable on y and
// auto-fits, and a plain grid box is not.
func TestGridAlignL007AndScroll(t *testing.T) {
	a := doc(t, `<tui version="2"><style>
#g { layout: grid; grid-columns: 2; align: center; }
#w { width: 5; }
#fr { height: 1fr; }
</style><screen id="s"><box id="g"><col><text>a</text><text>b</text><text>c</text></col><text id="short">x</text><text id="w">w</text><col id="fr"><text>f</text></col></box></screen></tui>`)
	f := a.Frame(20, 8)
	if s := f.ByID["short"]; s.Y != 1 || s.H != 1 || s.X != 10 {
		t.Errorf("align: center: short at (%d,%d) h %d", s.X, s.Y, s.H)
	}
	var l007 []string
	for _, d := range f.Diags {
		if d.Code == "L007" {
			l007 = append(l007, d.ID)
		}
	}
	if strings.Join(l007, " ") != "w fr" {
		t.Errorf("L007 for %v, want w and fr", l007)
	}

	sc := doc(t, `<tui version="2"><style>#g { layout: grid; grid-columns: 2; }</style>
<screen id="s"><scroll id="v" height="3"><box id="g" each="xs as x" key="x"><text>{x}</text></box></scroll><text id="after">z</text></screen></tui>`)
	bindJSON(t, sc, `{"xs": [1,2,3,4,5,6,7,8,9,10]}`)
	f = sc.Frame(20, 10)
	if g := f.ByID["g"]; g.H != 5 || f.ByID["v"].ContentH != 5 {
		t.Errorf("grid in a scroll: h %d, extent %d", g.H, f.ByID["v"].ContentH)
	}

	// 20 one-row cells in 2 columns: 10 rows; 6 rows leave 5 above "after".
	fit := func(tag, style string) (h int, after int) {
		cells := strings.Repeat(`<text>c</text>`, 20)
		b := doc(t, `<tui version="2"><style>#g { layout: grid; grid-columns: 2; `+style+` }</style>
<screen id="s"><`+tag+` id="g">`+cells+`</`+tag+`><text id="after">z</text></screen></tui>`)
		f := b.Frame(20, 6)
		return f.ByID["g"].H, f.ByID["after"].Y
	}
	if h, after := fit("box", "overflow: scroll;"); h != 5 || after != 5 {
		t.Errorf("overflow: scroll grid: h %d, after at %d", h, after)
	}
	if h, after := fit("scroll", ""); h != 5 || after != 5 {
		t.Errorf("scroll grid: h %d, after at %d", h, after)
	}
	if h, after := fit("box", ""); h != 10 || after != 10 {
		t.Errorf("plain grid box: h %d, after at %d (not shrinkable)", h, after)
	}
}

// 57 / 54. An empty keycap= counts as absent, as an empty tab short=
// does: the item shows its key (scope active and all), and the strip's
// tier 2 shows the tab's label.
func TestEmptyKeycapAndShortCountAsAbsent(t *testing.T) {
	a := doc(t, `<tui version="2"><keymap>
<bind keys="q" action="quit" label="quit"/>
<bind keys="enter" action="go" keycap="" label="go"/>
</keymap><screen id="main">
<tabs id="t"><tab id="a" label="alpha" short=""><text>x</text></tab><tab id="b" label="beta" short="b"><text>y</text></tab></tabs>
<hints id="h"/>
<hints id="all" scope="all" style="layout: column"/>
</screen></tui>`)
	f := a.Frame(20, 5)
	if got, laid := hintText(f, "h"); got != "q quit | enter go" || laid != 2 {
		t.Errorf("active: %q, %d laid out", got, laid)
	}
	if got, _ := hintText(f, "all"); got != "q quit | enter go" {
		t.Errorf("all: %q", got)
	}
	if g := f.Grid.Lines(); g[2] != "q quit  enter go    " {
		t.Errorf("hints row %q", g[2])
	}
	// W1 = 9 > 8 and W2 = width("alpha") + width("b") = 6: tier 2.
	if g := a.Frame(8, 5).Grid.Lines(); g[0] != "alphab  " {
		t.Errorf("tier 2 strip %q", g[0])
	}
}

// 58 (and v1 §11.3). A left dock in a <scroll> grid is pulled out of the
// content extent, not laid out as a grid cell (no L007), and the extent
// reaches the last row, so move-last scrolls to it.
func TestScrollGridWithDock(t *testing.T) {
	a := doc(t, `<tui version="2"><style>
#sv { layout: grid; grid-columns: 4; grid-min-width: 10; gap: 1; }
#side { dock: left; width: 20; }
</style><keymap><bind keys="G" action="move-last" to="#sv"/></keymap>
<screen id="main"><scroll id="sv"><text id="side">SIDE</text><text>c0</text><text>c1</text><text>c2</text><text>c3</text><text>c4</text><text id="c5">c5</text></scroll><text id="status">status line</text></screen></tui>`)
	f := a.Frame(42, 8)
	if len(f.Diags) != 0 {
		t.Fatalf("diags %v", f.Diags)
	}
	if s, sv := f.ByID["side"], f.ByID["sv"]; s.X != 0 || s.W != 20 || sv.ContentH != 5 || f.ByID["status"].Y != 5 {
		t.Errorf("side %dx%d @(%d,%d), extent %d, status y %d", s.W, s.H, s.X, s.Y, sv.ContentH, f.ByID["status"].Y)
	}
	run(a, 42, 4, r('G'))
	f = a.Frame(42, 4)
	if sv, c5 := f.ByID["sv"], f.ByID["c5"]; sv.ScrollY != 2 || c5.Y != 2 || !strings.Contains(f.Grid.Lines()[2], "c5") {
		t.Errorf("move-last: offset %d, c5 y %d, grid %q", sv.ScrollY, c5.Y, f.Grid.Lines())
	}
}
