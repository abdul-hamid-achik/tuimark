package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// press renders a frame (keys dispatch against the last frame), then
// handles each key and returns every event produced, in order.
func press(a *App, keys ...Key) []Event {
	var out []Event
	for _, k := range keys {
		a.Frame(80, 24)
		out = append(out, a.HandleKey(k)...)
	}
	a.Frame(80, 24)
	return out
}

func r(c rune) Key          { return Key{Name: string(c), Rune: c} }
func named(name string) Key { return Key{Name: name} }
func typed(s string) []Key {
	var out []Key
	for _, c := range s {
		if c == ' ' {
			out = append(out, Key{Name: "space", Rune: ' '})
			continue
		}
		out = append(out, r(c))
	}
	return out
}

func payload(t *testing.T, ev Event) string {
	t.Helper()
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// SPEC §8.2 payloads, byte for byte.
func TestEventPayloads(t *testing.T) {
	a := inbox(t)
	if a.Focus() != "" {
		t.Fatal("focus is set by the first frame")
	}
	a.Frame(80, 24)
	if a.Focus() != "query" {
		t.Fatalf("screen focus=\"#query\": focus = %q", a.Focus())
	}
	evs := press(a, named("tab"), named("enter"))
	if len(evs) != 1 {
		t.Fatalf("events = %+v", evs)
	}
	if got := payload(t, evs[0]); got != `{"action":"open","source":"inbox","keys":{"item":"t-12"},"value":null}` {
		t.Errorf("open = %s", got)
	}
	evs = press(a, named("down"))
	if got := payload(t, evs[0]); len(evs) != 1 || got != `{"action":"open","source":"inbox","keys":{"item":"t-18"},"value":null}` {
		t.Errorf("on:select = %s", got)
	}
	if get(t, a, "selected") != "t-18" {
		t.Error("list bind= follows the selection")
	}
	evs = press(a, r('/'))
	if len(evs) != 0 || a.Focus() != "query" {
		t.Errorf("/ focuses #query: %v focus=%q", evs, a.Focus())
	}
	evs = press(a, append(typed("ab c"), named("enter"))...)
	if len(evs) != 1 {
		t.Fatalf("events = %+v", evs)
	}
	if got := payload(t, evs[0]); got != `{"action":"search","source":"query","keys":{},"value":"ab c"}` {
		t.Errorf("submit = %s", got)
	}
}

func TestInputEditing(t *testing.T) {
	a := doc(t, v1(`<input id="q" bind="text" on:change="chg" on:submit="go"/>`))
	_ = a.Bind("text", "")
	var values []string
	record := func(evs []Event) {
		for _, e := range evs {
			values = append(values, e.Action+"="+fmt.Sprint(e.Value))
		}
	}
	steps := []struct {
		keys []Key
		want string // store value after the keys
	}{
		{typed("hi"), "hi"},
		{[]Key{named("left"), named("left"), r('x')}, "xhi"},
		{[]Key{named("end"), named("backspace")}, "xh"},
		{[]Key{named("home"), named("backspace")}, "xh"}, // nothing to delete
		{[]Key{named("end"), named("ctrl+h")}, "x"},      // ctrl+h erases like backspace in an input
		{typed("h"), "xh"},
		{[]Key{named("ctrl+e"), r('!')}, "xh!"},
		{[]Key{named("ctrl+a"), r('>')}, ">xh!"},
		{[]Key{named("right"), named("right"), named("right"), named("right"), named("right"), r('.')}, ">xh!."},
		{[]Key{named("ctrl+u")}, ""},
		{[]Key{named("ctrl+u")}, ""}, // already empty: no change event
		{typed("ñ é"), "ñ é"},
	}
	for i, s := range steps {
		record(press(a, s.keys...))
		if got := get(t, a, "text"); got != s.want {
			t.Errorf("step %d: text = %q, want %q", i, got, s.want)
		}
	}
	record(press(a, named("enter")))
	want := "chg=h chg=hi chg=xhi chg=xh chg=x chg=xh chg=xh! chg=>xh! chg=>xh!. chg= chg=ñ chg=ñ  chg=ñ é go=ñ é"
	if got := strings.Join(values, " "); got != want {
		t.Errorf("events:\n got %s\nwant %s", got, want)
	}
	f := a.Frame(20, 1)
	if b := f.ByID["q"]; b.Cursor != 3 || b.Text != "ñ é" {
		t.Errorf("cursor %d text %q", b.Cursor, b.Text)
	}
	if !f.Grid.CursorOn || f.Grid.CursorX != 3 {
		t.Errorf("terminal cursor at %d (on=%v)", f.Grid.CursorX, f.Grid.CursorOn)
	}

	// An unbound input keeps its own value; enter without on:submit is not
	// consumed, so the keymap sees it.
	b := doc(t, `<tui version="1"><keymap><bind keys="enter" action="ok"/></keymap><screen><input id="u" secret="true"/></screen></tui>`)
	evs := press(b, append(typed("pw"), named("enter"))...)
	if len(evs) != 1 || evs[0].Action != "ok" || evs[0].Value != "pw" {
		t.Errorf("unbound input: %+v", evs)
	}
	if d := b.Dump(10, 1, false); !strings.HasPrefix(d.Grid[0], "••") || d.Nodes[1].Text != "••" {
		t.Errorf("secret: %q %+v", d.Grid[0], d.Nodes)
	}
}

// Dispatch order: (1) esc -> top modal on:escape, (2) focused widget,
// (3) keymap rows in document order with when, (4) built-ins.
func TestDispatchOrder(t *testing.T) {
	src := `<tui version="1">
<keymap>
  <bind keys="q" action="quit"/>
  <bind keys="esc" action="back"/>
  <bind keys="x" action="first" when="#l:focus"/>
  <bind keys="x" action="second"/>
  <bind keys="enter" action="open" when="list:focus"/>
  <bind keys="ctrl+c" action="copy" when="#c:focus"/>
  <bind keys="/" action="focus" to="#q"/>
</keymap>
<screen id="s" focus="#q">
  <input id="q"/>
  <list id="l"><item><text>a</text></item><item><text>b</text></item></list>
  <button id="c" on:click="clicked">ok</button>
  <modal id="m" bind="dlg" on:escape="close"><button id="mb">x</button></modal>
</screen>
</tui>`
	a := doc(t, src)
	names := func(evs []Event) string {
		var out []string
		for _, e := range evs {
			out = append(out, e.Action+"@"+e.Source)
		}
		return strings.Join(out, " ")
	}
	cases := []struct {
		name  string
		focus string
		dlg   bool
		keys  []Key
		want  string
	}{
		{"input consumes printable keys before the keymap", "q", false, []Key{r('q'), r('x')}, ""},
		{"esc falls through the input to the keymap", "q", false, []Key{named("esc")}, "back@q"},
		{"esc goes to the open modal first", "mb", true, []Key{named("esc")}, "close@m"},
		{"keymap on the list", "l", false, []Key{r('q')}, "quit@l"},
		{"first matching row wins", "l", false, []Key{r('x')}, "first@l"},
		{"when filters rows", "c", false, []Key{r('x')}, "second@c"},
		{"list does not consume enter", "l", false, []Key{named("enter")}, "open@l"},
		{"when with a type selector", "q", false, []Key{named("enter")}, ""},
		{"button click on enter", "c", false, []Key{named("enter")}, "clicked@c"},
		{"button click on space", "c", false, []Key{named("space")}, "clicked@c"},
		{"keymap overrides ctrl+c when it matches", "c", false, []Key{named("ctrl+c")}, "copy@c"},
		{"built-in ctrl+c is quit", "l", false, []Key{named("ctrl+c")}, "quit@l"},
		{"unknown keys do nothing", "l", false, []Key{named("f5")}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_ = a.Bind("dlg", c.dlg)
			a.Frame(80, 24)
			_ = a.Set("@focus", "#"+c.focus)
			a.Frame(80, 24)
			if a.Focus() != c.focus {
				t.Fatalf("focus = %q, want %q", a.Focus(), c.focus)
			}
			if got := names(press(a, c.keys...)); got != c.want {
				t.Errorf("events = %q, want %q", got, c.want)
			}
		})
	}
	_ = a.Bind("dlg", false)
	_ = a.Set("@focus", "#c")
	press(a, r('/'))
	if a.Focus() != "q" {
		t.Errorf("focus action: %q", a.Focus())
	}
	// A disabled focused widget does not consume keys.
	b := doc(t, v1(`<button id="b" on:click="c" disabled="off">x</button>`))
	_ = b.Bind("off", false)
	b.Frame(10, 1)
	if evs := press(b, named("enter")); len(evs) != 1 {
		t.Fatalf("enabled button: %v", evs)
	}
	_ = b.Set("off", true)
	b.Frame(10, 1)
	_ = b.Set("@focus", "#b")
	if evs := press(b, named("enter")); len(evs) != 0 {
		t.Errorf("disabled button fired %v", evs)
	}
}

