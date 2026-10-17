package host

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
)

// The equivalence tests of the frame caches (framecache.go, listcache.go,
// the style sharing): a warm app, with every cache on, and a cold one
// (noCache), built from the same document and data, go through the same
// steps, and after each step every frame, dump (cells and --styles spans
// included), diagnostic, event, and pending event of the warm one must be
// those of the cold one. The frame is asked for twice per step, so the
// second answer of the warm app is the frame skip's.

// eqCtx is what a step acts on: one of the two apps, and the size the
// next frames are built at.
type eqCtx struct {
	a          *App
	cols, rows int
}

// eqStep is one step: do acts on the app and returns what it gave back
// (events, an error), which must be the same for both apps.
type eqStep struct {
	name string
	do   func(c *eqCtx) any
}

func eqKeys(tokens string) eqStep {
	return eqStep{"keys " + tokens, func(c *eqCtx) any {
		var out []any
		for _, t := range strings.Fields(tokens) {
			k := Key{Name: t}
			if len([]rune(t)) == 1 {
				k.Rune = []rune(t)[0]
			}
			out = append(out, c.a.HandleKey(k))
			// A frame between keys, as Run and play draw one.
			c.a.Frame(c.cols, c.rows)
		}
		return out
	}}
}

// eqKeysNoFrame sends keys with no frame between them.
func eqKeysNoFrame(tokens string) eqStep {
	return eqStep{"keys (no frames) " + tokens, func(c *eqCtx) any {
		var out []any
		for _, t := range strings.Fields(tokens) {
			out = append(out, c.a.HandleKey(Key{Name: t}))
		}
		return out
	}}
}

func eqSet(path string, v any) eqStep {
	return eqStep{fmt.Sprintf("set %s=%v", path, v), func(c *eqCtx) any { return errText(c.a.Set(path, v)) }}
}

func eqBind(path string, v any) eqStep {
	return eqStep{fmt.Sprintf("bind %q", path), func(c *eqCtx) any { return errText(c.a.Bind(path, v)) }}
}

func eqStrict(on bool) eqStep {
	return eqStep{fmt.Sprintf("strict %v", on), func(c *eqCtx) any { c.a.SetStrict(on); return nil }}
}

func eqResize(cols, rows int) eqStep {
	return eqStep{fmt.Sprintf("resize %dx%d", cols, rows), func(c *eqCtx) any { c.cols, c.rows = cols, rows; return nil }}
}

func eqWheel(x, y int, down bool) eqStep {
	return eqStep{fmt.Sprintf("wheel %d,%d down=%v", x, y, down), func(c *eqCtx) any {
		k := MouseWheelUp
		if down {
			k = MouseWheelDown
		}
		evs, on := c.a.HandleMouse(Mouse{Kind: k, X: x, Y: y})
		return []any{evs, on}
	}}
}

func eqClick(x, y int) eqStep {
	return eqStep{fmt.Sprintf("click %d,%d", x, y), func(c *eqCtx) any {
		e1, _ := c.a.HandleMouse(Mouse{Kind: MousePress, X: x, Y: y})
		e2, on := c.a.HandleMouse(Mouse{Kind: MouseRelease, X: x, Y: y})
		return []any{e1, e2, on}
	}}
}

