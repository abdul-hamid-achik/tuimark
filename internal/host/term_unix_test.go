//go:build unix

package host

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// Finding 25: while Run holds the terminal, SIGTERM is delivered to the
// loop instead of killing the process with the terminal still raw.
func TestStopSignalsCatchSIGTERM(t *testing.T) {
	sigs, release := stopSignals()
	defer release()
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-sigs:
		if s != syscall.SIGTERM {
			t.Errorf("got %v", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SIGTERM was not caught")
	}
}

// ptyChildEnv makes a re-run of this test binary act as the child of a
// pty test (its value is the mode).
const ptyChildEnv = "TUIMARK_PTY_CHILD"

// ptyRig is a child process (this test binary again, running one test in
// child mode) on a new pty that is its controlling terminal: stdin,
// stdout, and /dev/tty. Its stderr is a pipe the parent reads.
type ptyRig struct {
	cmd    *exec.Cmd
	master *os.File
	slave  *os.File
	screen *syncBuf // what the child wrote to the terminal
	stderr *syncBuf
	exited chan struct{}
}

func startPtyChild(t *testing.T, test, mode string) *ptyRig {
	t.Helper()
	return startPtyChildEnv(t, test, mode, nil)
}

// terminalEnv are the variables Run's color detection, probe, and
// environment overrides read (SPEC v0.2 §26.10).
var terminalEnv = []string{"TERM", "TERM_PROGRAM", "SSH_TTY", "WT_SESSION", "COLORTERM", "NO_COLOR", "TUIMARK_COLOR", "TUIMARK_THEME", "TUIMARK_SYNC", "TUIMARK_LOG", "COLORFGBG"}

// startPtyChildEnv is startPtyChild with a pinned terminal environment:
// with env non-nil, the child gets this process's environment without the
// terminalEnv variables, plus env ("NAME=value" entries).
func startPtyChildEnv(t *testing.T, test, mode string, env []string) *ptyRig {
	t.Helper()
	master, name, err := openPTY()
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	slave, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		t.Skipf("cannot open the pty slave %s: %v", name, err)
	}
	r := &ptyRig{master: master, slave: slave, screen: &syncBuf{}, stderr: &syncBuf{}, exited: make(chan struct{})}
	r.cmd = exec.Command(os.Args[0], "-test.run=^"+test+"$", "-test.count=1")
	r.cmd.Env = os.Environ()
	if env != nil {
		r.cmd.Env = nil
	next:
		for _, kv := range os.Environ() {
			for _, k := range terminalEnv {
				if strings.HasPrefix(kv, k+"=") {
					continue next
				}
			}
			r.cmd.Env = append(r.cmd.Env, kv)
		}
		r.cmd.Env = append(r.cmd.Env, env...)
	}
	r.cmd.Env = append(r.cmd.Env, ptyChildEnv+"="+mode)
	r.cmd.Stdin, r.cmd.Stdout, r.cmd.Stderr = slave, slave, r.stderr
	r.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := r.cmd.Start(); err != nil {
		slave.Close()
		master.Close()
		t.Skipf("cannot start a child on a pty: %v", err)
	}
	go func() { _, _ = io.Copy(r.screen, master) }() // ends once no slave is open
	go func() {
		_ = r.cmd.Wait()
		close(r.exited)
	}()
	t.Cleanup(func() {
		select {
		case <-r.exited:
		default:
			_ = r.cmd.Process.Kill()
			<-r.exited
		}
		slave.Close()
		master.Close()
	})
	return r
}

