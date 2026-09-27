package host

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// SPEC v0.2b §6.10 (tabs and tab) through the whole frame: the active tab
// (§6.10.2), focus (§6.10.3), labels and layout (§6.10.4), the key
// dispatch and the built-in actions on a tabs (§8.4, §8.6). §21 tests 54
// and 62 for tabs.

// run mimics Run's loop and play (SPEC §15.4): a frame, then each key on
// the live frame with its events, then the lifecycle events that frames
// queue (on:focus, …), redrawing between rounds, in order. The events of
// the first frame are included.
func run(a *App, cols, rows int, keys ...Key) []Event {
	var out []Event
	settle := func(evs []Event) {
		for round := 0; round < 8; round++ {
			out = append(out, evs...)
			a.Frame(cols, rows)
			evs = a.TakePending()
			if len(evs) == 0 {
				return
			}
		}
	}
	settle(nil)
	for _, k := range keys {
		settle(a.HandleKey(k))
	}
	return out
}

// evShort is "action:source:value" per event, space separated.
func evShort(evs []Event) string {
	var out []string
	for _, e := range evs {
		v := ""
		if s, ok := e.Value.(string); ok {
			v = ":" + s
		}
		out = append(out, e.Action+":"+e.Source+v)
	}
	return strings.Join(out, " ")
}

const tabsDoc = `<tui version="2">
<keymap>
  <bind keys="1" action="switch-to" to="#a"/>
  <bind keys="2" action="switch-to" to="#b"/>
  <bind keys="3" action="switch-to" to="#c"/>
  <bind keys="3" action="three"/>
  <bind keys="n" action="move-next" to="#nav"/>
  <bind keys="p" action="move-prev" to="#nav"/>
  <bind keys="f" action="move-first" to="#nav"/>
  <bind keys="e" action="move-last" to="#nav"/>
  <bind keys="d" action="move-page-down" when="#main"/>
  <bind keys="d" action="page"/>
  <bind keys="/" action="focus" to="#q"/>
  <bind keys="x" action="focus" to="#dis"/>
</keymap>
<screen id="main">
  <tabs id="nav" bind="view" mark="▸" gap="1" focusable="true" on:select="tab" on:focus="nav_focus">
    <tab id="a" label="alpha" focus="#la"><list id="la" each="xs as x" key="x" on:focus="la_focus"><item><text>{x}</text></item></list></tab>
    <tab id="b" label="beta"><input id="q" on:focus="q_focus"/><button id="dis" label="d" disabled="true"/></tab>
    <tab id="c" label="gamma" disabled="true"><text>c</text></tab>
  </tabs>
  <button id="other" label="o" on:focus="o_focus"/>
</screen>
</tui>`

func tabsApp(t *testing.T) *App {
	t.Helper()
	a := doc(t, tabsDoc)
	bindJSON(t, a, `{"view": null, "xs": ["x1", "x2", "x3"]}`)
	return a
}

// activeOf is the id of the active tab of tabs id in f ("" without one).
func activeOf(f *Frame, id string) string {
	if t := f.ByID[id]; t != nil && t.TabActive != nil {
		return t.TabActive.ID
	}
	return ""
}

