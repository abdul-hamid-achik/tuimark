package host

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// ids lists the ids of laid-out nodes in dump order.
func ids(d *dump.Dump) string {
	var out []string
	for _, n := range d.Nodes {
		if n.ID != "" {
			out = append(out, n.ID)
		}
	}
	return strings.Join(out, " ")
}

func nodeByID(d *dump.Dump, id string) (dump.Node, bool) {
	for _, n := range d.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return dump.Node{}, false
}

func grid(d *dump.Dump) string { return strings.Join(d.Grid, "\n") }

// SPEC §7 truthiness drives if= / hidden= / disabled=.
func TestIfGuards(t *testing.T) {
	a := doc(t, v1(`<col>
  <box id="yes" if="v"/>
  <box id="no" if="!v"/>
</col>`))
	for _, c := range []struct {
		name string
		v    any
		want string
	}{
		{"null", nil, "no"},
		{"false", false, "no"},
		{"0", 0, "no"},
		{`""`, "", "no"},
		{"[]", []any{}, "no"},
		{"true", true, "yes"},
		{"1", 1, "yes"},
		{`"0"`, "0", "yes"},
		{"[0]", []any{0}, "yes"},
		{"{}", map[string]any{}, "yes"},
	} {
		if err := a.Bind("v", c.v); err != nil {
			t.Fatal(err)
		}
		if got := ids(a.Dump(10, 4, false)); got != "main "+c.want {
			t.Errorf("v=%s: nodes %q, want %q", c.name, got, c.want)
		}
	}
	// A missing path is falsy and warns B002.
	b := doc(t, v1(`<box id="x" if="missing"/><box id="y" if="!missing"/>`))
	d := b.Dump(10, 4, false)
	if ids(d) != "main y" || len(d.Errors) != 2 || d.Errors[0].Code != "B002" || d.Errors[1].Code != "B002" || !d.OK {
		t.Errorf("missing guard: %q %v", ids(d), d.Errors)
	}
}

// SPEC §21 test 13: if="!selected_ticket" swaps the detail for "no selection".
func TestIfSelectedTicketNull(t *testing.T) {
	a := inbox(t)
	d := a.Dump(120, 24, false)
	if !strings.Contains(grid(d), "login loop on staging") || strings.Contains(grid(d), "no selection") {
		t.Fatalf("with a selection:\n%s", grid(d))
	}
	if err := a.Set("selected_ticket", nil); err != nil {
		t.Fatal(err)
	}
	d = a.Dump(120, 24, false)
	detail := ""
	for y := 2; y < 23; y++ {
		detail += string([]rune(d.Grid[y])[33:]) + "\n"
	}
	if strings.Contains(detail, "login loop on staging") || !strings.Contains(detail, "no selection") {
		t.Errorf("selected_ticket=null:\n%s", grid(d))
	}
	if !d.OK {
		t.Errorf("null is a value, not a missing path: %v", d.Errors)
	}
}

func TestHiddenAndDisabled(t *testing.T) {
	a := doc(t, v1(`<col>
  <input id="a"/>
  <input id="b" hidden="true"/>
  <input id="c" disabled="true"/>
  <input id="d" hidden="h"/>
  <input id="e" disabled="!enabled"/>
  <col disabled="true"><input id="f"/></col>
  <input id="g" style="visibility: hidden"/>
  <box id="h" focusable="true" style="height: 1"/>
  <button id="i" focusable="false">x</button>
  <input id="j"/>
</col>`))
	_ = a.Bind("", map[string]any{"h": true, "enabled": false})
	f := a.Frame(20, 12)
	if _, ok := f.ByID["b"]; ok {
		t.Error("hidden=true prunes the node")
	}
	if _, ok := f.ByID["d"]; ok {
		t.Error("hidden=path prunes the node")
	}
	var focusables []string
	for _, b := range f.Focusables {
		focusables = append(focusables, b.ID)
	}
	if got := strings.Join(focusables, " "); got != "a h j" {
		t.Errorf("focusables = %q (skip hidden, disabled, disabled ancestors, visibility:hidden, focusable=false)", got)
	}
	if !f.ByID["c"].Disabled || !f.ByID["e"].Disabled || !f.ByID["f"].Disabled {
		t.Error("disabled flags")
	}
	_ = a.Bind("enabled", true)
	f = a.Frame(20, 12)
	if f.ByID["e"].Disabled {
		t.Error("disabled=!path follows the store")
	}
}

