package host

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// SPEC v0.2b §8.5 (mouse), §26.8 (SGR reports), §26.11 (Run), §21 tests
// 44, 45, 46, and 48.

// mouseNames formats decoded inputs, mouse events included.
func mouseNames(ins []Input) string {
	var out []string
	for _, in := range ins {
		switch {
		case in.IsMouse:
			out = append(out, fmt.Sprintf("%s@%d,%d", in.Mouse.Kind, in.Mouse.X, in.Mouse.Y))
		case in.IsPaste:
			out = append(out, fmt.Sprintf("paste(%q)", in.Paste))
		default:
			out = append(out, in.Key.Name)
		}
	}
	return strings.Join(out, " ")
}

// 48. SGR reports decode into mouse events and never into keys; the
// modifier bits are ignored; motion, the middle and right buttons, the
// horizontal wheel, wheel releases, and malformed reports give nothing;
// X10 reports give neither a key nor an event; all of it holds when a read
// cuts the report anywhere.
func TestDecodeSGRMouse(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"left press", "\x1b[<0;5;3M", "press@4,2"},
		{"left release", "\x1b[<0;5;3m", "release@4,2"},
		{"click", "\x1b[<0;5;3M\x1b[<0;5;3m", "press@4,2 release@4,2"},
		{"wheel up", "\x1b[<64;1;1M", "wheel-up@0,0"},
		{"wheel down", "\x1b[<65;80;24M", "wheel-down@79,23"},
		{"shift", "\x1b[<4;5;3M", "press@4,2"},
		{"meta", "\x1b[<8;5;3m", "release@4,2"},
		{"ctrl", "\x1b[<16;5;3M", "press@4,2"},
		{"ctrl wheel", "\x1b[<81;2;2M", "wheel-down@1,1"},
		{"all modifiers", "\x1b[<92;2;2M", "wheel-up@1,1"},
		{"motion", "\x1b[<32;5;3M", ""},
		{"motion no button", "\x1b[<35;5;3M", ""},
		{"middle", "\x1b[<1;5;3M", ""},
		{"right", "\x1b[<2;5;3M\x1b[<2;5;3m", ""},
		{"horizontal wheel", "\x1b[<66;5;3M\x1b[<67;5;3M", ""},
		{"wheel release", "\x1b[<64;5;3m", ""},
		{"two parameters", "\x1b[<0;5M", ""},
		{"four parameters", "\x1b[<0;5;3;1M", ""},
		{"not decimal", "\x1b[<0;5:1;3M", ""},
		{"question mark", "\x1b[<0;?;3M", ""},
		{"empty parameter", "\x1b[<0;;3M", ""},
		{"signed", "\x1b[<0;+5;3M", ""},
		{"keys around", "a\x1b[<0;5;3Mb", "a press@4,2 b"},
		{"x10 with q", "\x1b[M q%x", "x"},
		{"x10 raw bytes", "\x1b[M\xff\x1b\x03y", "y"},
	}
	for _, c := range cases {
		if got := mouseNames(DecodeInput([]byte(c.in))); got != c.want {
			t.Errorf("%s: %q decodes to %q, want %q", c.name, c.in, got, c.want)
		}
		keys := 0
		for _, f := range strings.Fields(c.want) {
			if !strings.Contains(f, "@") {
				keys++
			}
		}
		if got := DecodeKeys([]byte(c.in)); len(got) != keys {
			t.Errorf("%s: DecodeKeys gives %v, want %d keys", c.name, got, keys)
		}
		for cut := 1; cut < len(c.in); cut++ {
			var d decoder
			a, _ := d.feed([]byte(c.in[:cut]), false)
			b, _ := d.feed([]byte(c.in[cut:]), true)
			if got := mouseNames(append(a, b...)); got != c.want {
				t.Errorf("%s cut at %d: %q, want %q", c.name, cut, got, c.want)
			}
		}
	}
}

