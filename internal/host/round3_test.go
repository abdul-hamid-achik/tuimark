package host

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// pendingActions lists the queued lifecycle events' actions and clears them.
func pendingActions(a *App) string {
	var out []string
	for _, ev := range a.TakePending() {
		out = append(out, ev.Action)
	}
	return strings.Join(out, " ")
}

// Round 3, finding 1: a focus request to another screen is a no-op while a
// modal is open on the current screen, whichever node it targets: the
// screen does not switch, the modal stays open with focus inside it, and
// neither on:focus nor on:close fires. That holds when the target is the
// other screen's fallback focus, when the modal took focus through
// Set("@focus") in the tick it opened, and when the modal opens in the same
// tick as the request. Closing the modal in the same tick as the request
// lets it through.
func TestCrossScreenFocusStaysInOpenModal(t *testing.T) {
	src := `<tui version="1"><keymap>
  <bind keys="t" action="focus" to="#other"/>
</keymap>
<screen id="one" focus="#q"><text>SCREEN ONE</text><input id="q"/>
  <modal id="m" bind="open" on:close="closed"><text>MODAL</text><button id="ok" label="ok"/></modal>
</screen>
<screen id="two"><text>SCREEN TWO</text><input id="other" on:focus="f2"/></screen></tui>`
	stays := func(t *testing.T, a *App, what string) {
		t.Helper()
		f := a.Frame(40, 6)
		if g := strings.Join(f.Grid.Lines(), "\n"); !strings.HasPrefix(g, "SCREEN ONE") || !strings.Contains(g, "MODAL") {
			t.Errorf("%s: left screen one or closed the modal:\n%s", what, g)
		}
		if a.Focus() != "ok" || f.Focus != "ok" {
			t.Errorf("%s: focus %q (frame %q), want ok", what, a.Focus(), f.Focus)
		}
		if p := pendingActions(a); p != "" {
			t.Errorf("%s: pending %q", what, p)
		}
		// The modal still gives focus back to #q when it closes.
		_ = a.Set("open", false)
		a.Frame(40, 6)
		if a.Focus() != "q" {
			t.Errorf("%s: focus after the modal closes %q, want q", what, a.Focus())
		}
	}
	open := func(t *testing.T, focusInto bool) *App {
		t.Helper()
		a := doc(t, src)
		_ = a.Bind("open", false)
		a.Frame(40, 6)
		_ = a.Set("open", true)
		if focusInto {
			_ = a.Set("@focus", "#ok")
		}
		a.Frame(40, 6)
		a.TakePending()
		if a.Focus() != "ok" {
			t.Fatalf("focus %q, want ok", a.Focus())
		}
		return a
	}
	for _, focusInto := range []bool{false, true} {
		name := "fallback"
		if focusInto {
			name = "focused into the modal"
		}
		t.Run(name+"/keymap", func(t *testing.T) {
			a := open(t, focusInto)
			if evs := a.HandleKey(r('t')); len(evs) != 0 {
				t.Errorf("events %+v", evs)
			}
			stays(t, a, "keymap")
		})
		t.Run(name+"/Set", func(t *testing.T) {
			a := open(t, focusInto)
			_ = a.Set("@focus", "#other")
			stays(t, a, "Set(@focus)")
		})
	}
	t.Run("modal opened in the same tick", func(t *testing.T) {
		a := doc(t, src)
		_ = a.Bind("open", false)
		a.Frame(40, 6)
		a.TakePending()
		_ = a.Set("open", true)
		_ = a.Set("@focus", "#other")
		stays(t, a, "same tick")
	})
	t.Run("modal closed in the same tick", func(t *testing.T) {
		a := open(t, false)
		_ = a.Set("open", false)
		_ = a.Set("@focus", "#other")
		f := a.Frame(40, 6)
		if !strings.HasPrefix(f.Grid.Lines()[0], "SCREEN TWO") || a.Focus() != "other" {
			t.Errorf("closing the modal and focusing another screen: %q focus %q", f.Grid.Lines()[0], a.Focus())
		}
		if p := pendingActions(a); p != "closed" {
			t.Errorf("pending %q, want closed", p)
		}
	})
	// Dump and Validate see the same no-op and leave the request pending.
	a := open(t, false)
	_ = a.Set("@focus", "#other")
	if d := a.Dump(40, 6, false); !strings.HasPrefix(d.Grid[0], "SCREEN ONE") || d.Focus == nil || *d.Focus != "ok" {
		t.Errorf("dump: %q focus %v", d.Grid[0], d.Focus)
	}
	a.Validate()
	stays(t, a, "after Dump and Validate")
}

