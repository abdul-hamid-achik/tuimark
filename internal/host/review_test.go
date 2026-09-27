package host

import (
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

func v010s(ds ir.Diags) []string {
	var out []string
	for _, d := range ds {
		if d.Code == "V010" {
			out = append(out, d.String())
		}
	}
	return out
}

// Finding 1: V010 is a parse-pass code, so Validate reports it for every
// node of the document without data: other screens, false if= branches,
// closed modals, and empty list templates, from style="" and from rules.
func TestValidateReportsV010Statically(t *testing.T) {
	cases := []struct {
		name, src string
		want      int
	}{
		{"second screen", `<tui version="1">
  <screen id="main"><text>x</text></screen>
  <screen id="settings">
    <box style="dock: top; height: 1fr"><text>a</text></box>
  </screen>
</tui>`, 1},
		{"false if and closed modal", `<tui version="1">
  <screen id="main">
    <text>x</text>
    <box if="show" style="dock: top; height: 1fr"><text>a</text></box>
    <modal id="m" open="dlg"><box style="dock: left; width: 2fr"><text>b</text></box></modal>
  </screen>
</tui>`, 2},
		{"empty each template", `<tui version="1">
  <screen id="main"><list id="l" each="rows as r"><item><box style="dock: top; height: 1fr"><text>{r}</text></box></item></list></screen>
</tui>`, 1},
		{"author rule and media rule", `<tui version="1">
  <style>
    .strip { dock: bottom; }
    #s2 > .strip { height: 2fr; }
    @media (max-cols: 40) { #side { dock: left; width: 1fr; } }
  </style>
  <screen id="main"><text>x</text></screen>
  <screen id="s2"><box class="strip"/><row><box id="side"/></row></screen>
</tui>`, 2},
		{"flex off the dock axis and state rules are not V010", `<tui version="1">
  <style>#f:focus { height: 1fr; }</style>
  <screen id="main"><text>x</text></screen>
  <screen id="s2">
    <box style="dock: left; width: 5; flex: 1"/>
    <row><box style="dock: top; height: 1; flex: 1"/></row>
    <input id="f" style="dock: top"/>
    <modal id="m" open="false" style="dock: top; height: 1fr"><text>m</text></modal>
  </screen>
</tui>`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := doc(t, c.src)
			got := v010s(a.Validate())
			if len(got) != c.want {
				t.Fatalf("V010 = %d, want %d: %v", len(got), c.want, got)
			}
			// A second Validate is identical: nothing leaks between calls.
			if again := v010s(a.Validate()); strings.Join(again, "|") != strings.Join(got, "|") {
				t.Errorf("second Validate: %v", again)
			}
		})
	}
	// Rendered and static reports of one node are deduplicated.
	a := doc(t, v1(`<box style="dock: top; height: 1fr"><text>a</text></box>`))
	if got := v010s(a.Validate()); len(got) != 1 {
		t.Errorf("active screen: %v", got)
	}
}

// Finding 7: display: none removes a modal (and a screen root) like any
// other node: not laid out, not painted, no focus trap, no on:open.
func TestDisplayNoneOnModalAndRoot(t *testing.T) {
	for _, css := range []string{
		`#m { display: none; }`,
		`modal { display: none; }`,
		`@media (max-cols: 40) { #m { display: none; } }`,
	} {
		a := doc(t, `<tui version="1"><style>`+css+`</style><screen id="s"><box id="a"><button id="b1">one</button></box><modal id="m" open="true" title="M" on:open="opened"><button id="b2">two</button></modal></screen></tui>`)
		f := a.Frame(20, 6)
		if len(f.Modals) != 0 || strings.Contains(strings.Join(f.Grid.Lines(), "\n"), "two") {
			t.Errorf("%s: modal still rendered:\n%s", css, strings.Join(f.Grid.Lines(), "\n"))
		}
		if a.Focus() != "b1" {
			t.Errorf("%s: focus = %q, want b1 (no trap)", css, a.Focus())
		}
		for _, ev := range a.TakePending() {
			if ev.Action == "opened" {
				t.Errorf("%s: on:open fired for a display:none modal", css)
			}
		}
		if _, ok := nodeByID(a.Dump(20, 6, false), "m"); ok {
			t.Errorf("%s: dump lists the modal", css)
		}
	}
	// The media rule only applies at its size.
	a := doc(t, `<tui version="1"><style>@media (max-cols: 40) { #m { display: none; } }</style><screen id="s"><button id="b1">one</button><modal id="m" open="true"><button id="b2">two</button></modal></screen></tui>`)
	if f := a.Frame(60, 10); len(f.Modals) != 1 || a.Focus() != "b2" {
		t.Errorf("60 cols: modals %d focus %q", len(f.Modals), a.Focus())
	}
	// A display:none screen renders an empty frame.
	r := doc(t, `<tui version="1"><style>screen { display: none; }</style><screen id="s"><button id="b1">one</button><modal id="m" open="true"><text>two</text></modal></screen></tui>`)
	f := r.Frame(20, 4)
	if g := strings.Join(f.Grid.Lines(), ""); strings.TrimSpace(g) != "" || len(f.Modals) != 0 || r.Focus() != "" {
		t.Errorf("display:none root: focus %q modals %d grid %q", r.Focus(), len(f.Modals), g)
	}
}

