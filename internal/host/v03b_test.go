package host

import (
	"fmt"
	"strings"
	"testing"
)

// SPEC v0.3b §6.2, §6.10.1, §6.10.4, §8.1, §8.3, §11.4: the runtime
// behavior of several <keymap> elements and <keymap when>, stick="bottom",
// {path} counters in a tab's label/short, and a modal's focus=. §21 tests
// 96-100 (runtime parts; the static parts are in internal/parse).

// 96. Several <keymap> elements: their rows dispatch as one keymap in
// document order (the first matching row across every keymap wins), and
// <hints scope="all"> lists them in that order too.
func TestSeveralKeymapsDispatchOrder(t *testing.T) {
	a := doc(t, `<tui version="2">
<keymap><bind keys="a" action="one" label="one"/></keymap>
<keymap><bind keys="a" action="two" label="two"/><bind keys="b" action="three" label="three"/></keymap>
<screen id="s"><hints id="h" scope="all"/></screen>
</tui>`)
	evs := press(a, r('a'))
	if len(evs) != 1 || evs[0].Action != "one" {
		t.Fatalf("the first row across keymaps wins, not a later one for the same key: %+v", evs)
	}
	evs = press(a, r('b'))
	if len(evs) != 1 || evs[0].Action != "three" {
		t.Fatalf("a key only the second keymap's row holds: %+v", evs)
	}
	f := a.Frame(80, 24)
	h := f.ByID["h"]
	var labels []string
	for _, row := range h.Children {
		labels = append(labels, row.Children[1].Text)
	}
	if got := strings.Join(labels, " "); got != "one two three" {
		t.Errorf("hints order %q, want \"one two three\"", got)
	}
}

// 97. Keymap when (version="3"): a row under a <keymap when> fires only
// while that selector matches; a row's own when overrides it; when=""
// fires everywhere even under a <keymap when>.
func TestKeymapWhenDispatch(t *testing.T) {
	a := doc(t, `<tui version="3">
<keymap when="#procs:focus">
  <bind keys="c" action="sort_cpu"/>
  <bind keys="e" action="everywhere" when=""/>
</keymap>
<keymap>
  <bind keys="c" action="global_c"/>
  <bind keys="d" action="own" when="#other:focus"/>
</keymap>
<screen id="s" focus="#procs">
  <list id="procs" each="xs as x" key="x"><item><text>{x}</text></item></list>
  <button id="other" label="o"/>
</screen>
</tui>`)
	bindJSON(t, a, `{"xs": ["a", "b"]}`)
	evs := press(a, r('c'))
	if len(evs) != 1 || evs[0].Action != "sort_cpu" {
		t.Fatalf("row under keymap when, focus on #procs: %+v", evs)
	}
	press(a, named("tab"))
	if a.Focus() != "other" {
		t.Fatalf("focus %q, want other", a.Focus())
	}
	evs = press(a, r('c'))
	if len(evs) != 1 || evs[0].Action != "global_c" {
		t.Fatalf("row under keymap when, focus elsewhere: does not fire, falls to the next keymap: %+v", evs)
	}
	evs = press(a, r('d'))
	if len(evs) != 1 || evs[0].Action != "own" {
		t.Fatalf("a row's own when overrides its keymap's: %+v", evs)
	}
	evs = press(a, r('e'))
	if len(evs) != 1 || evs[0].Action != "everywhere" {
		t.Fatalf("when=\"\" fires everywhere, even under a keymap when: %+v", evs)
	}
}

// 97. A <keymap when> whose id names no node (B005) is still the rows'
// effective when, exactly as the same when written on a row: dispatch
// and <hints scope="active"> see a selector that matches nothing, so
// neither row fires or shows (it must not fall back to "no when", which
// fires everywhere).
func TestKeymapWhenMissingIDLikeRowWhen(t *testing.T) {
	a := doc(t, `<tui version="3">
<keymap when="#nope"><bind keys="x" action="kmrow" label="km"/></keymap>
<keymap><bind keys="y" action="ownrow" when="#nope" label="own"/></keymap>
<screen id="s"><button id="b" label="b"/><hints id="h" scope="active"/></screen>
</tui>`)
	if evs := press(a, r('x'), r('y')); len(evs) != 0 {
		t.Errorf("rows under when=\"#nope\" fired: %+v", evs)
	}
	f := a.Frame(80, 24)
	if h := f.ByID["h"]; len(h.Children) != 0 {
		var labels []string
		for _, row := range h.Children {
			labels = append(labels, row.Children[1].Text)
		}
		t.Errorf("hints show %q, want none", labels)
	}
	// A when that does not parse (V003) is no selector at all, on a row
	// as on a <keymap>: both rows then fire and show alike.
	b := doc(t, `<tui version="3">
<keymap when="#"><bind keys="x" action="kmrow" label="km"/></keymap>
<keymap><bind keys="y" action="ownrow" when="#" label="own"/></keymap>
<screen id="s"><button id="b" label="b"/><hints id="h" scope="active"/></screen>
</tui>`)
	if evs := press(b, r('x'), r('y')); len(evs) != 2 || evs[0].Action != "kmrow" || evs[1].Action != "ownrow" {
		t.Errorf("V003 when: keymap and row differ: %+v", evs)
	}
	if h := b.Frame(80, 24).ByID["h"]; len(h.Children) != 2 {
		t.Errorf("V003 when: %d hints, want 2", len(h.Children))
	}
}

