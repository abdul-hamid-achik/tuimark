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
// decoded as it stands: a lone ESC is then the esc key and any other
// pending sequence is dropped (decoder.flush). The discard of an
// over-long sequence and the bytes the paste marker guard holds end the
// same way. Tests lengthen it.
var escTimeout = 25 * time.Millisecond

// Run drives the app in the terminal: raw mode on stdin, the alternate
// screen on w, frame diffs as ANSI. It returns nil on quit.
//
// Before it touches the terminal, Run reads TUIMARK_COLOR, TUIMARK_THEME,
// and TUIMARK_SYNC (SPEC v0.2 §26.10); an invalid value is returned as an
// error with nothing written and no raw mode. It also opens the
// TUIMARK_LOG file when that variable is set (SPEC v0.2b §26.12), and a
// file it cannot open is such an error too. The session (§26.1): enter
// the alternate screen with bracketed paste on; probe the terminal's
// capabilities once when w is a terminal (§26.2: DECRQM 2026 and 2027
// with a DA1 sentinel, at most probeWait; OSC 11 first when the theme is
// auto, and, under the Apple Terminal/SSH skip rule, only OSC 11 and DA1),
// queuing the keys and pastes that arrive meanwhile; resolve theme="auto"
// from the background reply, else COLORFGBG, else dark (§26.4); turn
// grapheme mode 2027 on when the terminal reports it off; then draw the
// first frame, handle the queued input, and loop. Frames are written in the color profile of §26.3, wrapped in
// synchronized output when the terminal supports it (§26.6), with CHA
// re-positioning after complex clusters unless mode 2027 is on (§26.5).
// Every way out (quit, EOF, error, stop signal) writes the leave sequence:
// mode 2027 off (when Run turned it on), bracketed paste off, attributes
// reset, cursor shown, main screen.
//
// SIGTERM, SIGHUP, and SIGINT (SIGINT only from outside: raw mode turns
// ctrl+c into a key) stop Run like a quit that restores the terminal first
// (cooked mode, main screen, visible cursor), and Run then returns a
// non-nil error naming the signal, so the caller can exit with a failure
// status. That holds even when the loop was stopping anyway (a handler
// returned ErrQuit, or input ended) as the signal came in, and during the
// probe. A handler that is still running when the signal arrives cannot
// hold the terminal: it is restored right away, and if Run has not
// returned within signalGrace (one second), or a second signal arrives,
// the process ends with the signal's default action. When Run returns,
// nothing is left reading stdin.
func (a *App) Run(w io.Writer) (err error) {
	start := time.Now()
	cfg, err := readRunConfig(os.Getenv)
	if err != nil {
		return err
	}
	// TUIMARK_LOG (SPEC v0.2b §26.12) is opened with the environment,
	// before the terminal is touched; its end record is written last, on
	// every way out, with the error Run finally returns.
	lg, err := openRunLog(cfg.log, start)
	if err != nil {
		return err
	}
	sess := &termSession{profile: cfg.profile, sync: cfg.sync, fgbg: cfg.fgbg, log: lg}
	defer func() { lg.end(endReason(err, sess)) }()
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return errors.New("tuimark: Run needs an interactive terminal on stdin (use Dump for headless output)")
	}
	sizeFd := fd
	outTTY := false
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		sizeFd, outTTY = int(f.Fd()), true
	}
	// theme="auto" (or @theme "auto") without TUIMARK_THEME puts OSC 11 in
	// the probe (§26.2, ADR 0010).
	themeQuery := a.wantsTerminalTheme(cfg.theme)
	sess.probe, sess.skipDECRQM = probePlan(os.Getenv, outTTY, cfg.sync, themeQuery)
	sess.themeQuery = themeQuery && sess.probe
	defer a.overrideTheme(cfg.theme)()
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
				s = sess.leave()
			}
			out.close(s)
		})
	}
	defer restore(false)
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
	err = a.session(os.Stdin, out, size, resizeSignal(stop), sigs, sess)
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

// endReason names how a Run session ended, for the end record of
// TUIMARK_LOG (SPEC v0.2b §26.12): signal, error, eof, or quit.
func endReason(err error, sess *termSession) string {
	var se *SignalError
	switch {
	case errors.As(err, &se):
		return "signal"
	case err != nil:
		return "error"
	case sess.eof:
		return "eof"
	}
	return "quit"
}

