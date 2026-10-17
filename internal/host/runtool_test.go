package host

// SPEC v0.4a §15.9: the internal hooks `tuimark run` adds to RunHost
// (overlay, F12 notice, second ctrl+c, app swap), and test 121's last
// bullet (F12 under Run() yields no key and no key record).

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// toolRig drives Run's session on a pipe with `tuimark run`'s hooks and
// records the actions handlers see, in order.
type toolRig struct {
	in   *io.PipeWriter
	out  *syncBuf
	done chan error
	mu   sync.Mutex
	seen []string
}

func (r *toolRig) record(s string) {
	r.mu.Lock()
	r.seen = append(r.seen, s)
	r.mu.Unlock()
}

func (r *toolRig) actions() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}

// waitFor waits until the recorded actions are exactly want.
func (r *toolRig) waitFor(t *testing.T, want ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Join(r.actions(), " ") == strings.Join(want, " ") {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("actions %v, want %v", r.actions(), want)
}

// waitOut waits until the session's output holds want.
func (r *toolRig) waitOut(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(r.out.String(), want) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("the output never held %q: %q", want, r.out.String())
}

func (r *toolRig) send(t *testing.T, s string) {
	t.Helper()
	if _, err := io.WriteString(r.in, s); err != nil {
		t.Fatal(err)
	}
}

func (r *toolRig) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-r.done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the session did not return")
	}
	return nil
}

// recordAll registers a handler that records the action for each name.
func (r *toolRig) recordAll(a *App, names ...string) {
	for _, n := range names {
		n := n
		a.On(n, func(Event) error {
			r.record(n)
			return nil
		})
	}
}

// startTool runs a's session (Run's loop, as RunHost runs it) on a pipe,
// at cols×rows, with hooks and an optional TUIMARK_LOG log.
func startTool(t *testing.T, a *App, cols, rows int, hooks HostHooks, lg *runLog) *toolRig {
	t.Helper()
	pr, pw := io.Pipe()
	r := &toolRig{in: pw, out: &syncBuf{}, done: make(chan error, 1)}
	go func() {
		if err := a.enterMode(modeRun); err != nil {
			r.done <- err
			return
		}
		defer a.exitMode()
		err := a.session(pr, r.out, func() (int, int) { return cols, rows }, nil, nil, &termSession{log: lg}, hooks)
		lg.end("quit")
		r.done <- err
	}()
	t.Cleanup(func() { pw.Close() })
	return r
}

func TestDecoderF12OnlyForRun(t *testing.T) {
	in := []byte("\x1b[24~a\x1b[24;2~b")
	var plain decoder
	ins, _ := plain.feed(in, true)
	if len(ins) != 2 || ins[0].Key.Name != "a" || ins[1].Key.Name != "b" {
		t.Errorf("without f12: %+v, want the keys a and b only", ins)
	}
	run := decoder{f12: true}
	ins, _ = run.feed(in, true)
	if len(ins) != 3 || !ins[0].IsF12 || ins[1].Key.Name != "a" || ins[2].Key.Name != "b" {
		t.Errorf("with f12: %+v, want the F12 notice, a, and b (CSI 24 ; 2 ~ is no notice)", ins)
	}
	// play's text: steps decode with DecodeInput: no notice, no key.
	for _, k := range DecodeInput([]byte("\x1b[24~")) {
		t.Errorf("DecodeInput gave %+v for F12", k)
	}
	// Cut across reads, it is still one notice.
	cut := decoder{f12: true}
	a, _ := cut.feed([]byte("\x1b[2"), false)
	b, _ := cut.feed([]byte("4~"), false)
	if len(a) != 0 || len(b) != 1 || !b[0].IsF12 {
		t.Errorf("cut F12: %+v then %+v", a, b)
	}
}