const mouseDoc = `<tui version="2" mouse="mouse">
<keymap><bind keys="q" action="quit"/></keymap>
<screen id="main" focus="#b1">
  <col id="app">
    <tabs id="t" on:select="tab">
      <tab id="one" label="one"><text>1</text></tab>
      <tab id="two" label="two"><text>2</text></tab>
      <tab id="three" label="three" disabled="true"><text>3</text></tab>
    </tabs>
    <list id="l" each="rows as r" key="r.id" on:select="pick" on:focus="lfocus" style="height: 3"><item><row><text id="nm">{r.name}</text></row></item></list>
    <table id="tb" each="rows as r" key="r.id" on:select="tpick" on:focus="tfocus" style="height: 4">
      <column id="c1" title="N" width="6" on:click="sort">{r.name}</column>
      <column title="ID" width="4">{r.id}</column>
    </table>
    <row style="height: 1; gap: 1">
      <button id="b1" label="go" on:click="go" on:focus="bfocus"/>
      <button id="b2" label="off" on:click="never" disabled="true"/>
      <box id="fb" focusable="true" on:focus="boxfocus" style="width: 5"><text>box</text></box>
      <text id="plain">plain</text>
    </row>
    <scroll id="sc" style="height: 2"><col><text>s0</text><text>s1</text><text>s2</text><text>s3</text></col></scroll>
  </col>
  <modal id="m" open="modal" on:escape="close"><button id="mb" label="ok" on:click="ok"/></modal>
</screen>
</tui>`

const mouseW, mouseH = 40, 20

func mouseApp(t *testing.T) *App {
	t.Helper()
	a := doc(t, mouseDoc)
	bindJSON(t, a, `{"mouse": true, "modal": false, "rows": [{"id": 1, "name": "ann"}, {"id": 2, "name": "bob"}, {"id": 3, "name": "cy"}]}`)
	if ds := a.Validate(); len(ds) > 0 {
		t.Fatalf("fixture diagnostics: %v", ds)
	}
	a.Frame(mouseW, mouseH)
	a.TakePending()
	return a
}

// settleFrame renders again and returns the lifecycle events it queued.
func settleFrame(a *App) []Event {
	var out []Event
	for i := 0; i < 8; i++ {
		a.Frame(mouseW, mouseH)
		p := a.TakePending()
		if len(p) == 0 {
			break
		}
		out = append(out, p...)
	}
	return out
}

func actions(evs []Event) string {
	var out []string
	for _, e := range evs {
		out = append(out, e.Action)
	}
	return strings.Join(out, " ")
}

// clickAt is a left press and release at (x, y) on the live frame, then
// a settled frame; it returns the events in order.
func clickAt(a *App, x, y int) []Event {
	evs, _ := a.HandleMouse(Mouse{Kind: MousePress, X: x, Y: y})
	e2, _ := a.HandleMouse(Mouse{Kind: MouseRelease, X: x, Y: y})
	return append(append(evs, e2...), settleFrame(a)...)
}

func wheelAt(a *App, x, y, dir int) []Event {
	k := MouseWheelDown
	if dir < 0 {
		k = MouseWheelUp
	}
	evs, _ := a.HandleMouse(Mouse{Kind: k, X: x, Y: y})
	return append(evs, settleFrame(a)...)
}

func live(a *App) *Frame {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.last
}

// label returns tab id's label box in f.
func label(f *Frame, tabs, id string) *layout.Box {
	for _, c := range f.ByID[tabs].Children {
		if c.Role == layout.RoleTabLabel && c.Key == id {
			return c
		}
	}
	return nil
}

// row returns row i (in the laid-out children) of list or table id.
func row(f *Frame, id string, i int) *layout.Box {
	k := 0
	for _, c := range f.ByID[id].Children {
		if c.Kind == "item" {
			if k == i {
				return c
			}
			k++
		}
	}
	return nil
}

func listIndex(a *App, id string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lists[id].index
}