func TestListKeys(t *testing.T) {
	a := doc(t, v1(`<list id="l" each="rows as r" key="r" bind="sel" on:select="pick" style="height: 2"><item><text>{r}</text></item></list>`))
	_ = a.Bind("", map[string]any{"rows": []any{"a", "b", "c", "d", "e"}})
	steps := []struct {
		key  string
		sel  string
		fire bool
	}{
		{"up", "", false}, // already at the top: no event
		{"down", "b", true},
		{"end", "e", true},
		{"down", "e", false},
		{"pgup", "c", true}, // page = viewport height (2)
		{"home", "a", true},
		{"pgdn", "c", true},
		{"left", "c", false},
	}
	for _, s := range steps {
		evs := press(a, named(s.key))
		if (len(evs) == 1) != s.fire {
			t.Errorf("%s: events %v", s.key, evs)
		}
		if s.sel != "" {
			if got, _ := a.Get("sel"); got != s.sel {
				t.Errorf("%s: sel = %v, want %s", s.key, got, s.sel)
			}
			if s.fire && evs[0].Keys["r"] != s.sel {
				t.Errorf("%s: payload keys = %v", s.key, evs[0].Keys)
			}
		}
	}
	// SPEC §8.1: j/k are NOT implicit. Without a keymap row they do nothing
	// on a focused list (from "c", j would select "d" and k "b").
	for _, c := range "jk" {
		if evs := press(a, r(c)); len(evs) != 0 {
			t.Errorf("%c: implicit list navigation fired %v", c, evs)
		}
		if got, _ := a.Get("sel"); got != "c" {
			t.Errorf("%c: sel = %v, want c (j/k must not be implicit)", c, got)
		}
	}
	// An app that wants them binds them in the keymap; the list still does
	// not move on its own.
	jk := doc(t, `<tui version="1"><keymap><bind keys="j" action="next" when="#l:focus"/></keymap>
<screen id="s"><list id="l" each="rows as r" key="r" bind="sel" on:select="pick"><item><text>{r}</text></item></list></screen></tui>`)
	_ = jk.Bind("", map[string]any{"rows": []any{"a", "b"}, "sel": "a"})
	if evs := press(jk, r('j')); len(evs) != 1 || evs[0].Action != "next" || evs[0].Keys["r"] != "a" {
		t.Errorf("bound j: events %v, want one next with r=a", evs)
	}
	if got, _ := jk.Get("sel"); got != "a" {
		t.Errorf("bound j: sel = %v, want a", got)
	}
	// An empty list consumes nothing.
	_ = a.Bind("rows", []any{})
	if evs := press(a, named("down")); len(evs) != 0 {
		t.Errorf("empty list: %v", evs)
	}
}