// Finding 8: a modal is still its screen's child for selector matching.
func TestChildCombinatorMatchesModal(t *testing.T) {
	for _, css := range []string{
		`modal { width: 10; height: 3; }`,
		`screen > modal { width: 10; height: 3; }`,
		`#s > #m { width: 10; height: 3; }`,
	} {
		a := doc(t, `<tui version="1"><style>`+css+` screen > modal > text { width: 2; }</style><screen id="s"><box id="a"><text>x</text></box><modal id="m" open="true"><text id="t">two</text></modal></screen></tui>`)
		d := a.Dump(20, 6, false)
		m, _ := nodeByID(d, "m")
		tx, _ := nodeByID(d, "t")
		if m.W != 10 || m.H != 3 || tx.W != 2 {
			t.Errorf("%s: m %dx%d t %d wide", css, m.W, m.H, tx.W)
		}
	}
}

// Finding 23: a focus request to a node that cannot take focus is a no-op:
// focus stays, and no on:focus fires, neither the target's nor another's.
func TestFocusRequestToUnfocusableIsNoop(t *testing.T) {
	src := `<tui version="1"><keymap>
  <bind keys="/" action="focus" to="#q"/>
  <bind keys="d" action="focus" to="#detail"/>
  <bind keys="h" action="focus" to="#hid"/>
  <bind keys="f" action="focus" to="#first"/>
</keymap>
<screen id="s" focus="#l">
  <input id="first" bind="x" on:focus="firstFocused"/>
  <input id="q" bind="x" disabled="true" on:focus="qFocused"/>
  <input id="hid" bind="x" hidden="true" on:focus="hidFocused"/>
  <list id="l" on:focus="lFocused"><item id="i1"><text>1</text></item></list>
  <text id="detail" on:focus="detailFocused">detail</text>
</screen></tui>`
	a := doc(t, src)
	_ = a.Bind("x", "")
	a.Frame(80, 24)
	a.TakePending()
	for _, k := range []rune{'/', 'd', 'h'} {
		evs := press(a, r(k))
		if a.Focus() != "l" || len(evs) != 0 {
			t.Errorf("%c: focus %q events %+v", k, a.Focus(), evs)
		}
		if p := a.TakePending(); len(p) != 0 {
			t.Errorf("%c: pending %+v", k, p)
		}
	}
	for _, id := range []string{"#q", "#detail", "#hid"} {
		if err := a.Set("@focus", id); err != nil {
			t.Fatal(err)
		}
		a.Frame(80, 24)
		if a.Focus() != "l" {
			t.Errorf("Set(@focus, %s): focus %q", id, a.Focus())
		}
		if p := a.TakePending(); len(p) != 0 {
			t.Errorf("Set(@focus, %s): pending %+v", id, p)
		}
	}
	// A focusable target still takes focus and fires on:focus once.
	evs := press(a, r('f'))
	if a.Focus() != "first" || len(evs) != 1 || evs[0].Action != "firstFocused" {
		t.Errorf("f: focus %q events %+v", a.Focus(), evs)
	}
	if p := a.TakePending(); len(p) != 0 {
		t.Errorf("f: on:focus fired twice: %+v", p)
	}
	// A target that becomes focusable in the same batch (a modal opened by
	// a handler) is honored by the next frame, and its on:focus fires then.
	b := doc(t, `<tui version="1"><keymap><bind keys="ctrl+g" action="focus" to="#m2"/></keymap>
<screen id="s"><input id="a"/><modal id="m" bind="dlg"><button id="m1">x</button><button id="m2" on:focus="m2Focused">y</button></modal></screen></tui>`)
	_ = b.Bind("dlg", false)
	b.Frame(40, 10)
	_ = b.Set("dlg", true)
	if evs := b.HandleKey(named("ctrl+g")); len(evs) != 0 { // the last frame has no modal yet
		t.Errorf("g: events before the frame %+v", evs)
	}
	b.Frame(40, 10)
	if b.Focus() != "m2" {
		t.Errorf("focus into a modal opened in the same batch: %q", b.Focus())
	}
	if p := b.TakePending(); len(p) != 1 || p[0].Action != "m2Focused" {
		t.Errorf("deferred on:focus: %+v", p)
	}
}