// 45. A click acts on the rule of §8.5 that applies, with its events in
// order (on:focus before on:select or on:click).
func TestMouseClickRules(t *testing.T) {
	cases := []struct {
		name  string
		at    func(f *Frame) (int, int)
		want  string
		focus string
	}{
		{"tab label", func(f *Frame) (int, int) { l := label(f, "t", "two"); return l.X + 2, l.Y }, "tab", "b1"},
		{"disabled tab label", func(f *Frame) (int, int) { l := label(f, "t", "three"); return l.X + 1, l.Y }, "", "b1"},
		{"list row", func(f *Frame) (int, int) { r := row(f, "l", 1); return r.X, r.Y }, "lfocus pick", "l"},
		{"table row", func(f *Frame) (int, int) { r := row(f, "tb", 2); return r.X + 8, r.Y }, "tfocus tpick", "tb"},
		{"table body cell under a column with on:click", func(f *Frame) (int, int) { c := row(f, "tb", 1).Children[0]; return c.X, c.Y }, "tfocus tpick", "tb"},
		{"column header", func(f *Frame) (int, int) { c := f.ByID["c1"]; return c.X, c.Y }, "sort", "b1"},
		{"disabled button", func(f *Frame) (int, int) { b := f.ByID["b2"]; return b.X + 1, b.Y }, "", "b1"},
		{"focusable box", func(f *Frame) (int, int) { b := f.ByID["fb"]; return b.X + 1, b.Y }, "boxfocus", "fb"},
		{"plain text", func(f *Frame) (int, int) { b := f.ByID["plain"]; return b.X, b.Y }, "", "b1"},
		{"focused button", func(f *Frame) (int, int) { b := f.ByID["b1"]; return b.X, b.Y }, "go", "b1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := mouseApp(t)
			x, y := c.at(live(a))
			if got := actions(clickAt(a, x, y)); got != c.want {
				t.Errorf("click at %d,%d fires %q, want %q", x, y, got, c.want)
			}
			if a.Focus() != c.focus {
				t.Errorf("focus %q, want %q", a.Focus(), c.focus)
			}
		})
	}
	// A button that is not focused takes focus first.
	a := mouseApp(t)
	fb := live(a).ByID["fb"]
	clickAt(a, fb.X, fb.Y)
	b1 := live(a).ByID["b1"]
	if got := actions(clickAt(a, b1.X, b1.Y)); got != "bfocus go" {
		t.Errorf("click on an unfocused button fires %q", got)
	}
	// The row moved to is the row clicked, and the tab label activates.
	a = mouseApp(t)
	r := row(live(a), "l", 2)
	clickAt(a, r.X, r.Y)
	if listIndex(a, "l") != 2 {
		t.Errorf("list cursor %d, want 2", listIndex(a, "l"))
	}
	l := label(live(a), "t", "two")
	ev := clickAt(a, l.X, l.Y)
	if len(ev) != 1 || ev[0].Value != "two" || ev[0].Source != "t" {
		t.Errorf("tab click event %+v", ev)
	}
	if got := live(a).ByID["t"].TabActive.ID; got != "two" {
		t.Errorf("active tab %q", got)
	}
}

// 45. A press on one node and a release on another does nothing, also
// when both rows hold the same template id; the pending press survives
// frames and keys, a new press replaces it, and a resize drops it.
func TestMousePressRelease(t *testing.T) {
	a := mouseApp(t)
	r0, r1 := row(live(a), "l", 0), row(live(a), "l", 1)
	if r0.Children[0].Children[0].ID != "nm" || r1.Children[0].Children[0].ID != "nm" {
		t.Fatal("the rows do not share the template id")
	}
	a.HandleMouse(Mouse{Kind: MousePress, X: r0.X, Y: r0.Y})
	evs, _ := a.HandleMouse(Mouse{Kind: MouseRelease, X: r1.X, Y: r1.Y})
	if len(evs) != 0 || listIndex(a, "l") != 0 {
		t.Errorf("press on row 0 and release on row 1 fired %v", evs)
	}
	// A release with nothing pending does nothing.
	if evs, _ := a.HandleMouse(Mouse{Kind: MouseRelease, X: r1.X, Y: r1.Y}); len(evs) != 0 {
		t.Errorf("a lone release fired %v", evs)
	}
	// The press survives a frame and a key; a second press replaces it.
	a.HandleMouse(Mouse{Kind: MousePress, X: r0.X, Y: r0.Y})
	a.Frame(mouseW, mouseH)
	a.HandleKey(Key{Name: "x", Rune: 'x'})
	a.HandleMouse(Mouse{Kind: MousePress, X: r1.X, Y: r1.Y})
	evs, _ = a.HandleMouse(Mouse{Kind: MouseRelease, X: r1.X, Y: r1.Y})
	if actions(evs) != "lfocus pick" {
		t.Errorf("press, frame, key, press, release fired %q", actions(evs))
	}
	// A resize drops the pending press.
	a = mouseApp(t)
	b1 := live(a).ByID["b1"]
	a.HandleMouse(Mouse{Kind: MousePress, X: b1.X, Y: b1.Y})
	a.DropPress()
	if evs, _ := a.HandleMouse(Mouse{Kind: MouseRelease, X: b1.X, Y: b1.Y}); len(evs) != 0 {
		t.Errorf("a release after a resize fired %v", evs)
	}
}