// SPEC §21 test 14: each="tickets as item" produces exactly three items.
func TestEachProducesItems(t *testing.T) {
	a := inbox(t)
	d := a.Dump(80, 24, false)
	var items []dump.Node
	for _, n := range d.Nodes {
		if n.Tag == "item" {
			items = append(items, n)
		}
	}
	if len(items) != 3 {
		t.Fatalf("items = %d", len(items))
	}
	for i, want := range []string{"t-12", "t-18", "t-21"} {
		if items[i].Key != want {
			t.Errorf("item %d key = %q, want %q", i, items[i].Key, want)
		}
		if items[i].Y != items[0].Y+i {
			t.Errorf("item %d at y=%d", i, items[i].Y)
		}
	}
	if !items[0].Selected || items[1].Selected {
		t.Error("selected follows bind=\"selected\" (t-12)")
	}
	if !strings.Contains(grid(d), "t-18cannot deploy europe-west") {
		t.Errorf("item template resolves {item.*}:\n%s", grid(d))
	}
	// The list re-inflates when the data changes.
	_ = a.Set("tickets", []any{map[string]any{"id": "x-1", "title": "only"}})
	d = a.Dump(80, 24, false)
	n := 0
	for _, nd := range d.Nodes {
		if nd.Tag == "item" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("after Set: %d items", n)
	}
}

func TestEachErrorsAndKeys(t *testing.T) {
	src := v1(`<list id="l" each="rows as r" key="r.id"><item><text>{r.name}</text></item></list>`)
	cases := []struct {
		name  string
		data  map[string]any
		codes string
		keys  string
	}{
		{"missing", map[string]any{}, "B001", ""},
		{"not an array", map[string]any{"rows": "x"}, "B001", ""},
		{"object", map[string]any{"rows": map[string]any{}}, "B001", ""},
		{"empty", map[string]any{"rows": []any{}}, "", ""},
		{"keys", map[string]any{"rows": []any{map[string]any{"id": "a", "name": "A"}, map[string]any{"id": 2, "name": "B"}}}, "", "a 2"},
		{"missing key", map[string]any{"rows": []any{map[string]any{"name": "A"}}}, "B003", "0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := doc(t, src)
			_ = a.Bind("", c.data)
			d := a.Dump(20, 5, false)
			var codes, keys []string
			for _, e := range d.Errors {
				codes = append(codes, e.Code)
			}
			for _, n := range d.Nodes {
				if n.Tag == "item" {
					keys = append(keys, n.Key)
				}
			}
			if strings.Join(codes, " ") != c.codes || strings.Join(keys, " ") != c.keys {
				t.Errorf("codes %v keys %v", codes, keys)
			}
			if c.codes == "B001" && d.OK {
				t.Error("B001 is an error")
			}
		})
	}
	// Without key= the index is the key (B006 is a parse warning).
	a := doc(t, v1(`<list id="l" each="rows as r"><item><text>{r}</text></item></list>`))
	_ = a.Bind("rows", []string{"x", "y"})
	d := a.Dump(10, 3, false)
	if d.Nodes[2].Key != "0" || d.Nodes[4].Key != "1" || !strings.HasPrefix(d.Grid[1], "y") {
		t.Errorf("index keys: %+v\n%s", d.Nodes, grid(d))
	}
	if len(d.Errors) != 1 || d.Errors[0].Code != "B006" {
		t.Errorf("errors = %v", d.Errors)
	}
}

func TestListSelectionFromBind(t *testing.T) {
	a := inbox(t)
	_ = a.Set("selected", "t-21")
	a.Frame(80, 24) // the live frame moves the selection; Dump is a snapshot
	d := a.Dump(80, 24, false)
	var sel []string
	for _, n := range d.Nodes {
		if n.Selected {
			sel = append(sel, n.Key)
		}
	}
	if strings.Join(sel, " ") != "t-21" {
		t.Errorf("selected = %v", sel)
	}
	// An unknown bound key keeps the current selection.
	_ = a.Set("selected", "nope")
	d = a.Dump(80, 24, false)
	for _, n := range d.Nodes {
		if n.Selected && n.Key != "t-21" {
			t.Errorf("selection moved to %s", n.Key)
		}
	}
	// Keys compare by type as well as text: 1 is not "1".
	b := doc(t, v1(`<list id="l" each="rows as r" key="r.id" bind="sel"><item><text>{r.id}</text></item></list>`))
	_ = b.Bind("", map[string]any{"sel": "1", "rows": []any{map[string]any{"id": 0}, map[string]any{"id": 1}, map[string]any{"id": "1"}}})
	d = b.Dump(10, 4, false)
	for _, n := range d.Nodes {
		if n.Selected && n.Key != "1" {
			t.Errorf("selected %q", n.Key)
		}
	}
	if !d.Nodes[6].Selected {
		t.Errorf("the string key \"1\" (third item) is selected: %+v", d.Nodes)
	}
}