// Round 3, finding 2: an explicit focus move made in the same tick as a
// modal closes wins over the modal's focus restore when its target can
// take focus; when it cannot, focus goes where the restore sends it.
func TestFocusRequestWinsOverModalRestore(t *testing.T) {
	src := `<tui version="1"><keymap><bind keys="g" action="focus" to="#b"/></keymap>
<screen id="one" focus="#q"><input id="q"/><button id="b" label="b" on:focus="fb"/>
  <button id="off" label="off" disabled="true"/>
  <modal id="m" bind="open"><text>MODAL</text><button id="ok" label="ok" on:click="done"/></modal>
</screen></tui>`
	open := func(t *testing.T) *App {
		t.Helper()
		a := doc(t, src)
		_ = a.Bind("open", false)
		a.Frame(40, 8)
		_ = a.Set("open", true)
		a.Frame(40, 8)
		a.TakePending()
		if a.Focus() != "ok" {
			t.Fatalf("focus %q, want ok", a.Focus())
		}
		return a
	}
	for _, order := range []string{"close first", "focus first"} {
		a := open(t)
		if order == "close first" {
			_ = a.Set("open", false)
			_ = a.Set("@focus", "#b")
		} else {
			_ = a.Set("@focus", "#b")
			_ = a.Set("open", false)
		}
		f := a.Frame(40, 8)
		if a.Focus() != "b" || f.Focus != "b" {
			t.Errorf("%s: focus %q, want b", order, a.Focus())
		}
		if p := pendingActions(a); p != "" { // Set("@focus") does not fire on:focus
			t.Errorf("%s: pending %q", order, p)
		}
		a.Frame(40, 8)
		if a.Focus() != "b" {
			t.Errorf("%s: next frame moved focus to %q", order, a.Focus())
		}
	}
	// The handler path: the modal's button closes it and focuses #b.
	a := open(t)
	a.On("done", func(Event) error {
		_ = a.Set("open", false)
		return a.Set("@focus", "#b")
	})
	for _, ev := range a.HandleKey(named("enter")) {
		if _, err := a.Dispatch(ev); err != nil {
			t.Fatal(err)
		}
	}
	a.Frame(40, 8)
	if a.Focus() != "b" {
		t.Errorf("handler: focus %q, want b", a.Focus())
	}
	// A target that cannot take focus leaves the restore in charge.
	a = open(t)
	_ = a.Set("open", false)
	_ = a.Set("@focus", "#off")
	a.Frame(40, 8)
	if a.Focus() != "q" {
		t.Errorf("disabled target: focus %q, want q (the focus before the modal)", a.Focus())
	}
	// action="focus" while the modal closes in the same tick (the host
	// closed it from another goroutine): the target takes focus and its
	// on:focus fires once the frame confirms it.
	a = open(t)
	_ = a.Set("open", false)
	if evs := a.HandleKey(r('g')); len(evs) != 0 {
		t.Errorf("g: events before the frame %+v", evs)
	}
	a.Frame(40, 8)
	if a.Focus() != "b" {
		t.Errorf("action=focus: focus %q, want b", a.Focus())
	}
	if p := pendingActions(a); p != "fb" {
		t.Errorf("action=focus: pending %q, want fb", p)
	}
}

// Round 3, finding 3: focusing into a modal in the tick it opens still
// records the focus from before the modal, so closing it goes back there.
func TestFocusIntoModalKeepsItsReturnTarget(t *testing.T) {
	src := `<tui version="1"><keymap><bind keys="ctrl+g" action="focus" to="#cancel"/></keymap>
<screen id="one" focus="#q"><input id="q"/><button id="b" label="b"/>
  <modal id="m" bind="open"><text>MODAL</text><button id="ok" label="ok"/><button id="cancel" label="cancel"/></modal>
</screen></tui>`
	for _, into := range []string{"", "#cancel", "#ok", "#q", "key", "after"} {
		a := doc(t, src)
		_ = a.Bind("open", false)
		a.Frame(40, 8)
		press(a, named("tab"))
		if a.Focus() != "b" {
			t.Fatalf("focus %q, want b", a.Focus())
		}
		_ = a.Set("open", true)
		switch into {
		case "", "after":
		case "key":
			a.HandleKey(named("ctrl+g"))
		default:
			_ = a.Set("@focus", into)
		}
		a.Frame(40, 8)
		if into == "after" {
			_ = a.Set("@focus", "#cancel")
			a.Frame(40, 8)
		}
		inModal := a.Focus()
		if inModal != "ok" && inModal != "cancel" {
			t.Errorf("into %q: focus %q is outside the modal", into, inModal)
		}
		_ = a.Set("open", false)
		a.Frame(40, 8)
		if a.Focus() != "b" {
			t.Errorf("into %q: focus after close %q, want b (the focus before the modal)", into, a.Focus())
		}
	}
}

