package host

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// leaveScreen resets attributes, shows the cursor, and leaves the
// alternate screen.
const leaveScreen = "\x1b[0m\x1b[?25h\x1b[?1049l"

// signalGrace is how long Run lets the loop return on its own after a stop
// signal (a handler may be running) before it ends the process with the
// signal's default action. Tests shorten or lengthen it.
var signalGrace = time.Second

// signalRestoreWait is how long a stop signal waits for the loop to return
// before the terminal is restored from the signal watcher instead (the
// loop is then busy in a handler).
const signalRestoreWait = 50 * time.Millisecond

// escTimeout is how long an ESC (or an unterminated escape sequence) at
// the end of a read waits for the rest of the sequence before it is
// decoded as it stands: a lone ESC is then the esc key. Tests lengthen it.
var escTimeout = 25 * time.Millisecond

// Run drives the app in the terminal: raw mode on stdin, the alternate
// screen on w, frame diffs as ANSI. It returns nil on quit.
//
// SIGTERM, SIGHUP, and SIGINT (SIGINT only from outside: raw mode turns
// ctrl+c into a key) stop Run like a quit that restores the terminal first
// (cooked mode, main screen, visible cursor), and Run then returns a
// non-nil error naming the signal, so the caller can exit with a failure
// status. That holds even when the loop was stopping anyway (a handler
// returned ErrQuit, or input ended) as the signal came in. A handler that
// is still running when the signal arrives cannot hold the terminal: it is
// restored right away, and if Run has not returned within signalGrace (one
// second), or a second signal arrives, the process ends with the signal's
// default action. When Run returns, nothing is left reading stdin.
func (a *App) Run(w io.Writer) (err error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return errors.New("tuimark: Run needs an interactive terminal on stdin (use Dump for headless output)")
	}
	// Catch the signals before raw mode; release them only after the
	// terminal is restored (defers run last-in, first-out).
	caught, release := stopSignals()
	// Runs after release: a signal caught after the watcher returned (the
	// loop was already stopping) is still reported, not dropped.
	defer func() {
		if err != nil {
			return
		}
		select {
		case s := <-caught:
			err = &SignalError{Signal: s}
		default:
		}
	}()
	defer release()
	old, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	out := &guardedWriter{w: w}
	var once sync.Once
	// restore puts the terminal back once, from whichever of the loop's
	// return and the signal watcher comes first; after it, frames are
	// dropped. leave also leaves the alternate screen (the loop does it on
	// its own way out).
	restore := func(leave bool) {
		once.Do(func() {
			_ = term.Restore(fd, old)
			s := ""
			if leave {
				s = leaveScreen
			}
			out.close(s)
		})
	}
	defer restore(false)
	sizeFd := fd
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		sizeFd = int(f.Fd())
	}
	size := func() (int, int) {
		c, r, err := term.GetSize(sizeFd)
		if err != nil || c <= 0 || r <= 0 {
			return 80, 24
		}
		return c, r
	}
	stop := make(chan struct{})
	defer close(stop)
	sigs := make(chan os.Signal, 1)
	loopDone := make(chan struct{})
	watched := make(chan os.Signal, 1)
	go func() {
		watched <- watchSignals(caught, sigs, loopDone, func() { restoreWithin(func() { restore(true) }, time.Second) }, signalGrace, reraise)
	}()
	err = a.loop(os.Stdin, out, size, resizeSignal(stop), sigs)
	close(loopDone)
	// A signal the loop did not take (it returned for a quit or EOF while
	// the signal was on its way) still makes Run report it.
	if first := <-watched; err == nil && first != nil {
		err = &SignalError{Signal: first}
	}
	return err
}

// guardedWriter serializes writes to the terminal so the signal watcher
// can restore it while the loop is running, and drops every write after
// close.
type guardedWriter struct {
	mu     sync.Mutex
	w      io.Writer
	closed bool
}

func (g *guardedWriter) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return len(p), nil
	}
	return g.w.Write(p)
}

// close writes final and drops later writes.
func (g *guardedWriter) close(final string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return
	}
	g.closed = true
	if final != "" {
		_, _ = io.WriteString(g.w, final)
	}
}

