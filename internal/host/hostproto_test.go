package host

import (
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The internal hooks of the host protocol bridge (SPEC v0.3b §19.1
// "Internal hooks"): the session-start hook, the stop request, and the
// stop-signal notice. cmd/tuimark's pty tests drive the whole protocol;
// these check the hooks themselves on a session run on pipes.

const hostProtoDoc = `<tui version="1">
<keymap>
  <bind keys="z" action="quit"/>
  <bind keys="a" action="hold"/>
  <bind keys="b" action="after"/>
</keymap>
<screen id="s"><text>HOSTPROTO</text></screen>
</tui>`

// startHostSession is startSession (caps_test.go) with hooks and the
// output buffer given by the caller, so a hook can read what the session
// wrote at the moment it fires.
func startHostSession(t *testing.T, src string, sess *termSession, out *syncBuf, hooks HostHooks, setup func(*App)) *sessRig {
	t.Helper()
	a := doc(t, src)
	if setup != nil {
		setup(a)
	}
	r := &sessRig{app: a, out: out, sigs: make(chan os.Signal, 1), done: make(chan error, 1), evs: make(chan Event, 64)}
	pr, pw := io.Pipe()
	r.in = pw
	go func() {
		r.done <- a.session(pr, r.out, func() (int, int) { return 20, 6 }, nil, r.sigs, sess, hooks)
	}()
	t.Cleanup(func() { pw.Close() })
	return r
}

type startCall struct {
	cols, rows int
	theme, out string
}

// §19.1 step 2: the session-start hook fires once, after the capability
// probe and before the first frame, with the size and the theme the
// probe resolved theme="auto" to.
func TestSessionHostStartHook(t *testing.T) {
	out := &syncBuf{}
	calls := make(chan startCall, 4)
	hooks := HostHooks{Start: func(cols, rows int, theme string) {
		calls <- startCall{cols, rows, theme, out.String()}
	}}
	r := startHostSession(t, autoDoc, &termSession{probe: true, themeQuery: true, wait: 10 * time.Second}, out, hooks, nil)
	r.waitOut(t, "\x1b[c")
	select {
	case c := <-calls:
		t.Fatalf("Start fired during the probe: %+v", c)
	default:
	}
	r.send(t, "\x1b]11;rgb:ffff/ffff/ffff\x1b\\\x1b[?64c")
	var c startCall
	select {
	case c = <-calls:
	case <-time.After(5 * time.Second):
		t.Fatal("Start never fired")
	}
	if c.cols != 20 || c.rows != 6 || c.theme != "light" {
		t.Errorf("Start(%d, %d, %q), want (20, 6, \"light\")", c.cols, c.rows, c.theme)
	}
	if want := enterScreen + queryBackground + "\x1b[?2026$p\x1b[?2027$p\x1b[c"; c.out != want {
		t.Errorf("at Start the session had written %q, want the enter sequence and the probe only (no frame): %q", c.out, want)
	}
	r.waitOut(t, "AUTO")
	r.send(t, "\x03")
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 {
		t.Errorf("Start fired %d more times", len(calls))
	}
}

// §19.1 "Parent gone": a HostStop{EOF: true} ends the session with a nil
// error and sess.eof set, as the end of stdin does.
func TestSessionHostStopEOF(t *testing.T) {
	stop := make(chan HostStop, 1)
	sess := &termSession{}
	r := startHostSession(t, hostProtoDoc, sess, &syncBuf{}, HostHooks{Stop: stop}, nil)
	r.waitOut(t, "HOSTPROTO")
	stop <- HostStop{EOF: true}
	if err := r.wait(t); err != nil {
		t.Errorf("session error = %v, want nil (eof)", err)
	}
	if !sess.eof {
		t.Error("sess.eof is false after HostStop{EOF: true}")
	}
}

// §19.1 "Backpressure": a HostStop carrying an error ends the session with
// that error.
func TestSessionHostStopError(t *testing.T) {
	stop := make(chan HostStop, 1)
	r := startHostSession(t, hostProtoDoc, &termSession{}, &syncBuf{}, HostHooks{Stop: stop}, nil)
	r.waitOut(t, "HOSTPROTO")
	want := errors.New("parent is not reading fd 4")
	stop <- HostStop{Err: want}
	if err := r.wait(t); !errors.Is(err, want) {
		t.Errorf("session error = %v, want %v", err, want)
	}
}

// The loop takes the stop request between one input and the next, as it
// takes a stop signal: a request made while a handler runs ends the
// session once the handler returns, and the keys queued behind it are not
// dispatched.
func TestSessionHostStopBetweenInputs(t *testing.T) {
	stop := make(chan HostStop, 1)
	entered, release, after := make(chan struct{}), make(chan struct{}), make(chan struct{}, 1)
	setup := func(a *App) {
		a.On("hold", func(Event) error {
			close(entered)
			<-release
			return nil
		})
		a.On("after", func(Event) error {
			after <- struct{}{}
			return nil
		})
	}
	sess := &termSession{}
	r := startHostSession(t, hostProtoDoc, sess, &syncBuf{}, HostHooks{Stop: stop}, setup)
	r.waitOut(t, "HOSTPROTO")
	r.send(t, "ab")
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the hold handler never ran")
	}
	stop <- HostStop{EOF: true}
	close(release)
	if err := r.wait(t); err != nil || !sess.eof {
		t.Errorf("session = %v, eof %v; want nil and eof", err, sess.eof)
	}
	select {
	case <-after:
		t.Error("a key queued behind the stop request was dispatched")
	default:
	}
}

// §19.1 "Internal hooks": the stop-signal notice fires once, for the first
// stop signal, before the signal reaches the loop.
func TestWatchSignalsNotifiesOnce(t *testing.T) {
	caught, toLoop := make(chan os.Signal, 4), make(chan os.Signal, 1)
	loopDone, notified := make(chan struct{}), make(chan os.Signal, 4)
	returned := make(chan os.Signal, 1)
	go func() {
		returned <- watchSignals(caught, toLoop, loopDone, func() {}, time.Hour, func(os.Signal) {}, func(s os.Signal) { notified <- s })
	}()
	caught <- syscall.SIGTERM
	select {
	case s := <-notified:
		if s != syscall.SIGTERM {
			t.Errorf("notified of %v", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no stop-signal notice")
	}
	<-toLoop
	close(loopDone)
	if s := <-returned; s != syscall.SIGTERM {
		t.Errorf("watchSignals returned %v", s)
	}
	if len(notified) != 0 {
		t.Errorf("%d more notices", len(notified))
	}
}

// The zero HostHooks (what Run passes) makes RunHost Run: no terminal on
// stdin is Run's error, before the session starts.
func TestRunHostZeroHooksEqualsRun(t *testing.T) {
	a := doc(t, hostProtoDoc)
	res := a.RunHost(io.Discard, HostHooks{})
	if res.Reason != "error" || res.Started || res.Err == nil || !strings.Contains(res.Err.Error(), "interactive terminal") {
		t.Errorf("RunHost with zero hooks = %+v, want Run's usage error before the session starts", res)
	}
}