// key renders at cols x rows (as the live frame), handles k, then settles
// (so the offset a viewport shows this frame is what the next assertion
// reads).
func key(a *App, cols, rows int, k Key) []Event {
	a.Frame(cols, rows)
	evs := a.HandleKey(k)
	a.Frame(cols, rows)
	return evs
}

// 98. stick="bottom" (version="3"): a 5-row scroll pins to the end as its
// content grows; a move-prev off the end leaves the offset in place
// through further growth; a move-last snaps back to the end and sticks
// again from the next growth on; Dump on the live app reads the same
// state without changing it.
func TestStickRuntime(t *testing.T) {
	a := doc(t, `<tui version="3">
<keymap>
  <bind keys="p" action="move-prev" to="#log"/>
  <bind keys="e" action="move-last" to="#log"/>
</keymap>
<screen id="s">
  <scroll id="log" stick="bottom" height="5">
    <col each="lines as l"><text>{l}</text></col>
  </scroll>
</screen>
</tui>`)
	bindJSON(t, a, `{"lines": ["1", "2", "3", "4", "5"]}`)
	a.Frame(10, 5)
	if off := a.scrolls["log"]; off[1] != 0 {
		t.Fatalf("5 lines in a 5-row viewport: offset %v, want 0", off)
	}
	bindJSON(t, a, `{"lines": ["1", "2", "3", "4", "5", "6", "7", "8"]}`)
	a.Frame(10, 5)
	if off, mx := a.scrolls["log"], a.stickMax["log"]; off[1] != mx || mx != 3 {
		t.Fatalf("first growth: offset %v max %d, want offset==max==3", off, mx)
	}
	bindJSON(t, a, `{"lines": ["1", "2", "3", "4", "5", "6", "7", "8", "9", "10"]}`)
	a.Frame(10, 5)
	if off, mx := a.scrolls["log"], a.stickMax["log"]; off[1] != mx || mx != 5 {
		t.Fatalf("second growth: offset %v max %d, want offset==max==5", off, mx)
	}
	key(a, 10, 5, r('p'))
	if off := a.scrolls["log"]; off[1] != 4 {
		t.Fatalf("move-prev off the end: offset %v, want 4", off)
	}
	bindJSON(t, a, `{"lines": ["1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"]}`)
	a.Frame(10, 5)
	if off := a.scrolls["log"]; off[1] != 4 {
		t.Errorf("offset stays put while the content grows: got %v, want 4", off)
	}
	key(a, 10, 5, r('e'))
	if off, mx := a.scrolls["log"], a.stickMax["log"]; off[1] != mx || mx != 6 {
		t.Fatalf("move-last: offset %v max %d, want offset==max==6", off, mx)
	}
	bindJSON(t, a, `{"lines": ["1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"]}`)
	a.Frame(10, 5)
	if off, mx := a.scrolls["log"], a.stickMax["log"]; off[1] != mx || mx != 7 {
		t.Errorf("sticks again after move-last: offset %v max %d, want offset==max==7", off, mx)
	}
	beforeOff, beforeMax := a.scrolls["log"], a.stickMax["log"]
	if d := a.Dump(10, 5, false); len(d.Nodes) == 0 {
		t.Fatal("Dump built no tree")
	}
	if d := a.Dump(6, 5, false); len(d.Nodes) == 0 {
		t.Fatal("Dump at a different size built no tree")
	}
	if a.scrolls["log"] != beforeOff || a.stickMax["log"] != beforeMax {
		t.Errorf("Dump changed the live state: scrolls %v (was %v) max %v (was %v)", a.scrolls["log"], beforeOff, a.stickMax["log"], beforeMax)
	}
}

// linesJSON is {"lines": ["1", …, "n"]}.
func linesJSON(n int) string {
	var b strings.Builder
	b.WriteString(`{"lines": [`)
	for i := 1; i <= n; i++ {
		if i > 1 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", fmt.Sprint(i))
	}
	b.WriteString("]}")
	return b.String()
}