// The built-in list > item:selected { reverse: true } must be visible: the
// whole selected row is reversed, glyphs and gaps alike; other rows are not.
func TestSelectedItemIsHighlighted(t *testing.T) {
	a := inbox(t)
	f := a.Frame(80, 24)
	list := f.ByID["inbox"]
	sel := list.Children[0]
	if !sel.Selected || !sel.Style.Reverse {
		t.Fatalf("selected item style: %+v", sel.Style)
	}
	for x := sel.X; x < sel.X+sel.W; x++ {
		if c := f.Grid.At(x, sel.Y); c.Attrs&paint.Reverse == 0 {
			t.Fatalf("cell (%d,%d) %q of the selected row is not reversed", x, sel.Y, c.Ch)
		}
	}
	other := list.Children[1]
	for x := other.X; x < other.X+other.W; x++ {
		if c := f.Grid.At(x, other.Y); c.Attrs&paint.Reverse != 0 {
			t.Fatalf("cell (%d,%d) of an unselected row is reversed", x, other.Y)
		}
	}
	// The border next to the list is untouched.
	if c := f.Grid.At(sel.X-1, sel.Y); c.Ch != '│' || c.Attrs&paint.Reverse != 0 {
		t.Errorf("border cell = %+v", c)
	}
}

func TestStaticListAndItems(t *testing.T) {
	a := doc(t, v1(`<list id="l" on:select="pick"><item id="first"><text>A</text></item><item><text>B</text></item></list>`))
	f := a.Frame(10, 3)
	l := f.ByID["l"]
	if len(l.Children) != 2 || l.Children[0].Key != "first" || l.Children[1].Key != "1" || !l.Children[0].Selected {
		t.Fatalf("static items: %+v", l.Children)
	}
	if _, ok := f.ByID["first"]; ok {
		t.Error("list items are not addressable by id (their list navigates them)")
	}
	evs := a.HandleKey(Key{Name: "down"})
	if len(evs) != 1 || evs[0].Value != "1" && evs[0].Value != 1.0 {
		t.Errorf("static list select = %+v", evs)
	}
	if evs[0].Action != "pick" || evs[0].Source != "l" || len(evs[0].Keys) != 0 {
		t.Errorf("payload = %+v", evs[0])
	}
	a.Frame(10, 3)
	evs = a.HandleKey(Key{Name: "up"})
	if len(evs) != 1 || evs[0].Value != "first" {
		t.Errorf("static item with id: value = %#v", evs)
	}
}

func TestDisplayNoneAndMedia(t *testing.T) {
	a := inbox(t)
	for _, c := range []struct {
		cols   int
		header bool
		stack  bool
	}{
		{120, true, false},
		{81, true, false},
		{80, true, true},
		{40, false, true},
	} {
		d := a.Dump(c.cols, 24, false)
		_, hasHeader := nodeByID(d, "header")
		side, _ := nodeByID(d, "sidebar")
		det, _ := nodeByID(d, "detail")
		stacked := side.X == det.X && det.Y > side.Y
		if hasHeader != c.header || stacked != c.stack {
			t.Errorf("%d cols: header=%v stacked=%v", c.cols, hasHeader, stacked)
		}
	}
}