// Finding 24: Dump is a snapshot; the order of dumps does not matter and
// it does not change what Run would start with.
func TestDumpIsASnapshot(t *testing.T) {
	src := `<tui version="1"><style>@media (max-cols: 60) { #side { display: none; } }</style>
<screen id="main" focus="#inbox"><row>
<col id="side"><list id="inbox" each="items as it" key="it"><item><text>{it}</text></item></list></col>
<col><input id="query" bind="q" on:focus="qfocus"/></col>
</row></screen></tui>`
	fresh := func() *App {
		a := doc(t, src)
		_ = a.Bind("", map[string]any{"items": []any{"a", "b"}, "q": ""})
		return a
	}
	a := fresh()
	want := a.Dump(80, 24, false)
	if want.Focus == nil || *want.Focus != "inbox" {
		t.Fatalf("80x24 focus = %v", want.Focus)
	}
	if d := a.Dump(40, 24, false); d.Focus == nil || *d.Focus != "query" {
		t.Errorf("40x24 focus = %v", d.Focus)
	}
	if d := a.Dump(80, 24, false); d.Focus == nil || *d.Focus != "inbox" || grid(d) != grid(want) {
		t.Errorf("80x24 after 40x24: focus %v", d.Focus)
	}
	a.Validate()
	if a.Focus() != "" || len(a.TakePending()) != 0 || a.last != nil {
		t.Errorf("Dump/Validate changed live state: focus %q", a.Focus())
	}
	// The live frame after dumps matches a fresh app's.
	b := fresh()
	fa, fb := a.Frame(80, 24), b.Frame(80, 24)
	if fa.Focus != fb.Focus || strings.Join(fa.Grid.Lines(), "\n") != strings.Join(fb.Grid.Lines(), "\n") {
		t.Errorf("first frame after dumps: %q vs %q", fa.Focus, fb.Focus)
	}
	pa, pb := a.TakePending(), b.TakePending()
	if len(pa) != len(pb) {
		t.Errorf("pending after dumps %+v, fresh %+v", pa, pb)
	}
}

// Finding 27: the dump's cols/rows always describe its grid.
func TestDumpClampsNegativeSizes(t *testing.T) {
	a := doc(t, v1(`<text>hi</text>`))
	for _, sz := range [][2]int{{-5, 3}, {5, -3}, {-1, -1}, {0, 2}} {
		d := a.Dump(sz[0], sz[1], false)
		if d.Cols < 0 || d.Rows < 0 || len(d.Grid) != d.Rows {
			t.Errorf("%v: cols %d rows %d grid %d", sz, d.Cols, d.Rows, len(d.Grid))
		}
		for _, line := range d.Grid {
			if len([]rune(line)) != d.Cols {
				t.Errorf("%v: line %q is not %d wide", sz, line, d.Cols)
			}
		}
	}
}

// Finding 22: when Loop returns, nothing keeps reading the input: the next
// read by the host, or the next Loop, sees every byte.
func TestLoopLeavesNoReaderBehind(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("inputs are read with plain blocking reads on Windows")
	}
	a := doc(t, loopDoc)
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	defer wr.Close()
	size := func() (int, int) { return 20, 4 }
	runOnce := func() {
		done := make(chan error, 1)
		go func() { done <- a.Loop(rd, io.Discard, size, nil) }()
		if _, err := wr.WriteString("\x03"); err != nil { // ctrl+c: built-in quit
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Loop did not see its first key (a previous Loop's reader took it)")
		}
	}
	runOnce()
	runOnce()
	if _, err := wr.WriteString("hello\n"); err != nil {
		t.Fatal(err)
	}
	_ = rd.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 16)
	n, err := rd.Read(buf)
	if string(buf[:n]) != "hello\n" {
		t.Errorf("host read after Loop = %q, %v: a leaked reader consumed it", buf[:n], err)
	}
}

