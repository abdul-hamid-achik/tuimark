package host

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Regressions from the ergonomics review of 0.2b: keys of one read see
// the frame an earlier key changed (SPEC v0.2b §8.6), list rows whose
// item is pruned (§6.14), classes without repeats (§6.13), a tab
// activation whose bind path cannot be written (§6.10.2), and check-* on
// a checked path that cannot be written (§6.14, §8.4).

// Regression (SPEC v0.2b §8.6): in a version="2" document every input
// that changes state the live frame shows marks it stale (TakeDirty), so
// Run and play render it again before the next input of the same read:
// typing (one key or a coalesced run), an edit, an input cursor move, a
// paste, a list cursor moved by a widget key, a viewport offset. An input
// that changes nothing does not. A version="1" document never sets the
// flag (the v0.1 loop).
func TestInputsMarkFrameStale(t *testing.T) {
	const body = `<screen id="s" focus="#q">
<input id="q" bind="query"/>
<list id="l" each="rows as r" key="r" height="2"><item><text>{r}</text></item></list>
<scroll id="v" height="2" focusable="true"><text>1</text><text>2</text><text>3</text></scroll>
</screen></tui>`
	for _, version := range []string{"1", "2"} {
		a := doc(t, `<tui version="`+version+`">`+body)
		bindJSON(t, a, `{"query":"","rows":["a","b"]}`)
		a.Frame(20, 8)
		step := func(what string, want bool, apply func()) {
			t.Helper()
			apply()
			if version == "1" {
				want = false
			}
			if got := a.TakeDirty(); got != want {
				t.Errorf("version=%s %s: TakeDirty %v, want %v", version, what, got, want)
			}
			a.Frame(20, 8)
		}
		step("typed x", true, func() { a.HandleKey(r('x')) })
		step("typed run", true, func() { a.HandleKeyRun(typed("yz")) })
		step("left", true, func() { a.HandleKey(named("left")) })
		step("home", true, func() { a.HandleKey(named("home")) })
		step("home at 0", false, func() { a.HandleKey(named("home")) })
		step("backspace at 0", false, func() { a.HandleKey(named("backspace")) })
		step("paste", true, func() { a.HandlePaste("p") })
		step("empty paste", false, func() { a.HandlePaste("\x1b") })
		if err := a.Set("@focus", "#l"); err != nil {
			t.Fatal(err)
		}
		a.Frame(20, 8)
		a.TakeDirty()
		step("list down", true, func() { a.HandleKey(named("down")) })
		step("list down on the last row", false, func() { a.HandleKey(named("down")) })
		if err := a.Set("@focus", "#v"); err != nil {
			t.Fatal(err)
		}
		a.Frame(20, 8)
		a.TakeDirty()
		step("viewport down", true, func() { a.HandleKey(named("down")) })
		step("viewport down at its maximum", false, func() { a.HandleKey(named("down")) })
		step("viewport end at its maximum", false, func() { a.HandleKey(named("end")) })
		step("viewport home", true, func() { a.HandleKey(named("home")) })
	}
}

// loopEvents runs Loop over a pipe, writes each chunk as one write (one
// read), and returns the first n events the handlers of actions record.
func loopEvents(t *testing.T, a *App, actions []string, n int, chunks ...string) []string {
	t.Helper()
	evs := make(chan string, 16)
	for _, act := range actions {
		a.On(act, func(ev Event) error {
			b, _ := json.Marshal(ev)
			evs <- string(b)
			return nil
		})
	}
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- a.Loop(pr, io.Discard, func() (int, int) { return 20, 4 }, nil) }()
	for _, c := range chunks {
		if _, err := io.WriteString(pw, c); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for len(got) < n {
		select {
		case e := <-evs:
			got = append(got, e)
		case <-time.After(3 * time.Second):
			t.Fatalf("got %v, want %d events", got, n)
		}
	}
	pw.Close()
	<-done
	return got
}