// SignalError is what Run returns when a signal stopped it.
type SignalError struct{ Signal os.Signal }

func (e *SignalError) Error() string {
	return fmt.Sprintf("tuimark: stopped by signal (%v); the terminal was restored", e.Signal)
}

// Loop is the terminal event loop behind Run, with injectable input,
// output, size, and resize notifications (tests drive it with pipes). It
// writes Run's enter and leave sequences (bracketed paste included) but
// never probes the terminal: frames are truecolor, not synchronized, with
// CHA re-positioning.
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
	return a.session(in, out, size, resize, sigs, &termSession{})
}

// session is loop in a terminal session sess: Run's probe, color profile,
// synchronized output, and grapheme mode (SPEC v0.2 §26).
func (a *App) session(in io.Reader, out io.Writer, size func() (int, int), resize <-chan struct{}, sigs <-chan os.Signal, sess *termSession) error {
	io.WriteString(out, enterScreen)
	defer func() { io.WriteString(out, sess.leave()) }()
	// What the session resolved theme="auto" to is forgotten when it ends
	// (Dump and Validate never see it, MUST 13).
	defer a.endRun()
	lg := sess.log
	if lg != nil {
		c, r := size()
		lg.write("start", member{"version", Version}, member{"cols", c}, member{"rows", r})
	}

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

	// dec decodes every byte read (SPEC v0.2 §26.8). It keeps the
	// undecoded tail of the input: a UTF-8 rune, an escape sequence, or a
	// paste end marker the last read cut in two. The next read completes
	// it; an ESC-led tail that nothing follows within escTimeout is
	// decoded as it stands (a lone ESC is the esc key), and when it was a
	// start of the paste start marker the next read may still complete
	// the marker (decoder.flush). A paste waits for its end marker however
	// long it takes.
	var dec decoder

	// The capability probe (§26.2), once, before the first frame. Keys and
	// pastes that arrive before it ends are queued and handled after the
	// first frame, and so is the end of the input. The esc timeout is not
	// armed meanwhile, so a reply that arrives in pieces is not cut.
	var caps termCaps
	var queued []Input
	var endErr error
	if sess.probe {
		if _, err := io.WriteString(out, sess.queries()); err != nil {
			return err
		}
		capTimer := time.NewTimer(sess.probeCap())
		defer capTimer.Stop()
	probe:
		for {
			select {
			case c := <-reads:
				ins, reps := dec.feed(c.data, c.err != nil)
				queued = append(queued, ins...)
				for _, r := range reps {
					caps.record(r)
				}
				if c.err != nil {
					endErr = c.err
					break probe
				}
				if caps.done {
					break probe
				}
			case <-capTimer.C:
				break probe
			case s := <-sigs:
				return &SignalError{Signal: s}
			}
		}
	}
	// Grapheme width parity (§26.5): mode 2027 when the terminal reports
	// it off (Ps = 2); no CHA re-positioning while it is on.
	if caps.turnGraphemeOn() {
		sess.setGrapheme()
		if _, err := io.WriteString(out, graphemeOn); err != nil {
			return err
		}
	}
	opts := paint.Options{Profile: sess.profile, NoCHA: caps.graphemeActive()}
	syncFrames := sess.syncFrames(caps)
	// theme="auto" (§26.4): the probe's background, else COLORFGBG, else
	// dark, for the first frame and for a later @theme "auto".
	_, autoFrom := a.resolveAuto(caps.bg, sess.fgbg)
	if lg != nil {
		a.mu.Lock()
		theme, from := a.frameTheme(), a.themeSource(autoFrom)
		a.mu.Unlock()
		lg.write("caps", member{"probe", sess.probe}, member{"color", sess.profile.String()}, member{"sync", syncFrames},
			member{"grapheme", caps.graphemeState()}, member{"theme", theme}, member{"theme_from", from})
	}

	var prev *paint.Grid
	lastCols, lastRows := -1, -1
	// draw writes one frame in one write, wrapped in synchronized output
	// when it is on (§26.6).
	draw := func() error {
		t0 := time.Now()
		cols, rows := size()
		if cols != lastCols || rows != lastRows {
			if lastCols >= 0 {
				lg.write("resize", member{"cols", cols}, member{"rows", rows})
			}
			prev = nil
			lastCols, lastRows = cols, rows
		}
		f := a.Frame(cols, rows)
		s := paint.DiffWith(prev, f.Grid, opts)
		if syncFrames {
			s = syncBegin + s + syncEnd
		}
		if _, err := io.WriteString(out, s); err != nil {
			return err
		}
		prev = f.Grid
		if lg != nil {
			lg.write("frame", member{"us", time.Since(t0).Microseconds()}, member{"bytes", len(s)})
		}
		return nil
	}
	// runEvents dispatches events and pending lifecycle events until quiet.
	runEvents := func(evs []Event) (bool, error) {
		for round := 0; round < 8; round++ {
			for _, ev := range evs {
				if lg != nil {
					lg.action(ev, a.secretInput(ev.Source))
				}
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
	// dispatch runs the events of one key or paste. A frame is drawn when
	// they fired something or focus moved: handlers may open modals and
	// keys may move focus, and later input in the same read must see the
	// new frame (:focus, modal trap, focusables).
	dispatch := func(evs []Event, focus string) (stop bool, err error) {
		quit, err := runEvents(evs)
		if err != nil {
			return true, err
		}
		if quit {
			return true, nil
		}
		// A built-in action may have changed the frame without an event
		// or a focus move (TakeDirty).
		if changed := a.TakeDirty(); len(evs) > 0 || a.Focus() != focus || changed {
			if err := draw(); err != nil {
				return true, err
			}
		}
		return false, nil
	}
	// handleInputs handles keys and pastes in order. A stop signal that
	// arrives while they run (handlers may be slow) is honored before the
	// next one. Printable keys in a row that go to the focused input are
	// one edit with one on:change (HandleKeyRun), so a paste on a terminal
	// without bracketed paste costs one frame, not one per character. A
	// bracketed paste is one edit too, or nothing (HandlePaste).
	handleInputs := func(ins []Input) (stop bool, err error) {
		signaled := func() error {
			select {
			case s := <-sigs:
				return &SignalError{Signal: s}
			default:
				return nil
			}
		}
		for len(ins) > 0 {
			if ins[0].IsPaste {
				if err := signaled(); err != nil {
					return true, err
				}
				focus := a.Focus()
				if lg != nil {
					lg.paste(NormalizePaste(ins[0].Paste), a.secretFocused())
				}
				evs := a.HandlePaste(ins[0].Paste)
				ins = ins[1:]
				if stop, err := dispatch(evs, focus); stop {
					return true, err
				}
				continue
			}
			n := 1
			for n < len(ins) && !ins[n].IsPaste {
				n++
			}
			keys := make([]Key, n)
			for i := range keys {
				keys[i] = ins[i].Key
			}
			ins = ins[n:]
			for len(keys) > 0 {
				if err := signaled(); err != nil {
					return true, err
				}
				focus := a.Focus()
				secret := lg != nil && a.secretFocused()
				evs, used := a.HandleKeyRun(keys)
				if lg != nil {
					for _, k := range keys[:used] {
						lg.key(k, secret)
					}
				}
				keys = keys[used:]
				if stop, err := dispatch(evs, focus); stop {
					return true, err
				}
			}
		}
		return false, nil
	}
	var escWait <-chan time.Time
	onChunk := func(c chunk) (stop bool, err error) {
		// Replies that arrive after the probe (or without one) are
		// swallowed without effect.
		ins, _ := dec.feed(c.data, c.err != nil)
		escWait = nil
		if stop, err := handleInputs(ins); stop {
			return true, err
		}
		if c.err != nil {
			if errors.Is(c.err, io.EOF) {
				sess.eof = true
				return true, nil
			}
			return true, c.err
		}
		if stop, err := settle(); stop {
			return true, err
		}
		if dec.escPending() {
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
	if len(queued) > 0 || endErr != nil {
		if stop, err := handleInputs(queued); stop {
			return err
		}
		if endErr != nil {
			if errors.Is(endErr, io.EOF) {
				sess.eof = true
				return nil
			}
			return endErr
		}
		if stop, err := settle(); stop {
			return err
		}
	}
	if dec.escPending() {
		escWait = time.After(escTimeout)
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
			if stop, err := handleInputs(dec.flush()); stop {
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