func TestFocusCycling(t *testing.T) {
	a := doc(t, v1(`<col>
  <input id="a"/>
  <input id="b" hidden="true"/>
  <input id="c" disabled="true"/>
  <box id="plain"/>
  <list id="d"/>
  <box id="e" focusable="true" on:focus="entered"/>
  <button id="f">x</button>
</col>`))
	a.Frame(80, 24)
	if a.Focus() != "a" {
		t.Fatalf("no screen focus: the first focusable gets focus, got %q", a.Focus())
	}
	var order []string
	for i := 0; i < 5; i++ {
		evs := press(a, named("tab"))
		order = append(order, a.Focus())
		if a.Focus() == "e" && (len(evs) != 1 || evs[0].Action != "entered" || evs[0].Source != "e") {
			t.Errorf("on:focus = %v", evs)
		}
	}
	if got := strings.Join(order, " "); got != "d e f a d" {
		t.Errorf("tab order = %s", got)
	}
	order = nil
	for i := 0; i < 4; i++ {
		press(a, named("shift+tab"))
		order = append(order, a.Focus())
	}
	if got := strings.Join(order, " "); got != "a f e d" {
		t.Errorf("shift+tab order = %s", got)
	}
	// Focus lost (the node disappears): the next frame picks a valid target.
	b := doc(t, v1(`<input id="x" if="show"/><input id="y"/>`))
	_ = b.Bind("show", true)
	b.Frame(10, 2)
	_ = b.Set("@focus", "#x")
	b.Frame(10, 2)
	_ = b.Set("show", false)
	b.Frame(10, 2)
	if b.Focus() != "y" {
		t.Errorf("focus after its node vanished = %q", b.Focus())
	}
	// Nothing focusable: tab is a no-op.
	c := doc(t, v1(`<text>x</text>`))
	if evs := press(c, named("tab")); len(evs) != 0 || c.Focus() != "" {
		t.Errorf("no focusables: %v %q", evs, c.Focus())
	}
}