// Finding 25: a signal stops the loop, which restores the screen and
// cursor on the way out and reports the signal.
func TestLoopStopsOnSignal(t *testing.T) {
	a := doc(t, loopDoc)
	pr, pw := io.Pipe()
	defer pw.Close()
	out := &syncBuf{}
	sigs := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() { done <- a.loop(pr, out, func() (int, int) { return 20, 4 }, nil, sigs) }()
	sigs <- syscall.SIGTERM
	select {
	case err := <-done:
		var se *SignalError
		if !errors.As(err, &se) || se.Signal != syscall.SIGTERM {
			t.Fatalf("loop = %v, want a SignalError for SIGTERM", err)
		}
		if !strings.Contains(err.Error(), "terminated") {
			t.Errorf("message %q does not name the signal", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a signal did not stop the loop")
	}
	if got := out.String(); !strings.HasSuffix(got, "\x1b[0m\x1b[?25h\x1b[?7h\x1b[?1049l") {
		t.Errorf("terminal not restored: …%q", got[max(0, len(got)-24):])
	}
}

// Round 2, finding 1: a focus request to a node on another screen that
// cannot take focus there (disabled, hidden, not focusable) is a complete
// no-op: the screen does not switch, focus stays, and no on:focus fires,
// neither the target's nor a fallback's. Both paths: keymap action="focus"
// and Set("@focus").
func TestFocusRequestToUnfocusableOnOtherScreenIsNoop(t *testing.T) {
	src := `<tui version="1"><keymap>
  <bind keys="x" action="focus" to="#b2"/>
  <bind keys="y" action="focus" to="#hid"/>
  <bind keys="z" action="focus" to="#plain"/>
  <bind keys="g" action="focus" to="#b3"/>
</keymap>
<screen id="one" focus="#b1"><text>SCREEN ONE</text><button id="b1" label="one" on:focus="onfocus"/></screen>
<screen id="two"><text>SCREEN TWO</text>
  <button id="b2" label="two" disabled="true" on:focus="onfocus2"/>
  <button id="hid" label="hid" hidden="true" on:focus="onfocus2"/>
  <text id="plain" on:focus="onfocus2">plain</text>
  <button id="b3" label="three" on:focus="onfocus2"/>
</screen></tui>`
	check := func(t *testing.T, a *App, what string) {
		t.Helper()
		f := a.Frame(40, 6)
		if lines := f.Grid.Lines(); !strings.HasPrefix(lines[0], "SCREEN ONE") {
			t.Errorf("%s: screen switched:\n%s", what, strings.Join(lines, "\n"))
		}
		if a.Focus() != "b1" || f.Focus != "b1" {
			t.Errorf("%s: focus %q (frame %q), want b1", what, a.Focus(), f.Focus)
		}
		if p := a.TakePending(); len(p) != 0 {
			t.Errorf("%s: pending %+v", what, p)
		}
		if d := a.Dump(40, 6, false); !strings.HasPrefix(d.Grid[0], "SCREEN ONE") || d.Focus == nil || *d.Focus != "b1" {
			t.Errorf("%s: dump shows %q focus %v", what, d.Grid[0], d.Focus)
		}
	}
	a := doc(t, src)
	a.Frame(40, 6)
	if p := a.TakePending(); len(p) != 1 || p[0].Action != "onfocus" {
		t.Fatalf("initial focus: %+v", p)
	}
	for _, k := range []rune{'x', 'y', 'z'} {
		a.Frame(40, 6)
		if evs := a.HandleKey(r(k)); len(evs) != 0 {
			t.Errorf("%c: events %+v", k, evs)
		}
		check(t, a, "key "+string(k))
	}
	for _, id := range []string{"#b2", "#hid", "#plain"} {
		if err := a.Set("@focus", id); err != nil {
			t.Fatal(err)
		}
		check(t, a, "Set(@focus, "+id+")")
	}
	// Two requests before a frame: the last one cannot land, so the state
	// before the first one comes back.
	_ = a.Set("@focus", "#b3")
	_ = a.Set("@focus", "#b2")
	check(t, a, "two requests")
	// A request made before the first frame: screen one still gets its
	// initial focus (and its on:focus) as if nothing had been asked.
	b := doc(t, src)
	_ = b.Set("@focus", "#b2")
	f := b.Frame(40, 6)
	if !strings.HasPrefix(f.Grid.Lines()[0], "SCREEN ONE") || b.Focus() != "b1" {
		t.Errorf("before the first frame: %q focus %q", f.Grid.Lines()[0], b.Focus())
	}
	if p := b.TakePending(); len(p) != 1 || p[0].Action != "onfocus" || p[0].Source != "b1" {
		t.Errorf("before the first frame: pending %+v", p)
	}
	// A focusable target on the other screen still switches and focuses it.
	a.Frame(40, 6)
	if evs := a.HandleKey(r('g')); len(evs) != 0 {
		t.Errorf("g: events before the frame %+v", evs)
	}
	f = a.Frame(40, 6)
	if !strings.HasPrefix(f.Grid.Lines()[0], "SCREEN TWO") || a.Focus() != "b3" {
		t.Errorf("g: %q focus %q", f.Grid.Lines()[0], a.Focus())
	}
	if p := a.TakePending(); len(p) != 1 || p[0].Action != "onfocus2" || p[0].Source != "b3" {
		t.Errorf("g: on:focus %+v", p)
	}
}

// Round 2, finding 15 (runtime side): ids under a list <item> are never
// focus targets. screen focus=, keymap to=, and Set("@focus") that name one
// behave as if the id were missing: no screen switch, no focus, and no
// crash.
func TestFocusToIDUnderItemIsMissing(t *testing.T) {
	a := doc(t, `<tui version="1"><keymap>
  <bind keys="x" action="focus" to="#del"/>
  <bind keys="s" action="focus" to="#sel"/>
</keymap>
<screen id="one" focus="#b1"><text>ONE</text><button id="b1" label="one"/>
  <list id="l1"><item id="st"><button id="sel" label="s"/></item></list>
</screen>
<screen id="two" focus="#del"><text>TWO</text>
  <list id="rows" each="rows as r" key="r.id"><item><row><text>{r.id}</text><button id="del" label="x" on:click="delete" on:focus="delFocused"/></row></item></list>
  <button id="after" label="after"/>
</screen></tui>`)
	_ = a.Bind("rows", []any{map[string]any{"id": "a"}, map[string]any{"id": "b"}})
	a.Frame(40, 8)
	a.TakePending()
	for _, k := range []rune{'x', 's'} {
		a.Frame(40, 8)
		if evs := a.HandleKey(r(k)); len(evs) != 0 {
			t.Errorf("%c: events %+v", k, evs)
		}
		f := a.Frame(40, 8)
		if !strings.HasPrefix(f.Grid.Lines()[0], "ONE") || a.Focus() != "b1" {
			t.Errorf("%c: %q focus %q", k, f.Grid.Lines()[0], a.Focus())
		}
	}
	for _, id := range []string{"#del", "#sel", "#st"} {
		_ = a.Set("@focus", id)
		f := a.Frame(40, 8)
		if !strings.HasPrefix(f.Grid.Lines()[0], "ONE") || a.Focus() != "b1" {
			t.Errorf("Set(@focus, %s): %q focus %q", id, f.Grid.Lines()[0], a.Focus())
		}
	}
	if p := a.TakePending(); len(p) != 0 {
		t.Errorf("pending %+v", p)
	}
	// screen focus="#del" on screen two falls back like a missing id: the
	// list (the first focusable) takes focus, never a row button.
	if err := a.Set("@screen", "two"); err != nil {
		t.Fatal(err)
	}
	a.Frame(40, 8)
	if a.Focus() != "rows" {
		t.Errorf("screen focus=#del: focus %q, want rows", a.Focus())
	}
	for _, ev := range a.TakePending() {
		if ev.Action == "delFocused" {
			t.Errorf("a row button took focus: %+v", ev)
		}
	}
	for i := 0; i < 3; i++ {
		press(a, named("tab"))
		if f := a.Focus(); f != "rows" && f != "after" {
			t.Errorf("tab %d reached %q", i, f)
		}
	}
}