// keyRecords returns the "key" members of a TUIMARK_LOG file's key
// records, in order.
func keyRecords(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, `"ev":"key"`) {
			i := strings.Index(line, `"key":"`)
			if i < 0 {
				keys = append(keys, "?"+line)
				continue
			}
			rest := line[i+len(`"key":"`):]
			keys = append(keys, rest[:strings.IndexByte(rest, '"')])
		}
	}
	return keys
}

const f12Doc = `<tui version="1">
<keymap><bind keys="q" action="quit"/><bind keys="x" action="hit"/></keymap>
<screen id="s"><text>F12APP</text></screen>
</tui>`

// Test 121, "Under Run()": the F12 sequence yields no key and no key
// record; with `tuimark run`'s hook it is the notice, and still no key.
func TestF12UnderRunIsNoKey(t *testing.T) {
	for _, withHook := range []bool{false, true} {
		a := doc(t, f12Doc)
		logPath := filepath.Join(t.TempDir(), "run.log")
		lg, err := openRunLog(logPath, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		var notices int
		var hooks HostHooks
		if withHook {
			hooks.F12 = func() { notices++ }
		}
		r := startTool(t, a, 40, 6, hooks, lg)
		r.recordAll(a, "hit")
		r.waitOut(t, "F12APP")
		r.send(t, "\x1b[24~")
		r.send(t, "x")
		r.waitFor(t, "hit")
		r.send(t, "\x1b[24~q")
		if err := r.wait(t); err != nil {
			t.Fatalf("hook %v: %v", withHook, err)
		}
		if got := keyRecords(t, logPath); strings.Join(got, " ") != "x q" {
			t.Errorf("hook %v: key records %v, want x q", withHook, got)
		}
		want := 0
		if withHook {
			want = 2
		}
		if notices != want {
			t.Errorf("hook %v: %d notices, want %d", withHook, notices, want)
		}
	}
}

// overlayDoc has a button on its last row: a click there goes to it.
const overlayDoc = `<tui version="2" mouse="true">
<screen id="s">
  <text>TOPROW</text>
  <spacer/>
  <button id="b" on:click="clicked" height="1">BOTTOMBUTTON</button>
</screen>
</tui>`

// The overlay is painted over a copy of the frame's grid and diffed with
// it: the terminal gets it, but the frame, the dump, and the mouse hit
// test never see it, so a click on the strip acts on the frame under it.
func TestOverlayIsNotPartOfTheFrame(t *testing.T) {
	a := doc(t, overlayDoc)
	var mu sync.Mutex
	strip := []string{}
	hooks := HostHooks{Overlay: func(cols, rows int) (top, bottom []string) {
		mu.Lock()
		defer mu.Unlock()
		return []string{"PANELLINE"}, append([]string(nil), strip...)
	}}
	r := startTool(t, a, 30, 6, hooks, nil)
	r.recordAll(a, "clicked")
	r.waitOut(t, "PANELLINE")
	mu.Lock()
	strip = []string{"STRIPLINE"}
	mu.Unlock()
	a.Wake()
	r.waitOut(t, "STRIPLINE")
	// The live frame still has the document's rows.
	a.mu.Lock()
	lines := a.last.Grid.Lines()
	a.mu.Unlock()
	if !strings.HasPrefix(lines[0], "TOPROW") || !strings.Contains(lines[5], "BOTTOMBUTTON") {
		t.Errorf("the frame's grid holds the overlay: %q", lines)
	}
	if d := a.Dump(30, 6, false); strings.Contains(strings.Join(d.Grid, "\n"), "STRIP") || strings.Contains(strings.Join(d.Grid, "\n"), "PANEL") {
		t.Errorf("the dump holds the overlay: %q", d.Grid)
	}
	// A click on the strip's row (SGR, 1-based) acts on the button under it.
	r.send(t, "\x1b[<0;3;6M\x1b[<0;3;6m")
	r.waitFor(t, "clicked")
	r.in.Close()
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
}

func TestOverlayPaintsPanelThenStrip(t *testing.T) {
	a := doc(t, overlayDoc)
	f := a.Frame(10, 2)
	got := overlay(f.Grid, []string{"PANEL-LONG-LINE"}, []string{"STRIP", "S2"})
	lines := got.Lines()
	// Two rows: the panel's row 0 is under the strip's first row (the
	// strip wins), and every row is cut at the width.
	if lines[0] != "STRIP     " || lines[1] != "S2        " {
		t.Errorf("overlay rows %q", lines)
	}
	if c := got.At(0, 0); c.Attrs != stripAttrs {
		t.Errorf("strip attrs %v, want %v", c.Attrs, stripAttrs)
	}
	if f.Grid.Lines()[0] == lines[0] {
		t.Error("overlay painted the frame's own grid")
	}
	top := overlay(f.Grid, []string{"PANEL-LONG-LINE"}, nil)
	if l := top.Lines()[0]; l != "PANEL-LONG" {
		t.Errorf("panel row %q, want it cut at 10 columns", l)
	}
	if c := top.At(0, 0); c.Attrs != panelAttrs {
		t.Errorf("panel attrs %v, want %v", c.Attrs, panelAttrs)
	}
}

const ctrlCDoc = `<tui version="1">
<keymap><bind keys="ctrl+c" action="cc"/></keymap>
<screen id="s"><text>CTRLC</text></screen>
</tui>`

// Two ctrl+c within a second end the session as a quit, even when the
// document binds ctrl+c; the first is dispatched as usual, the second
// never is. Without the hook both are dispatched.
func TestCtrlCTwice(t *testing.T) {
	a := doc(t, ctrlCDoc)
	r := startTool(t, a, 30, 4, HostHooks{CtrlCTwice: true}, nil)
	r.recordAll(a, "cc")
	r.waitOut(t, "CTRLC")
	r.send(t, "\x03")
	r.waitFor(t, "cc")
	r.send(t, "\x03")
	if err := r.wait(t); err != nil {
		t.Fatalf("second ctrl+c: %v, want a quit", err)
	}
	if got := r.actions(); len(got) != 1 {
		t.Errorf("actions %v, want cc once", got)
	}

	// Without the hook, ctrl+c is an ordinary key.
	b := doc(t, ctrlCDoc)
	r2 := startTool(t, b, 30, 4, HostHooks{}, nil)
	r2.recordAll(b, "cc")
	r2.waitOut(t, "CTRLC")
	r2.send(t, "\x03")
	r2.send(t, "\x03")
	r2.waitFor(t, "cc", "cc")
	r2.in.Close()
	if err := r2.wait(t); err != nil {
		t.Fatal(err)
	}
}

// Two presses farther apart than the window are two dispatches.
func TestCtrlCTwiceOutsideTheWindow(t *testing.T) {
	old := ctrlCWindow
	ctrlCWindow = 30 * time.Millisecond
	defer func() { ctrlCWindow = old }()
	a := doc(t, ctrlCDoc)
	r := startTool(t, a, 30, 4, HostHooks{CtrlCTwice: true}, nil)
	r.recordAll(a, "cc")
	r.waitOut(t, "CTRLC")
	r.send(t, "\x03")
	r.waitFor(t, "cc")
	time.Sleep(60 * time.Millisecond)
	r.send(t, "\x03")
	r.waitFor(t, "cc", "cc")
	r.send(t, "\x03")
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
}

// reloadDoc is the document a reload keeps state across: two screens, a
// tabs without bind, a list with a cursor, an input without bind, and a
// scroll with an offset. MARK is replaced to tell the two versions apart.
const reloadDoc = `<tui version="2">
<keymap>
  <bind keys="w" action="switch-to" to="work"/>
  <bind keys="2" action="switch-to" to="t2"/>
  <bind keys="J" action="move-next" to="#sc"/>
  <bind keys="ctrl+r" action="probe"/>
</keymap>
<screen id="home"><text>HOME</text><button id="hb">go</button></screen>
<screen id="work">
  <text>MARK</text>
  <tabs id="tb">
    <tab id="t1" label="one"><text>T1</text></tab>
    <tab id="t2" label="two">
      <col>
        <list id="l" each="rows as r" key="r" height="3"><item><text>row-{r}</text></item></list>
        <input id="in"/>
        <scroll id="sc" height="2"><text>s0</text><text>s1</text><text>s2</text><text>s3</text></scroll>
      </col>
    </tab>
  </tabs>
</screen>
</tui>`

// A swap moves the session into a new app: the store, and by id and tag
// the focus, the active screen, a list cursor, an input's text and cursor,
// a scroll offset, and the active tab of a tabs without bind.
func TestSwapKeepsStateByIDAndTag(t *testing.T) {
	a := doc(t, strings.Replace(reloadDoc, "MARK", "VERSION-ONE", 1))
	_ = a.Bind("", map[string]any{"rows": []any{"a", "b", "c", "d"}})
	swaps := make(chan HostSwap)
	r := startTool(t, a, 40, 14, HostHooks{Swap: swaps}, nil)
	r.recordAll(a, "probe")
	r.waitOut(t, "HOME")
	r.send(t, "w")
	r.send(t, "2")
	r.send(t, "\x1b[B\x1b[B") // the list (focused by the tab change): row 2
	r.send(t, "J")            // the scroll: offset 1
	r.send(t, "\t")           // the input
	r.send(t, "hi")
	r.send(t, "\x12") // ctrl+r: the input does not take it
	r.waitFor(t, "probe")

	next := doc(t, strings.Replace(reloadDoc, "MARK", "VERSION-TWO", 1))
	r.recordAll(next, "probe")
	applied := make(chan struct{})
	swaps <- HostSwap{App: next, KeepStore: true, Applied: func() { close(applied) }}
	<-applied
	r.send(t, "\x12")
	r.waitFor(t, "probe", "probe")
	r.waitOut(t, "TWO") // the diff rewrites only what changed

	next.mu.Lock()
	focus, screen := next.focus, next.doc.Screens[next.screen].ID
	cursor := next.lists["l"].index
	in := *next.inputs["in"]
	off := next.scrolls["sc"]
	tab := next.tabMem["tb"]
	rows, _ := lookup(next.store, "rows")
	next.mu.Unlock()
	if focus != "in" || screen != "work" || cursor != 2 || in.value != "hi" || in.cursor != 2 || off != [2]int{0, 1} || tab != "t2" {
		t.Errorf("kept: focus %q screen %q cursor %d input %+v scroll %v tab %q", focus, screen, cursor, in, off, tab)
	}
	if len(rows.([]any)) != 4 {
		t.Errorf("the store did not move: rows %v", rows)
	}
	// The old app gave the Run mode back; the new one holds it.
	if err := a.EnterPlay(); err != nil {
		t.Errorf("the old app still holds the Run mode: %v", err)
	} else {
		a.ExitPlay()
	}
	if err := next.EnterPlay(); err == nil {
		next.ExitPlay()
		t.Error("the new app does not hold the Run mode")
	}
	r.in.Close()
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	if err := next.EnterPlay(); err != nil {
		t.Errorf("the session ended but the new app still holds the Run mode: %v", err)
	}
}

// What is not kept by id and tag starts as on a first frame; without
// KeepStore the new app keeps its own store.
func TestSwapDropsWhatChangedTag(t *testing.T) {
	a := doc(t, strings.Replace(reloadDoc, "MARK", "ONE", 1))
	_ = a.Bind("", map[string]any{"rows": []any{"a", "b", "c", "d"}})
	swaps := make(chan HostSwap)
	r := startTool(t, a, 40, 14, HostHooks{Swap: swaps}, nil)
	r.recordAll(a, "probe")
	r.waitOut(t, "HOME")
	r.send(t, "w2\x1b[B\x1b[B\x12")
	r.waitFor(t, "probe")

	// The list l is a table now (another tag), and the store is the new
	// app's own.
	src := strings.Replace(reloadDoc, "MARK", "TWO", 1)
	src = strings.Replace(src, `<list id="l" each="rows as r" key="r" height="3"><item><text>row-{r}</text></item></list>`,
		`<table id="l" each="rows as r" key="r" height="3"><column>{r}</column></table>`, 1)
	next := doc(t, src)
	_ = next.Bind("", map[string]any{"rows": []any{"x", "y", "z", "w"}})
	r.recordAll(next, "probe")
	swaps <- HostSwap{App: next}
	r.send(t, "\x12")
	r.waitFor(t, "probe", "probe")
	next.mu.Lock()
	cursor := next.lists["l"].index
	rows, _ := lookup(next.store, "rows")
	next.mu.Unlock()
	if cursor != 0 {
		t.Errorf("the table l took the list l's cursor: %d", cursor)
	}
	if rows.([]any)[0] != "x" {
		t.Errorf("KeepStore false moved the old store: %v", rows)
	}
	r.in.Close()
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
}

const lifeDoc = `<tui version="1">
<screen id="s">
  <button id="b1" on:focus="f1">one</button>
  <button id="b2" on:focus="f2">two</button>
  <text>MARK</text>
  <modal id="m" open="true" on:open="opened"><button id="mb" on:focus="fm">ok</button></modal>
</screen>
</tui>`

// The first frame after a swap compares with the last frame before it: a
// modal open in both fires no on:open, and focus that moves because its
// id is gone fires on:focus where it lands.
func TestSwapLifecycleEvents(t *testing.T) {
	a := doc(t, strings.Replace(lifeDoc, "MARK", "ONE", 1))
	swaps := make(chan HostSwap)
	r := startTool(t, a, 40, 12, HostHooks{Swap: swaps}, nil)
	r.recordAll(a, "f1", "f2", "fm", "opened")
	r.waitFor(t, "fm", "opened")

	// The modal stays open, and its focused button is gone: focus lands on
	// the modal's other button, which fires its on:focus; no on:open.
	src := strings.Replace(lifeDoc, "MARK", "TWO", 1)
	src = strings.Replace(src, `<button id="mb" on:focus="fm">ok</button>`, `<button id="mc" on:focus="fc">fine</button>`, 1)
	next := doc(t, src)
	r.recordAll(next, "f1", "f2", "fc", "opened")
	swaps <- HostSwap{App: next, KeepStore: true}
	r.waitFor(t, "fm", "opened", "fc")
	r.waitOut(t, "TWO")

	// A modal that only the newer document opens fires on:open.
	third := doc(t, strings.Replace(strings.Replace(src, `id="m" open="true"`, `id="m2" open="true"`, 1), "TWO", "THREE", 1))
	r.recordAll(third, "f1", "f2", "fc", "opened")
	swaps <- HostSwap{App: third, KeepStore: true}
	r.waitFor(t, "fm", "opened", "fc", "opened")
	r.in.Close()
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.actions(), " "); got != "fm opened fc opened" {
		t.Errorf("actions at the end %q: a swap fired more", got)
	}
}

// A swap without an app only runs Applied and draws again.
func TestSwapWithoutAppRedraws(t *testing.T) {
	a := doc(t, f12Doc)
	swaps := make(chan HostSwap)
	var mu sync.Mutex
	panel := []string{}
	hooks := HostHooks{Swap: swaps, Overlay: func(cols, rows int) (top, bottom []string) {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), panel...), nil
	}}
	r := startTool(t, a, 30, 6, hooks, nil)
	r.waitOut(t, "F12APP")
	swaps <- HostSwap{Applied: func() {
		mu.Lock()
		panel = []string{"error V005 broken"}
		mu.Unlock()
	}}
	r.waitOut(t, "error V005 broken")
	r.send(t, "q")
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
}
