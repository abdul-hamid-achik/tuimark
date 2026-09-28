//go:build unix

package main

// SPEC v0.3b §21 tests 103-105: `tuimark host FILE`, under a pty and
// driven by a test parent over fd 3 and 4 (§19.1). hostRig plays the
// parent: it types into the child's pty (fd 0), reads its frames from the
// pty (fd 1), and speaks the wire protocol over two pipes passed as fd 3
// (parent -> child) and fd 4 (child -> parent) through exec.Cmd.ExtraFiles,
// §19.1's Go recipe. Test 106 is specs/glyphrun/host_ts.yml.
//
// internal/host's pty helpers live in its _test.go files, so this package
// keeps its own copy (§19.1 "Testing").

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// hostPtyChildEnv, set to a JSON array of the child's argv (["host", FILE,
// …flags], or ["run-plain", FILE, DATA]), makes a re-run of this test
// binary the child. The argv travels in the environment, since go test's
// own flag parsing would reject --data and the other flags.
const hostPtyChildEnv = "TUIMARK_HOST_PTY_ARGS"

// hostProbeReplies answers Run's capability probe (§26.2): DECRQM 2026
// and 2027 supported but reset, and the DA1 sentinel.
const hostProbeReplies = "\x1b[?2026;2$y\x1b[?2027;2$y\x1b[?64;1;22c"

const hostDoc = `<tui version="1">
<keymap><bind keys="q" action="quit"/></keymap>
<screen id="s" focus="#l">
  <text>HOSTAPP</text>
  <text>note={note}</text>
  <list id="l" each="rows as r" key="r" on:select="pick"><item><text>row-{r}</text></item></list>
</screen>
</tui>`

const hostData = `{"note": "n0", "rows": ["a", "b", "c"]}`

// hostAutoDoc resolves its theme from the terminal: the probe's OSC 11.
const hostAutoDoc = `<tui version="2" theme="auto">
<keymap><bind keys="q" action="quit"/></keymap>
<screen id="s" focus="#l">
  <text>HOSTAPP</text>
  <list id="l" each="rows as r" key="r" on:select="pick"><item><text>row-{r}</text></item></list>
</screen>
</tui>`

// hostBadDoc has an error-severity static diagnostic (V002).
const hostBadDoc = `<tui version="1">
<keymap><bind keys="q" action="quit"/></keymap>
<screen id="s"><text bogus="1">HOSTAPP</text></screen>
</tui>`

// TestHostSubcommandChild is not a real test: with TUIMARK_HOST_PTY_ARGS
// set, the rigs below re-exec this test binary with
// -test.run=^TestHostSubcommandChild$, and it becomes the child: `tuimark
// host …` on the pty (fd 0-2) and the pipes (fd 3 and 4), or, for
// "run-plain", Run() itself on the same document, whose fd 1 test 105
// compares byte for byte.
func TestHostSubcommandChild(t *testing.T) {
	raw := os.Getenv(hostPtyChildEnv)
	if raw == "" {
		t.Skip("not running as the host pty child")
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		fmt.Fprintln(os.Stderr, "tuimark: bad child args:", err)
		os.Exit(1)
	}
	if len(args) == 3 && args[0] == "run-plain" {
		app, err := load(args[1], args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, "tuimark:", err)
			os.Exit(1)
		}
		if err := app.Run(os.Stdout); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	c := &cli{stdout: os.Stdout, stderr: os.Stderr}
	os.Exit(c.run(args))
}

// hostEnv is the child's environment: this process's, without the
// variables Run reads (§26.10) or its probe's skip rule looks at, plus a
// pinned TERM and color profile, plus extra.
func hostEnv(extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch {
		case strings.HasPrefix(k, "TUIMARK_"), k == "COLORFGBG", k == "COLORTERM", k == "TERM", k == "TERM_PROGRAM", k == "SSH_TTY", k == "WT_SESSION":
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "TERM=xterm-256color", "TUIMARK_COLOR=truecolor")
	return append(env, extra...)
}

// hostChildCmd re-execs this test binary as the child with this argv.
func hostChildCmd(argv, env []string) *exec.Cmd {
	raw, _ := json.Marshal(argv)
	cmd := exec.Command(os.Args[0], "-test.run=^TestHostSubcommandChild$", "-test.count=1")
	cmd.Env = append(env, hostPtyChildEnv+"="+string(raw))
	return cmd
}

// hostOpts configures a hostRig.
type hostOpts struct {
	env        []string // extra environment for the child
	cols, rows int      // the pty's size; 0 for 100x30
	probe      string   // the probe's replies; "" for hostProbeReplies
	noProbe    bool     // the session never starts: do not answer a probe
	noRead     bool     // never read fd 4 (a parent that stopped reading)
}

// hostLine is one fd 4 message: its raw line and its parsed members.
type hostLine struct {
	raw string
	m   map[string]any
}