func errText(err error) any {
	if err == nil {
		return nil
	}
	return err.Error()
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// eqApps parses src twice, the second app cold, and binds a copy of data
// into each.
func eqApps(t *testing.T, src string, data map[string]any) (warm, cold *App) {
	t.Helper()
	apps := make([]*App, 2)
	for i := range apps {
		a, err := Parse(strings.NewReader(src))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if data != nil {
			var v any
			if err := json.Unmarshal([]byte(jsonOf(t, data)), &v); err != nil {
				t.Fatal(err)
			}
			if err := a.Bind("", v); err != nil {
				t.Fatalf("bind: %v", err)
			}
		}
		apps[i] = a
	}
	apps[1].noCache = true
	return apps[0], apps[1]
}

// runEquivalence runs steps on a warm and a cold app built from src and
// data, comparing them after each step.
func runEquivalence(t *testing.T, src string, data map[string]any, cols, rows int, steps []eqStep) {
	t.Helper()
	warm, cold := eqApps(t, src, data)
	w, c := &eqCtx{warm, cols, rows}, &eqCtx{cold, cols, rows}
	check := func(label string) {
		t.Helper()
		for pass := 0; pass < 2; pass++ {
			fw, fc := warm.Frame(w.cols, w.rows), cold.Frame(c.cols, c.rows)
			if d := FrameDiff(fw, fc); d != "" {
				t.Fatalf("after %s (frame %d): warm and cold frames differ: %s", label, pass, d)
			}
			if a, b := jsonOf(t, warm.TakePending()), jsonOf(t, cold.TakePending()); a != b {
				t.Fatalf("after %s (frame %d): pending events differ:\nwarm %s\ncold %s", label, pass, a, b)
			}
		}
		for _, sz := range [][2]int{{w.cols, w.rows}, {w.cols + 17, w.rows + 5}, {max(1, w.cols/2), max(1, w.rows/2)}} {
			dw, _ := dump.JSON(warm.Dump(sz[0], sz[1], true))
			dc, _ := dump.JSON(cold.Dump(sz[0], sz[1], true))
			if string(dw) != string(dc) {
				t.Fatalf("after %s: Dump at %dx%d differs:\n%s", label, sz[0], sz[1], firstLineDiff(string(dw), string(dc)))
			}
		}
		if a, b := diagsText(warm.Validate()), diagsText(cold.Validate()); a != b {
			t.Fatalf("after %s: Validate differs:\n%s", label, firstLineDiff(a, b))
		}
		if a, b := jsonOf(t, warm.Focus()), jsonOf(t, cold.Focus()); a != b {
			t.Fatalf("after %s: focus %s vs %s", label, a, b)
		}
	}
	check("the first frame")
	for _, s := range steps {
		rw, rc := s.do(w), s.do(c)
		if a, b := jsonOf(t, rw), jsonOf(t, rc); a != b {
			t.Fatalf("step %s: warm gave %s, cold %s", s.name, a, b)
		}
		check(s.name)
	}
}

// eqRows is n rows for the equivalence documents.
func eqRows(n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = map[string]any{
			"id": fmt.Sprintf("r%03d", i), "name": strings.Repeat("word ", i%5+1) + fmt.Sprint(i),
			"on": i%3 == 0, "n": float64(i * 7 % 100), "kids": []any{map[string]any{"t": "a"}, map[string]any{"t": fmt.Sprint(i)}},
			"hist": []any{1.0, float64(i % 4), nil, 2.0},
		}
	}
	return out
}

func eqData(n int) map[string]any {
	return map[string]any{"rows": eqRows(n), "cur": "r002", "flag": true, "prefix": "P:", "sel": []any{"r001"}, "dis": false, "other": 1.0}
}

// eqNav is the steps every list document goes through: moving the
// cursor, the store paths its template reads and one it does not, the
// list's disabled path, the cursor's bound path, the array growing,
// shrinking under the cursor, becoming something else and coming back,
// --strict, the theme, and the size.
func eqNav() []eqStep {
	return []eqStep{
		eqKeys("down down down"),
		eqKeysNoFrame("down down"),
		eqKeys("pgdn end up home"),
		eqSet("prefix", "Q:"),
		eqSet("flag", false),
		eqSet("other", 2.0),
		eqSet("rows.3.name", "renamed"),
		eqSet("rows.4.on", true),
		eqSet("dis", true),
		eqKeys("down"),
		eqSet("dis", false),
		eqSet("cur", "r007"),
		eqSet("cur", "nope"),
		eqKeys("space down space"),
		eqStrict(true),
		eqSet("prefix", nil),
		eqStrict(false),
		eqSet("rows", eqRows(40)),
		eqKeys("end"),
		eqSet("rows", eqRows(3)),
		eqSet("rows", 5.0),
		eqKeys("down"),
		eqSet("rows", eqRows(12)),
		eqBind("", eqData(9)),
		eqKeys("down down"),
		eqSet("@theme", "light"),
		eqSet("@theme", "dark"),
		eqResize(30, 8),
		eqKeys("down down down"),
		eqResize(120, 30),
		eqKeys("tab"),
		eqSet("@focus", "#l"),
		eqKeys("up"),
	}
}

