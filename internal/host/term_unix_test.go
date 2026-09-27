//go:build unix

package host

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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
	r.cmd.Env = append(os.Environ(), ptyChildEnv+"="+mode)
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
		if s := r.screen.String(); !strings.Contains(s[max(0, strings.Index(s, "STUCKAPP")):], "\x1b[?25h\x1b[?1049l") {
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
			watchSignals(r.caught, r.toLoop, r.loopDone, func() { r.restored <- struct{}{} }, grace, func(s os.Signal) { r.died <- s })
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