// 54. The initial focus comes from the first tabs whose active tab has a
// focus= target that can take focus (there is no screen focus=); the
// built-ins move the active tab, wrapping over the enabled visible tabs
// (gamma is disabled); a user activation writes the bound path and fires
// on:select with the tab's id; focus elsewhere stays where it is.
func TestTabsBuiltinsAndBind(t *testing.T) {
	a := tabsApp(t)
	if got := evShort(run(a, 40, 8)); got != "la_focus:la" {
		t.Fatalf("initial events %q", got)
	}
	if f := a.Frame(40, 8); a.Focus() != "la" || activeOf(f, "nav") != "a" {
		t.Fatalf("focus %q active %q", a.Focus(), activeOf(f, "nav"))
	}
	// Focus elsewhere: activations do not move it.
	run(a, 40, 8, named("tab"))
	if a.Focus() != "other" {
		t.Fatalf("focus %q, want other", a.Focus())
	}
	for _, c := range []struct {
		key    Key
		events string
		active string
	}{
		{r('n'), "tab:nav:b", "b"},
		{r('n'), "tab:nav:a", "a"}, // wraps over the disabled gamma
		{r('p'), "tab:nav:b", "b"},
		{r('p'), "tab:nav:a", "a"},
		{r('e'), "tab:nav:b", "b"},
		{r('f'), "tab:nav:a", "a"},
		{r('1'), "", "a"}, // switch-to the active tab: consumed, no event
		{r('3'), "three:other", "a"},
		{r('d'), "page:other", "a"}, // move-page-* on a tabs never matches
	} {
		evs := run(a, 40, 8, c.key)
		f := a.Frame(40, 8)
		if got := evShort(evs); got != c.events || activeOf(f, "nav") != c.active {
			t.Errorf("%s: events %q active %q, want %q %q", c.key.Name, got, activeOf(f, "nav"), c.events, c.active)
		}
		if a.Focus() != "other" {
			t.Errorf("%s: focus moved to %q", c.key.Name, a.Focus())
		}
	}
	run(a, 40, 8, r('2'))
	if s := storeJSON(t, a, "view"); s != `"b"` {
		t.Errorf("bind: view = %s", s)
	}
	// A host Set activates a tab without on:select.
	if err := a.Set("view", "a"); err != nil {
		t.Fatal(err)
	}
	if evs := run(a, 40, 8); len(evs) != 0 || activeOf(a.Frame(40, 8), "nav") != "a" {
		t.Errorf("Set: events %v", evShort(evs))
	}
}

// 54. The focus rules of §6.10.3: a tab change moves focus that was inside
// the old tab (to the new tab's first focusable node), on the strip (to
// the new tab's focus= target), or nowhere; on:select comes before the
// on:focus of the node the activation focuses; a focusable tabs consumes
// left and right.
func TestTabsActivationMovesFocus(t *testing.T) {
	a := tabsApp(t)
	run(a, 40, 8)
	// From inside the old tab (la in a) to b's first focusable node.
	if got := evShort(run(a, 40, 8, r('2'))); got != "tab:nav:b q_focus:q" || a.Focus() != "q" {
		t.Errorf("from inside: events %q focus %q", got, a.Focus())
	}
	// From the strip: shift+tab from q is the strip; left goes to a,
	// whose focus= is la.
	if got := evShort(run(a, 40, 8, named("shift+tab"))); got != "nav_focus:nav" {
		t.Fatalf("shift+tab: %q", got)
	}
	if got := evShort(run(a, 40, 8, named("left"))); got != "tab:nav:a la_focus:la" || a.Focus() != "la" {
		t.Errorf("from the strip: events %q focus %q", got, a.Focus())
	}
	// right on the strip wraps over the disabled gamma to... beta.
	run(a, 40, 8, named("shift+tab"))
	if got := evShort(run(a, 40, 8, named("right"))); got != "tab:nav:b q_focus:q" {
		t.Errorf("right: %q", got)
	}

	// From nowhere: nothing can take focus until the second tab is active.
	b := doc(t, `<tui version="2"><keymap><bind keys="2" action="switch-to" to="#y"/></keymap>
<screen id="s"><tabs id="t" on:select="sel"><tab id="x" label="x"><text>x</text></tab><tab id="y" label="y"><input id="i" on:focus="i_focus"/><input id="j"/></tab></tabs></screen></tui>`)
	run(b, 20, 4)
	if b.Focus() != "" {
		t.Fatalf("focus %q", b.Focus())
	}
	if got := evShort(run(b, 20, 4, r('2'))); got != "sel:t:y i_focus:i" || b.Focus() != "i" {
		t.Errorf("from nowhere: events %q focus %q", got, b.Focus())
	}
}