const eqKeymap = `<keymap><bind keys="space" action="check-toggle" to="#l"/><bind keys="a" action="check-all" to="#l"/><bind keys="x" action="move-next" to="#l"/></keymap>`

func TestCacheEquivalenceList(t *testing.T) {
	docs := map[string]string{
		"template paths": `<tui version="2"><style>.hot > row > text { bold: true; } item:checked { reverse: true; }</style><screen id="s" focus="#l">
<list id="l" each="rows as r" key="r.id" bind="cur" disabled="dis" checked="sel" mark="✓" on:select="picked" on:change="checked"><item class:hot="flag" if="!r.on"><row><text>{prefix}{r.name}</text><text if="flag">{r.n}</text><progress bind="r.n"/></row></item></list>
<input id="q" bind="prefix"/></screen>` + eqKeymap + `</tui>`,
		"no id":                  `<tui version="2"><screen id="s"><list each="rows as r" key="r.id"><item><text>{r.name}{prefix}</text></item></list><button id="b" label="b"/></screen></tui>`,
		"nested each":            `<tui version="2"><screen id="s" focus="#l"><list id="l" each="rows as r" key="r.id" bind="cur"><item><row each="r.kids as k" key="k.t"><text>{k.t}/{r.n}{prefix}</text></row><sparkline bind="r.hist"/></item></list></screen></tui>`,
		"display none rows":      `<tui version="2"><style>.gone { display: none; } item:selected { padding: 0 2; border: single; }</style><screen id="s" focus="#l"><list id="l" each="rows as r" key="r.id" bind="cur"><item class:gone="r.on"><text>{r.name}</text></item></list></screen></tui>`,
		"version 1":              `<tui version="1"><screen id="s" focus="#l"><list id="l" each="rows as r" key="r.id" bind="cur"><item><text>{r.name} {prefix}</text></item></list><input id="q" bind="prefix"/></screen></tui>`,
		"version 3 clipping":     `<tui version="3"><style>item { height: 1; } .w { width: 6; } #l { height: 6; }</style><screen id="s" focus="#l"><col><list id="l" each="rows as r" key="r.id" bind="cur"><item><row><text class="w">{r.name}</text><text>{prefix}{r.n}</text></row></item></list><text>below</text></col></screen></tui>`,
		"missing key":            `<tui version="2"><screen id="s" focus="#l"><list id="l" each="rows as r" key="r.nokey" bind="cur"><item><text>{r.missing}{prefix}</text></item></list></screen></tui>`,
		"modal over list":        `<tui version="2"><screen id="s" focus="#l"><text style="dock: top">top {prefix}</text><list id="l" each="rows as r" key="r.id" bind="cur"><item><text>{r.name}</text></item></list><modal id="m" open="!flag"><text>modal {prefix}</text><button id="ok" label="ok"/></modal></screen></tui>`,
		"align start auto items": `<tui version="3"><style>#l { align: start; } item { width: auto; } .half { width: 50%; } .half > text { wrap: wrap; } .sx { height: 1; }</style><screen id="s" focus="#l"><list id="l" each="rows as r" key="r.id" bind="cur"><item><scroll class="sx" axis="x"><text>{r.name} {r.name} {r.name} {r.name} {r.name} {r.name} {r.name}</text></scroll><col class="half"><text>{r.name} {r.name} {r.name}</text></col></item></list></screen></tui>`,
	}
	for name, src := range docs {
		t.Run(name, func(t *testing.T) {
			runEquivalence(t, src, eqData(30), 80, 24, eqNav())
		})
	}
}