// Round 3, finding 4: stacked modals that close in the same frame restore
// focus to where it was before the first of them opened, and fire on:close
// from the top down, every time. A modal that closes under another one
// hands its return target to the one above.
func TestStackedModalsCloseDeterministically(t *testing.T) {
	src := `<tui version="1">
<screen id="one" focus="#q"><input id="q"/><button id="b" label="b"/>
  <modal id="m1" bind="o1" on:close="c1" on:open="p1"><button id="ok1" label="ok1"/></modal>
  <modal id="m2" bind="o2" on:close="c2" on:open="p2"><button id="ok2" label="ok2"/></modal>
</screen></tui>`
	stack := func(t *testing.T) *App {
		t.Helper()
		a := doc(t, src)
		_ = a.Bind("", map[string]any{"o1": false, "o2": false})
		a.Frame(40, 8)
		press(a, named("tab"))
		_ = a.Set("o1", true)
		a.Frame(40, 8)
		_ = a.Set("o2", true)
		a.Frame(40, 8)
		if a.Focus() != "ok2" {
			t.Fatalf("focus %q, want ok2", a.Focus())
		}
		if p := pendingActions(a); p != "p1 p2" {
			t.Fatalf("on:open %q", p)
		}
		return a
	}
	for i := 0; i < 50; i++ {
		a := stack(t)
		_ = a.Set("o1", false)
		_ = a.Set("o2", false)
		a.Frame(40, 8)
		if a.Focus() != "b" {
			t.Fatalf("run %d: focus %q after both close, want b", i, a.Focus())
		}
		if p := pendingActions(a); p != "c2 c1" {
			t.Fatalf("run %d: on:close order %q, want c2 c1", i, p)
		}
	}
	// Opening both in one frame: on:open in document order.
	a := doc(t, src)
	_ = a.Bind("", map[string]any{"o1": true, "o2": true})
	a.Frame(40, 8)
	if p := pendingActions(a); p != "p1 p2" {
		t.Errorf("on:open order %q", p)
	}
	// The lower modal closes first: focus stays in the top one, and closing
	// that one later goes back to #b, not into the closed modal.
	a = stack(t)
	_ = a.Set("o1", false)
	a.Frame(40, 8)
	if a.Focus() != "ok2" || pendingActions(a) != "c1" {
		t.Errorf("lower modal closed: focus %q", a.Focus())
	}
	_ = a.Set("o2", false)
	a.Frame(40, 8)
	if a.Focus() != "b" {
		t.Errorf("after both closed one by one: focus %q, want b", a.Focus())
	}
	// The top one closes first: back to the lower modal, then to #b.
	a = stack(t)
	_ = a.Set("o2", false)
	a.Frame(40, 8)
	if a.Focus() != "ok1" {
		t.Errorf("top modal closed: focus %q, want ok1", a.Focus())
	}
	_ = a.Set("o1", false)
	a.Frame(40, 8)
	if a.Focus() != "b" {
		t.Errorf("then the lower one: focus %q, want b", a.Focus())
	}
}