func TestScrollStateIsRemembered(t *testing.T) {
	a := doc(t, v1(`<scroll id="s" focusable="true" style="height: 3"><text>1
2
3
4
5
6</text></scroll>`))
	a.Frame(10, 5)
	for i := 0; i < 10; i++ {
		a.HandleKey(Key{Name: "down"})
	}
	f := a.Frame(10, 5)
	if s := f.ByID["s"]; s.ScrollY != 3 {
		t.Errorf("offset clamped to content-viewport: %d", s.ScrollY)
	}
	if d := a.Dump(10, 5, false); !strings.HasPrefix(d.Grid[0], "4") {
		t.Errorf("scrolled grid:\n%s", grid(d))
	}
	a.HandleKey(Key{Name: "home"})
	if d := a.Dump(10, 5, false); !strings.HasPrefix(d.Grid[0], "1") {
		t.Errorf("home:\n%s", grid(d))
	}
	a.HandleKey(Key{Name: "end"})
	if f := a.Frame(10, 5); f.ByID["s"].ScrollY != 3 {
		t.Errorf("end: %d", f.ByID["s"].ScrollY)
	}
	a.HandleKey(Key{Name: "pgup"})
	if f := a.Frame(10, 5); f.ByID["s"].ScrollY != 0 {
		t.Errorf("pgup: %d", f.ByID["s"].ScrollY)
	}
	a.HandleKey(Key{Name: "pgdn"})
	if f := a.Frame(10, 5); f.ByID["s"].ScrollY != 3 {
		t.Errorf("pgdn: %d", f.ByID["s"].ScrollY)
	}
	for _, k := range []string{"left", "right"} {
		if evs := a.HandleKey(Key{Name: k}); evs != nil {
			t.Errorf("%s: %v", k, evs)
		}
	}
}

func TestListFollowsSelection(t *testing.T) {
	a := doc(t, v1(`<list id="l" each="rows as r" style="height: 3"><item><text>{r}</text></item></list>`))
	_ = a.Bind("rows", []string{"a", "b", "c", "d", "e", "f"})
	a.Frame(10, 3)
	for i := 0; i < 4; i++ {
		a.HandleKey(Key{Name: "down"})
	}
	a.Frame(10, 3) // the live frame remembers the offset; Dump is a snapshot
	d := a.Dump(10, 3, false)
	if got := grid(d); got != "c         \nd         \ne         " {
		t.Errorf("list scrolled to keep the selection visible:\n%s", got)
	}
	// Moving up inside the viewport keeps the offset (no jump back to 0).
	a.HandleKey(Key{Name: "up"})
	if got := grid(a.Dump(10, 3, false)); got != "c         \nd         \ne         " {
		t.Errorf("after up:\n%s", got)
	}
	a.HandleKey(Key{Name: "down"})
	a.HandleKey(Key{Name: "pgup"}) // page = viewport height (3)
	d = a.Dump(10, 3, false)
	if got := grid(d); got != "b         \nc         \nd         " {
		t.Errorf("after pgup:\n%s", got)
	}
	if f := a.Frame(10, 3); f.ByID["l"].Children[1].Selected != true {
		t.Error("pgup moved the selection by one page (4 -> 1)")
	}
}

func TestFrameBoxesAreFresh(t *testing.T) {
	a := inbox(t)
	f1 := a.Frame(80, 24)
	f2 := a.Frame(120, 24)
	if f1.Root == f2.Root {
		t.Error("each frame builds a new box tree")
	}
	var boxes int
	f2.Root.Walk(func(*layout.Box) { boxes++ })
	if boxes != len(a.Dump(120, 24, false).Nodes) {
		t.Error("dump lists every laid-out node")
	}
}

// overflow: scroll turns any container into a viewport like <scroll>.
func TestOverflowScroll(t *testing.T) {
	a := doc(t, v1(`<box id="b" focusable="true" style="height: 2; overflow: scroll"><text>1</text><text>2</text><text>3</text><text>4</text></box><box id="c" style="height: 2"><text>1</text><text>2</text><text>3</text></box>`))
	f := a.Frame(10, 4)
	if b := f.ByID["b"]; b.ContentH != 4 || !b.Scrolls() || f.ByID["c"].Scrolls() {
		t.Fatalf("viewport: content %d", b.ContentH)
	}
	press(a, named("down"), named("down"), named("down"))
	d := a.Dump(10, 4, false)
	if got := grid(d); got != "3         \n4         \n1         \n2         " {
		t.Errorf("scrolled to the end (clamped at 2):\n%s", got)
	}
	if f := a.Frame(10, 4); f.ByID["b"].ScrollY != 2 {
		t.Errorf("offset = %d", f.ByID["b"].ScrollY)
	}
}

// open= accepts !path like hidden= and disabled=.
func TestModalOpenGuard(t *testing.T) {
	a := doc(t, v1(`<text>under</text><modal id="m" open="!closed"><text>dialog</text></modal>`))
	for _, c := range []struct {
		closed bool
		open   bool
	}{{true, false}, {false, true}} {
		_ = a.Bind("closed", c.closed)
		f := a.Frame(20, 6)
		if (len(f.Modals) == 1) != c.open {
			t.Errorf("closed=%v: modals %d", c.closed, len(f.Modals))
		}
	}
}