func TestModalFocusTrapAndRestore(t *testing.T) {
	a := doc(t, `<tui version="1">
<keymap><bind keys="/" action="focus" to="#a"/></keymap>
<screen id="s" focus="#a">
  <input id="a"/>
  <button id="b">x</button>
  <modal id="m" bind="dlg" title="Confirm" on:escape="close" on:open="opened" on:close="closed">
    <input id="m1"/>
    <button id="m2">ok</button>
  </modal>
</screen>
</tui>`)
	_ = a.Bind("dlg", false)
	a.Frame(80, 24)
	_ = a.Set("@focus", "#b")
	a.Frame(80, 24)
	a.TakePending()

	_ = a.Set("dlg", true)
	f := a.Frame(80, 24)
	if a.Focus() != "m1" {
		t.Fatalf("opening a modal moves focus inside it, got %q", a.Focus())
	}
	if len(f.Modals) != 1 || !f.Modals[0].Modal || f.ByID["m1"] == nil {
		t.Fatal("modal frame")
	}
	if p := a.TakePending(); len(p) != 1 || p[0].Action != "opened" || p[0].Source != "m" {
		t.Errorf("on:open = %+v", p)
	}
	var order []string
	for i := 0; i < 3; i++ {
		press(a, named("tab"))
		order = append(order, a.Focus())
	}
	if got := strings.Join(order, " "); got != "m2 m1 m2" {
		t.Errorf("tab is trapped in the modal: %s", got)
	}
	press(a, r('/')) // a keymap focus outside the modal snaps back
	if a.Focus() != "m1" && a.Focus() != "m2" {
		t.Errorf("focus escaped the modal: %q", a.Focus())
	}
	evs := press(a, named("esc"))
	if len(evs) != 1 || evs[0].Action != "close" || evs[0].Source != "m" {
		t.Fatalf("esc = %+v", evs)
	}
	_ = a.Set("dlg", false)
	a.Frame(80, 24)
	if a.Focus() != "b" {
		t.Errorf("closing the modal restores focus to #b, got %q", a.Focus())
	}
	if p := a.TakePending(); len(p) != 1 || p[0].Action != "closed" || p[0].Source != "m" {
		t.Errorf("on:close = %+v", p)
	}
	// The modal is painted over the screen and clears its rectangle.
	_ = a.Set("dlg", true)
	d := a.Dump(40, 10, false)
	if !strings.Contains(strings.Join(d.Grid, "\n"), "┌─ Confirm ") {
		t.Errorf("modal title:\n%s", strings.Join(d.Grid, "\n"))
	}
	if d.Nodes[len(d.Nodes)-3].ID != "m" {
		t.Errorf("modal nodes come last: %+v", d.Nodes)
	}
}