// restoreWithin runs restore, giving up waiting after d (a terminal whose
// output is stalled must not keep the process alive).
func restoreWithin(restore func(), d time.Duration) {
	done := make(chan struct{})
	go func() {
		restore()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
	}
}

// watchSignals handles the stop signals Run catches, outside the event
// goroutine (which may be blocked in a handler). The first signal goes to
// the loop, which returns a *SignalError as soon as it is between events.
// If the loop has not returned within signalRestoreWait, the terminal is
// restored from here; if it has not returned within grace, or a second
// signal arrives, die ends the process with the signal's default action.
// watchSignals returns when loopDone is closed, with the first signal it
// caught (nil for none), so Run reports a signal the loop never took.
func watchSignals(caught <-chan os.Signal, toLoop chan<- os.Signal, loopDone <-chan struct{}, restore func(), grace time.Duration, die func(os.Signal)) (first os.Signal) {
	var restoreAt, deadline <-chan time.Time
	for {
		select {
		case <-loopDone:
			return first
		case s := <-caught:
			if first != nil {
				restore()
				die(s)
				return first
			}
			first = s
			select {
			case toLoop <- s:
			default:
			}
			restoreAt = time.After(signalRestoreWait)
			deadline = time.After(grace)
		case <-restoreAt:
			restoreAt = nil
			select {
			case <-loopDone:
				return first
			default:
			}
			restore()
		case <-deadline:
			select {
			case <-loopDone:
				return first
			default:
			}
			restore()
			die(first)
			return first
		}
	}
}

// SignalError is what Run returns when a signal stopped it.
type SignalError struct{ Signal os.Signal }

func (e *SignalError) Error() string {
	return fmt.Sprintf("tuimark: stopped by signal (%v); the terminal was restored", e.Signal)
}

// Loop is the terminal event loop behind Run, with injectable input,
// output, size, and resize notifications (tests drive it with pipes).
func (a *App) Loop(in io.Reader, out io.Writer, size func() (int, int), resize <-chan struct{}) error {
	return a.loop(in, out, size, resize, nil)
}

// chunk is one read: raw bytes plus the read error. One channel carries
// both, so bytes read just before EOF are handled before the loop stops.
type chunk struct {
	data []byte
	err  error
}