// 45. While a modal is open, a click outside it does nothing and one
// inside it acts; events outside the grid are dropped.
func TestMouseModalAndGrid(t *testing.T) {
	a := mouseApp(t)
	_ = a.Set("modal", true)
	settleFrame(a)
	f := live(a)
	m := f.Modals[0]
	if got := actions(clickAt(a, 0, 0)); got != "" || !(m.X > 0 || m.Y > 0) {
		t.Errorf("click outside the modal fired %q", got)
	}
	l := label(f, "t", "two")
	if got := actions(clickAt(a, l.X, l.Y)); got != "" {
		t.Errorf("click on a label under the modal fired %q", got)
	}
	mb := f.ByID["mb"]
	if got := actions(clickAt(a, mb.X+1, mb.Y)); got != "ok" {
		t.Errorf("click on the modal button fired %q", got)
	}
	if got := actions(wheelAt(a, 0, 0, 1)); got != "" {
		t.Errorf("wheel outside the modal fired %q", got)
	}
	b := mouseApp(t)
	for _, xy := range [][2]int{{-1, 0}, {0, -1}, {mouseW, 0}, {0, mouseH}} {
		if got := actions(clickAt(b, xy[0], xy[1])); got != "" {
			t.Errorf("click at %v fired %q", xy, got)
		}
	}
}

// 46. The wheel moves a list or table cursor by one with on:select (none
// at the ends) without moving focus, a viewport's offset by one row, and
// the tab strip with wrap over the enabled tabs; nothing over a disabled
// subtree.
func TestMouseWheel(t *testing.T) {
	a := mouseApp(t)
	r := row(live(a), "l", 0)
	if got := actions(wheelAt(a, r.X, r.Y, -1)); got != "" {
		t.Errorf("wheel up at the first row fired %q", got)
	}
	for i, want := range []string{"pick", "pick", ""} {
		r := row(live(a), "l", 0)
		if got := actions(wheelAt(a, r.X, r.Y, 1)); got != want {
			t.Errorf("wheel down %d fired %q, want %q", i, got, want)
		}
	}
	if listIndex(a, "l") != 2 || a.Focus() != "b1" {
		t.Errorf("cursor %d focus %q", listIndex(a, "l"), a.Focus())
	}
	tr := row(live(a), "tb", 0)
	if got := actions(wheelAt(a, tr.X, tr.Y, 1)); got != "tpick" || listIndex(a, "tb") != 1 || a.Focus() != "b1" {
		t.Errorf("wheel over the table fired %q (cursor %d, focus %q)", got, listIndex(a, "tb"), a.Focus())
	}
	sc := live(a).ByID["sc"]
	wheelAt(a, sc.X, sc.Y, 1)
	if off := a.scrolls["sc"]; off[1] != 1 {
		t.Errorf("scroll offset %v after wheel down", off)
	}
	sc = live(a).ByID["sc"]
	wheelAt(a, sc.X, sc.Y, -1)
	wheelAt(a, sc.X, sc.Y, -1)
	if off := a.scrolls["sc"]; off[1] != 0 {
		t.Errorf("scroll offset %v after two wheels up (clamped)", off)
	}
	// The strip: one -> two -> (three is disabled) one; up from one wraps
	// to two. A cell between labels on the strip row counts.
	strip := live(a).ByID["t"]
	gapX := label(live(a), "t", "one").X + label(live(a), "t", "one").W
	for i, want := range []string{"two", "one"} {
		evs := wheelAt(a, gapX, strip.Content.Y, 1)
		if len(evs) != 1 || evs[0].Value != want {
			t.Errorf("strip wheel down %d: %+v, want %s", i, evs, want)
		}
	}
	one := label(live(a), "t", "one")
	if evs := wheelAt(a, one.X, one.Y, -1); len(evs) != 1 || evs[0].Value != "two" {
		t.Errorf("label wheel up: %+v", evs)
	}
	b2 := live(a).ByID["b2"]
	if got := actions(wheelAt(a, b2.X, b2.Y, 1)); got != "" {
		t.Errorf("wheel over a disabled button fired %q", got)
	}
	// With mouse false nothing happens, and the report is not "on".
	_ = a.Set("mouse", false)
	settleFrame(a)
	r = row(live(a), "l", 0)
	if evs, on := a.HandleMouse(Mouse{Kind: MouseWheelUp, X: r.X, Y: r.Y}); len(evs) != 0 || on {
		t.Errorf("wheel with mouse off: %v on=%v", evs, on)
	}
	if got := actions(clickAt(a, one.X, one.Y)); got != "" {
		t.Errorf("click with mouse off fired %q", got)
	}
}

