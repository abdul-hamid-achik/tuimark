package host

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/term"
)

// syncBuf is an io.Writer the test can read while the loop writes.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type loopRig struct {
	app  *App
	in   *io.PipeWriter
	out  *syncBuf
	done chan error
	evs  chan Event
}

const loopDoc = `<tui version="1">
<keymap><bind keys="q" action="quit"/></keymap>
<screen id="s" focus="#q">
  <input id="q" bind="text" on:change="chg" on:submit="go" on:focus="hello"/>
  <list id="l" each="rows as r" key="r" on:select="pick"><item><text>{r}</text></item></list>
  <text>[{text}]</text>
</screen>
</tui>`

// startLoop runs Loop on a pipe; handlers for every action in the document
// forward their events to rig.evs (quit is left to the built-in).
func startLoop(t *testing.T, setup func(a *App)) *loopRig {
	t.Helper()
	a := doc(t, loopDoc)
	_ = a.Bind("", map[string]any{"text": "", "rows": []any{"a", "b"}})
	rig := &loopRig{app: a, out: &syncBuf{}, done: make(chan error, 1), evs: make(chan Event, 64)}
	for _, act := range []string{"chg", "go", "hello", "pick"} {
		a.On(act, func(ev Event) error { rig.evs <- ev; return nil })
	}
	if setup != nil {
		setup(a)
	}
	pr, pw := io.Pipe()
	rig.in = pw
	go func() { rig.done <- a.Loop(pr, rig.out, func() (int, int) { return 30, 6 }, nil) }()
	t.Cleanup(func() { pw.Close() })
	return rig
}

func (r *loopRig) send(t *testing.T, s string) {
	t.Helper()
	if _, err := io.WriteString(r.in, s); err != nil {
		t.Fatal(err)
	}
}

func (r *loopRig) next(t *testing.T) Event {
	t.Helper()
	select {
	case ev := <-r.evs:
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for an event")
	}
	return Event{}
}

func (r *loopRig) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-r.done:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("Loop did not return")
	}
	return nil
}