// A focused viewport's keys move from the offset the live frame shows,
// clamped to its maximum, so down (or end) at the end followed by up in
// the same read (no render between them, since down moved nothing) moves
// one row up: a stick scroll leaves the end and stays there as the
// content grows (SPEC v0.3b §11.4), and a plain scroll ends at max-1.
func TestFocusedViewportKeysClampToMax(t *testing.T) {
	const cols, rows = 20, 6
	for _, c := range []struct {
		name, stick string
		keys        []Key
		want        int // offset after one frame, then after growing to 12 lines (stick only)
	}{
		{"stick down up", ` stick="bottom"`, []Key{named("down"), named("up")}, 5},
		{"stick end up", ` stick="bottom"`, []Key{named("end"), named("up")}, 5},
		{"plain down up", "", []Key{named("end"), named("down"), named("up")}, 5},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := doc(t, `<tui version="3"><screen id="s" focus="#log"><scroll id="log" focusable="true"`+c.stick+` height="4"><col each="lines as l"><text>{l}</text></col></scroll></screen></tui>`)
			bindJSON(t, a, linesJSON(10))
			a.Frame(cols, rows)
			if c.stick == "" {
				// Bring the plain scroll to its end first, on its own frame.
				a.HandleKey(c.keys[0])
				a.Frame(cols, rows)
				c.keys = c.keys[1:]
			}
			if off := a.scrolls["log"]; off[1] != 6 {
				t.Fatalf("at the end: offset %v, want 6", off)
			}
			// One read: render between keys only when the frame is stale,
			// as Run and play do.
			for i, k := range c.keys {
				if i > 0 && a.TakeDirty() {
					a.Frame(cols, rows)
				}
				a.HandleKey(k)
			}
			a.Frame(cols, rows)
			if off := a.scrolls["log"]; off[1] != c.want {
				t.Fatalf("after %v: offset %v, want %d", c.keys, off, c.want)
			}
			bindJSON(t, a, linesJSON(12))
			a.Frame(cols, rows)
			if off := a.scrolls["log"]; off[1] != c.want {
				t.Errorf("after growing to 12 lines: offset %v, want %d (left the end, so it stays)", off, c.want)
			}
		})
	}
}

// 99. Label counters (version="3"): {path} in a tab's label and short
// resolves every frame; a growing counter moves the strip from tier 1 to
// tier 2; a missing path is B003 and an empty string; a short that
// resolves to "" falls back to the (resolved) label; a resolved LF is a
// space.
func TestTabLabelCounters(t *testing.T) {
	a := doc(t, `<tui version="3">
<screen id="s">
  <tabs id="nav" mark="▸" gap="1">
    <tab id="a" label="{n} processes" short="{n} p"><text>a</text></tab>
    <tab id="b" label="settings"><text>b</text></tab>
  </tabs>
</screen>
</tui>`)
	bindJSON(t, a, `{"n": 7}`)
	f := a.Frame(25, 3)
	if got := label(f, "nav", "a").Text; got != "7 processes" {
		t.Fatalf("tier 1 label %q, want \"7 processes\"", got)
	}
	// A growing counter widens the label past the strip's width, moving
	// tier 1 to tier 2 at the same screen width.
	bindJSON(t, a, `{"n": 700000000}`)
	f = a.Frame(25, 3)
	if got := label(f, "nav", "a").Text; got != "700000000 p" {
		t.Fatalf("tier 2 label (short) %q, want \"700000000 p\"", got)
	}

	// A missing path is B003 and resolves to the empty string.
	b := doc(t, `<tui version="3"><screen id="s"><tabs id="nav"><tab id="a" label="{missing}"><text>a</text></tab></tabs></screen></tui>`)
	bindJSON(t, b, `{}`)
	f = b.Frame(20, 3)
	if got := label(f, "nav", "a").Text; got != "" {
		t.Errorf("missing path label %q, want \"\"", got)
	}
	if !hasDiag(f, "B003", "missing") {
		t.Errorf("no B003 for the missing path: %v", f.Diags)
	}

	// A short that resolves to "" falls back to the (resolved) label; an
	// LF a resolved value brings becomes a space.
	c := doc(t, `<tui version="3"><screen id="s"><tabs id="nav" mark="▸" gap="1">
  <tab id="a" label="one two" short="{missing}"><text>a</text></tab>
  <tab id="b" label="{ml}" short="cfg"><text>b</text></tab>
</tabs></screen></tui>`)
	bindJSON(t, c, `{"ml": "line one\nline two"}`)
	// Wide enough for tier 1 (W1 = 28): the full, resolved label shows,
	// its LF turned into a space.
	f = c.Frame(30, 3)
	if got := label(f, "nav", "b").Text; got != "line one line two" {
		t.Errorf("resolved LF becomes a space: %q, want \"line one line two\"", got)
	}
	// Between W2 (13) and W1 (28): tier 2, the short form. #a's short=
	// resolves to "" (a missing path) and falls back to its (resolved)
	// label.
	f = c.Frame(15, 3)
	if got := label(f, "nav", "a").Text; got != "one two" {
		t.Errorf("short resolving to \"\" falls back to the label: %q, want \"one two\"", got)
	}
	if got := label(f, "nav", "b").Text; got != "cfg" {
		t.Errorf("tier 2 short %q, want \"cfg\"", got)
	}
}