// startReader reads in on its own goroutine until done is closed. It
// returns a channel closed when that goroutine has exited, or nil when the
// goroutine cannot be joined: a plain blocking reader (pipes that are not
// files, and every input on Windows) can only notice done after its
// current Read returns. On unix, a file (the terminal) is polled with a
// short timeout and read only when readable, so once done is closed the
// reader stops without consuming more input.
func startReader(in io.Reader, reads chan<- chunk, done <-chan struct{}) <-chan struct{} {
	ready := inputReady(in)
	exited := make(chan struct{})
	closed := func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}
	go func() {
		defer close(exited)
		buf := make([]byte, 4096)
		for {
			// Wait for input, noticing done at least every poll timeout.
			for ready != nil && !closed() && !ready() {
			}
			if closed() {
				return // never start a read once the loop has returned
			}
			n, err := in.Read(buf)
			c := chunk{err: err}
			if n > 0 {
				c.data = append([]byte(nil), buf[:n]...)
			}
			if (n > 0 || err != nil) && !closed() {
				select {
				case reads <- c:
				case <-done:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	if ready == nil {
		return nil
	}
	return exited
}

// loop is Loop plus sigs: a signal stops the loop (terminal restored by the
// deferred writes and Run's defers) and is returned as a *SignalError.
func (a *App) loop(in io.Reader, out io.Writer, size func() (int, int), resize <-chan struct{}, sigs <-chan os.Signal) error {
	io.WriteString(out, "\x1b[?1049h\x1b[?25l\x1b[H\x1b[2J")
	defer io.WriteString(out, leaveScreen)

	reads := make(chan chunk, 16)
	done := make(chan struct{})
	exited := startReader(in, reads, done)
	defer func() {
		close(done)
		if exited != nil {
			// Nothing reads the terminal after Loop returns. The reader
			// notices done within one poll timeout; the bound is a guard.
			select {
			case <-exited:
			case <-time.After(time.Second):
			}
		}
	}()

	var prev *paint.Grid
	lastCols, lastRows := -1, -1
	draw := func() error {
		cols, rows := size()
		if cols != lastCols || rows != lastRows {
			prev = nil
			lastCols, lastRows = cols, rows
		}
		f := a.Frame(cols, rows)
		if _, err := io.WriteString(out, paint.Diff(prev, f.Grid)); err != nil {
			return err
		}
		prev = f.Grid
		return nil
	}
	// runEvents dispatches events and pending lifecycle events until quiet.
	runEvents := func(evs []Event) (bool, error) {
		for round := 0; round < 8; round++ {
			for _, ev := range evs {
				quit, err := a.Dispatch(ev)
				if err != nil || quit {
					return quit, err
				}
			}
			evs = a.TakePending()
			if len(evs) == 0 {
				return false, nil
			}
			if err := draw(); err != nil {
				return false, err
			}
		}
		return false, nil
	}
	// settle redraws and runs the lifecycle events queued so far. stop
	// reports that the loop must return err.
	settle := func() (stop bool, err error) {
		if err := draw(); err != nil {
			return true, err
		}
		if quit, err := runEvents(nil); quit || err != nil {
			return true, err
		}
		return false, nil
	}
	// handleKeys handles keys in order. A stop signal that arrives while
	// they run (handlers may be slow) is honored before the next key.
	// Printable keys in a row that go to the focused input are one edit
	// with one on:change (HandleKeyRun), so a paste costs one frame, not
	// one per character.
	handleKeys := func(keys []Key) (stop bool, err error) {
		for len(keys) > 0 {
			select {
			case s := <-sigs:
				return true, &SignalError{Signal: s}
			default:
			}
			focus := a.Focus()
			evs, n := a.HandleKeyRun(keys)
			keys = keys[n:]
			quit, err := runEvents(evs)
			if err != nil {
				return true, err
			}
			if quit {
				return true, nil
			}
			if len(evs) > 0 || a.Focus() != focus {
				// Handlers may open modals and keys may move focus; later
				// keys in the same read must see the new frame (:focus,
				// modal trap, focusables).
				if err := draw(); err != nil {
					return true, err
				}
			}
		}
		return false, nil
	}
	// pending holds the undecoded tail of the input: a UTF-8 rune or an
	// escape sequence the last read cut in two. The next read completes
	// it; an ESC-led tail that nothing follows within escTimeout is
	// decoded as it stands (a lone ESC is the esc key).
	var pending []byte
	var escWait <-chan time.Time
	onChunk := func(c chunk) (stop bool, err error) {
		keys, rest := decodeKeys(append(pending, c.data...), c.err != nil)
		pending = append([]byte(nil), rest...)
		escWait = nil
		if stop, err := handleKeys(keys); stop {
			return true, err
		}
		if c.err != nil {
			if errors.Is(c.err, io.EOF) {
				return true, nil
			}
			return true, c.err
		}
		if stop, err := settle(); stop {
			return true, err
		}
		if len(pending) > 0 && pending[0] == 0x1b {
			escWait = time.After(escTimeout)
		}
		return false, nil
	}
	if err := draw(); err != nil {
		return err
	}
	if quit, err := runEvents(nil); quit || err != nil {
		return err
	}
	for {
		select {
		case c := <-reads:
			if stop, err := onChunk(c); stop {
				return err
			}
		case <-escWait:
			escWait = nil
			// A read that arrived meanwhile continues the sequence.
			select {
			case c := <-reads:
				if stop, err := onChunk(c); stop {
					return err
				}
				continue
			default:
			}
			keys, _ := decodeKeys(pending, true)
			pending = nil
			if stop, err := handleKeys(keys); stop {
				return err
			}
			if stop, err := settle(); stop {
				return err
			}
		case s := <-sigs:
			return &SignalError{Signal: s}
		case <-resize:
			// A new size can open or hide modals and move focus (@media):
			// their on:open/on:close/on:focus run now, like after a key.
			if stop, err := settle(); stop {
				return err
			}
		case <-a.wake:
			if stop, err := settle(); stop {
				return err
			}
		case <-time.After(250 * time.Millisecond):
			// Poll for size changes where no resize signal exists.
			if c, r := size(); c != lastCols || r != lastRows {
				if stop, err := settle(); stop {
					return err
				}
			}
		}
	}
}