// 54. action="focus" into an inactive tab activates it: on:select, then
// the target's on:focus; Set("@focus") activates without either; a
// request that cannot land undoes the activation and leaves a bound value
// the host set in between.
func TestTabsFocusRequests(t *testing.T) {
	a := tabsApp(t)
	run(a, 40, 8)
	if got := evShort(run(a, 40, 8, r('/'))); got != "tab:nav:b q_focus:q" || a.Focus() != "q" || storeJSON(t, a, "view") != `"b"` {
		t.Errorf("action=focus: events %q focus %q view %s", got, a.Focus(), storeJSON(t, a, "view"))
	}

	a = tabsApp(t)
	run(a, 40, 8)
	if err := a.Set("@focus", "#q"); err != nil {
		t.Fatal(err)
	}
	if got := evShort(run(a, 40, 8)); got != "" || a.Focus() != "q" || activeOf(a.Frame(40, 8), "nav") != "b" {
		t.Errorf("Set(@focus): events %q focus %q", got, a.Focus())
	}

	// #dis is disabled: the request cannot land, and b is not active.
	a = tabsApp(t)
	run(a, 40, 8)
	if got := evShort(run(a, 40, 8, r('x'))); got != "" || a.Focus() != "la" || storeJSON(t, a, "view") != "null" {
		t.Errorf("undone: events %q focus %q view %s", got, a.Focus(), storeJSON(t, a, "view"))
	}
	if f := a.Frame(40, 8); activeOf(f, "nav") != "a" {
		t.Errorf("undone: active %q", activeOf(f, "nav"))
	}
	// The host sets the bound path between the request and its frame: the
	// undo leaves the host's value.
	a.Frame(40, 8)
	a.HandleKey(r('x'))
	if err := a.Set("view", "a"); err != nil {
		t.Fatal(err)
	}
	a.Frame(40, 8)
	if s := storeJSON(t, a, "view"); s != `"a"` {
		t.Errorf("host value overwritten: view = %s", s)
	}

	// Without bind, a remembered tab is put back.
	b := doc(t, strings.Replace(tabsDoc, ` bind="view"`, "", 1))
	bindJSON(t, b, `{"xs": ["x1"]}`)
	run(b, 40, 8, r('x'))
	if m, ok := b.tabMem["nav"]; ok || activeOf(b.Frame(40, 8), "nav") != "a" {
		t.Errorf("nothing was remembered, now %q", m)
	}
	run(b, 40, 8, named("shift+tab"), named("right"), named("shift+tab"), named("left")) // remember a
	run(b, 40, 8, r('x'))
	if b.tabMem["nav"] != "a" || activeOf(b.Frame(40, 8), "nav") != "a" {
		t.Errorf("remembered %q", b.tabMem["nav"])
	}
}

// 54. Inactive tabs are not in the frame or the dump, and widgets inside
// them keep their state by id: a list's cursor and an input's text.
func TestTabsKeepStateAcrossSwitches(t *testing.T) {
	a := tabsApp(t)
	run(a, 40, 8, named("down"), r('2'))
	run(a, 40, 8, typed("hi")...)
	d := a.Dump(40, 8, false)
	for _, n := range d.Nodes {
		if n.ID == "la" || n.ID == "a" || n.ID == "c" {
			t.Errorf("inactive %s dumped", n.ID)
		}
	}
	run(a, 40, 8, named("shift+tab"), named("left"))
	f := a.Frame(40, 8)
	if f.ByID["q"] != nil {
		t.Error("the input of the inactive tab b is in the frame")
	}
	sel := ""
	for _, it := range f.ByID["la"].Children {
		if it.Selected {
			sel = it.Key
		}
	}
	if sel != "x2" {
		t.Errorf("list cursor %q, want x2", sel)
	}
	run(a, 40, 8, named("shift+tab"), named("right"))
	if v := a.Frame(40, 8).ByID["q"]; v == nil || v.Text != "hi" {
		t.Errorf("input text lost: %+v", v)
	}
}