// waitScreen waits until the last frame the loop drew contains want (the
// output stream only carries diffs).
func (r *loopRig) waitScreen(t *testing.T, want string) {
	t.Helper()
	screen := func() string {
		r.app.mu.Lock()
		defer r.app.mu.Unlock()
		if r.app.last == nil || r.app.last.Grid == nil {
			return ""
		}
		return strings.Join(r.app.last.Grid.Lines(), "\n")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(screen(), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("screen never showed %q:\n%s", want, screen())
}

func TestLoopTypingEventsAndQuit(t *testing.T) {
	rig := startLoop(t, nil)
	if ev := rig.next(t); ev.Action != "hello" || ev.Source != "q" {
		t.Fatalf("initial on:focus = %+v", ev)
	}
	// One key per read (typing): one change event per key.
	for _, want := range []string{"h", "hi"} {
		rig.send(t, want[len(want)-1:])
		if ev := rig.next(t); ev.Action != "chg" || ev.Value != want {
			t.Fatalf("change event = %+v, want value %q", ev, want)
		}
	}
	rig.send(t, "\r")
	if ev := rig.next(t); ev.Action != "go" || ev.Source != "q" || ev.Value != "hi" {
		t.Fatalf("submit = %+v", ev)
	}
	rig.waitScreen(t, "[hi]")
	// Keys in one read are handled in order: tab moves focus to the list,
	// so the following arrow and q go to the list and the keymap.
	rig.send(t, "\t\x1b[B")
	if ev := rig.next(t); ev.Action != "pick" || ev.Source != "l" || ev.Keys["r"] != "b" {
		t.Fatalf("select = %+v", ev)
	}
	rig.send(t, "q")
	if err := rig.wait(t); err != nil {
		t.Fatalf("built-in quit returns nil, got %v", err)
	}
	out := rig.out.String()
	if !strings.HasPrefix(out, "\x1b[?1049h") || !strings.HasSuffix(out, "\x1b[?1049l") {
		t.Errorf("alternate screen not entered/left: %q … %q", out[:12], out[len(out)-12:])
	}
	if got, _ := rig.app.Get("text"); got != "hi" {
		t.Errorf("text = %v", got)
	}
}

func TestLoopQuitHandler(t *testing.T) {
	quits := 0
	rig := startLoop(t, func(a *App) {
		a.On("quit", func(Event) error {
			quits++
			if quits == 1 {
				return nil // the handler decides: stay
			}
			return ErrQuit
		})
	})
	rig.next(t) // hello
	rig.send(t, "\t")
	rig.send(t, "q")
	time.Sleep(50 * time.Millisecond)
	select {
	case err := <-rig.done:
		t.Fatalf("Loop returned %v although the quit handler returned nil", err)
	default:
	}
	rig.send(t, "\x03") // ctrl+c is the built-in quit
	if err := rig.wait(t); err != nil || quits != 2 {
		t.Fatalf("ErrQuit: err=%v quits=%d", err, quits)
	}
}

func TestLoopHandlerError(t *testing.T) {
	boom := errors.New("boom")
	rig := startLoop(t, func(a *App) {
		a.On("go", func(Event) error { return boom })
	})
	rig.next(t)
	rig.send(t, "\r")
	if err := rig.wait(t); !errors.Is(err, boom) {
		t.Fatalf("Loop = %v, want the handler error", err)
	}
}

// Keys read right before EOF are handled before the loop stops.
func TestLoopEOFDrainsKeys(t *testing.T) {
	for i := 0; i < 20; i++ {
		rig := startLoop(t, nil)
		rig.send(t, "abc")
		rig.in.Close()
		if err := rig.wait(t); err != nil {
			t.Fatalf("EOF returns nil, got %v", err)
		}
		if got, _ := rig.app.Get("text"); got != "abc" {
			t.Fatalf("run %d: text = %v, want abc", i, got)
		}
	}
}

func TestLoopReadError(t *testing.T) {
	a := doc(t, loopDoc)
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- a.Loop(pr, io.Discard, func() (int, int) { return 10, 3 }, nil) }()
	bad := errors.New("tty gone")
	pw.CloseWithError(bad)
	select {
	case err := <-done:
		if !errors.Is(err, bad) {
			t.Errorf("Loop = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Loop did not return")
	}
}

func TestLoopRedrawsOnSetAndResize(t *testing.T) {
	a := doc(t, loopDoc)
	_ = a.Bind("text", "")
	pr, pw := io.Pipe()
	defer pw.Close()
	out := &syncBuf{}
	var mu sync.Mutex
	cols := 30
	size := func() (int, int) {
		mu.Lock()
		defer mu.Unlock()
		return cols, 6
	}
	resize := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() { done <- a.Loop(pr, out, size, resize) }()
	rig := &loopRig{app: a, out: out, in: pw, done: done}
	rig.waitScreen(t, "[]")
	if err := a.Set("text", "from another goroutine"); err != nil {
		t.Fatal(err)
	}
	rig.waitScreen(t, "[from another goroutine]")

	before := strings.Count(out.String(), "\x1b[2J")
	mu.Lock()
	cols = 40
	mu.Unlock()
	resize <- struct{}{}
	deadline := time.Now().Add(3 * time.Second)
	for strings.Count(out.String(), "\x1b[2J") == before {
		if time.Now().After(deadline) {
			t.Fatal("a resize repaints the whole screen")
		}
		time.Sleep(5 * time.Millisecond)
	}
	pw.Close()
	if err := rig.wait(t); err != nil {
		t.Fatal(err)
	}
}

type failWriter struct{ n int }

func (f *failWriter) Write(p []byte) (int, error) {
	f.n++
	if f.n > 1 { // the alternate-screen preamble succeeds, the first frame fails
		return 0, errors.New("write failed")
	}
	return len(p), nil
}

func TestLoopWriteError(t *testing.T) {
	a := doc(t, loopDoc)
	pr, pw := io.Pipe()
	defer pw.Close()
	if err := a.Loop(pr, &failWriter{}, func() (int, int) { return 10, 3 }, nil); err == nil || err.Error() != "write failed" {
		t.Errorf("Loop = %v", err)
	}
}

func TestRunNeedsATerminal(t *testing.T) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("stdin is a terminal; Run would take it over")
	}
	a := doc(t, loopDoc)
	if err := a.Run(io.Discard); err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Errorf("Run without a terminal = %v", err)
	}
}