// hostRig is a child on a pty, with fd 3 and 4 piped to this process,
// which acts as its parent.
type hostRig struct {
	cmd      *exec.Cmd
	master   *os.File
	slave    *os.File
	screen   *syncBuf
	stderr   *syncBuf
	toFd3    *os.File // this process writes; the child reads it as fd 3
	fromFd4  *os.File // this process reads; the child writes it as fd 4
	exited   chan struct{}
	readDone chan struct{}

	mu    sync.Mutex
	lines []hostLine // every fd 4 message, in arrival order
}

// syncBuf is a concurrency-safe growable buffer.
type syncBuf struct {
	mu  sync.Mutex
	buf []byte
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, p...)
	return len(p), nil
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.buf)
}

// startHost starts the child with argv on a fresh pty, with fd 3 and 4
// piped to the returned rig, and answers the capability probe.
func startHost(t *testing.T, o hostOpts, argv ...string) *hostRig {
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
	cols, rows := o.cols, o.rows
	if cols == 0 {
		cols, rows = 100, 30
	}
	if err := setWinsize(slave, cols, rows); err != nil {
		t.Fatal(err)
	}
	fd3ForChild, toFd3, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	fromFd4, fd4ForChild, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r := &hostRig{master: master, slave: slave, screen: &syncBuf{}, stderr: &syncBuf{}, toFd3: toFd3, fromFd4: fromFd4, exited: make(chan struct{}), readDone: make(chan struct{})}
	r.cmd = hostChildCmd(argv, hostEnv(o.env...))
	r.cmd.Stdin, r.cmd.Stdout, r.cmd.Stderr = slave, slave, r.stderr
	r.cmd.ExtraFiles = []*os.File{fd3ForChild, fd4ForChild}
	r.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := r.cmd.Start(); err != nil {
		slave.Close()
		master.Close()
		t.Skipf("cannot start a child on a pty: %v", err)
	}
	// Only the child holds its ends now, so closing ours reaches it as
	// end of file (fd 3) and a parent that is gone (fd 4).
	_ = fd3ForChild.Close()
	_ = fd4ForChild.Close()
	go func() { _, _ = io.Copy(r.screen, master) }()
	if o.noRead {
		close(r.readDone)
	} else {
		go r.readFd4()
	}
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
		toFd3.Close()
		fromFd4.Close()
	})
	if !o.noProbe {
		if !r.waitFor(r.screen, "\x1b[c", 10*slowdown*time.Second) {
			t.Fatalf("no capability probe; screen %q; %s", r.screen.String(), r.describe())
		}
		probe := o.probe
		if probe == "" {
			probe = hostProbeReplies
		}
		r.write(t, probe)
	}
	return r
}

// startHostApp starts `tuimark host FILE --data DATA args…` and waits for
// its first frame.
func startHostApp(t *testing.T, o hostOpts, file, data string, args ...string) *hostRig {
	t.Helper()
	r := startHost(t, o, append([]string{"host", file, "--data", data}, args...)...)
	if !r.waitFor(r.screen, "HOSTAPP", 10*slowdown*time.Second) {
		t.Fatalf("no first frame; %s", r.describe())
	}
	return r
}

func (r *hostRig) readFd4() {
	defer close(r.readDone)
	sc := bufio.NewScanner(r.fromFd4)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var m map[string]any
		_ = json.Unmarshal(sc.Bytes(), &m)
		r.mu.Lock()
		r.lines = append(r.lines, hostLine{raw: sc.Text(), m: m})
		r.mu.Unlock()
	}
}

// send writes one message line to the child's fd 3.
func (r *hostRig) send(t *testing.T, v any) {
	t.Helper()
	line, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	r.sendRaw(t, string(line))
}

func (r *hostRig) sendRaw(t *testing.T, line string) {
	t.Helper()
	if _, err := r.toFd3.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}

// snapshot returns the fd 4 messages so far.
func (r *hostRig) snapshot() []hostLine {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]hostLine(nil), r.lines...)
}