// 54. B010 for a bound value that names no visible tab (the first visible
// tab is active and the store is left alone); B003 for a missing path; a
// null value activates the first visible tab silently; a hidden tab is
// not visible.
func TestTabsBindFallbacks(t *testing.T) {
	src := `<tui version="2"><screen id="s"><tabs id="t" bind="v"><tab id="h" label="h" hidden="true"/><tab id="x" label="x"/><tab id="y" label="y"/></tabs></screen></tui>`
	for _, c := range []struct {
		data, code, active string
	}{
		{`{"v": "zzz"}`, "B010", "x"},
		{`{"v": 5}`, "B010", "x"},
		{`{"v": "h"}`, "B010", "x"},
		{`{}`, "B003", "x"},
		{`{"v": null}`, "", "x"},
		{`{"v": "y"}`, "", "y"},
	} {
		a := doc(t, src)
		bindJSON(t, a, c.data)
		f := a.Frame(20, 3)
		var codes []string
		for _, d := range f.Diags {
			codes = append(codes, d.Code)
		}
		if strings.Join(codes, " ") != c.code || activeOf(f, "t") != c.active {
			t.Errorf("%s: codes %v active %q", c.data, codes, activeOf(f, "t"))
		}
		before := storeJSON(t, a, "v")
		a.Frame(20, 3)
		if storeJSON(t, a, "v") != before {
			t.Errorf("%s: the store was written", c.data)
		}
	}
}

// 54. Without bind, the runtime remembers the active tab; a fallback to
// the first visible tab (the remembered one is hidden) does not change
// what it remembers.
func TestTabsRememberedFallback(t *testing.T) {
	a := doc(t, `<tui version="2"><keymap><bind keys="2" action="switch-to" to="#y"/></keymap>
<screen id="s"><tabs id="t"><tab id="x" label="x"/><tab id="y" label="y" if="!hide"/></tabs></screen></tui>`)
	bindJSON(t, a, `{"hide": false}`)
	run(a, 20, 3, r('2'))
	if activeOf(a.Frame(20, 3), "t") != "y" {
		t.Fatal("switch-to y")
	}
	_ = a.Set("hide", true)
	if activeOf(a.Frame(20, 3), "t") != "x" || a.tabMem["t"] != "y" {
		t.Errorf("fallback changed the remembered tab: %q", a.tabMem["t"])
	}
	_ = a.Set("hide", false)
	if activeOf(a.Frame(20, 3), "t") != "y" {
		t.Error("the remembered tab is back when visible again")
	}
}

// 54. A tabs new to the frame has no previous active tab, so it never
// moves focus: when a tabs appears while nothing is focused, the §8.3
// fallback picks the first focusable node, not its active tab's focus=.
func TestTabsNewToTheFrameDoNotMoveFocus(t *testing.T) {
	a := doc(t, `<tui version="2"><screen id="s"><box if="show"><tabs id="t"><tab id="x" label="x" focus="#second"><input id="first"/><input id="second"/></tab></tabs></box></screen></tui>`)
	bindJSON(t, a, `{"show": false}`)
	run(a, 20, 4)
	_ = a.Set("show", true)
	run(a, 20, 4)
	if a.Focus() != "first" {
		t.Errorf("focus %q, want first", a.Focus())
	}
	// On a screen's first frame, the active tab's focus= supplies the
	// initial focus instead.
	b := doc(t, `<tui version="2"><screen id="s"><tabs id="t"><tab id="x" label="x" focus="#second"><input id="first"/><input id="second"/></tab></tabs></screen></tui>`)
	run(b, 20, 4)
	if b.Focus() != "second" {
		t.Errorf("initial focus %q, want second", b.Focus())
	}
	// screen@focus wins, and does not activate an inactive tab.
	c := doc(t, `<tui version="2"><screen id="s" focus="#z"><tabs id="t"><tab id="x" label="x" focus="#second"><input id="first"/><input id="second"/></tab><tab id="y" label="y"><input id="z"/></tab></tabs></screen></tui>`)
	run(c, 20, 4)
	if c.Focus() != "second" || activeOf(c.Frame(20, 4), "t") != "x" {
		t.Errorf("screen focus into an inactive tab: focus %q", c.Focus())
	}
}