// Regression (SPEC v0.2b §8.6, §6.13): keys that arrive in one read are
// dispatched each on the frame the previous one left, exactly as when
// they arrive in separate reads: text typed and then enter in one write
// sees the input's class guard (when="#q.filled"); a list moved by the
// down key and then x sees the list's guard; a filter typed and then
// down sees the list its if= just showed.
func TestLoopKeysOfOneReadSeeTheirEdits(t *testing.T) {
	cases := []struct {
		name, src, data, act string
		chunks               []string
		want                 string
	}{
		{
			"typing then enter", `<tui version="2"><keymap><bind keys="enter" action="go" when="#q.filled"/></keymap>
<screen id="s" focus="#q"><input id="q" bind="query" class:filled="query"/></screen></tui>`,
			`{"query":""}`, "go", []string{"ab\r"},
			`{"action":"go","source":"q","keys":{},"value":"ab"}`,
		},
		{
			"paste then enter", `<tui version="2"><keymap><bind keys="enter" action="go" when="#q.filled"/></keymap>
<screen id="s" focus="#q"><input id="q" bind="query" class:filled="query"/></screen></tui>`,
			`{"query":""}`, "go", []string{"\x1b[200~ab\x1b[201~\r"},
			`{"action":"go","source":"q","keys":{},"value":"ab"}`,
		},
		{
			"list down then x", `<tui version="2"><keymap><bind keys="x" action="picked" when="#l.picked"/></keymap>
<screen id="s" focus="#l"><list id="l" each="rows as r" key="r" bind="cur" class:picked="cur"><item><text>{r}</text></item></list></screen></tui>`,
			`{"cur":null,"rows":["a","b"]}`, "picked", []string{"\x1b[Bx"},
			`{"action":"picked","source":"l","keys":{"r":"b"},"value":null}`,
		},
		{
			"filter then down", `<tui version="2"><keymap><bind keys="down" action="move-next" to="#res"/></keymap>
<screen id="s" focus="#q"><input id="q" bind="query"/>
<list id="res" if="query" each="rows as r" key="r" bind="cur" on:select="picked"><item><text>{r}</text></item></list></screen></tui>`,
			`{"query":"","cur":null,"rows":["a","b","c"]}`, "picked", []string{"x\x1b[B"},
			`{"action":"picked","source":"res","keys":{"r":"b"},"value":null}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := doc(t, c.src)
			bindJSON(t, a, c.data)
			got := loopEvents(t, a, []string{c.act}, 1, c.chunks...)
			if got[0] != c.want {
				t.Errorf("events %v, want %s", got, c.want)
			}
		})
	}
}

// Regression (SPEC §6.9.3, v0.2b §6.14): a list follows its cursor row
// among the rows that are laid out. Rows whose item if= pruned before the
// cursor row do not push it out of view (version="1" and "2"), nor do
// rows dropped by display: none; a cursor row without an item has
// nothing to follow and leaves the offset alone.
func TestListFollowsCursorPastPrunedRows(t *testing.T) {
	const rows = `{"cur":"c","rows":[{"id":"a"},{"id":"b"},{"id":"c","show":1},{"id":"d","show":1},{"id":"e","show":1},{"id":"f","show":1}]}`
	for _, c := range []struct{ name, src string }{
		{"v1 if", `<tui version="1"><screen id="s" focus="#l"><list id="l" each="rows as r" key="r.id" bind="cur" height="2"><item if="r.show"><text>{r.id}</text></item></list></screen></tui>`},
		{"v2 if", `<tui version="2"><screen id="s" focus="#l"><list id="l" each="rows as r" key="r.id" bind="cur" height="2"><item if="r.show"><text>{r.id}</text></item></list></screen></tui>`},
		{"v2 hidden", `<tui version="2"><screen id="s" focus="#l"><list id="l" each="rows as r" key="r.id" bind="cur" height="2"><item hidden="!r.show"><text>{r.id}</text></item></list></screen></tui>`},
		{"v2 display none", `<tui version="2"><style>.gone { display: none; }</style><screen id="s" focus="#l"><list id="l" each="rows as r" key="r.id" bind="cur" height="2"><item class:gone="!r.show"><text>{r.id}</text></item></list></screen></tui>`},
	} {
		a := doc(t, c.src)
		bindJSON(t, a, rows)
		d := a.Dump(10, 3, false)
		if got := strings.Join(d.Grid[:2], "|"); got != "c         |d         " {
			t.Errorf("%s: grid %q, want the cursor row c at the top", c.name, got)
		}
		for _, n := range d.Nodes {
			if n.Selected && n.Y != 0 {
				t.Errorf("%s: the selected row is at y=%d, want 0", c.name, n.Y)
			}
		}
		// The cursor on a pruned row: nothing to follow, offset 0.
		if err := a.Set("cur", "a"); err != nil {
			t.Fatal(err)
		}
		d = a.Dump(10, 3, false)
		if got := strings.Join(d.Grid[:2], "|"); got != "c         |d         " {
			t.Errorf("%s: cursor on a pruned row: grid %q", c.name, got)
		}
	}
}

// Regression (SPEC v0.2b §6.14, §8.4): a list's rows are the elements of
// its each array, whether or not if= keeps their item. The cursor can rest
// on a row without an item, check-toggle toggles it, check-all appends
// every row key, and a list whose items are all pruned still has rows:
// its check-* and move-* rows match and its widget keys are consumed.
func TestPrunedListItemsAreRows(t *testing.T) {
	a := doc(t, `<tui version="2"><keymap>
<bind keys="space" action="check-toggle" when="#l:focus"/>
<bind keys="a" action="check-all"/>
<bind keys="j" action="move-next"/>
</keymap><screen id="s" focus="#l">
<list id="l" each="rows as r" key="r.id" bind="cur" checked="marked" on:change="changed" on:select="sel"><item if="r.show"><text>{r.id}</text></item></list>
</screen></tui>`)
	bindJSON(t, a, `{"cur":null,"marked":[],"rows":[{"id":"a"},{"id":"b","show":true},{"id":"c"}]}`)
	got := evText(t, keysAt(a, 20, 4, named("space"), r('a'), r('j'), named("down"), named("space")))
	want := strings.Join([]string{
		`{"action":"changed","source":"l","keys":{"r":"a"},"value":["a"]}`,
		`{"action":"changed","source":"l","keys":{"r":"a"},"value":["a","b","c"]}`,
		`{"action":"sel","source":"l","keys":{"r":"b"},"value":null}`,
		`{"action":"sel","source":"l","keys":{"r":"c"},"value":null}`,
		`{"action":"changed","source":"l","keys":{"r":"c"},"value":["a","b"]}`,
	}, "\n")
	if got != want {
		t.Errorf("events:\n%s\nwant\n%s", got, want)
	}
	bindJSON(t, a, `{"cur":"a","marked":[],"rows":[{"id":"a"},{"id":"b"}]}`)
	got = evText(t, keysAt(a, 20, 4, r('j'), named("space")))
	want = `{"action":"sel","source":"l","keys":{"r":"b"},"value":null}` + "\n" +
		`{"action":"changed","source":"l","keys":{"r":"b"},"value":["b"]}`
	if got != want {
		t.Errorf("every item pruned: events:\n%s\nwant\n%s", got, want)
	}
}

// Regression (SPEC v0.2b §6.13, §13.2, §15.7): in a version="2" document
// an element's classes have no repeats, static names included (the first
// occurrence wins): the element, a table's header cell, its rows, and its
// body cells, in the dump and in inspect. A version="1" document keeps
// its class names as written.
func TestClassesHaveNoRepeats(t *testing.T) {
	a := doc(t, `<tui version="2"><screen id="s">
<text id="t" class="a b a" class:b="t" class:c="t">x</text>
<table id="tb" each="rows as r" key="r"><item class="z z"/><column id="col" class="n n" class:n="t" title="T">{r}</column></table>
</screen></tui>`)
	bindJSON(t, a, `{"t":true,"rows":[1]}`)
	d := a.Dump(20, 4, false)
	var got []string
	for _, n := range d.Nodes {
		if len(n.Classes) > 0 {
			got = append(got, n.Tag+":"+strings.Join(n.Classes, ","))
		}
	}
	if want := "text:a,b,c column:n item:z text:n"; strings.Join(got, " ") != want {
		t.Errorf("classes %q, want %q", strings.Join(got, " "), want)
	}
	f := a.Frame(20, 4)
	in, err := a.Inspect(f, InspectTarget{ID: "t"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range in.Classes {
		names = append(names, c.Name+"/"+c.From)
	}
	if want := "a/class b/class b/class:b c/class:c"; strings.Join(names, " ") != want {
		t.Errorf("inspect classes %q, want %q", strings.Join(names, " "), want)
	}
	v1 := doc(t, `<tui version="1"><screen id="s"><text id="t" class="a b a">x</text></screen></tui>`)
	if got := v1.doc.IDs["t"].Classes; !reflect.DeepEqual(got, []string{"a", "b", "a"}) {
		t.Errorf("version=1 classes %v, want them as written", got)
	}
}

// Regression (SPEC v0.2b §6.10.2, §8.4): a user activation of a tab whose
// tabs' bind path cannot be written (a value along it is not an object)
// cannot change the active tab, so on:select never fires, from switch-to,
// move-next, or the widget keys; the row still matches and consumes its
// key. With a writable path, the same keys fire on:select once.
func TestTabActivationWithUnwritableBind(t *testing.T) {
	const src = `<tui version="2"><keymap>
<bind keys="2" action="switch-to" to="#t2"/>
<bind keys="n" action="move-next" to="#nav"/>
<bind keys="2,n" action="fallback"/>
</keymap><screen id="s" focus="#nav">
<tabs id="nav" bind="ui.view" focusable="true" on:select="sel"><tab id="t1" label="1"><text>one</text></tab><tab id="t2" label="2"><text>two</text></tab></tabs>
</screen></tui>`
	a := doc(t, src)
	bindJSON(t, a, `{"ui":5}`)
	a.Frame(20, 3)
	for _, k := range []Key{r('2'), r('2'), r('n'), named("right")} {
		if evs := a.HandleKey(k); len(evs) != 0 {
			t.Errorf("key %s with an unwritable bind path: events %s", k.Name, evText(t, evs))
		}
		if a.TakeDirty() {
			t.Errorf("key %s with an unwritable bind path marked the frame stale", k.Name)
		}
		a.Frame(20, 3)
	}
	if got := grid(a.Dump(20, 3, false)); !strings.Contains(got, "one") {
		t.Errorf("the active tab changed:\n%s", got)
	}
	b := doc(t, src)
	bindJSON(t, b, `{"ui":{}}`)
	got := evText(t, keysAt(b, 20, 3, r('2'), r('2')))
	if want := `{"action":"sel","source":"nav","keys":{},"value":"t2"}`; got != want {
		t.Errorf("writable path: events\n%s\nwant\n%s", got, want)
	}
}

// Regression (SPEC v0.2b §6.14, §8.4): a checked path that is missing
// because a value along it is present and not an object cannot be written
// without overwriting that value, so check-* does not match on the widget:
// its key reaches the next row, and <hints> shows that row. A missing path
// under an object or null is written as before.
func TestCheckActionsSkipUnwritablePath(t *testing.T) {
	const src = `<tui version="2"><keymap>
<bind keys="space" action="check-toggle" label="mark"/>
<bind keys="space" action="fallback" label="fb"/>
</keymap><screen id="s" focus="#l">
<list id="l" each="rows as r" key="r" checked="sel.marked" on:change="chg"><item><text>{r}</text></item></list>
<hints id="h"/>
</screen></tui>`
	for data, want := range map[string]string{
		`{"rows":["a"],"sel":5}`:     `{"action":"fallback","source":"l","keys":{"r":"a"},"value":null}`,
		`{"rows":["a"],"sel":"s"}`:   `{"action":"fallback","source":"l","keys":{"r":"a"},"value":null}`,
		`{"rows":["a"],"sel":[1,2]}`: `{"action":"fallback","source":"l","keys":{"r":"a"},"value":null}`,
		`{"rows":["a"],"sel":{}}`:    `{"action":"chg","source":"l","keys":{"r":"a"},"value":["a"]}`,
		`{"rows":["a"],"sel":null}`:  `{"action":"chg","source":"l","keys":{"r":"a"},"value":["a"]}`,
		`{"rows":["a"]}`:             `{"action":"chg","source":"l","keys":{"r":"a"},"value":["a"]}`,
	} {
		a := doc(t, src)
		bindJSON(t, a, data)
		f := a.Frame(30, 3)
		hint, _ := hintText(f, "h")
		wantHint := "space mark"
		if strings.Contains(want, "fallback") {
			wantHint = "space fb"
		}
		if hint != wantHint {
			t.Errorf("%s: hints %q, want %q", data, hint, wantHint)
		}
		if got := evText(t, a.HandleKey(named("space"))); got != want {
			t.Errorf("%s: events %s, want %s", data, got, want)
		}
	}
}

// canAssign agrees with assign on every shape of path.
func TestCanAssignAgreesWithAssign(t *testing.T) {
	for _, data := range []string{`null`, `{}`, `{"a":5}`, `{"a":null}`, `{"a":{"b":1}}`, `{"a":[1,{"c":2}]}`, `{"a":"s"}`, `[1]`} {
		for _, path := range []string{"", "a", "a.b", "a.b.c", "a.0", "a.1.c", "a.2", "a.x.y"} {
			var root any
			if err := json.Unmarshal([]byte(data), &root); err != nil {
				t.Fatal(err)
			}
			want := canAssign(root, path)
			_, err := assign(root, path, "v")
			if want != (err == nil) {
				t.Errorf("%s %q: canAssign %v, assign error %v", data, path, want, err)
			}
		}
	}
}