func TestReservedPaths(t *testing.T) {
	a := doc(t, `<tui version="1">
<screen id="one" focus="#b"><input id="a"/><input id="b"/></screen>
<screen id="two" focus="#d"><text>second</text><input id="c"/><input id="d"/></screen>
</tui>`)
	a.Frame(20, 3)
	if a.Focus() != "b" {
		t.Fatalf("initial focus = %q", a.Focus())
	}
	if err := a.Set("@screen", "two"); err != nil {
		t.Fatal(err)
	}
	a.Frame(20, 3) // the live frame resolves focus; Dump is a snapshot
	d := a.Dump(20, 3, false)
	if !strings.HasPrefix(d.Grid[0], "second") || a.Focus() != "d" {
		t.Errorf("screen two: %q focus %q", d.Grid[0], a.Focus())
	}
	if err := a.Set("@screen", "nope"); err == nil {
		t.Error("unknown screen accepted")
	}
	if err := a.Set("@focus", "#a"); err != nil {
		t.Fatal(err)
	}
	a.Frame(20, 3)
	d = a.Dump(20, 3, false)
	if a.Focus() != "a" || strings.HasPrefix(d.Grid[0], "second") {
		t.Errorf("@focus switches to the target's screen: focus %q grid %q", a.Focus(), d.Grid[0])
	}
	_ = a.Set("@focus", "#missing")
	if a.Focus() != "a" {
		t.Errorf("@focus to an unknown id is ignored: %q", a.Focus())
	}
	_ = a.Set("@screen", "one") // same screen: focus kept
	a.Frame(20, 3)
	if a.Focus() != "a" {
		t.Errorf("re-selecting the current screen reset focus: %q", a.Focus())
	}
	if n, ok := nodeByID(a.Dump(20, 3, false), "a"); !ok || !n.Focused {
		t.Error("dump marks the focused node")
	}
}

func TestQuitSemantics(t *testing.T) {
	a := doc(t, v1(`<text>x</text>`))
	if quit, err := a.Dispatch(Event{Action: "quit"}); !quit || err != nil {
		t.Errorf("built-in quit without a handler: %v %v", quit, err)
	}
	if quit, err := a.Dispatch(Event{Action: "unknown"}); quit || err != nil {
		t.Errorf("an unhandled action is a no-op: %v %v", quit, err)
	}
	calls := 0
	a.On("quit", func(Event) error { calls++; return nil })
	if quit, err := a.Dispatch(Event{Action: "quit"}); quit || err != nil || calls != 1 {
		t.Errorf("a quit handler decides: %v %v %d", quit, err, calls)
	}
	a.On("quit", func(Event) error { return ErrQuit })
	if quit, err := a.Dispatch(Event{Action: "quit"}); !quit || err != nil {
		t.Errorf("ErrQuit: %v %v", quit, err)
	}
	a.On("done", func(Event) error { return fmt.Errorf("wrapped: %w", ErrQuit) })
	if quit, err := a.Dispatch(Event{Action: "done"}); !quit || err != nil {
		t.Errorf("wrapped ErrQuit: %v %v", quit, err)
	}
	boom := errors.New("boom")
	a.On("fail", func(Event) error { return boom })
	if quit, err := a.Dispatch(Event{Action: "fail"}); quit || !errors.Is(err, boom) {
		t.Errorf("handler error: %v %v", quit, err)
	}
	var got Event
	a.On("open", func(ev Event) error { got = ev; return nil })
	_, _ = a.Dispatch(Event{Action: "open", Source: "s", Keys: map[string]any{"item": "t"}})
	if got.Source != "s" || got.Keys["item"] != "t" {
		t.Errorf("handler received %+v", got)
	}
}

func TestInitialFocusEvent(t *testing.T) {
	a := doc(t, v1(`<input id="q" on:focus="hello"/>`))
	a.Frame(10, 1)
	p := a.TakePending()
	if len(p) != 1 || p[0].Action != "hello" || p[0].Source != "q" {
		t.Errorf("pending = %+v", p)
	}
	if p := a.TakePending(); len(p) != 0 {
		t.Errorf("TakePending clears: %+v", p)
	}
	// Initial focus from screen focus="#id" fires on:focus too, once.
	b := doc(t, `<tui version="1"><screen focus="#y"><input id="x" on:focus="fx"/><input id="y" on:focus="fy"/></screen></tui>`)
	b.Frame(10, 2)
	b.Frame(10, 2)
	if p := b.TakePending(); len(p) != 1 || p[0].Action != "fy" || p[0].Source != "y" {
		t.Errorf("screen focus: pending = %+v", p)
	}
	// A screen focus that cannot be honored falls back to the first
	// focusable, which gets the event instead.
	c := doc(t, `<tui version="1"><screen focus="#y"><input id="x" on:focus="fx"/><input id="y" hidden="true" on:focus="fy"/></screen></tui>`)
	c.Frame(10, 2)
	if p := c.TakePending(); len(p) != 1 || p[0].Action != "fx" || c.Focus() != "x" {
		t.Errorf("fallback focus: pending = %+v focus %q", p, c.Focus())
	}
}