// 54 / 62. switch-to a tab of a tabs that is not in the frame (it sits in
// an inactive tab) does not match, so the key reaches the next row; nor
// does switch-to a disabled tab.
func TestSwitchToNestedInactiveTabs(t *testing.T) {
	a := doc(t, `<tui version="2"><keymap>
<bind keys="9" action="switch-to" to="#in2"/><bind keys="9" action="nine"/>
<bind keys="2" action="switch-to" to="#o2"/>
</keymap><screen id="s"><tabs id="outer"><tab id="o1" label="o1"><text>one</text></tab>
<tab id="o2" label="o2"><tabs id="inner" on:select="inner"><tab id="in1" label="i1"/><tab id="in2" label="i2"/></tabs></tab></tabs></screen></tui>`)
	if got := evShort(run(a, 30, 4, r('9'))); got != "nine:" {
		t.Errorf("inner tabs not in the frame: %q", got)
	}
	if got := evShort(run(a, 30, 4, r('2'), r('9'))); got != "inner:inner:in2" {
		t.Errorf("inner tabs in the frame: %q", got)
	}
}

// 54. Label positions do not depend on the active tab in tiers 1 and 2;
// tier 3 shows the active tab's label alone; the labels and the panel
// ignore their own box properties.
func TestTabLabelGeometry(t *testing.T) {
	src := `<tui version="2"><style>
.tab-label { padding: 2; margin: 1; border: single; width: 30; display: none; }
.tab-label:selected { reverse: true; }
tab { width: 3; height: 2; margin: 1; }
</style><screen id="s"><tabs id="t" bind="v" mark="▸" gap="2">
<tab id="overview" label="1 overview" short="1 ovr"><text>o</text></tab>
<tab id="processes" label="7 processes" short="7 proc"><text>p</text></tab>
<tab id="settings" label="8 settings" short="8 cfg"><text>s</text></tab>
</tabs></screen></tui>`
	labels := func(active string, cols int) string {
		a := doc(t, src)
		bindJSON(t, a, `{"v": "`+active+`"}`)
		f := a.Frame(cols, 3)
		var out []string
		for _, l := range f.ByID["t"].Children {
			if l.Role == layout.RoleTabLabel && l.Laid {
				out = append(out, fmt.Sprintf("%s@%dw%d", l.Key, l.X, l.W))
			}
		}
		if p := f.ByID[active]; p == nil || p.X != 0 || p.Y != 1 || p.W != cols || p.H != 1 {
			t.Errorf("panel %+v", p)
		}
		return strings.Join(out, " ")
	}
	for _, cols := range []int{40, 30} {
		want := labels("overview", cols)
		for _, act := range []string{"processes", "settings"} {
			if got := labels(act, cols); got != want {
				t.Errorf("%d cols, %s active: %s, want %s", cols, act, got, want)
			}
		}
	}
	if got := labels("settings", 20); got != "settings@0w15" {
		t.Errorf("tier 3: %s", got)
	}
	// The active label is reversed over its whole rect, mark slot included.
	a := doc(t, src)
	bindJSON(t, a, `{"v": "processes"}`)
	g := a.Frame(40, 3).Grid
	for x := 13; x < 25; x++ {
		if g.At(x, 0).Attrs&0x10 == 0 {
			t.Errorf("column %d of the active label is not reversed", x)
		}
	}
	if g.At(13, 0).Ch != '▸' {
		t.Errorf("mark %q", g.At(13, 0).Ch)
	}
}

// 54. The activation rule applies to a tab change for any reason: a host
// Set of the bound path moves focus that was inside the old tab into the
// new one (on:focus fires, on:select does not).
func TestTabsHostSetMovesFocus(t *testing.T) {
	a := tabsApp(t)
	run(a, 40, 8)
	if a.Focus() != "la" {
		t.Fatalf("focus %q", a.Focus())
	}
	_ = a.Set("view", "b")
	if got := evShort(run(a, 40, 8)); got != "q_focus:q" || a.Focus() != "q" {
		t.Errorf("events %q focus %q", got, a.Focus())
	}
}