// Round 3, finding 5: lifecycle events a new terminal size causes (a
// modal @media opens, a focused node @media hides) run right away, with
// no key or Set after the resize, whether the size change comes as a
// resize notification or through the size poll.
func TestLoopResizeRunsLifecycleEvents(t *testing.T) {
	const withModal = `<tui version="1"><style>
#m { display: none; }
@media (max-cols: 40) { #q { display: none; } #m { display: flex; } }
</style>
<screen id="s" focus="#q"><input id="q" on:focus="fq"/><button id="l" label="l" on:focus="fl"/>
<modal id="m" open="true" on:open="o"><button id="ok" label="ok" on:focus="fok"/></modal></screen></tui>`
	const noModal = `<tui version="1"><style>@media (max-cols: 40) { #q { display: none; } }</style>
<screen id="s" focus="#q"><input id="q" on:focus="fq"/><button id="l" label="l" on:focus="fl"/></screen></tui>`
	for _, c := range []struct {
		name, src string
		want      []string
	}{
		{"modal opens", withModal, []string{"fok", "o"}},
		{"focus falls back", noModal, []string{"fl"}},
	} {
		for _, notify := range []bool{true, false} {
			name := c.name + "/poll"
			if notify {
				name = c.name + "/resize"
			}
			t.Run(name, func(t *testing.T) {
				a := doc(t, c.src)
				evs := make(chan string, 16)
				for _, act := range []string{"fq", "fl", "o", "fok"} {
					a.On(act, func(Event) error { evs <- act; return nil })
				}
				var mu sync.Mutex
				cols := 80
				size := func() (int, int) {
					mu.Lock()
					defer mu.Unlock()
					return cols, 10
				}
				var resize chan struct{}
				if notify {
					resize = make(chan struct{}, 1)
				}
				pr, pw := io.Pipe()
				defer pw.Close()
				done := make(chan error, 1)
				go func() { done <- a.Loop(pr, io.Discard, size, resize) }()
				next := func() string {
					select {
					case ev := <-evs:
						return ev
					case <-time.After(3 * time.Second):
						return "(none)"
					}
				}
				if ev := next(); ev != "fq" {
					t.Fatalf("initial focus event %q", ev)
				}
				mu.Lock()
				cols = 40
				mu.Unlock()
				if notify {
					resize <- struct{}{}
				}
				var got []string
				for range c.want {
					got = append(got, next())
				}
				if strings.Join(got, " ") != strings.Join(c.want, " ") {
					t.Errorf("events after the resize %v, want %v", got, c.want)
				}
				pw.Close()
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// Round 3, finding 6 (unit side): the signal watcher reports the first
// signal it caught when the loop is done, so Run can return a
// *SignalError for a signal the loop never took (TestRunSignals/quit
// covers Run itself).
func TestWatchSignalsReportsFirstSignal(t *testing.T) {
	run := func(send []os.Signal) os.Signal {
		caught, toLoop := make(chan os.Signal, 4), make(chan os.Signal, 1)
		loopDone, res := make(chan struct{}), make(chan os.Signal, 1)
		go func() {
			res <- watchSignals(caught, toLoop, loopDone, func() {}, time.Hour, func(os.Signal) {})
		}()
		for _, s := range send {
			caught <- s
		}
		if len(send) > 0 {
			<-toLoop // the watcher took it
		}
		close(loopDone)
		select {
		case s := <-res:
			return s
		case <-time.After(3 * time.Second):
			t.Fatal("watchSignals did not return")
		}
		return nil
	}
	if s := run([]os.Signal{syscall.SIGTERM}); s != syscall.SIGTERM {
		t.Errorf("first signal = %v, want SIGTERM", s)
	}
	if s := run(nil); s != nil {
		t.Errorf("no signal: got %v", s)
	}
}

// Round 3, finding 7: printable keys in a row that go to the focused input
// are one edit with one on:change carrying the final value (a paste);
// other keys split the run and keep their meaning.
func TestHandleKeyRunTypesARunAsOneEdit(t *testing.T) {
	a := doc(t, `<tui version="1"><keymap><bind keys="x" action="ex"/></keymap>
<screen id="s" focus="#q"><input id="q" bind="text" on:change="chg"/><input id="plain"/>
<input id="off" disabled="true"/><list id="l" each="rows as r" key="r"><item><text>{r}</text></item></list></screen></tui>`)
	_ = a.Bind("", map[string]any{"text": "", "rows": []any{"a", "b"}})
	a.Frame(40, 6)
	keys := append(typed("ab c"), named("left"), r('d'), named("home"))
	keys = append(keys, typed("xé😀")...)
	var got []string
	for len(keys) > 0 {
		evs, n := a.HandleKeyRun(keys)
		if n < 1 {
			t.Fatalf("HandleKeyRun used %d keys", n)
		}
		keys = keys[n:]
		for _, ev := range evs {
			got = append(got, ev.Value.(string))
		}
	}
	if want := "ab c|ab dc|xé😀ab dc"; strings.Join(got, "|") != want {
		t.Errorf("change values %q, want %q", strings.Join(got, "|"), want)
	}
	if v := get(t, a, "text"); v != "xé😀ab dc" || a.inputs["q"].cursor != 3 {
		t.Errorf("value %q cursor %d", v, a.inputs["q"].cursor)
	}
	// An unbound input without on:change: same edit, no events.
	_ = a.Set("@focus", "#plain")
	a.Frame(40, 6)
	if evs, n := a.HandleKeyRun(typed("hello")); len(evs) != 0 || n != 5 || a.inputs["plain"].value != "hello" {
		t.Errorf("plain input: %+v %d %q", evs, n, a.inputs["plain"].value)
	}
	// Anything but an enabled input takes one key at a time: the keymap
	// sees each one.
	for _, id := range []string{"#l", "#off"} {
		_ = a.Set("@focus", id)
		a.Frame(40, 6)
		if evs, n := a.HandleKeyRun(typed("xx")); n != 1 || len(evs) != 1 || evs[0].Action != "ex" {
			t.Errorf("%s: %+v %d", id, evs, n)
		}
	}
}

// Round 3, finding 7: a paste costs one edit and one frame per read, not
// one per character, so a large paste is fast even with on:change and a
// long list on screen (it used to take seconds: a full frame per key),
// and typing into the inbox search is linear in the paste.
func TestLoopPasteIsFast(t *testing.T) {
	rows := make([]any, 200)
	for i := range rows {
		rows[i] = fmt.Sprintf("row %d", i)
	}
	rig := startLoop(t, func(a *App) { _ = a.Bind("rows", rows) })
	if ev := rig.next(t); ev.Action != "hello" {
		t.Fatalf("initial focus event %+v", ev)
	}
	paste := strings.Repeat("é", 5000) // 10KB
	start := time.Now()
	rig.send(t, paste+"\r")
	changes, last := 0, ""
	for {
		ev := rig.next(t)
		if ev.Action == "go" {
			if ev.Value != paste {
				t.Errorf("submitted %d runes, want 5000", len([]rune(ev.Value.(string))))
			}
			break
		}
		changes++
		last = ev.Value.(string)
	}
	if d := time.Since(start); d > slowdown*time.Second {
		t.Errorf("a 10KB paste with on:change took %v", d)
	}
	if changes > 10 || last != paste {
		t.Errorf("%d change events (one per read, not per key), last has %d runes", changes, len([]rune(last)))
	}

	for _, n := range []int{10_000, 100_000} {
		a := inbox(t)
		got := make(chan string, 1)
		a.On("search", func(ev Event) error { got <- ev.Value.(string); return nil })
		pr, pw := io.Pipe()
		done := make(chan error, 1)
		go func() { done <- a.Loop(pr, io.Discard, func() (int, int) { return 100, 30 }, nil) }()
		start := time.Now()
		if _, err := io.WriteString(pw, strings.Repeat("x", n)+"\r"); err != nil {
			t.Fatal(err)
		}
		select {
		case v := <-got:
			if len(v) != n {
				t.Errorf("%d: submitted %d bytes", n, len(v))
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%d: no submit", n)
		}
		limit := slowdown * time.Second
		if n > 10_000 {
			// Quadratic editing took ~17s here without the race detector.
			limit = slowdown * 2 * time.Second
		}
		if d := time.Since(start); d > limit {
			t.Errorf("a %d-byte paste into the inbox search took %v (limit %v)", n, d, limit)
		}
		pw.Close()
		<-done
	}
}

// BenchmarkPasteIntoInput types a 10KB paste into a bound input with
// on:change, the way the loop hands over one read.
func BenchmarkPasteIntoInput(b *testing.B) {
	keys := typed(strings.Repeat("abcdefghij", 1000))
	for i := 0; i < b.N; i++ {
		a, _ := Parse(strings.NewReader(`<tui version="1"><screen id="s" focus="#q"><input id="q" bind="text" on:change="chg"/></screen></tui>`))
		_ = a.Bind("text", "")
		a.Frame(40, 3)
		for rest := keys; len(rest) > 0; {
			_, n := a.HandleKeyRun(rest)
			rest = rest[n:]
		}
	}
}