// 45. The hit node of every cell is inspect --at's node, and a click's
// hit identity path leads from its layer's root to it.
func TestMouseHitMatchesInspect(t *testing.T) {
	for _, modal := range []bool{false, true} {
		a := mouseApp(t)
		_ = a.Set("modal", modal)
		settleFrame(a)
		f := live(a)
		for y := 0; y < f.Rows; y++ {
			for x := 0; x < f.Cols; x++ {
				in, err := a.Inspect(f, InspectTarget{At: true, X: x, Y: y})
				if err != nil {
					t.Fatal(err)
				}
				a.mu.Lock()
				h, root, id := a.hit(f, x, y)
				a.mu.Unlock()
				if h == nil {
					if !modal {
						t.Fatalf("no hit node at %d,%d", x, y)
					}
					continue
				}
				if h != in.Box {
					t.Fatalf("hit node at %d,%d is %s, inspect says %s", x, y, LayoutPath(f, h), in.Path)
				}
				b := root
				for _, k := range id.path {
					if !k.row {
						b = b.Children[k.index]
						continue
					}
					var next *layout.Box
					for _, c := range b.Children {
						if rowWidget(c) != nil && c.Index == k.index && c.Key == k.key {
							next = c
						}
					}
					if next == nil {
						t.Fatalf("identity at %d,%d names row %d (%q), which %s does not lay out", x, y, k.index, k.key, LayoutPath(f, b))
					}
					b = next
				}
				if b != h || id.layer != root.Src {
					t.Fatalf("identity at %d,%d does not lead to the hit node", x, y)
				}
			}
		}
	}
}

// 44. The mouse attribute: a missing path is B002 and keeps the mouse
// off; a version="1" document has no mouse.
func TestMouseAttributeFrame(t *testing.T) {
	a := doc(t, `<tui version="2" mouse="nope"><screen id="s"><text>x</text></screen></tui>`)
	f := a.Frame(10, 2)
	if f.Mouse || !strings.Contains(diagText(f.Diags), "B002") {
		t.Errorf("missing path: mouse %v, diags %s", f.Mouse, diagText(f.Diags))
	}
	for src, want := range map[string]bool{
		`<tui version="2" mouse="true"><screen id="s"/></tui>`:  true,
		`<tui version="2" mouse="false"><screen id="s"/></tui>`: false,
		`<tui version="2" mouse="!off"><screen id="s"/></tui>`:  true,
		`<tui version="2"><screen id="s"/></tui>`:               false,
		`<tui version="1" mouse="true"><screen id="s"/></tui>`:  false,
	} {
		a := doc(t, src)
		_ = a.Bind("off", false)
		if got := a.Frame(10, 2).Mouse; got != want {
			t.Errorf("%s: mouse %v, want %v", src, got, want)
		}
	}
}