// 54. A tabs inside an open modal is activated, labeled, and laid out like
// one on the screen; its strip and panel are clipped by the modal.
func TestTabsInsideAModal(t *testing.T) {
	a := doc(t, `<tui version="2"><keymap><bind keys="2" action="switch-to" to="#y"/></keymap>
<screen id="s"><text>under</text><modal id="m" open="true" style="width: 20; height: 5"><tabs id="t"><tab id="x" label="x"><text>ex</text></tab><tab id="y" label="y"><button id="b" label="go"/></tab></tabs></modal></screen></tui>`)
	run(a, 30, 7, r('2'))
	f := a.Frame(30, 7)
	if activeOf(f, "t") != "y" || a.Focus() != "b" {
		t.Errorf("active %q focus %q", activeOf(f, "t"), a.Focus())
	}
	if g := f.Grid.Lines(); !strings.Contains(g[2], "│xy ") || !strings.Contains(g[3], "[ go ]") {
		t.Errorf("grid %q", g)
	}
}

// 54 / 59. :focus-within reaches through a tab to the content step 3
// inflates: with focus on an input inside the active tab, the tab and the
// tabs are in the focus chain, so #b:focus-within > input matches, while
// the tab's own style never sees :focus-within (SPEC §10.1).
func TestTabsFocusWithin(t *testing.T) {
	a := doc(t, `<tui version="2"><style>
#b:focus-within > input { bold: true; }
#nav:focus-within { italic: true; }
tab:focus-within { underline: true; }
</style><screen id="s" focus="#q"><tabs id="nav" bind="v"><tab id="a" label="a"/><tab id="b" label="b"><input id="q"/></tab></tabs></screen></tui>`)
	bindJSON(t, a, `{"v": "b"}`)
	f := a.Frame(20, 3)
	q, tab, nav := f.ByID["q"], f.ByID["nav"].TabActive, f.ByID["nav"]
	if a.Focus() != "q" || !q.Style.Bold || !nav.Style.Italic || !tab.FocusWithin || tab.Style.Underline {
		t.Errorf("focus %q: input bold %v, tabs italic %v, tab chain %v underline %v", a.Focus(), q.Style.Bold, nav.Style.Italic, tab.FocusWithin, tab.Style.Underline)
	}
}

// 54. When one frame changes the active tab of several tabs while focus
// is nowhere, the activation rule of §6.10.3 applies to the first of them
// in document order only: focus lands on its new tab's focus= target, and
// only that node's on:focus fires. The second tabs does not move focus in
// a later pass of the same render. One tabs changing alone still moves
// focus into its new tab.
func TestTabsSeveralChangeInOneFrame(t *testing.T) {
	const src = `<tui version="2"><screen id="main">
<tabs id="t1" bind="a"><tab id="a1" label="A1"><text>x</text></tab><tab id="a2" label="A2" focus="#in1"><input id="in1" on:focus="f1"/></tab></tabs>
<tabs id="t2" bind="b"><tab id="b1" label="B1"><text>y</text></tab><tab id="b2" label="B2" focus="#in2"><input id="in2" on:focus="f2"/></tab></tabs>
</screen></tui>`
	a := doc(t, src)
	bindJSON(t, a, `{"a": "a1", "b": "b1"}`)
	if got := evShort(run(a, 20, 6)); got != "" || a.Focus() != "" {
		t.Fatalf("first frame: events %q focus %q", got, a.Focus())
	}
	if err := a.Set("", map[string]any{"a": "a2", "b": "b2"}); err != nil {
		t.Fatal(err)
	}
	if got := evShort(run(a, 20, 6)); got != "f1:in1" || a.Focus() != "in1" {
		t.Errorf("both change: events %q focus %q, want f1:in1 and in1", got, a.Focus())
	}
	f := a.Frame(20, 6)
	if activeOf(f, "t1") != "a2" || activeOf(f, "t2") != "b2" {
		t.Errorf("active %q %q", activeOf(f, "t1"), activeOf(f, "t2"))
	}

	a = doc(t, src)
	bindJSON(t, a, `{"a": "a1", "b": "b1"}`)
	run(a, 20, 6)
	if err := a.Set("b", "b2"); err != nil {
		t.Fatal(err)
	}
	if got := evShort(run(a, 20, 6)); got != "f2:in2" || a.Focus() != "in2" {
		t.Errorf("t2 alone: events %q focus %q", got, a.Focus())
	}
}