// waitFor waits until buf contains want.
func (r *ptyRig) waitFor(buf *syncBuf, want string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), want) {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func (r *ptyRig) write(t *testing.T, s string) {
	t.Helper()
	if _, err := r.master.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func (r *ptyRig) signal(t *testing.T, s syscall.Signal) {
	t.Helper()
	if err := r.cmd.Process.Signal(s); err != nil {
		t.Fatal(err)
	}
}

// wait waits for the child to exit and reports whether it did.
func (r *ptyRig) wait(d time.Duration) bool {
	select {
	case <-r.exited:
		return true
	case <-time.After(d):
		return false
	}
}

func (r *ptyRig) alive() bool {
	select {
	case <-r.exited:
		return false
	default:
		return true
	}
}

// cooked reports whether the terminal is back in cooked mode (canonical
// input and echo, which raw mode turns off).
func (r *ptyRig) cooked(t *testing.T) bool {
	t.Helper()
	tio, err := unix.IoctlGetTermios(int(r.slave.Fd()), ioctlGetTermios)
	if err != nil {
		t.Fatal(err)
	}
	return tio.Lflag&unix.ICANON != 0 && tio.Lflag&unix.ECHO != 0
}

func (r *ptyRig) status() syscall.WaitStatus {
	ws, _ := r.cmd.ProcessState.Sys().(syscall.WaitStatus)
	return ws
}

func (r *ptyRig) describe() string {
	return fmt.Sprintf("child stderr:\n%s", r.stderr.String())
}

// Round 2, finding 3: on darwin, poll(2) reports POLLNVAL for /dev/tty.
// The reader must still be joinable (select(2) waits on it), so when Loop
// returns nothing is left reading the terminal: the host's next read gets
// the next bytes typed. Linux polls /dev/tty; the test holds there too.
func TestLoopJoinsReaderOnDevTTY(t *testing.T) {
	if os.Getenv(ptyChildEnv) == "devtty" {
		devTTYChild(t)
		return
	}
	r := startPtyChild(t, "TestLoopJoinsReaderOnDevTTY", "devtty")
	if !r.waitFor(r.stderr, "READY", 10*time.Second) {
		if strings.Contains(r.stderr.String(), "NO-TTY") {
			t.Skipf("the child has no /dev/tty: %s", r.stderr.String())
		}
		t.Fatalf("child never got ready; %s", r.describe())
	}
	if !strings.Contains(r.stderr.String(), "WAITER ok") {
		t.Errorf("no way to wait on /dev/tty, reads cannot be joined; %s", r.describe())
	}
	r.write(t, "\x03") // ctrl+c: the built-in quit
	if !r.waitFor(r.stderr, "LOOP-DONE <nil>", 5*time.Second) {
		t.Fatalf("Loop did not return on ctrl+c; %s", r.describe())
	}
	r.write(t, "hello\n")
	if !r.wait(10 * time.Second) {
		t.Fatalf("child did not exit; %s", r.describe())
	}
	if !strings.Contains(r.stderr.String(), `READ "hello\n"`) {
		t.Errorf("the host's read after Loop did not get the next input (a leftover reader took it); %s", r.describe())
	}
}

// devTTYChild runs Loop on /dev/tty (opened like os.Stdin: a blocking
// file), then reads it once as the host.
func devTTYChild(t *testing.T) {
	fd, err := syscall.Open("/dev/tty", syscall.O_RDWR, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NO-TTY %v\n", err)
		os.Exit(0)
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NO-TTY raw: %v\n", err)
		os.Exit(0)
	}
	tty := os.NewFile(uintptr(fd), "/dev/tty")
	waiter := "ok"
	if inputReady(tty) == nil {
		waiter = "nil"
	}
	fmt.Fprintf(os.Stderr, "WAITER %s\n", waiter)
	a := doc(t, loopDoc)
	fmt.Fprintln(os.Stderr, "READY")
	err = a.Loop(tty, io.Discard, func() (int, int) { return 20, 4 }, nil)
	fmt.Fprintf(os.Stderr, "LOOP-DONE %v\n", err)
	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := tty.Read(buf)
		got <- string(buf[:n])
	}()
	select {
	case s := <-got:
		fmt.Fprintf(os.Stderr, "READ %q\n", s)
	case <-time.After(3 * time.Second):
		fmt.Fprintln(os.Stderr, "READ-TIMEOUT")
	}
	_ = term.Restore(fd, old)
	os.Exit(0)
}

const stuckDoc = `<tui version="1"><keymap><bind keys="s" action="stuck"/></keymap><screen id="s"><text>STUCKAPP</text></screen></tui>`

// signalChild runs Run on the pty with a handler that never returns in
// time ("s"). mode sets the grace period; in mode "quit" the handler
// returns ErrQuit after a second instead.
func signalChild(t *testing.T, mode string) {
	switch mode {
	case "grace":
		signalGrace = time.Second
	case "twice":
		signalGrace = time.Hour
	case "quit":
		signalGrace = time.Minute
	}
	a := doc(t, stuckDoc)
	a.On("stuck", func(Event) error {
		fmt.Fprintln(os.Stderr, "HANDLER")
		if mode == "quit" {
			time.Sleep(time.Second)
			return ErrQuit
		}
		time.Sleep(time.Hour)
		return nil
	})
	err := a.Run(os.Stdout)
	fmt.Fprintf(os.Stderr, "RUN-RETURNED %v\n", err)
	// The parent cannot look at the terminal once this session leader
	// exits (darwin revokes it), so report its mode from here.
	tio, err := unix.IoctlGetTermios(0, ioctlGetTermios)
	fmt.Fprintf(os.Stderr, "COOKED %v\n", err == nil && tio.Lflag&unix.ICANON != 0 && tio.Lflag&unix.ECHO != 0)
	os.Exit(0)
}

// Round 2, finding 2: a stop signal restores the terminal promptly even
// when the event goroutine is blocked in a handler. Idle, Run returns a
// *SignalError as before. With a handler running, the terminal is restored
// at once, and the process ends with the signal's default action when the
// grace period runs out or a second signal arrives.
func TestRunSignals(t *testing.T) {
	if mode := os.Getenv(ptyChildEnv); mode != "" {
		signalChild(t, mode)
		return
	}
	start := func(t *testing.T, mode string, stuck bool) *ptyRig {
		t.Helper()
		r := startPtyChild(t, "TestRunSignals", mode)
		if !r.waitFor(r.screen, "STUCKAPP", 10*time.Second) {
			t.Fatalf("the app never rendered; %s", r.describe())
		}
		if stuck {
			r.write(t, "s")
			if !r.waitFor(r.stderr, "HANDLER", 5*time.Second) {
				t.Fatalf("the handler never ran; %s", r.describe())
			}
		}
		return r
	}
	killedBy := func(t *testing.T, r *ptyRig, want syscall.Signal) {
		t.Helper()
		if ws := r.status(); !ws.Signaled() || ws.Signal() != want {
			t.Errorf("child ended with %v, want killed by %v; %s", r.cmd.ProcessState, want, r.describe())
		}
		if strings.Contains(r.stderr.String(), "RUN-RETURNED") {
			t.Errorf("Run returned although its handler was stuck; %s", r.describe())
		}
	}
	t.Run("idle", func(t *testing.T) {
		r := start(t, "idle", false)
		r.signal(t, syscall.SIGTERM)
		if !r.wait(5 * time.Second) {
			t.Fatalf("an idle Run did not stop on SIGTERM; %s", r.describe())
		}
		if !strings.Contains(r.stderr.String(), "RUN-RETURNED tuimark: stopped by signal (terminated)") || r.cmd.ProcessState.ExitCode() != 0 {
			t.Errorf("Run did not return a SignalError: %v; %s", r.cmd.ProcessState, r.describe())
		}
		if !strings.Contains(r.stderr.String(), "COOKED true") {
			t.Errorf("terminal left raw; %s", r.describe())
		}
	})
	t.Run("grace", func(t *testing.T) {
		r := start(t, "grace", true)
		sent := time.Now()
		r.signal(t, syscall.SIGHUP)
		// The terminal comes back while the handler still runs (the
		// grace period is 1s here).
		for r.alive() && !r.cooked(t) && time.Since(sent) < 800*time.Millisecond {
			time.Sleep(5 * time.Millisecond)
		}
		if !r.alive() || !r.cooked(t) {
			t.Errorf("terminal not restored within 800ms of SIGHUP (alive %v); %s", r.alive(), r.describe())
		}
		if !r.wait(5 * time.Second) {
			t.Fatalf("SIGHUP with a stuck handler did not end the process; %s", r.describe())
		}
		if d := time.Since(sent); d < 500*time.Millisecond {
			t.Errorf("ended after %v, before the grace period", d)
		}
		killedBy(t, r, syscall.SIGHUP)
		r.slave.Close() // let the copy of the screen finish
		time.Sleep(50 * time.Millisecond)
		if s := r.screen.String(); !strings.Contains(s[max(0, strings.Index(s, "STUCKAPP")):], "\x1b[?25h\x1b[?7h\x1b[?1049l") {
			t.Errorf("alternate screen not left, cursor not shown: …%q", s[max(0, len(s)-40):])
		}
	})
	// Round 3, finding 6: a signal that arrives while a handler runs is
	// not lost when that handler then quits: Run still returns the
	// *SignalError (it used to return nil, so the program exited 0).
	t.Run("quit", func(t *testing.T) {
		r := start(t, "quit", true)
		r.signal(t, syscall.SIGTERM)
		if !r.wait(5 * time.Second) {
			t.Fatalf("Run did not return after the handler quit; %s", r.describe())
		}
		if !strings.Contains(r.stderr.String(), "RUN-RETURNED tuimark: stopped by signal (terminated)") || r.cmd.ProcessState.ExitCode() != 0 {
			t.Errorf("Run did not report the signal: %v; %s", r.cmd.ProcessState, r.describe())
		}
		if !strings.Contains(r.stderr.String(), "COOKED true") {
			t.Errorf("terminal left raw; %s", r.describe())
		}
	})
	t.Run("twice", func(t *testing.T) {
		r := start(t, "twice", true)
		r.signal(t, syscall.SIGTERM)
		deadline := time.Now().Add(3 * time.Second)
		for !r.cooked(t) && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		if !r.cooked(t) {
			t.Fatalf("terminal not restored while the handler runs; %s", r.describe())
		}
		if !r.alive() {
			t.Fatalf("child ended before the grace period (1h here); %s", r.describe())
		}
		r.signal(t, syscall.SIGTERM)
		if !r.wait(5 * time.Second) {
			t.Fatalf("a second SIGTERM did not end the process; %s", r.describe())
		}
		killedBy(t, r, syscall.SIGTERM)
	})
}

// watchSignals, unit level: the first signal reaches the loop; a loop
// that returns in time is left alone; a busy one gets the terminal
// restored, then the signal's default action after grace or on a second
// signal.
func TestWatchSignals(t *testing.T) {
	type rig struct {
		caught   chan os.Signal
		toLoop   chan os.Signal
		loopDone chan struct{}
		restored chan struct{}
		died     chan os.Signal
		returned chan struct{}
	}
	start := func(grace time.Duration) *rig {
		r := &rig{make(chan os.Signal, 4), make(chan os.Signal, 1), make(chan struct{}), make(chan struct{}, 4), make(chan os.Signal, 4), make(chan struct{})}
		go func() {
			watchSignals(r.caught, r.toLoop, r.loopDone, func() { r.restored <- struct{}{} }, grace, func(s os.Signal) { r.died <- s }, nil)
			close(r.returned)
		}()
		return r
	}
	recv := func(t *testing.T, what string, c <-chan os.Signal) os.Signal {
		t.Helper()
		select {
		case s := <-c:
			return s
		case <-time.After(3 * time.Second):
			t.Fatalf("%s: nothing", what)
		}
		return nil
	}
	// Idle: the loop returns at once; nothing is restored from here.
	r := start(time.Hour)
	r.caught <- syscall.SIGTERM
	if s := recv(t, "idle", r.toLoop); s != syscall.SIGTERM {
		t.Errorf("idle: loop got %v", s)
	}
	close(r.loopDone)
	<-r.returned
	if len(r.restored) != 0 || len(r.died) != 0 {
		t.Errorf("idle: restored %d died %d", len(r.restored), len(r.died))
	}
	// Busy past the grace period: restore, then die with the signal.
	r = start(100 * time.Millisecond)
	r.caught <- syscall.SIGHUP
	recv(t, "grace", r.toLoop)
	if s := recv(t, "grace", r.died); s != syscall.SIGHUP {
		t.Errorf("grace: died with %v", s)
	}
	if len(r.restored) == 0 {
		t.Error("grace: died without restoring the terminal")
	}
	// A second signal: restore and die at once.
	r = start(time.Hour)
	r.caught <- syscall.SIGTERM
	recv(t, "twice", r.toLoop)
	select {
	case <-r.restored:
	case <-time.After(3 * time.Second):
		t.Fatal("twice: terminal not restored while the loop is busy")
	}
	r.caught <- syscall.SIGINT
	if s := recv(t, "twice", r.died); s != syscall.SIGINT {
		t.Errorf("twice: died with %v", s)
	}
}

const runPtyDoc = `<tui version="1">
<style>screen { background: $bg; color: $fg; }</style>
<keymap><bind keys="ctrl+s" action="stuck"/></keymap>
<screen id="s" focus="#q">
  <input id="q" bind="text" on:change="chg" on:focus="hello"/>
  <list id="l" each="rows as r" key="r" on:select="pick"><item><text>{r}</text></item></list>
  <text>PTYAPP 微信</text>
</screen>
</tui>`

// runPtyChild runs Run on the pty. Every event is reported on stderr;
// ctrl+s runs a handler that never returns in time.
func runPtyChild(t *testing.T) {
	signalGrace = time.Second
	a := doc(t, runPtyDoc)
	_ = a.Bind("", map[string]any{"text": "", "rows": []any{"a", "b"}})
	for _, act := range []string{"chg", "hello", "pick"} {
		a.On(act, func(ev Event) error {
			fmt.Fprintf(os.Stderr, "EVENT %s %q\n", ev.Action, fmt.Sprint(ev.Value))
			return nil
		})
	}
	a.On("stuck", func(Event) error {
		fmt.Fprintln(os.Stderr, "HANDLER")
		time.Sleep(time.Hour)
		return nil
	})
	err := a.Run(os.Stdout)
	fmt.Fprintf(os.Stderr, "RUN-RETURNED %v\n", err)
	os.Exit(0)
}

// startRunPty starts runPtyChild with a pinned environment and, when
// replies is not "", answers the capability probe with them.
func startRunPty(t *testing.T, env []string, replies string) *ptyRig {
	t.Helper()
	r := startPtyChildEnv(t, "TestRunPty", "run", env)
	if replies != "" {
		if !r.waitFor(r.screen, "\x1b[?2027$p\x1b[c", 10*time.Second) {
			t.Fatalf("no probe; screen %q; %s", r.screen.String(), r.describe())
		}
		r.write(t, replies)
	}
	if !r.waitFor(r.screen, "PTYAPP", 10*time.Second) || !r.waitFor(r.stderr, "EVENT hello", 5*time.Second) {
		t.Fatalf("the app never rendered; %s", r.describe())
	}
	return r
}

// leaveWith2027 is the leave sequence of a session that turned mode 2027 on.
const leaveWith2027 = "\x1b[?2027l\x1b[?2004l\x1b[0m\x1b[?25h\x1b[?7h\x1b[?1049l"

// SPEC v0.2 §21 tests 32, 34, 36, and 37 through a real pty: Run probes
// the terminal, turns bracketed paste and grapheme mode on, delivers a
// paste as one edit, ignores a paste off an input, honors TUIMARK_THEME,
// TUIMARK_COLOR, and TUIMARK_SYNC, skips the probe for Apple Terminal and
// SSH, and writes the leave sequence on quit and on a stop signal, from
// the loop and from the signal watcher.
func TestRunPty(t *testing.T) {
	if os.Getenv(ptyChildEnv) == "run" {
		runPtyChild(t)
		return
	}
	probeReplies := "\x1b[?2026;2$y\x1b[?2027;2$y\x1b[?64;1;22c"
	base := []string{"TERM=xterm-256color", "TUIMARK_COLOR=truecolor"}
	// §21 test 32: CSI 200 ~, q, CR LF, x, CSI 201 ~ with focus on an input
	// gives "q x" and one on:change; on a list, nothing happens and the app
	// keeps running. §21 test 36: quit leaves with 2027 and 2004 off.
	t.Run("paste", func(t *testing.T) {
		r := startRunPty(t, base, probeReplies)
		r.write(t, "\x1b[200~q\r\nx\x1b[201~")
		if !r.waitFor(r.stderr, `EVENT chg "q x"`, 5*time.Second) {
			t.Fatalf("no change event for the paste; %s", r.describe())
		}
		r.write(t, "\t")
		r.write(t, "\x1b[200~q\x1b[B\x1b[201~")
		time.Sleep(300 * time.Millisecond)
		if !r.alive() {
			t.Fatalf("a paste on the list ended the app; %s", r.describe())
		}
		r.write(t, "\x1b[B")
		if !r.waitFor(r.stderr, `EVENT pick "<nil>"`, 5*time.Second) {
			t.Fatalf("the list did not move after the paste; %s", r.describe())
		}
		if n := strings.Count(r.stderr.String(), "EVENT chg"); n != 1 {
			t.Errorf("%d change events, want 1; %s", n, r.describe())
		}
		if n := strings.Count(r.stderr.String(), "EVENT pick"); n != 1 {
			t.Errorf("%d select events (the pasted down key must not move the list); %s", n, r.describe())
		}
		r.write(t, "\x03")
		if !r.wait(10 * time.Second) {
			t.Fatalf("ctrl+c did not quit; %s", r.describe())
		}
		if !strings.Contains(r.stderr.String(), "RUN-RETURNED <nil>") {
			t.Errorf("Run: %s", r.describe())
		}
		r.slave.Close()
		time.Sleep(50 * time.Millisecond)
		s := r.screen.String()
		enter := "\x1b[?1049h\x1b[?25l\x1b[?7l\x1b[H\x1b[2J\x1b[?2004h\x1b[?2026$p\x1b[?2027$p\x1b[c\x1b[?2027h\x1b[?2026h"
		if !strings.HasPrefix(s, enter) {
			t.Errorf("start = %q, want %q", s[:min(len(s), len(enter)+10)], enter)
		}
		if !strings.HasSuffix(s, leaveWith2027) {
			t.Errorf("leave = %q", s[max(0, len(s)-50):])
		}
		if chaRe := regexp.MustCompile(`微\x1b\[\d+G`); chaRe.MatchString(s) {
			t.Errorf("CHA with mode 2027 on")
		}
	})
	// Finding 14 (§26.7, §26.8): a paste start marker the terminal's writes
	// split by more than the esc timeout still starts the paste, so its
	// payload never becomes keys: the ctrl+c byte in it does not quit.
	t.Run("paste marker split by the esc timeout", func(t *testing.T) {
		r := startRunPty(t, base, probeReplies)
		r.write(t, "\t")
		for _, split := range []string{"\x1b", "\x1b[2", "\x1b[200"} {
			r.write(t, split)
			time.Sleep(150 * time.Millisecond)
			r.write(t, strings.TrimPrefix("\x1b[200~", split)+"ab\x03q\x1b[201~")
		}
		time.Sleep(300 * time.Millisecond)
		if !r.alive() {
			t.Fatalf("the paste reached the keymap (ctrl+c quit); %s", r.describe())
		}
		r.write(t, "\x1b[B")
		if !r.waitFor(r.stderr, `EVENT pick "<nil>"`, 5*time.Second) {
			t.Fatalf("the list did not move after the pastes; %s", r.describe())
		}
		if n := strings.Count(r.stderr.String(), "EVENT chg"); n != 0 {
			t.Errorf("%d change events; %s", n, r.describe())
		}
		r.write(t, "\x03")
		if !r.wait(10 * time.Second) {
			t.Fatalf("ctrl+c did not quit; %s", r.describe())
		}
	})
	// §21 test 36: a stop signal, idle (the loop returns) and with a stuck
	// handler (the signal watcher restores the terminal), leaves with 2027
	// and 2004 off.
	for _, stuck := range []bool{false, true} {
		t.Run(fmt.Sprintf("signal stuck=%v", stuck), func(t *testing.T) {
			r := startRunPty(t, base, probeReplies)
			if stuck {
				r.write(t, "\x13") // ctrl+s
				if !r.waitFor(r.stderr, "HANDLER", 5*time.Second) {
					t.Fatalf("the handler never ran; %s", r.describe())
				}
			}
			r.signal(t, syscall.SIGTERM)
			if !r.waitFor(r.screen, leaveWith2027, 5*time.Second) {
				s := r.screen.String()
				t.Errorf("no leave sequence with 2027 off: %q; %s", s[max(0, len(s)-60):], r.describe())
			}
			if !r.wait(10 * time.Second) {
				t.Fatalf("SIGTERM did not end the child; %s", r.describe())
			}
		})
	}
	// §21 tests 35 and 37: TUIMARK_THEME and TUIMARK_COLOR change Run's
	// frames; TUIMARK_SYNC=1 wraps them and leaves the 2026 query out.
	// TERM=dumb skips the probe; TUIMARK_COLOR still forces the profile.
	for _, c := range []struct {
		name     string
		env      []string
		replies  string
		want     []string
		dontWant []string
	}{
		{"light truecolor", []string{"TERM=dumb", "TUIMARK_COLOR=truecolor", "TUIMARK_THEME=light"}, "", []string{"48;2;255;255;255"}, []string{"48;2;13;17;23", "$p", "\x1b[?2026h"}},
		{"dark truecolor", []string{"TERM=dumb", "TUIMARK_COLOR=truecolor"}, "", []string{"48;2;13;17;23"}, []string{"48;2;255;255;255", "$p"}},
		{"dark 256", []string{"TERM=dumb", "TUIMARK_COLOR=256"}, "", []string{"48;5;233"}, []string{"48;2;"}},
		{"none", []string{"TERM=xterm-256color", "NO_COLOR=1", "TUIMARK_SYNC=0"}, "\x1b[?2027;0$y\x1b[?62c", []string{"\x1b[0;7m"}, []string{"48;", "38;", "\x1b[?2026", "2027h"}},
		{"sync forced", []string{"TERM=xterm-256color", "TUIMARK_SYNC=1"}, "\x1b[?62c", []string{"\x1b[?2027$p\x1b[c", "\x1b[?2026h", "48;5;233"}, []string{"\x1b[?2026$p", "2027h"}},
		{"Apple Terminal skips the probe", []string{"TERM=xterm-256color", "TERM_PROGRAM=Apple_Terminal"}, "", []string{"48;5;233"}, []string{"$p", "\x1b[c"}},
		{"SSH skips the probe", []string{"TERM=xterm-256color", "SSH_TTY=/dev/pts/9"}, "", nil, []string{"$p", "\x1b[c"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := startRunPty(t, c.env, c.replies)
			r.write(t, "\x03")
			if !r.wait(10 * time.Second) {
				t.Fatalf("ctrl+c did not quit; %s", r.describe())
			}
			r.slave.Close()
			time.Sleep(50 * time.Millisecond)
			s := r.screen.String()
			for _, w := range c.want {
				if !strings.Contains(s, w) {
					t.Errorf("no %q in the output", w)
				}
			}
			for _, w := range c.dontWant {
				if strings.Contains(s, w) {
					t.Errorf("%q in the output", w)
				}
			}
			if !strings.HasSuffix(s, leaveScreen) {
				t.Errorf("leave = %q", s[max(0, len(s)-40):])
			}
		})
	}
}