const modeDoc = `<tui version="2" mouse="m">
<keymap><bind keys="q" action="quit"/></keymap>
<screen id="s"><list id="l" each="rows as r" key="r" on:select="pick"><item><text>{r}</text></item></list></screen>
</tui>`

// 44. In a session the mouse modes follow the frame's mouse value: on
// before the first frame's bytes (outside synchronized output), off when
// the value turns false, and off in the leave sequence only while on;
// mode 1003 is never written. Mouse events act in arrival order with
// keys, and TUIMARK_LOG records the events and the mode changes.
func TestSessionMouseModes(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log")
	lg, err := openRunLog(logPath, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	sess := &termSession{sync: syncOn, log: lg}
	r := startSession(t, modeDoc, sess)
	_ = r.app.Bind("m", true)
	r.waitOut(t, "a")
	out := r.out.String()
	on := strings.Index(out, mouseModesOn)
	first := strings.Index(out, syncBegin)
	if on < 0 || first < 0 || on > first {
		t.Fatalf("modes on not before the first frame: %q", out)
	}
	// A click on row b (y = 1) selects it.
	r.send(t, "\x1b[<0;1;2M\x1b[<0;1;2m")
	if ev := r.next(t); ev.Action != "pick" || ev.Keys["r"] != "b" {
		t.Fatalf("click event %+v", ev)
	}
	_ = r.app.Set("m", false)
	r.waitOut(t, mouseModesOff)
	// With the modes off a click (in flight) does nothing; a key still acts.
	r.send(t, "\x1b[<0;1;1M\x1b[<0;1;1m")
	r.send(t, "\x1b[A")
	if ev := r.next(t); ev.Action != "pick" || ev.Keys["r"] != "a" {
		t.Fatalf("after the modes went off: %+v", ev)
	}
	r.send(t, "q")
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	lg.end("quit")
	out = r.out.String()
	leave := out[strings.LastIndex(out, "\x1b[?2004l"):]
	if strings.Contains(out, "\x1b[?1003h") || strings.Count(out, mouseModesOff) != 1 || strings.Contains(leave, mouseModesOff) {
		t.Errorf("modes: %q", out)
	}
	recs := readLog(t, logPath)
	var got []string
	for _, rec := range recs {
		switch rec["ev"] {
		case "mode":
			got = append(got, fmt.Sprintf("mode %v", rec["mouse"]))
		case "mouse":
			got = append(got, fmt.Sprintf("mouse %v %v,%v", rec["kind"], rec["x"], rec["y"]))
		}
	}
	if s := strings.Join(got, " | "); s != "mode true | mouse press 0,1 | mouse release 0,1 | mode false" {
		t.Errorf("log records: %s", s)
	}

	// Leaving while the modes are on turns them off before 2004.
	r = startSession(t, modeDoc, &termSession{})
	_ = r.app.Bind("m", true)
	r.waitOut(t, "a")
	r.send(t, "q")
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	out = r.out.String()
	if !strings.HasSuffix(out, mouseModesOff+leaveScreen) {
		t.Errorf("leave with the modes on: %q", out[max(0, len(out)-60):])
	}
}

// 44 (signal restore path): the leave sequence the restore writes turns
// the modes off while they are on.
func TestLeaveTurnsMouseOff(t *testing.T) {
	s := &termSession{}
	if s.leave() != leaveScreen {
		t.Error("leave with the modes off")
	}
	if s.mouseChange(true) != mouseModesOn || s.mouseChange(true) != "" {
		t.Error("mouseChange on")
	}
	s.setGrapheme()
	if s.leave() != graphemeOff+mouseModesOff+leaveScreen {
		t.Errorf("leave %q", s.leave())
	}
	if s.mouseChange(false) != mouseModesOff || s.leave() != graphemeOff+leaveScreen {
		t.Error("mouseChange off")
	}
}

// gridLine is row y of a fresh dump of a at the mouse fixture size, with
// its trailing spaces trimmed.
func gridLine(a *App, y int) string {
	return strings.TrimRight(a.Dump(mouseW, mouseH, false).Grid[y], " ")
}

// 46 (regression, review of 0.2b, SPEC v0.2b §8.5 wheel rule 3): the
// wheel moves a viewport without an id as it moves one with an id. Its
// offset is kept under its element: it survives frames, is clamped, is
// not shared with another viewport without an id, stays with the element
// when a sibling before it comes and goes, and, inside an each template,
// stays with the element's key when the array is reordered.
func TestMouseWheelViewportWithoutID(t *testing.T) {
	a := doc(t, `<tui version="2" mouse="true"><screen id="s"><col>
<text if="extra">extra</text>
<scroll style="height: 2"><col><text>s0</text><text>s1</text><text>s2</text><text>s3</text></col></scroll>
<scroll style="height: 2"><col><text>u0</text><text>u1</text><text>u2</text></col></scroll>
<col each="grp as g" key="g"><scroll style="height: 1"><col><text>{g}0</text><text>{g}1</text></col></scroll></col>
</col></screen></tui>`)
	bindJSON(t, a, `{"extra": false, "grp": ["a", "b"]}`)
	if ds := a.Validate(); len(ds) > 0 {
		t.Fatalf("fixture diagnostics: %v", ds)
	}
	settleFrame(a)
	for i := 0; i < 3; i++ {
		if evs := wheelAt(a, 0, 0, 1); len(evs) != 0 {
			t.Errorf("wheel over a scroll fired %v", evs)
		}
	}
	wheelAt(a, 0, 5, 1)
	want := func(when string, lines map[int]string) {
		t.Helper()
		for y, w := range lines {
			if got := gridLine(a, y); got != w {
				t.Errorf("%s: row %d is %q, want %q", when, y, got, w)
			}
		}
	}
	want("three wheels down (clamped at 2) and one over b", map[int]string{0: "s2", 1: "s3", 2: "u0", 4: "a0", 5: "b1"})
	_ = a.Set("extra", true)
	settleFrame(a)
	want("a sibling before it appears", map[int]string{0: "extra", 1: "s2", 3: "u0", 5: "a0", 6: "b1"})
	wheelAt(a, 0, 1, -1)
	want("one wheel up", map[int]string{1: "s1", 2: "s2", 3: "u0"})
	_ = a.Set("grp", []any{"b", "a"})
	settleFrame(a)
	want("the each array reordered", map[int]string{5: "b1", 6: "a0"})
}

// 45 (regression, review of 0.2b, SPEC v0.2b §8.5 hit test): an open
// modal that does not paint its rect (visibility: hidden) still traps the
// mouse. A click or a wheel report inside its rect, over a screen node
// that shows through, does nothing, and inspect --at names the modal
// there; a visible child of the modal still takes clicks.
func TestMouseHiddenModalTrapsEvents(t *testing.T) {
	a := doc(t, `<tui version="2" mouse="true"><screen id="s"><col>
<box style="height: 8"/>
<button id="under" label="under-button-wide-enough-to-cross" on:click="under" on:focus="ufocus"/>
<list id="l" each="rows as r" key="r" on:select="pick" style="height: 3"><item><text>{r}-row-wide-enough-to-cross-the-modal</text></item></list>
</col>
<modal id="m" open="true" style="width: 20; height: 6; visibility: hidden"><button id="mb" label="ok" style="width: 6; visibility: visible" on:click="ok"/></modal>
</screen></tui>`)
	bindJSON(t, a, `{"rows": ["x", "y", "z"]}`)
	if ds := a.Validate(); len(ds) > 0 {
		t.Fatalf("fixture diagnostics: %v", ds)
	}
	settleFrame(a)
	f := live(a)
	m := f.Modals[0]
	var onUnder, onList [][2]int
	for y := m.Y; y < m.Y+m.H; y++ {
		for x := m.X; x < m.X+m.W; x++ {
			switch dumpOwner(f, x, y) {
			case "under":
				onUnder = append(onUnder, [2]int{x, y})
			case "l":
				onList = append(onList, [2]int{x, y})
			}
			if h := HitNode(f, x, y); dumpOwner(f, x, y) != "mb" && h != m {
				t.Errorf("hit node at %d,%d is %s, want the modal", x, y, LayoutPath(f, h))
			}
		}
	}
	if len(onUnder) == 0 || len(onList) == 0 {
		t.Fatalf("the fixture has no screen cell inside the modal (button %d, list %d)", len(onUnder), len(onList))
	}
	for _, c := range onUnder {
		if got := actions(clickAt(a, c[0], c[1])); got != "" || a.Focus() == "under" {
			t.Fatalf("click at %v under the hidden modal fired %q (focus %q)", c, got, a.Focus())
		}
	}
	for _, c := range onList {
		if got := actions(wheelAt(a, c[0], c[1], 1)); got != "" || listIndex(a, "l") != 0 {
			t.Fatalf("wheel at %v under the hidden modal fired %q (cursor %d)", c, got, listIndex(a, "l"))
		}
	}
	mb := live(a).ByID["mb"]
	if got := actions(clickAt(a, mb.X+1, mb.Y)); !strings.HasSuffix(got, "ok") {
		t.Errorf("click on the modal's visible button fired %q", got)
	}
}

// dumpOwner is the cell-owner id of cell (x, y) of f.
func dumpOwner(f *Frame, x, y int) string {
	return dump.CellAt(f.Grid, x, y).ID
}

// 45 (regression, review of 0.2b, SPEC v0.2b §8.5 step 4): a row's hit
// identity is its row, not its place in the laid-out tree. A press on a
// table row, wheel reports that move the table's offset so that another
// row takes that cell, and a release there do nothing; a list row that the
// host reorders under the pointer between press and release does nothing
// either; a press and a release on the same row still click it.
func TestMouseRowIdentityIsTheRow(t *testing.T) {
	a := doc(t, `<tui version="2" mouse="true"><screen id="s" focus="#tb"><col>
<table id="tb" each="rows as r" key="r.id" on:select="tpick" style="height: 4"><column title="N" width="6">{r.name}</column></table>
<list id="l" each="items as i" key="i" on:select="lpick" style="height: 3"><item><text>{i}</text></item></list>
</col></screen></tui>`)
	bindJSON(t, a, `{"rows": [{"id": 1, "name": "r1"}, {"id": 2, "name": "r2"}, {"id": 3, "name": "r3"}, {"id": 4, "name": "r4"}, {"id": 5, "name": "r5"}, {"id": 6, "name": "r6"}], "items": ["x", "y", "z"]}`)
	if ds := a.Validate(); len(ds) > 0 {
		t.Fatalf("fixture diagnostics: %v", ds)
	}
	settleFrame(a)
	r0 := row(live(a), "tb", 0)
	x, y := r0.X, r0.Y
	a.HandleMouse(Mouse{Kind: MousePress, X: x, Y: y})
	for i := 0; i < 3; i++ {
		wheelAt(a, x, y, 1)
	}
	if got := row(live(a), "tb", 0); got.Index != 1 || got.Y != y {
		t.Fatalf("after three wheels the first slot holds row %d at y %d, want row 1 at %d", got.Index, got.Y, y)
	}
	if evs, _ := a.HandleMouse(Mouse{Kind: MouseRelease, X: x, Y: y}); len(evs) != 0 || listIndex(a, "tb") != 3 {
		t.Errorf("press on r1 and release on r2 fired %v (cursor %d, want 3)", evs, listIndex(a, "tb"))
	}
	if evs := clickAt(a, x, y); actions(evs) != "tpick" || listIndex(a, "tb") != 1 {
		t.Errorf("a click on r2 fired %q (cursor %d, want 1)", actions(evs), listIndex(a, "tb"))
	}
	l0 := row(live(a), "l", 0)
	a.HandleMouse(Mouse{Kind: MousePress, X: l0.X, Y: l0.Y})
	_ = a.Set("items", []any{"y", "x", "z"})
	settleFrame(a)
	if evs, _ := a.HandleMouse(Mouse{Kind: MouseRelease, X: l0.X, Y: l0.Y}); len(evs) != 0 {
		t.Errorf("press on x and release on y (reordered under it) fired %v", evs)
	}
}