// 54. A focus request into a nested tabs that is not in the frame, and
// whose tab is already its active tab by a §6.10.2 fallback (a null,
// missing, or unknown bound value; nothing remembered), activates only
// the tabs whose tab was inactive: only that on:select fires, and the
// nested tabs' bound value and remembered tab stay as they were (the
// runtime never writes the store for a fallback), as when a user
// activation reaches the same frame. A bound value or remembered tab
// naming another tab still activates the target's tab.
func TestTabsFocusRequestIntoFallbackTab(t *testing.T) {
	const src = `<tui version="2"><keymap><bind keys="f" action="focus" to="#x1"/></keymap><screen id="main">
<tabs id="outer" bind="o" on:select="osel" focusable="true"><tab id="o1" label="o1"><text>one</text></tab><tab id="o2" label="o2">
<tabs id="inner" bind="i" on:select="isel"><tab id="i1" label="i1"><input id="x1" on:focus="xf"/></tab><tab id="i2" label="i2"><text>two</text></tab></tabs>
</tab></tabs><text id="show">i=[{i}]</text></screen></tui>`
	for _, c := range []struct {
		data, events, i, shown string
	}{
		{`{"o": "o1", "i": null}`, "osel:outer:o2 xf:x1", "null", "i=[]"},
		{`{"o": "o1"}`, "osel:outer:o2 xf:x1", "<missing>", "i=[]"},
		{`{"o": "o1", "i": "zzz"}`, "osel:outer:o2 xf:x1", `"zzz"`, "i=[zzz]"},
		{`{"o": "o1", "i": "i2"}`, "osel:outer:o2 isel:inner:i1 xf:x1", `"i1"`, "i=[i1]"},
	} {
		a := doc(t, src)
		bindJSON(t, a, c.data)
		run(a, 20, 4)
		got := evShort(run(a, 20, 4, r('f')))
		f := a.Frame(20, 4)
		if got != c.events || a.Focus() != "x1" || storeJSON(t, a, "i") != c.i || storeJSON(t, a, "o") != `"o2"` {
			t.Errorf("%s: events %q focus %q i %s o %s", c.data, got, a.Focus(), storeJSON(t, a, "i"), storeJSON(t, a, "o"))
		}
		if activeOf(f, "inner") != "i1" || !strings.HasPrefix(f.Grid.Lines()[3], c.shown+" ") {
			t.Errorf("%s: inner active %q, grid %q", c.data, activeOf(f, "inner"), f.Grid.Lines())
		}
	}

	// Without bind on the nested tabs: nothing remembered, then i2
	// remembered.
	nb := strings.Replace(src, ` bind="i"`, "", 1)
	a := doc(t, nb)
	bindJSON(t, a, `{"o": "o1"}`)
	run(a, 20, 4)
	if got := evShort(run(a, 20, 4, r('f'))); got != "osel:outer:o2 xf:x1" || a.Focus() != "x1" {
		t.Errorf("no bind: events %q focus %q", got, a.Focus())
	}
	if m, ok := a.tabMem["inner"]; ok {
		t.Errorf("no bind: inner remembers %q", m)
	}
	a = doc(t, nb)
	bindJSON(t, a, `{"o": "o1"}`)
	run(a, 20, 4)
	a.tabMem["inner"] = "i2"
	if got := evShort(run(a, 20, 4, r('f'))); got != "osel:outer:o2 isel:inner:i1 xf:x1" || a.tabMem["inner"] != "i1" {
		t.Errorf("no bind, i2 remembered: events %q, remembered %q", got, a.tabMem["inner"])
	}
}