func TestCacheEquivalenceTabsAndWidgets(t *testing.T) {
	src := `<tui version="3" mouse="true"><style>.sc { height: 2; border: single; } hints { dock: bottom; }</style><screen id="s">
<tabs id="t" focusable="true" bind="view"><tab id="a" label="A {prefix}"><list id="l" each="rows as r" key="r.id" bind="cur" checked="sel" mark="*"><item><scroll class="sc"><text>{r.name}</text><text>two</text><text>three</text></scroll></item></list></tab>
<tab id="b" label="B"><table id="tb" each="rows as r" key="r.id" bind="tcur"><column title="Name" width="1fr">{r.name}</column><column title="N" priority="1">{r.n}</column></table></tab>
<tab id="c" label="C"><sparkline bind="rows.0.hist" scale="g"/><sparkline bind="rows.1.hist" scale="g"/></tab></tabs><hints/></screen>
<keymap><bind keys="1" action="switch-to" to="#a" label="list"/><bind keys="2" action="switch-to" to="#b" label="table"/><bind keys="3" action="switch-to" to="#c"/><bind keys="space" action="check-toggle" to="#l" label="check"/></keymap></tui>`
	steps := []eqStep{
		eqKeys("tab down down"),
		eqWheel(3, 3, true),
		eqWheel(3, 3, true),
		eqKeys("down"),
		eqWheel(3, 6, true),
		eqWheel(3, 6, false),
		eqClick(3, 7),
		eqKeys("space"),
		eqKeys("2 down down"),
		eqSet("rows.1.name", "renamed"),
		eqKeys("1"),
		eqSet("prefix", "Z"),
		eqKeys("3"),
		eqSet("rows.0.hist", []any{5.0, 9.0}),
		eqKeys("1 down"),
		eqResize(50, 14),
		eqKeys("right left"),
		eqSet("view", "b"),
		eqSet("view", "a"),
		eqWheel(3, 3, true),
	}
	runEquivalence(t, src, eqData(25), 80, 24, steps)
}

// TestListFollowAllocationUnderAlignStart pins the geometry of a list
// whose rows are auto-width items under align: start, holding a percent
// width child with wrapped text and a row that is shrinkable across: the
// list's allocation that follows the cursor stays silent there (its
// cross sizes would mark the items auto before their heights are
// measured), so these heights are those of 0.3.2.
func TestListFollowAllocationUnderAlignStart(t *testing.T) {
	src := `<tui version="3"><style>#l { align: start; } item { width: auto; } .half { width: 50%; } .half > text { wrap: wrap; } .sx { height: 1; }</style><screen id="s" focus="#l">
<list id="l" each="rows as r" key="r.id" bind="cur"><item><scroll class="sx" axis="x"><text>{r.name} {r.name} {r.name} {r.name} {r.name} {r.name} {r.name} {r.name} {r.name} {r.name} {r.name}</text></scroll><col class="half"><text>{r.name} {r.name} {r.name} {r.name} {r.name} {r.name}</text></col></item></list></screen></tui>`
	rows := make([]any, 6)
	for i := range rows {
		rows[i] = map[string]any{"id": fmt.Sprintf("r%03d", i), "name": strings.Repeat("word ", i%7+1) + fmt.Sprint(i)}
	}
	for _, cold := range []bool{false, true} {
		a, err := Parse(strings.NewReader(src))
		if err != nil {
			t.Fatal(err)
		}
		a.noCache = cold
		if err := a.Bind("", map[string]any{"rows": rows, "cur": "r002"}); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, n := range a.Dump(80, 24, false).Nodes {
			if n.Tag == "item" {
				got = append(got, fmt.Sprintf("%dx%d@%d,%d", n.W, n.H, n.X, n.Y))
			}
		}
		want := "76x3@0,0 80x3@0,3 80x4@0,6 80x5@0,10 80x6@0,15 80x7@0,21"
		if g := strings.Join(got, " "); g != want {
			t.Errorf("cold=%v: items %s, want %s", cold, g, want)
		}
	}
}

// A Derive registration rebuilds the next frame even when the recomputed
// value equals the stored one, so recompute wrote nothing: what the
// runtime may write changed, and the frame skip must not hand back a
// frame built before it (SPEC v0.4a §18.2).
func TestDeriveRegistrationRebuildsFrame(t *testing.T) {
	a, err := Parse(strings.NewReader(`<tui version="3"><screen id="s"><text>{n}</text></screen></tui>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Bind("", map[string]any{"n": float64(1)}); err != nil {
		t.Fatal(err)
	}
	f1 := a.Frame(20, 3)
	if f2 := a.Frame(20, 3); f2 != f1 {
		t.Fatalf("an unchanged app rebuilt its frame: the skip is off, so this test checks nothing")
	}
	if err := a.Derive("n", func(func(string) (any, bool)) (any, error) { return float64(1), nil }); err != nil {
		t.Fatal(err)
	}
	if f3 := a.Frame(20, 3); f3 == f1 {
		t.Errorf("the frame built before the Derive registration was handed back")
	}
}
