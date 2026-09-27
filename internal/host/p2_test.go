package host

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// SPEC v0.2b ergonomics: each on containers (§6.8), multi-select (§6.14),
// :checked and :focus-within (§10.1), the built-in actions (§8.4), when
// over the focus chain (§8.1, ADR 0008), and the key dispatch of §8.6.

// evText formats events as their JSON payloads, one per line.
func evText(t *testing.T, evs []Event) string {
	t.Helper()
	var out []string
	for _, e := range evs {
		out = append(out, payload(t, e))
	}
	return strings.Join(out, "\n")
}

// keysAt renders a cols×rows frame before each key (keys dispatch on the
// live frame) and returns every event, in order.
func keysAt(a *App, cols, rows int, keys ...Key) []Event {
	var out []Event
	for _, k := range keys {
		a.Frame(cols, rows)
		out = append(out, a.HandleKey(k)...)
	}
	a.Frame(cols, rows)
	return out
}

func bindJSON(t *testing.T, a *App, data string) {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		t.Fatal(err)
	}
	if err := a.Bind("", v); err != nil {
		t.Fatal(err)
	}
}

func storeJSON(t *testing.T, a *App, path string) string {
	t.Helper()
	v, ok := a.Get(path)
	if !ok {
		return "<missing>"
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func hasDiag(f *Frame, code, substr string) bool {
	for _, d := range f.Diags {
		if d.Code == code && strings.Contains(d.Msg, substr) {
			return true
		}
	}
	return false
}

// 49. each on col/row/box: the template is every child element, inflated
// once per element in array order; top-level generated nodes carry the
// element's key (the index without key=), their descendants do not; an
// empty array gives :empty; if on the container is evaluated first,
// without the alias; nested each resolves both aliases; a non-array or a
// missing path is B001; a key path missing on an element is B003.
func TestEachOnContainers(t *testing.T) {
	a := doc(t, `<tui version="2"><style>#e:empty { bold: true; }</style><screen id="s">
<col id="c" each="xs as x"><row><text>{x.n}</text></row><text>-</text></col>
<col id="k" each="xs as x" key="x.id"><text>{x.n}</text></col>
<col id="e" each="none as x"><text>{x}</text></col>
<col id="g" if="x" each="xs as x"><text>{x.n}</text></col>
<col id="n" each="xs as x"><row each="x.ys as y"><text>{x.n}{y}</text></row></col>
<col id="bad" each="scalar as x"><text>{x}</text></col>
<col id="miss" each="nope as x"><text>{x}</text></col>
</screen></tui>`)
	bindJSON(t, a, `{"xs":[{"n":"a","id":"p","ys":[1,2]},{"n":"b","id":"q","ys":[]},{"n":"c","ys":[3]}],"none":[],"scalar":5}`)
	f := a.Frame(40, 40)

	c := f.ByID["c"]
	var keys []string
	for _, ch := range c.Children {
		keys = append(keys, ch.Tag+":"+ch.Key)
	}
	if got := strings.Join(keys, " "); got != "row:0 text:0 row:1 text:1 row:2 text:2" {
		t.Errorf("template children %q", got)
	}
	if k := c.Children[0].Children[0].Key; k != "" || c.Children[0].Children[0].Text != "a" {
		t.Errorf("a descendant of a template child carries key %q (text %q)", k, c.Children[0].Children[0].Text)
	}
	keys = nil
	for _, ch := range f.ByID["k"].Children {
		keys = append(keys, ch.Key+"="+ch.Text)
	}
	if got := strings.Join(keys, " "); got != "p=a q=b 2=c" {
		t.Errorf("keyed children %q", got)
	}
	if !hasDiag(f, "B003", `key="x.id" is missing on element 2`) {
		t.Errorf("no B003 for the missing key: %v", f.Diags)
	}
	if e := f.ByID["e"]; len(e.Children) != 0 || !e.HasPseudo("empty") || !e.Style.Bold {
		t.Errorf("empty array: children %d, bold %v", len(e.Children), e.Style.Bold)
	}
	if f.ByID["g"] != nil || !hasDiag(f, "B002", `path "x" is missing`) {
		t.Errorf("if on the container must be evaluated without the alias: %v", f.Diags)
	}
	var texts []string
	for _, row := range f.ByID["n"].Children {
		for _, txt := range row.Children {
			texts = append(texts, row.Key+":"+txt.Text)
		}
	}
	if got := strings.Join(texts, " "); got != "0:a1 0:a2 2:c3" {
		t.Errorf("nested each %q", got)
	}
	for _, id := range []string{"bad", "miss"} {
		if len(f.ByID[id].Children) != 0 {
			t.Errorf("#%s has children", id)
		}
	}
	if !hasDiag(f, "B001", `"scalar" is a number, not an array`) || !hasDiag(f, "B001", `path "nope" is missing`) {
		t.Errorf("B001: %v", f.Diags)
	}
	for _, d := range f.Diags {
		if d.Code == "B001" && d.Severity != "error" {
			t.Errorf("B001 is an error: %v", d)
		}
	}
	// The dump carries key on the top-level generated nodes only.
	d := f.Dump(false)
	n := 0
	for _, dn := range d.Nodes {
		if dn.Key != "" {
			n++
		}
	}
	// c: 6, k: 3, n: 3 rows and the 3 texts its inner each generated.
	if n != 15 {
		t.Errorf("%d dump nodes carry a key, want 15", n)
	}
}

// Nothing in a container template takes focus, even in a document with
// errors (a V001 button inside the template).
func TestEachTemplateNeverFocused(t *testing.T) {
	a := doc(t, `<tui version="2"><screen id="s"><col each="xs as x"><button id="b" label="{x}"/></col><button id="ok" label="ok"/></screen></tui>`)
	bindJSON(t, a, `{"xs":[1,2]}`)
	f := a.Frame(20, 5)
	if len(f.Focusables) != 1 || f.Focusables[0].ID != "ok" || f.Focus != "ok" {
		t.Errorf("focusables %v focus %q", f.Focusables, f.Focus)
	}
}

const msDoc = `<tui version="2">
<style>#l > item:checked { bold: true; }</style>
<keymap>
  <bind keys="space" action="check-toggle" when="#l:focus"/>
  <bind keys="ctrl+a" action="check-all"/>
  <bind keys="ctrl+d" action="check-none" to="#l"/>
  <bind keys="ctrl+a" action="all_fallback"/>
  <bind keys="ctrl+d" action="none_fallback"/>
  <bind keys="space" action="space_fallback"/>
</keymap>
<screen id="s" focus="#l">
  <list id="l" each="rows as r" key="r.id" checked="marked" mark="✓" on:change="marks" on:select="sel">
    <item><text>{r.name}</text></item>
  </list>
  <button id="b" label="b"/>
  <input id="q"/>
</screen></tui>`

const msRows = `"rows":[{"id":"a","name":"alpha"},{"id":2,"name":"two"},{"id":"c","name":"gamma"}]`

// 64. check-toggle, check-all, and check-none as §6.14: duplicates removed
// by check-toggle, numeric keys kept as numbers, keys of vanished rows
// kept by check-all, no on:change when nothing changed (the row still
// consumes its key), and the on:change payload.
func TestCheckActions(t *testing.T) {
	a := doc(t, msDoc)
	bindJSON(t, a, `{`+msRows+`,"marked":["a","a",2]}`)
	sp := Key{Name: "space", Rune: ' '}
	got := evText(t, keysAt(a, 30, 6, sp))
	if want := `{"action":"marks","source":"l","keys":{"r":"a"},"value":[2]}`; got != want {
		t.Errorf("toggle off:\n got %s\nwant %s", got, want)
	}
	if s := storeJSON(t, a, "marked"); s != `[2]` {
		t.Errorf("marked %s", s)
	}
	got = evText(t, keysAt(a, 30, 6, named("down"), sp))
	if want := `{"action":"sel","source":"l","keys":{"r":2},"value":null}` + "\n" + `{"action":"marks","source":"l","keys":{"r":2},"value":[]}`; got != want {
		t.Errorf("toggle a numeric key off:\n got %s\nwant %s", got, want)
	}
	keysAt(a, 30, 6, sp)
	if s := storeJSON(t, a, "marked"); s != `[2]` {
		t.Errorf("toggle on keeps the key a number: %s", s)
	}
	// A string "2" is not the numeric key 2.
	bindJSON(t, a, `{`+msRows+`,"marked":["2","gone"]}`)
	f := a.Frame(30, 6)
	if f.ByID["l"].Children[1].Checked {
		t.Error(`the row keyed 2 is checked by "2"`)
	}
	got = evText(t, keysAt(a, 30, 6, Key{Name: "ctrl+a"}))
	if want := `{"action":"marks","source":"l","keys":{"r":2},"value":["2","gone","a",2,"c"]}`; got != want {
		t.Errorf("check-all:\n got %s\nwant %s", got, want)
	}
	if got := keysAt(a, 30, 6, Key{Name: "ctrl+a"}); len(got) != 0 {
		t.Errorf("check-all with nothing to add fired %s (and must consume ctrl+a)", evText(t, got))
	}
	got = evText(t, keysAt(a, 30, 6, Key{Name: "ctrl+d"}))
	if want := `{"action":"marks","source":"l","keys":{"r":2},"value":[]}`; got != want {
		t.Errorf("check-none:\n got %s\nwant %s", got, want)
	}
	if got := keysAt(a, 30, 6, Key{Name: "ctrl+d"}); len(got) != 0 {
		t.Errorf("check-none on [] fired %s", evText(t, got))
	}
	// A missing path counts as [] (B003) and is created by the first check.
	bindJSON(t, a, `{`+msRows+`}`)
	if f := a.Frame(30, 6); !hasDiag(f, "B003", `"marked"`) {
		t.Errorf("no B003 for a missing checked path: %v", f.Diags)
	}
	keysAt(a, 30, 6, sp)
	if s := storeJSON(t, a, "marked"); s != `[2]` {
		t.Errorf("marked %s", s)
	}
}

// 64. A checked value that is not an array is B008 (an error): it shows
// nothing checked, and the check-* rows do not match, so their keys reach
// the next rows and the array is never overwritten.
func TestCheckedNotAnArray(t *testing.T) {
	a := doc(t, msDoc)
	bindJSON(t, a, `{`+msRows+`,"marked":"a"}`)
	f := a.Frame(30, 6)
	if !hasDiag(f, "B008", `"marked" is a string`) {
		t.Fatalf("no B008: %v", f.Diags)
	}
	for _, d := range f.Diags {
		if d.Code == "B008" && d.Severity != "error" {
			t.Errorf("B008 is an error: %v", d)
		}
	}
	for _, it := range f.ByID["l"].Children {
		if it.Checked {
			t.Error("a row is checked by a non-array")
		}
	}
	got := evText(t, keysAt(a, 30, 6, Key{Name: "space", Rune: ' '}, Key{Name: "ctrl+a"}, Key{Name: "ctrl+d"}))
	want := strings.Join([]string{
		`{"action":"space_fallback","source":"l","keys":{"r":"a"},"value":null}`,
		`{"action":"all_fallback","source":"l","keys":{"r":"a"},"value":null}`,
		`{"action":"none_fallback","source":"l","keys":{"r":"a"},"value":null}`,
	}, "\n")
	if got != want {
		t.Errorf("events:\n got %s\nwant %s", got, want)
	}
	if s := storeJSON(t, a, "marked"); s != `"a"` {
		t.Errorf("the runtime overwrote a B008 value: %s", s)
	}
}

// §8.4: a check-all row without when does not take ctrl+a away from a
// focused widget without checked (the next row fires), nor from an input,
// which consumes ctrl+a itself (§8.6 step 2).
func TestCheckAllWithoutWhen(t *testing.T) {
	a := doc(t, msDoc)
	bindJSON(t, a, `{`+msRows+`,"marked":[]}`)
	_ = a.Set("@focus", "#b")
	got := evText(t, keysAt(a, 30, 6, Key{Name: "ctrl+a"}))
	if want := `{"action":"all_fallback","source":"b","keys":{},"value":null}`; got != want {
		t.Errorf("button focused:\n got %s\nwant %s", got, want)
	}
	_ = a.Set("@focus", "#q")
	if got := keysAt(a, 30, 6, Key{Name: "ctrl+a"}); len(got) != 0 {
		t.Errorf("input focused: %s", evText(t, got))
	}
	if s := storeJSON(t, a, "marked"); s != `[]` {
		t.Errorf("marked %s", s)
	}
}

// 64, 59. Checked rows match :checked (a +10 pseudo-class), dump
// "checked": true, and end their text line with " checked" after
// " selected"; the mark channel shifts every item's content, checked or
// not, and grows the list's intrinsic width; a checked row paints its mark
// in the channel's first columns, and a reverse selection covers it.
func TestMarkChannelAndChecked(t *testing.T) {
	src := `<tui version="2"><style>#s { align: start; } #l { width: auto; } #l > item { bold: false; } #l > item:checked { bold: true; }</style><screen id="s" focus="#l">
<list id="l" each="rows as r" key="r" bind="cur" checked="m" MARK><item pad="0 1"><text>{r}</text></item></list></screen></tui>`
	a := doc(t, strings.Replace(src, "MARK", `mark="✓"`, 1))
	bindJSON(t, a, `{"rows":["ab","c"],"m":["c"],"cur":"c"}`)
	f := a.Frame(10, 3)
	l := f.ByID["l"]
	if l.W != 6 {
		t.Errorf("list width %d, want 6 (text 2 + padding 2 + channel 2)", l.W)
	}
	for i, it := range l.Children {
		if it.Content.X != it.X+3 || it.Content.W != 2 {
			t.Errorf("row %d content %+v at x %d", i, it.Content, it.X)
		}
	}
	if !l.Children[1].Checked || l.Children[0].Checked || !l.Children[1].Style.Bold || l.Children[0].Style.Bold {
		t.Errorf(":checked: %v/%v bold %v/%v", l.Children[0].Checked, l.Children[1].Checked, l.Children[0].Style.Bold, l.Children[1].Style.Bold)
	}
	d := f.Dump(false)
	if d.Grid[0] != "   ab     " || d.Grid[1] != " ✓ c      " {
		t.Errorf("grid %q", d.Grid)
	}
	if c := f.Grid.At(1, 1); c.Attrs&paint.Reverse == 0 || c.Attrs&paint.Bold == 0 {
		t.Errorf("the mark of the selected checked row: attrs %b", c.Attrs)
	}
	var lines []string
	for _, n := range d.Nodes {
		if n.Tag == "item" {
			lines = append(lines, dump.NodeLine(n, 1, 4))
		}
	}
	if got := strings.Join(lines, "\n"); got != "-  item  6x1 @(0,0)\n-  item  6x1 @(0,1) selected checked" {
		t.Errorf("text node lines:\n%s", got)
	}
	js, _ := json.Marshal(d.Nodes)
	if !strings.Contains(string(js), `"key":"c","selected":true,"checked":true}`) {
		t.Errorf("dump nodes %s", js)
	}
	// Without mark there is no channel.
	b := doc(t, strings.Replace(src, "MARK", "", 1))
	bindJSON(t, b, `{"rows":["ab","c"],"m":["c"],"cur":"c"}`)
	if l := b.Frame(10, 3).ByID["l"]; l.W != 4 || l.Children[0].Chan != 0 {
		t.Errorf("list without mark: width %d chan %d", l.W, l.Children[0].Chan)
	}
}

const mvDoc = `<tui version="2">
<style>#hidden { display: none; }</style>
<keymap>
  <bind keys="j" action="move-next"/>
  <bind keys="k" action="move-prev"/>
  <bind keys="g" action="move-first" to="#l"/>
  <bind keys="G" action="move-last" to="#l"/>
  <bind keys="d" action="move-page-down" to="#l"/>
  <bind keys="u" action="move-page-up" to="#l"/>
  <bind keys="j" action="j_fallback"/>
  <bind keys="n" action="move-next" to="#v"/>
  <bind keys="p" action="move-prev" to="#v"/>
  <bind keys="e" action="move-last" to="#v"/>
  <bind keys="f" action="move-page-down" to="#v"/>
  <bind keys="h" action="move-next" to="#hidden"/>
  <bind keys="h" action="h_fallback"/>
  <bind keys="x" action="move-next" to="#off"/>
  <bind keys="x" action="x_fallback"/>
  <bind keys="z" action="move-next" to="#empty"/>
  <bind keys="z" action="z_fallback"/>
  <bind keys="2" action="switch-to" to="#other"/>
  <bind keys="3" action="switch-to" to="#l"/>
  <bind keys="3" action="three_fallback"/>
</keymap>
<screen id="s" focus="#b">
  <button id="b" label="b"/>
  <list id="l" each="rows as r" key="r" bind="cur" on:select="sel" height="3"><item><text>{r}</text></item></list>
  <scroll id="v" height="2"><text>1</text><text>2</text><text>3</text><text>4</text><text>5</text></scroll>
  <list id="hidden" each="rows as r" key="r"><item><text>{r}</text></item></list>
  <list id="off" each="rows as r" key="r" disabled="true" height="2"><item><text>{r}</text></item></list>
  <list id="empty" each="none as r" key="r"><item><text>{r}</text></item></list>
</screen>
<screen id="other"><text>o</text></screen>
</tui>`

func mvApp(t *testing.T) *App {
	a := doc(t, mvDoc)
	bindJSON(t, a, `{"rows":["r0","r1","r2","r3","r4","r5","r6","r7","r8","r9"],"none":[],"cur":null}`)
	return a
}

func sel(key string) string {
	return `{"action":"sel","source":"l","keys":{"r":"` + key + `"},"value":null}`
}

// 62. move-* on a list: the target is to=, else the focused node; the
// cursor moves as a user move (bind written, on:select when it changed),
// clamped; a matching row consumes its key even when nothing moves; a row
// whose target is incompatible (a button) does not match, and the key
// reaches the next row.
func TestMoveActionsOnList(t *testing.T) {
	a := mvApp(t)
	got := evText(t, keysAt(a, 40, 20, r('j')))
	if want := `{"action":"j_fallback","source":"b","keys":{},"value":null}`; got != want {
		t.Errorf("move-next on a button:\n got %s\nwant %s", got, want)
	}
	// to= moves the list while the button keeps focus.
	got = evText(t, keysAt(a, 40, 20, r('G'), r('d'), r('u'), r('g'), r('g')))
	want := strings.Join([]string{sel("r9"), sel("r6"), sel("r0")}, "\n")
	if got != want {
		t.Errorf("move-last/page/first:\n got %s\nwant %s", got, want)
	}
	if a.Focus() != "b" {
		t.Errorf("focus moved to %q", a.Focus())
	}
	_ = a.Set("@focus", "#l")
	got = evText(t, keysAt(a, 40, 20, r('j'), r('j'), r('k'), r('d')))
	want = strings.Join([]string{sel("r1"), sel("r2"), sel("r1"), sel("r4")}, "\n")
	if got != want {
		t.Errorf("move-next/prev on the focused list:\n got %s\nwant %s", got, want)
	}
	if s := storeJSON(t, a, "cur"); s != `"r4"` {
		t.Errorf("bind %s", s)
	}
	// At the last row move-next consumes j silently: j_fallback never fires.
	if got := keysAt(a, 40, 20, r('G'), r('j'), r('j')); evText(t, got) != sel("r9") {
		t.Errorf("at the end: %s", evText(t, got))
	}
	f := a.Frame(40, 20)
	l := f.ByID["l"]
	if l.ScrollY != 7 || !l.Children[9].Selected {
		t.Errorf("the list follows its cursor: offset %d", l.ScrollY)
	}
}

// 62. A row whose target is missing from the frame (display: none),
// disabled, or incompatible (an empty list) does not match: the key goes
// on to the next row, as for a row whose when fails.
func TestBuiltinTargetsThatDoNotMatch(t *testing.T) {
	a := mvApp(t)
	got := evText(t, keysAt(a, 40, 20, r('h'), r('x'), r('z'), r('3')))
	want := strings.Join([]string{
		`{"action":"h_fallback","source":"b","keys":{},"value":null}`,
		`{"action":"x_fallback","source":"b","keys":{},"value":null}`,
		`{"action":"z_fallback","source":"b","keys":{},"value":null}`,
		`{"action":"three_fallback","source":"b","keys":{},"value":null}`,
	}, "\n")
	if got != want {
		t.Errorf("events:\n got %s\nwant %s", got, want)
	}
}

// 62. move-* on a viewport moves its offset on its scroll axis by one, to
// its maximum, or by its content-box size, clamped; switch-to a screen
// switches screens as Set("@screen") does.
func TestMoveViewportAndSwitchTo(t *testing.T) {
	a := mvApp(t)
	offset := func() int { return a.Frame(40, 20).ByID["v"].ScrollY }
	if evs := keysAt(a, 40, 20, r('n')); len(evs) != 0 || offset() != 1 {
		t.Errorf("move-next: %v offset %d", evs, offset())
	}
	keysAt(a, 40, 20, r('p'), r('p'))
	if offset() != 0 {
		t.Errorf("move-prev clamps at 0: %d", offset())
	}
	keysAt(a, 40, 20, r('e'))
	if offset() != 3 {
		t.Errorf("move-last: %d, want 3 (5 rows in 2)", offset())
	}
	keysAt(a, 40, 20, r('p'), r('f'))
	if offset() != 3 {
		t.Errorf("move-prev then move-page-down: %d", offset())
	}
	keysAt(a, 40, 20, r('2'))
	if f := a.Frame(40, 20); f.Root == nil || f.Root.ID != "other" {
		t.Errorf("switch-to did not switch screens")
	}
}

// 65. The same keymap in a version="1" and a version="2" document: a row
// whose when names a container fires only in version="2"; when="#kill"
// fires only while focus is inside that modal; with nothing focused a row
// whose when names the screen fires in version="2" only; a host action
// fired by a row whose when matched an ancestor has the focused node as
// its source (§8.2).
func TestWhenOverFocusChain(t *testing.T) {
	body := `<keymap>
  <bind keys="x" action="in_pane" when="#pane"/>
  <bind keys="y" action="in_kill" when="#kill"/>
  <bind keys="z" action="on_screen" when="#main"/>
  <bind keys="w" action="pane_child" when="#pane > list"/>
</keymap>
<screen id="main" focus="#l">
  <col id="pane"><list id="l" each="rows as r" key="r"><item><text>{r}</text></item></list></col>
  <button id="other" label="o"/>
  <modal id="kill" open="killing"><button id="yes" label="yes"/></modal>
</screen></tui>`
	run := func(version string) (pane, kill, killOpen string) {
		a := doc(t, `<tui version="`+version+`">`+body)
		bindJSON(t, a, `{"rows":["a","b"],"killing":false}`)
		pane = evText(t, keysAt(a, 30, 10, r('x'), r('y'), r('w')))
		_ = a.Set("killing", true)
		a.Frame(30, 10)
		kill = evText(t, keysAt(a, 30, 10, r('x'), r('y')))
		_ = a.Set("killing", false)
		a.Frame(30, 10)
		killOpen = a.Focus()
		return
	}
	pane, kill, _ := run("2")
	want := `{"action":"in_pane","source":"l","keys":{"r":"a"},"value":null}` + "\n" +
		`{"action":"pane_child","source":"l","keys":{"r":"a"},"value":null}`
	if pane != want {
		t.Errorf("version=2, focus in #pane:\n got %s\nwant %s", pane, want)
	}
	if want := `{"action":"in_kill","source":"yes","keys":{},"value":null}`; kill != want {
		t.Errorf("version=2, focus in #kill:\n got %s\nwant %s", kill, want)
	}
	pane, kill, _ = run("1")
	if want := `{"action":"pane_child","source":"l","keys":{"r":"a"},"value":null}`; pane != want || kill != "" {
		t.Errorf("version=1: pane %s kill %s", pane, kill)
	}
	// Nothing focused: the chain is the top modal, if any, then the screen.
	for _, c := range []struct{ version, want string }{
		{"2", `{"action":"on_screen","source":"","keys":{},"value":null}`},
		{"1", ""},
	} {
		a := doc(t, `<tui version="`+c.version+`"><keymap><bind keys="z" action="on_screen" when="#main"/></keymap><screen id="main"><text>t</text></screen></tui>`)
		if got := evText(t, keysAt(a, 10, 2, r('z'))); got != c.want {
			t.Errorf("version=%s, nothing focused: %s", c.version, got)
		}
	}
}

// 59, 65. :focus-within matches every node of the focus chain, through a
// modal, and nothing while nothing is focused; it counts +10; in when it
// follows the chain too.
func TestFocusWithin(t *testing.T) {
	a := doc(t, `<tui version="2"><style>
#pane:focus-within { bold: true; } #pane { bold: false; }
screen:focus-within { italic: true; }
</style>
<keymap><bind keys="x" action="within" when="#pane:focus-within"/></keymap>
<screen id="main" focus="#q">
  <col id="pane"><input id="q"/></col>
  <button id="other" label="o"/>
  <modal id="m" open="open"><button id="yes" label="yes"/></modal>
</screen></tui>`)
	bindJSON(t, a, `{"open":false}`)
	f := a.Frame(30, 8)
	if !f.ByID["pane"].Style.Bold || !f.ByID["main"].Style.Italic || f.ByID["other"].FocusWithin {
		t.Errorf("focus in #pane: pane bold %v, screen italic %v", f.ByID["pane"].Style.Bold, f.ByID["main"].Style.Italic)
	}
	_ = a.Set("@focus", "#other")
	f = a.Frame(30, 8)
	if f.ByID["pane"].Style.Bold || !f.ByID["other"].FocusWithin {
		t.Error("focus on #other: #pane still matches :focus-within")
	}
	if evs := keysAt(a, 30, 8, r('x')); len(evs) != 0 {
		t.Errorf("when=#pane:focus-within fired outside the pane: %s", evText(t, evs))
	}
	_ = a.Set("@focus", "#q")
	if got := evText(t, keysAt(a, 30, 8, Key{Name: "ctrl+x"}, r('x'))); got != "" {
		// (x is typed into the input.)
		t.Errorf("typed x fired %s", got)
	}
	_ = a.Set("open", true)
	f = a.Frame(30, 8)
	if f.Focus != "yes" || !f.ByID["main"].FocusWithin || !f.ByID["main"].Style.Italic || !f.Modals[0].FocusWithin || f.ByID["pane"].FocusWithin {
		t.Errorf("focus in the modal: focus %q", f.Focus)
	}
	b := doc(t, `<tui version="2"><style>screen:focus-within { italic: true; }</style><screen id="main"><text>t</text></screen></tui>`)
	if f := b.Frame(10, 2); f.ByID["main"].Style.Italic || f.ByID["main"].FocusWithin {
		t.Error(":focus-within matched with nothing focused")
	}
}

// 63. class:NAME guards per list row, in the row's scope.
func TestClassGuardsPerListRow(t *testing.T) {
	a := doc(t, `<tui version="2"><screen id="s"><list id="l" each="rows as r" key="r.id"><item class="row" class:hot="r.hot" class:row="r.hot"><text class:x="!r.hot">{r.id}</text></item></list></screen></tui>`)
	bindJSON(t, a, `{"rows":[{"id":"a","hot":true},{"id":"b","hot":false}]}`)
	f := a.Frame(10, 3)
	rows := f.ByID["l"].Children
	if got := strings.Join(rows[0].Classes, " "); got != "row hot" {
		t.Errorf("row a classes %q", got)
	}
	if got := strings.Join(rows[1].Classes, " "); got != "row" {
		t.Errorf("row b classes %q", got)
	}
	if !rows[1].Children[0].HasClass("x") || rows[0].Children[0].HasClass("x") {
		t.Error("the text guard is not evaluated per row")
	}
}

// §8.6: planKey decides which step takes a key without side effects;
// handleKey applies it.
func TestPlanKeySteps(t *testing.T) {
	a := doc(t, `<tui version="2"><keymap>
<bind keys="space" action="check-toggle" when="#l:focus"/>
<bind keys="q" action="quit"/>
<bind keys="esc" action="esc_row"/>
</keymap><screen id="s" focus="#l">
<list id="l" each="rows as r" key="r" checked="m" on:change="c"><item><text>{r}</text></item></list>
<modal id="m" open="open" on:escape="close"><button id="ok" label="ok"/></modal>
</screen></tui>`)
	bindJSON(t, a, `{"rows":["a","b"],"m":[],"open":false}`)
	a.Frame(20, 6)
	a.mu.Lock()
	v := a.liveView()
	for _, c := range []struct {
		k    Key
		step keyStep
		row  int
	}{
		{Key{Name: "space", Rune: ' '}, stepKeymap, 0},
		{r('q'), stepKeymap, 1},
		{named("esc"), stepKeymap, 2},
		{named("down"), stepWidget, 0},
		{named("tab"), stepBuiltinKey, 0},
		{named("ctrl+c"), stepBuiltinKey, 0},
		{r('?'), stepNone, 0},
	} {
		p := a.planKey(v, c.k)
		if p.step != c.step || (c.step == stepKeymap && p.row != c.row) {
			t.Errorf("%s: step %d row %d, want %d %d", c.k.Name, p.step, p.row, c.step, c.row)
		}
	}
	idx := a.lists["l"].index
	a.mu.Unlock()
	if s := storeJSON(t, a, "m"); s != `[]` || idx != 0 {
		t.Fatalf("planKey changed state: m %s index %d", s, idx)
	}
	_ = a.Set("open", true)
	a.Frame(20, 6)
	a.mu.Lock()
	if p := a.planKey(a.liveView(), named("esc")); p.step != stepEscape {
		t.Errorf("esc with a modal that has on:escape: step %d", p.step)
	}
	a.mu.Unlock()
}

// A built-in that changes the frame without an event marks it stale
// (TakeDirty), so the loop redraws before the next key; Frame clears it.
// Host actions and version="1" key handling never set it.
func TestBuiltinMarksFrameStale(t *testing.T) {
	a := mvApp(t)
	a.Frame(40, 20)
	a.HandleKey(r('n'))
	if !a.TakeDirty() || a.TakeDirty() {
		t.Error("move-next on a viewport: TakeDirty")
	}
	a.HandleKey(r('n'))
	a.Frame(40, 20)
	if a.TakeDirty() {
		t.Error("Frame did not clear the flag")
	}
	a.HandleKey(r('j')) // j_fallback: a host action
	if a.TakeDirty() {
		t.Error("a host action set the flag")
	}
	b := doc(t, `<tui version="1"><screen id="s" focus="#l"><list id="l" each="rows as r" key="r"><item><text>{r}</text></item></list></screen></tui>`)
	bindJSON(t, b, `{"rows":["a","b"]}`)
	b.Frame(10, 3)
	b.HandleKey(named("down"))
	if b.TakeDirty() {
		t.Error("a version=1 list move set the flag")
	}
}

// Run's loop dispatches the built-ins through the same procedure as play:
// keys in one read see the frame a built-in changed (switch-to then a key
// whose when names the new screen).
func TestLoopBuiltinsInOneRead(t *testing.T) {
	a := doc(t, `<tui version="2"><keymap>
<bind keys="j" action="move-next"/>
<bind keys="space" action="check-toggle"/>
<bind keys="2" action="switch-to" to="#two"/>
<bind keys="x" action="on_one" when="#one"/>
<bind keys="x" action="on_two" when="#two"/>
</keymap>
<screen id="one" focus="#l"><list id="l" each="rows as r" key="r" checked="m" on:select="sel" on:change="marks"><item><text>{r}</text></item></list></screen>
<screen id="two"><text>two</text></screen></tui>`)
	bindJSON(t, a, `{"rows":["a","b"],"m":[]}`)
	evs := make(chan string, 8)
	for _, act := range []string{"sel", "marks", "on_one", "on_two"} {
		a.On(act, func(ev Event) error {
			b, _ := json.Marshal(ev)
			evs <- string(b)
			return nil
		})
	}
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- a.Loop(pr, io.Discard, func() (int, int) { return 20, 4 }, nil) }()
	if _, err := io.WriteString(pw, "j 2x"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`{"action":"sel","source":"l","keys":{"r":"b"},"value":null}`,
		`{"action":"marks","source":"l","keys":{"r":"b"},"value":["b"]}`,
		`{"action":"on_two","source":"","keys":{},"value":null}`,
	}
	var got []string
	for len(got) < len(want) {
		select {
		case e := <-evs:
			got = append(got, e)
		case <-time.After(3 * time.Second):
			t.Fatalf("got %v", got)
		}
	}
	pw.Close()
	<-done
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events:\n got %v\nwant %v", got, want)
	}
}

// The live view's frame membership: nodes dropped by display: none are
// not in the frame although the frame indexes them by id.
func TestKeyViewInFrame(t *testing.T) {
	a := mvApp(t)
	a.Frame(40, 20)
	a.mu.Lock()
	defer a.mu.Unlock()
	v := a.liveView()
	for id, want := range map[string]bool{"l": true, "v": true, "hidden": false, "b": true} {
		if b := v.byID[id]; b == nil || v.inFrame(b) != want {
			t.Errorf("#%s in frame: want %v", id, want)
		}
	}
	var none *layout.Box
	if v.inFrame(none) {
		t.Error("nil in frame")
	}
}