// 100. A modal's focus= (version="3"): the target the modal opens with;
// falls back to the first entry of the cycle when it is disabled; does
// not move focus already inside the modal; is also the fallback when the
// focused node inside the modal is lost.
func TestModalFocusRuntime(t *testing.T) {
	a := doc(t, `<tui version="3">
<screen id="s" focus="#trigger">
  <button id="trigger" label="open" on:click="open_modal"/>
  <modal id="m" bind="open" focus="#no">
    <button id="yes" label="yes" if="showYes" on:click="doyes"/>
    <button id="no" label="no" on:click="dono" on:focus="no_focus"/>
  </modal>
</screen>
</tui>`)
	bindJSON(t, a, `{"open": false, "showYes": true}`)
	a.Frame(40, 10)
	_ = a.Set("open", true)
	a.Frame(40, 10)
	if a.Focus() != "no" {
		t.Fatalf("opening the modal focuses its focus=, got %q", a.Focus())
	}
	evs := press(a, named("enter"))
	if len(evs) != 1 || evs[0].Action != "dono" {
		t.Fatalf("enter on the focused #no: %+v", evs)
	}
	// Focus already inside the modal does not move: send it to #yes, then
	// render again for an unrelated reason.
	if err := a.Set("@focus", "#yes"); err != nil {
		t.Fatal(err)
	}
	a.Frame(40, 10)
	if a.Focus() != "yes" {
		t.Fatalf("focus %q, want yes", a.Focus())
	}
	a.Frame(40, 10) // an unrelated re-render
	if a.Focus() != "yes" {
		t.Fatalf("focus moved on an unrelated re-render: %q", a.Focus())
	}
	// Lost focus: #yes leaves the frame (if= turns false); focus falls
	// back to the modal's focus=, which fires on:focus.
	_ = a.Set("showYes", false)
	a.Frame(40, 10)
	if a.Focus() != "no" {
		t.Fatalf("lost focus falls back to focus=, got %q", a.Focus())
	}
	fired := false
	for _, e := range a.TakePending() {
		if e.Action == "no_focus" && e.Source == "no" {
			fired = true
		}
	}
	if !fired {
		t.Errorf("no on:focus for #no on the lost-focus fallback: %v", a.TakePending())
	}
}

// A modal focus= target disabled this frame falls back to the first
// entry of the modal's focus cycle.
func TestModalFocusDisabledFallsBack(t *testing.T) {
	a := doc(t, `<tui version="3">
<screen id="s" focus="#trigger">
  <button id="trigger" label="open" on:click="open_modal"/>
  <modal id="m" bind="open" focus="#no">
    <button id="yes" label="yes"/>
    <button id="no" label="no" disabled="true"/>
  </modal>
</screen>
</tui>`)
	bindJSON(t, a, `{"open": false}`)
	a.Frame(40, 10)
	_ = a.Set("open", true)
	a.Frame(40, 10)
	if a.Focus() != "yes" {
		t.Fatalf("a disabled focus= target falls back to the first entry, got %q", a.Focus())
	}
}

// A modal's focus= is version="3" only: in a version="1"/"2" document it
// is V002 and the runtime ignores it, so the modal opens on the first
// entry of its cycle, as in 0.3.0.
func TestModalFocusIgnoredBeforeV3(t *testing.T) {
	for _, v := range []string{"1", "2"} {
		a := doc(t, `<tui version="`+v+`">
<screen id="s">
  <modal id="m" open="true" focus="#no">
    <button id="yes" label="yes"/>
    <button id="no" label="no"/>
  </modal>
</screen>
</tui>`)
		a.Frame(40, 10)
		if a.Focus() != "yes" {
			t.Errorf("version=%q: focus %q, want yes (focus= ignored)", v, a.Focus())
		}
	}
}