// Keys delivered in one read see each other's effects: after tab moves
// focus, a keymap row with when="#l:focus" matches the next key.
func TestLoopKeysInOneReadSeeFocusChanges(t *testing.T) {
	a := doc(t, `<tui version="1">
<keymap><bind keys="enter" action="open" when="#l:focus"/></keymap>
<screen id="s" focus="#q">
  <input id="q"/>
  <list id="l" each="rows as r" key="r"><item><text>{r}</text></item></list>
</screen>
</tui>`)
	_ = a.Bind("rows", []any{"a", "b"})
	evs := make(chan Event, 4)
	a.On("open", func(ev Event) error { evs <- ev; return nil })
	pr, pw := io.Pipe()
	defer pw.Close()
	done := make(chan error, 1)
	go func() { done <- a.Loop(pr, io.Discard, func() (int, int) { return 20, 4 }, nil) }()
	if _, err := io.WriteString(pw, "\t\r"); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-evs:
		if ev.Source != "l" || ev.Keys["r"] != "a" {
			t.Errorf("open = %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("enter after tab in the same read did not reach the keymap row")
	}
	pw.Close()
	<-done
}

// chunkReader returns data at most n bytes per Read, then EOF: a paste
// the terminal hands over in pieces that cut runes and sequences.
type chunkReader struct {
	data []byte
	n    int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	k := min(c.n, len(c.data), len(p))
	copy(p, c.data[:k])
	c.data = c.data[k:]
	return k, nil
}

// Round 2, finding 4: a fast paste keeps every character. Multi-byte runes
// and escape sequences cut at read boundaries are completed by the next
// read instead of being dropped or typed as literal bytes.
func TestLoopPasteAcrossReads(t *testing.T) {
	saved := escTimeout
	escTimeout = time.Minute // only the next read may complete a cut sequence
	defer func() { escTimeout = saved }()
	var paste strings.Builder
	var want strings.Builder
	paste.WriteString("a")
	want.WriteString("a")
	for i := 0; i < 300; i++ {
		paste.WriteString("é€")
		want.WriteString("é€")
	}
	for i := 0; i < 60; i++ {
		paste.WriteString("xy\x1b[A") // up: the input ignores it
		want.WriteString("xy")
	}
	paste.WriteString("ñ\x1bOB😀\r")
	want.WriteString("ñ😀")
	for _, n := range []int{1, 2, 3, 5, 7, 4096} {
		a := doc(t, `<tui version="1"><screen id="main" focus="#b"><input id="b" on:submit="submit"/></screen></tui>`)
		var got []string
		a.On("submit", func(ev Event) error { got = append(got, ev.Value.(string)); return nil })
		err := a.Loop(&chunkReader{data: []byte(paste.String()), n: n}, io.Discard, func() (int, int) { return 30, 3 }, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != want.String() {
			v := ""
			if len(got) > 0 {
				v = got[0]
			}
			t.Errorf("%d-byte reads: %d submits, value has %d runes, want %d (%q…)", n, len(got), len([]rune(v)), len([]rune(want.String())), string([]rune(v)[:min(len([]rune(v)), 30)]))
		}
	}
}

// A lone ESC at the end of a read is still the esc key once nothing
// follows it within escTimeout; it closes the modal through on:escape.
func TestLoopLoneEscIsEscKey(t *testing.T) {
	a := doc(t, `<tui version="1"><screen id="s"><input id="q"/><modal id="m" open="true" on:escape="close"><button id="ok">ok</button></modal></screen></tui>`)
	evs := make(chan Event, 4)
	a.On("close", func(ev Event) error { evs <- ev; return nil })
	pr, pw := io.Pipe()
	defer pw.Close()
	done := make(chan error, 1)
	go func() { done <- a.Loop(pr, io.Discard, func() (int, int) { return 30, 8 }, nil) }()
	if _, err := io.WriteString(pw, "\x1b"); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-evs:
		if ev.Source != "m" {
			t.Errorf("on:escape = %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a lone ESC never became the esc key")
	}
	pw.Close()
	<-done
}