// waitMsg waits for the first fd 4 message at index from or later that
// matches pred, and returns the index after it (where the next search
// starts) and the message.
func (r *hostRig) waitMsg(t *testing.T, from int, pred func(hostLine) bool) (int, hostLine) {
	t.Helper()
	deadline := time.Now().Add(10 * slowdown * time.Second)
	for {
		lines := r.snapshot()
		for i := from; i < len(lines); i++ {
			if lines[i].m != nil && pred(lines[i]) {
				return i + 1, lines[i]
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no matching fd 4 message; %s", r.describe())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// waitRaw waits for the fd 4 message whose line is exactly raw, and
// returns the index after it.
func (r *hostRig) waitRaw(t *testing.T, from int, raw string) int {
	t.Helper()
	i, _ := r.waitMsg(t, from, func(l hostLine) bool { return l.raw == raw })
	return i
}

func ofType(typ string) func(hostLine) bool {
	return func(l hostLine) bool { return l.m["type"] == typ }
}

func (r *hostRig) waitFor(buf *syncBuf, want string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), want) {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func (r *hostRig) write(t *testing.T, s string) {
	t.Helper()
	if _, err := r.master.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func (r *hostRig) alive() bool {
	select {
	case <-r.exited:
		return false
	default:
		return true
	}
}

// exit waits for the child to exit (and for fd 4 to reach end of file)
// and returns its exit code; a child killed by a signal fails the test.
func (r *hostRig) exit(t *testing.T, d time.Duration) int {
	t.Helper()
	select {
	case <-r.exited:
	case <-time.After(d):
		t.Fatalf("the child did not exit within %v; %s", d, r.describe())
	}
	select {
	case <-r.readDone:
	case <-time.After(5 * slowdown * time.Second):
		t.Fatalf("fd 4 never reached end of file; %s", r.describe())
	}
	if ws, ok := r.cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		t.Fatalf("the child was killed by %v instead of exiting; %s", ws.Signal(), r.describe())
	}
	return r.cmd.ProcessState.ExitCode()
}

// assertRestored checks that the child left the terminal as it found it:
// the leave sequence is the last thing on the screen (§26.1 step 7) and
// the pty is in cooked mode again.
func (r *hostRig) assertRestored(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(3 * slowdown * time.Second)
	for !strings.HasSuffix(r.screen.String(), "\x1b[?1049l") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if s := r.screen.String(); !strings.HasSuffix(s, "\x1b[?1049l") {
		t.Errorf("the screen does not end with the leave sequence: ...%q", s[max(0, len(s)-80):])
	}
	// The master, not the slave: the child's session owned the slave, and
	// darwin revokes it when that session's leader exits.
	tio, err := unix.IoctlGetTermios(int(r.master.Fd()), ioctlGetTermios)
	if err != nil {
		t.Fatal(err)
	}
	if tio.Lflag&unix.ICANON == 0 || tio.Lflag&unix.ECHO == 0 {
		t.Error("the terminal was left in raw mode")
	}
}

// assertLast checks that the last fd 4 message is exactly raw ("exit" is
// "once, last").
func (r *hostRig) assertLast(t *testing.T, raw string) {
	t.Helper()
	lines := r.snapshot()
	if len(lines) == 0 || lines[len(lines)-1].raw != raw {
		t.Errorf("the last fd 4 message is not %s; %s", raw, r.describe())
	}
	if n := countType(lines, "exit"); n != 1 {
		t.Errorf("%d exit messages, want 1", n)
	}
}

func countType(lines []hostLine, typ string) int {
	n := 0
	for _, l := range lines {
		if l.m["type"] == typ {
			n++
		}
	}
	return n
}

func (r *hostRig) describe() string {
	var b strings.Builder
	fmt.Fprintf(&b, "stderr: %q\nfd 4:\n", r.stderr.String())
	for _, l := range r.snapshot() {
		b.WriteString("  " + l.raw + "\n")
	}
	return b.String()
}

// writeHostFixture writes a document and hostData to a temporary
// directory and returns their paths.
func writeHostFixture(t *testing.T, doc string) (file, data string) {
	t.Helper()
	dir := t.TempDir()
	file, data = dir+"/app.tui", dir+"/sample.json"
	if err := os.WriteFile(file, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(data, []byte(hostData), 0o644); err != nil {
		t.Fatal(err)
	}
	return file, data
}

// Test 103: handshake and data.
func TestHostProtocolHandshakeAndData(t *testing.T) {
	if os.Getenv(hostPtyChildEnv) != "" {
		t.Skip("child process")
	}
	quit := func(t *testing.T, r *hostRig) {
		t.Helper()
		r.write(t, "q")
		if code := r.exit(t, 10*slowdown*time.Second); code != 0 {
			t.Errorf("exit code %d, want 0", code)
		}
		r.assertLast(t, `{"type":"exit","reason":"quit"}`)
		r.assertRestored(t)
	}

	// ready: first, after the probe (the theme it carries is the one the
	// probe's OSC 11 reply resolved theme="auto" to), with protocol 1, the
	// bare version, the size, and the catalog entries in order.
	t.Run("ready", func(t *testing.T) {
		file, data := writeHostFixture(t, hostAutoDoc)
		r := startHostApp(t, hostOpts{cols: 90, rows: 20, probe: "\x1b]11;rgb:ffff/ffff/ffff\x1b\\" + hostProbeReplies}, file, data)
		// The pty is in raw mode now, as its master reports it (the check
		// assertRestored makes after the child exits).
		if tio, err := unix.IoctlGetTermios(int(r.master.Fd()), ioctlGetTermios); err != nil || tio.Lflag&unix.ICANON != 0 {
			t.Errorf("the running session's pty is not in raw mode (%v)", err)
		}
		_, first := r.waitMsg(t, 0, func(hostLine) bool { return true })
		want := `{"type":"ready","protocol":1,"version":"` + version + `","cols":90,"rows":20,"theme":"light","catalog":[` +
			`{"name":"pick","builtin":false,"sources":["#l on:select"]},` +
			`{"name":"quit","builtin":true,"sources":["keys q"]}],"diagnostics":[]}`
		if first.raw != want {
			t.Errorf("first fd 4 message\n got %s\nwant %s", first.raw, want)
		}
		quit(t, r)
	})

	// A document with an error-severity diagnostic still runs, and ready
	// carries its static diagnostics in the §14 JSON form.
	t.Run("ready carries diagnostics", func(t *testing.T) {
		file, data := writeHostFixture(t, hostBadDoc)
		r := startHostApp(t, hostOpts{}, file, data)
		_, ready := r.waitMsg(t, 0, ofType("ready"))
		diags, _ := ready.m["diagnostics"].([]any)
		if len(diags) != 1 {
			t.Fatalf("diagnostics = %v, want one V002", ready.m["diagnostics"])
		}
		d, _ := diags[0].(map[string]any)
		if d["code"] != "V002" || d["severity"] != "error" || d["msg"] == nil {
			t.Errorf("diagnostic = %v", d)
		}
		quit(t, r)
	})

	// --theme is Set("@theme", …), and TUIMARK_THEME beats it (§15.1).
	t.Run("theme", func(t *testing.T) {
		file, data := writeHostFixture(t, hostDoc)
		for _, c := range []struct {
			env  []string
			want string
		}{{nil, "light"}, {[]string{"TUIMARK_THEME=dark"}, "dark"}} {
			r := startHostApp(t, hostOpts{env: c.env}, file, data, "--theme", "light")
			_, ready := r.waitMsg(t, 0, ofType("ready"))
			if ready.m["theme"] != c.want {
				t.Errorf("env %v: ready theme %v, want %s", c.env, ready.m["theme"], c.want)
			}
			quit(t, r)
		}
	})

	t.Run("set bind batch get", func(t *testing.T) {
		file, data := writeHostFixture(t, hostDoc)
		r := startHostApp(t, hostOpts{}, file, data)
		i, _ := r.waitMsg(t, 0, ofType("ready"))

		r.send(t, map[string]any{"type": "set", "id": 1, "path": "note", "value": "SETNOTE"})
		i = r.waitRaw(t, i, `{"type":"ack","id":1,"ok":true}`)
		if !r.waitFor(r.screen, "SETNOTE", 5*slowdown*time.Second) {
			t.Fatalf("the frame never showed the set; %s", r.describe())
		}
		r.send(t, map[string]any{"type": "bind", "id": "b1", "path": "rows.0", "value": "BOUND"})
		i = r.waitRaw(t, i, `{"type":"ack","id":"b1","ok":true}`)
		if !r.waitFor(r.screen, "BOUND", 5*slowdown*time.Second) {
			t.Fatalf("the frame never showed the bind; %s", r.describe())
		}
		r.send(t, map[string]any{"type": "batch", "id": 2, "writes": []any{
			map[string]any{"path": "rows.1", "value": "BAT1"},
			map[string]any{"path": "note", "value": "batnote"},
		}})
		i = r.waitRaw(t, i, `{"type":"ack","id":2,"ok":true}`)
		if !r.waitFor(r.screen, "BAT1", 5*slowdown*time.Second) || !r.waitFor(r.screen, "batnote", 5*slowdown*time.Second) {
			t.Fatalf("the frame never showed the batch; %s", r.describe())
		}
		r.send(t, map[string]any{"type": "get", "id": "g1", "path": "rows"})
		i = r.waitRaw(t, i, `{"type":"ack","id":"g1","ok":true,"found":true,"value":["BOUND","BAT1","c"]}`)
		r.send(t, map[string]any{"type": "get", "id": 3, "path": "nope"})
		i = r.waitRaw(t, i, `{"type":"ack","id":3,"ok":true,"found":false}`)

		// A failing batch applies nothing, and its ack carries the error.
		r.send(t, map[string]any{"type": "batch", "id": 4, "writes": []any{
			map[string]any{"path": "note", "value": "NOPE"},
			map[string]any{"path": "a..b", "value": 1},
		}})
		var ack hostLine
		i, ack = r.waitMsg(t, i, func(l hostLine) bool { return l.m["type"] == "ack" && l.m["id"] == float64(4) })
		if ack.m["ok"] != false || !strings.Contains(fmt.Sprint(ack.m["error"]), `bad path "a..b"`) {
			t.Errorf("failing batch ack = %s", ack.raw)
		}
		r.send(t, map[string]any{"type": "set", "id": 5, "path": "a..b", "value": 1})
		i, ack = r.waitMsg(t, i, func(l hostLine) bool { return l.m["type"] == "ack" && l.m["id"] == float64(5) })
		if ack.m["ok"] != false || ack.m["error"] == nil {
			t.Errorf("failing set ack = %s", ack.raw)
		}

		// Writes without an id: nothing on success, an error on failure.
		r.send(t, map[string]any{"type": "set", "path": "note", "value": "QUIET"})
		r.send(t, map[string]any{"type": "set", "path": "a..b", "value": 1})
		r.send(t, map[string]any{"type": "bind", "path": "a..b", "value": 1})
		r.send(t, map[string]any{"type": "batch", "writes": []any{map[string]any{"path": "a..b", "value": 1}}})
		r.send(t, map[string]any{"type": "get", "id": "g2", "path": "note"})
		j := r.waitRaw(t, i, `{"type":"ack","id":"g2","ok":true,"found":true,"value":"QUIET"}`)
		between := r.snapshot()[i : j-1]
		if len(between) != 3 {
			t.Errorf("want 3 messages for the 3 failing writes without id, got %d; %s", len(between), r.describe())
		}
		for _, l := range between {
			if l.raw != `{"type":"error","error":"tuimark: bad path \"a..b\""}` {
				t.Errorf("a failed write without id got %s", l.raw)
			}
		}
		quit(t, r)
	})

	// Started without fd 3 and fd 4, so that the Go runtime's own poller
	// is at fd 3: a non-blocking socket on stdin makes the child's os
	// package hand fd 0 to the poller at start-up, so the poller (kqueue on
	// darwin, epoll on Linux) takes fd 3, the lowest free number. The child
	// is started with syscall.ForkExec because os/exec would put the
	// socket back in blocking mode.
	t.Run("no fd 3 and 4", func(t *testing.T) {
		file, _ := writeHostFixture(t, hostDoc)
		fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(fds[0])
		defer unix.Close(fds[1])
		if err := unix.SetNonblock(fds[1], true); err != nil {
			t.Fatal(err)
		}
		out, err := os.CreateTemp(t.TempDir(), "out")
		if err != nil {
			t.Fatal(err)
		}
		defer out.Close()
		cmd := hostChildCmd([]string{"host", file}, hostEnv())
		pid, err := syscall.ForkExec(cmd.Path, cmd.Args, &syscall.ProcAttr{Env: cmd.Env, Files: []uintptr{uintptr(fds[1]), out.Fd(), out.Fd()}})
		if err != nil {
			t.Fatal(err)
		}
		var ws syscall.WaitStatus
		if _, err := syscall.Wait4(pid, &ws, 0, nil); err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(out.Name())
		s := string(raw)
		if !ws.Exited() || ws.ExitStatus() != 1 {
			t.Fatalf("status %v, want exit 1; output %q", ws, s)
		}
		if !strings.HasPrefix(s, "tuimark: host: fd 3: ") || strings.Count(s, "\n") != 1 {
			t.Errorf("output = %q, want one tuimark: host: fd 3: line", s)
		}
		if runtime.GOOS == "darwin" && !strings.Contains(s, "poller") {
			t.Errorf("output = %q, want the kqueue at fd 3 named as the runtime's poller", s)
		}
		// Nothing at all at fd 3 is refused too.
		cmd = hostChildCmd([]string{"host", file}, hostEnv())
		var plain syncBuf
		cmd.Stdout, cmd.Stderr = &plain, &plain
		if err := cmd.Run(); err == nil || !strings.HasPrefix(plain.String(), "tuimark: host: fd 3: ") {
			t.Errorf("exit %v, output %q", err, plain.String())
		}
	})

	// Pre-session failures: a stderr line and exit 1, no ready, nothing on
	// fd 4 at all, and the terminal untouched.
	preSession := func(t *testing.T, r *hostRig, want string) {
		t.Helper()
		if code := r.exit(t, 10*slowdown*time.Second); code != 1 {
			t.Errorf("exit code %d, want 1", code)
		}
		if s := r.stderr.String(); !strings.HasPrefix(s, "tuimark: host: ") || !strings.Contains(s, want) || strings.Count(s, "\n") != 1 {
			t.Errorf("stderr = %q, want one tuimark: host: line naming %q", s, want)
		}
		if lines := r.snapshot(); len(lines) != 0 {
			t.Errorf("fd 4 got %d messages, want none; %s", len(lines), r.describe())
		}
	}
	t.Run("invalid TUIMARK_ value", func(t *testing.T) {
		file, data := writeHostFixture(t, hostDoc)
		r := startHost(t, hostOpts{env: []string{"TUIMARK_THEME=bogus"}, noProbe: true}, "host", file, "--data", data)
		preSession(t, r, "TUIMARK_THEME")
		if s := r.screen.String(); s != "" {
			t.Errorf("the terminal was touched: %q", s)
		}
	})
	t.Run("stdin not a terminal", func(t *testing.T) {
		// No pty here: stdin is a pipe, fd 3 and fd 4 are real.
		file, data := writeHostFixture(t, hostDoc)
		fd3ForChild, toFd3, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer toFd3.Close()
		fromFd4, fd4ForChild, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer fromFd4.Close()
		stdin, stdinW, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer stdinW.Close()
		cmd := hostChildCmd([]string{"host", file, "--data", data}, hostEnv())
		var stderr syncBuf
		cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, &stderr, &stderr
		cmd.ExtraFiles = []*os.File{fd3ForChild, fd4ForChild}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		stdin.Close()
		fd3ForChild.Close()
		fd4ForChild.Close()
		werr := cmd.Wait()
		if ee, ok := werr.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
			t.Fatalf("exit = %v, want 1; stderr %q", werr, stderr.String())
		}
		if s := stderr.String(); !strings.HasPrefix(s, "tuimark: host: ") || !strings.Contains(s, "terminal") || strings.Count(s, "\n") != 1 {
			t.Errorf("stderr = %q", s)
		}
		if got, _ := io.ReadAll(fromFd4); len(got) != 0 {
			t.Errorf("fd 4 got %q, want nothing", got)
		}
	})
}

// Test 104: events.
func TestHostProtocolEvents(t *testing.T) {
	if os.Getenv(hostPtyChildEnv) != "" {
		t.Skip("child process")
	}
	file, data := writeHostFixture(t, hostDoc)
	const down, up = "\x1b[B", "\x1b[A"

	// A key fires a host action: event seq 1, and the loop waits for the
	// reply. set/get are served meanwhile, nothing is drawn, and the set
	// shows in the first frame after the reply.
	t.Run("event and reply", func(t *testing.T) {
		r := startHostApp(t, hostOpts{}, file, data)
		r.write(t, down)
		i := r.waitRaw(t, 0, `{"type":"event","seq":1,"action":"pick","source":"l","keys":{"r":"b"},"value":null}`)
		r.send(t, map[string]any{"type": "set", "id": "w", "path": "note", "value": "REPLIED"})
		i = r.waitRaw(t, i, `{"type":"ack","id":"w","ok":true}`)
		r.send(t, map[string]any{"type": "get", "id": "r", "path": "note"})
		i = r.waitRaw(t, i, `{"type":"ack","id":"r","ok":true,"found":true,"value":"REPLIED"}`)
		time.Sleep(300 * time.Millisecond)
		if strings.Contains(r.screen.String(), "REPLIED") {
			t.Fatal("a frame was drawn while the handler waited for its reply")
		}
		// A reply that carries an id is acked, like every message with an
		// id (§19.1 "Acks"; test 103).
		r.send(t, map[string]any{"type": "reply", "id": "rep", "seq": 1})
		r.waitRaw(t, i, `{"type":"ack","id":"rep","ok":true}`)
		if !r.waitFor(r.screen, "REPLIED", 5*slowdown*time.Second) {
			t.Fatalf("the write sent before the reply never showed; %s", r.describe())
		}
		r.write(t, "q")
		if code := r.exit(t, 10*slowdown*time.Second); code != 0 {
			t.Errorf("exit code %d", code)
		}
		r.assertLast(t, `{"type":"exit","reason":"quit"}`)
		if n := countType(r.snapshot(), "error"); n != 0 {
			t.Errorf("%d error messages; %s", n, r.describe())
		}
	})

	for _, c := range []struct {
		name  string
		reply map[string]any
		exit  string
		code  int
	}{
		{"quit reply", map[string]any{"type": "reply", "seq": 1, "quit": true}, `{"type":"exit","reason":"quit"}`, 0},
		{"error reply", map[string]any{"type": "reply", "seq": 1, "error": "boom"}, `{"type":"exit","reason":"error","error":"boom"}`, 1},
		{"error wins over quit", map[string]any{"type": "reply", "seq": 1, "quit": true, "error": "both"}, `{"type":"exit","reason":"error","error":"both"}`, 1},
		{"quit reply with an id", map[string]any{"type": "reply", "id": 5, "seq": 1, "quit": true}, `{"type":"exit","reason":"quit"}`, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := startHostApp(t, hostOpts{}, file, data)
			r.write(t, down)
			r.waitMsg(t, 0, ofType("event"))
			r.send(t, c.reply)
			if code := r.exit(t, 10*slowdown*time.Second); code != c.code {
				t.Errorf("exit code %d, want %d; %s", code, c.code, r.describe())
			}
			r.assertLast(t, c.exit)
			r.assertRestored(t)
			// The ack of a reply with an id comes right before the exit it
			// leads to; a reply without one is not acked.
			lines := r.snapshot()
			if _, ok := c.reply["id"]; ok {
				if len(lines) < 2 || lines[len(lines)-2].raw != `{"type":"ack","id":5,"ok":true}` {
					t.Errorf("no ack right before the exit; %s", r.describe())
				}
			} else if n := countType(lines, "ack"); n != 0 {
				t.Errorf("%d acks for a reply without an id; %s", n, r.describe())
			}
		})
	}

	// No reply within --reply-timeout: an error names the seq, the keys
	// pressed meanwhile are dispatched afterwards in order, and a late
	// reply (or one for a seq nothing waits on) gets an error naming it.
	t.Run("reply timeout", func(t *testing.T) {
		r := startHostApp(t, hostOpts{}, file, data, "--reply-timeout", "100ms")
		r.write(t, down)
		i := r.waitRaw(t, 0, `{"type":"event","seq":1,"action":"pick","source":"l","keys":{"r":"b"},"value":null}`)
		r.write(t, down+up)
		i = r.waitRaw(t, i, `{"type":"error","seq":1,"error":"reply timeout"}`)
		i = r.waitRaw(t, i, `{"type":"event","seq":2,"action":"pick","source":"l","keys":{"r":"c"},"value":null}`)
		i = r.waitRaw(t, i, `{"type":"error","seq":2,"error":"reply timeout"}`)
		i = r.waitRaw(t, i, `{"type":"event","seq":3,"action":"pick","source":"l","keys":{"r":"b"},"value":null}`)
		i = r.waitRaw(t, i, `{"type":"error","seq":3,"error":"reply timeout"}`)
		r.send(t, map[string]any{"type": "reply", "seq": 1, "quit": true})
		var late hostLine
		i, late = r.waitMsg(t, i, ofType("error"))
		if late.m["seq"] != float64(1) || late.m["error"] == "reply timeout" || late.m["id"] != nil {
			t.Errorf("late reply got %s", late.raw)
		}
		r.send(t, map[string]any{"type": "reply", "seq": 99})
		var unknown hostLine
		i, unknown = r.waitMsg(t, i, ofType("error"))
		if unknown.m["seq"] != float64(99) {
			t.Errorf("reply for an unknown seq got %s", unknown.raw)
		}
		// With an id, the error about a reply echoes it next to the seq.
		r.send(t, map[string]any{"type": "reply", "id": "late", "seq": 2})
		r.waitRaw(t, i, `{"type":"error","id":"late","seq":2,"error":"reply after the reply timeout"}`)
		if !r.alive() {
			t.Fatalf("a late quit reply ended the session; %s", r.describe())
		}
		r.write(t, "q")
		if code := r.exit(t, 10*slowdown*time.Second); code != 0 {
			t.Errorf("exit code %d", code)
		}
		r.assertLast(t, `{"type":"exit","reason":"quit"}`)
	})
}

// Test 105: robustness.
func TestHostProtocolRobustness(t *testing.T) {
	if os.Getenv(hostPtyChildEnv) != "" {
		t.Skip("child process")
	}
	file, data := writeHostFixture(t, hostDoc)
	const down = "\x1b[B"

	t.Run("malformed input", func(t *testing.T) {
		r := startHostApp(t, hostOpts{}, file, data)
		i, _ := r.waitMsg(t, 0, ofType("ready"))
		for _, c := range []struct{ line, want string }{
			{`not json`, `{"type":"error","error":"not a JSON object"}`},
			{`[1,2]`, `{"type":"error","error":"not a JSON object"}`},
			{`null`, `{"type":"error","error":"not a JSON object"}`},
			{`{"id":1,"path":"note","value":"X"}`, `{"type":"error","id":1,"error":"missing or non-string \"type\""}`},
			{`{"type":"nope","id":"u"}`, `{"type":"error","id":"u","error":"unknown type \"nope\""}`},
			{`{"type":"set","id":7,"value":"X"}`, `{"type":"error","id":7,"error":"\"set\" needs a string \"path\""}`},
			{`{"type":"set","id":8,"path":"note"}`, `{"type":"error","id":8,"error":"\"set\" needs \"value\""}`},
			{`{"type":"bind","path":3,"value":"X"}`, `{"type":"error","error":"\"bind\" needs a string \"path\""}`},
			{`{"type":"set","id":{},"path":"note","value":"X"}`, `{"type":"error","error":"\"id\" must be a number or a string"}`},
			{`{"type":"get","path":"note"}`, `{"type":"error","error":"\"get\" needs \"id\" and a string \"path\""}`},
			{`{"type":"batch","id":9,"writes":{"path":"note","value":"X"}}`, `{"type":"error","id":9,"error":"\"batch\" needs a \"writes\" array"}`},
			{`{"type":"batch","id":10,"writes":[{"value":"X"}]}`, `{"type":"error","id":10,"error":"\"writes\"[0] needs a string \"path\" and a \"value\""}`},
			{`{"type":"reply","seq":"1"}`, `{"type":"error","error":"\"reply\" needs an integer \"seq\""}`},
			{`{"type":"reply","seq":1,"quit":"yes"}`, `{"type":"error","seq":1,"error":"\"quit\" must be true or false"}`},
		} {
			r.sendRaw(t, c.line)
			i = r.waitRaw(t, i, c.want)
		}
		r.send(t, map[string]any{"type": "get", "id": "after", "path": ""})
		j := r.waitRaw(t, i, `{"type":"ack","id":"after","ok":true,"found":true,"value":{"note":"n0","rows":["a","b","c"]}}`)
		if j != i+1 {
			t.Errorf("unexpected messages between the last error and the get; %s", r.describe())
		}
		if !r.alive() {
			t.Fatal("malformed input ended the session")
		}
		r.write(t, "q")
		if code := r.exit(t, 10*slowdown*time.Second); code != 0 {
			t.Errorf("exit code %d", code)
		}
	})

	t.Run("fd 3 closed while nothing waits", func(t *testing.T) {
		r := startHostApp(t, hostOpts{}, file, data)
		r.waitMsg(t, 0, ofType("ready"))
		_ = r.toFd3.Close()
		if code := r.exit(t, 5*slowdown*time.Second); code != 0 {
			t.Errorf("exit code %d, want 0", code)
		}
		r.assertLast(t, `{"type":"exit","reason":"eof"}`)
		r.assertRestored(t)
	})

	t.Run("fd 3 closed while a handler waits", func(t *testing.T) {
		r := startHostApp(t, hostOpts{}, file, data, "--reply-timeout", "5s")
		r.write(t, down)
		r.waitMsg(t, 0, ofType("event"))
		closed := time.Now()
		_ = r.toFd3.Close()
		if code := r.exit(t, 10*slowdown*time.Second); code != 0 {
			t.Errorf("exit code %d, want 0", code)
		}
		if d := time.Since(closed); d > slowdown*time.Second {
			t.Errorf("the session took %v to end, want the wait to end at once (not the 5s reply timeout)", d)
		}
		r.assertLast(t, `{"type":"exit","reason":"eof"}`)
		if n := countType(r.snapshot(), "error"); n != 0 {
			t.Errorf("%d error messages; %s", n, r.describe())
		}
		r.assertRestored(t)
	})

	t.Run("SIGTERM while a handler waits", func(t *testing.T) {
		r := startHostApp(t, hostOpts{}, file, data, "--reply-timeout", "5s")
		r.write(t, down)
		r.waitMsg(t, 0, ofType("event"))
		sent := time.Now()
		if err := r.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		if code := r.exit(t, 10*slowdown*time.Second); code != 143 {
			t.Errorf("exit code %d, want 143", code)
		}
		if d := time.Since(sent); d > slowdown*time.Second {
			t.Errorf("the child took %v to exit, want within the 1s signal grace", d)
		}
		r.assertLast(t, `{"type":"exit","reason":"signal","signal":"SIGTERM"}`)
		r.assertRestored(t)
	})

	// A parent that keeps fd 4 open but never reads it: once the pipe's
	// buffer and the 1024-message queue are full, the session ends with
	// reason error (the TUIMARK_LOG end record says so; the exit message
	// itself cannot reach a parent that does not read) and the terminal
	// is restored.
	t.Run("parent not reading fd 4", func(t *testing.T) {
		logFile := t.TempDir() + "/session.ndjson"
		r := startHostApp(t, hostOpts{noRead: true, env: []string{"TUIMARK_LOG=" + logFile}}, file, data)
		for i := 0; r.alive() && i < 50000; i++ {
			if _, err := fmt.Fprintf(r.toFd3, `{"type":"get","id":%d,"path":"rows"}`+"\n", i); err != nil {
				break
			}
		}
		if code := r.exit(t, 10*slowdown*time.Second); code != 1 {
			t.Errorf("exit code %d, want 1 (reason error)", code)
		}
		r.assertRestored(t)
		if s := r.stderr.String(); s != "" {
			t.Errorf("stderr = %q, want nothing once the session started", s)
		}
		raw, _ := io.ReadAll(r.fromFd4)
		if !bytes.HasPrefix(raw, []byte(`{"type":"ready",`)) {
			t.Errorf("fd 4 does not start with ready: %.80q", raw)
		}
		log, err := os.ReadFile(logFile)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(log)), "\n")
		if last := lines[len(lines)-1]; !strings.Contains(last, `"end"`) || !strings.Contains(last, `"reason":"error"`) {
			t.Errorf("the log's last record is %s, want the end record with reason error", last)
		}
	})

	t.Run("a write to fd 4 failing", func(t *testing.T) {
		r := startHostApp(t, hostOpts{noRead: true}, file, data)
		_ = r.fromFd4.Close()
		r.send(t, map[string]any{"type": "get", "id": 1, "path": "rows"})
		if code := r.exit(t, 10*slowdown*time.Second); code != 1 {
			t.Errorf("exit code %d, want 1 (reason error)", code)
		}
		r.assertRestored(t)
	})

	// fd 1 gets exactly what Run() writes for the same document, input,
	// and terminal; no protocol byte reaches fd 1 or fd 2.
	t.Run("fd 1 is Run's", func(t *testing.T) {
		screens := map[string]string{}
		for _, mode := range []string{"host", "run-plain"} {
			argv := []string{"run-plain", file, data}
			if mode == "host" {
				argv = []string{"host", file, "--data", data}
			}
			r := startHost(t, hostOpts{}, argv...)
			if !r.waitFor(r.screen, "HOSTAPP", 10*slowdown*time.Second) {
				t.Fatalf("%s: no first frame; %s", mode, r.describe())
			}
			r.write(t, "q")
			if code := r.exit(t, 10*slowdown*time.Second); code != 0 {
				t.Fatalf("%s: exit code %d; %s", mode, code, r.describe())
			}
			r.assertRestored(t)
			if s := r.stderr.String(); s != "" {
				t.Errorf("%s: stderr = %q, want nothing", mode, s)
			}
			screens[mode] = r.screen.String()
		}
		if screens["host"] != screens["run-plain"] {
			t.Errorf("fd 1 differs from Run's\nhost: %q\n run: %q", screens["host"], screens["run-plain"])
		}
		if strings.Contains(screens["host"], `"type"`) {
			t.Errorf("a protocol message reached fd 1: %q", screens["host"])
		}
	})
}
