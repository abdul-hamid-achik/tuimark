//go:build unix

package main

// SPEC v0.4a §15.9, test 121: whole `tuimark run` sessions under a pty,
// through host_pty_test.go's rig (the child re-execs this test binary with
// its argv; run does not use fd 3 and 4, which it leaves alone).

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// runPtyDoc fires save from s, pick from its list, and cc from ctrl+c.
const runPtyDoc = `<tui version="1">
<keymap>
  <bind keys="s" action="save"/>
  <bind keys="ctrl+c" action="cc"/>
  <bind keys="q" action="quit"/>
</keymap>
<screen id="s" focus="#l">
  <text>RUNAPP {saved}</text>
  <list id="l" each="rows as r" key="r" on:select="pick"><item><text>row-{r}</text></item></list>
</screen>
</tui>`

const runPtyData = `{"saved": "no", "rows": ["a", "b", "c"]}`

// startRun starts `tuimark run FILE args…` on a pty and waits for its
// first frame.
func startRun(t *testing.T, o hostOpts, file string, args ...string) *hostRig {
	t.Helper()
	r := startHost(t, o, append([]string{"run", file}, args...)...)
	if !r.waitFor(r.screen, "RUNAPP", 10*slowdown*time.Second) {
		t.Fatalf("no first frame; %s", runDescribe(r))
	}
	return r
}

func runFiles(t *testing.T) (dir, file, data string) {
	t.Helper()
	dir = t.TempDir()
	return dir, writeIn(t, dir, "app.tui", runPtyDoc), writeIn(t, dir, "data.json", runPtyData)
}

// Test 121: a host action applies its mock and is recorded in --events
// (0600) and in the strip, which F12 shows; quit exits 0 and restores the
// terminal.
func TestRunPtyMockEventsAndStrip(t *testing.T) {
	dir, file, data := runFiles(t)
	mock := writeIn(t, dir, "mock.json", `{"save": [{"path": "saved", "value": "yes"}]}`)
	ev := filepath.Join(dir, "ev.ndjson")
	r := startRun(t, hostOpts{cols: 60, rows: 12}, file, "--data", data, "--mock", mock, "--events", ev)
	r.write(t, "s")
	if !r.waitFor(r.screen, "yes", 5*slowdown*time.Second) {
		t.Fatalf("the mock was not applied; %s", runDescribe(r))
	}
	if strings.Contains(r.screen.String(), `save l {"r":"a"} null`) {
		t.Error("the strip shows before F12")
	}
	r.write(t, "\x1b[24~")
	if !r.waitFor(r.screen, `save l {"r":"a"} null`, 5*slowdown*time.Second) {
		t.Fatalf("F12 did not show the strip; %s", runDescribe(r))
	}
	r.write(t, "\x1b[B") // the list: pick, with the row's key
	// The diff rewrites only the cells that changed: "save" becomes "pick".
	if !r.waitFor(r.screen, "pick", 5*slowdown*time.Second) {
		t.Fatalf("no pick line; %s", runDescribe(r))
	}
	r.write(t, "q")
	if code := r.exit(t, 5*slowdown*time.Second); code != 0 {
		t.Fatalf("quit exited %d; %s", code, runDescribe(r))
	}
	r.assertRestored(t)
	raw, err := os.ReadFile(ev)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"seq":1,"action":"save","source":"l","keys":{"r":"a"},"value":null}
{"seq":2,"action":"pick","source":"l","keys":{"r":"b"},"value":null}
`
	if string(raw) != want {
		t.Errorf("--events:\n%s\nwant:\n%s", raw, want)
	}
	if st, err := os.Stat(ev); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("--events mode %v (%v), want 0600", st.Mode().Perm(), err)
	}
}

// Test 121, "Always an exit": the document binds ctrl+c; the first press
// is dispatched (its strip line shows), the second within a second exits
// 0.
func TestRunPtyDoubleCtrlC(t *testing.T) {
	_, file, data := runFiles(t)
	r := startRun(t, hostOpts{cols: 60, rows: 12}, file, "--data", data, "--strip")
	r.write(t, "\x03")
	if !r.waitFor(r.screen, `cc l {"r":"a"} null`, 5*slowdown*time.Second) {
		t.Fatalf("the first ctrl+c was not dispatched; %s", runDescribe(r))
	}
	r.write(t, "\x03")
	if code := r.exit(t, 5*slowdown*time.Second); code != 0 {
		t.Fatalf("two ctrl+c exited %d; %s", code, runDescribe(r))
	}
	r.assertRestored(t)
}

// A stop signal exits 128 + its number, as host does.
func TestRunPtySignalExit(t *testing.T) {
	_, file, data := runFiles(t)
	r := startRun(t, hostOpts{cols: 60, rows: 12}, file, "--data", data)
	if err := r.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if code := r.exit(t, 5*slowdown*time.Second); code != 128+int(syscall.SIGTERM) {
		t.Fatalf("SIGTERM exited %d, want %d; %s", code, 128+int(syscall.SIGTERM), runDescribe(r))
	}
	r.assertRestored(t)
}

// Test 121, "Reload": an edit that breaks the document shows its
// diagnostics in the panel while the old app runs on; the fix reloads it
// (the strip's "reload" line), and the session goes on.
func TestRunPtyReload(t *testing.T) {
	_, file, data := runFiles(t)
	r := startRun(t, hostOpts{cols: 70, rows: 12}, file, "--data", data, "--strip")
	writeAtomic(t, file, strings.Replace(runPtyDoc, "<screen", "<screen bogus=\"1\"", 1))
	if !r.waitFor(r.screen, "V002", 5*slowdown*time.Second) {
		t.Fatalf("no panel for the broken document; %s", runDescribe(r))
	}
	r.write(t, "s") // the old app still runs
	if !r.waitFor(r.screen, `save l {"r":"a"} null`, 5*slowdown*time.Second) {
		t.Fatalf("the old app stopped; %s", runDescribe(r))
	}
	writeAtomic(t, file, strings.Replace(runPtyDoc, "RUNAPP", "EDITED", 1))
	if !r.waitFor(r.screen, "EDITED", 5*slowdown*time.Second) || !r.waitFor(r.screen, "reload", 5*slowdown*time.Second) {
		t.Fatalf("no reload; %s", runDescribe(r))
	}
	r.write(t, "q")
	if code := r.exit(t, 5*slowdown*time.Second); code != 0 {
		t.Fatalf("quit after a reload exited %d; %s", code, runDescribe(r))
	}
	r.assertRestored(t)
}

// runDescribe is describe plus the end of what the child wrote to the
// terminal.
func runDescribe(r *hostRig) string {
	s := r.screen.String()
	return r.describe() + "screen: ..." + strings.ReplaceAll(s[max(0, len(s)-1500):], "\x1b", "^[")
}

// Test 121: --events naming a terminal is refused before the session.
func TestRunEventsTerminalRefused(t *testing.T) {
	master, name, err := openPTY()
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	defer master.Close()
	_, file, data := runFiles(t)
	code, stdout, stderr := runCLI("run", file, "--data", data, "--events", name)
	if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "tuimark: run: --events: ") || !strings.Contains(stderr, "is a terminal") || strings.Count(stderr, "\n") != 1 {
		t.Errorf("exit %d stdout %q stderr %q", code, stdout, stderr)
	}
}

// SPEC v0.4b test 188, "tuimark run": space toggles the checkbox, its
// on:change applies the mock, and the strip and --events carry the new
// boolean with empty keys.
func TestRunPtyCheckboxMock(t *testing.T) {
	dir := t.TempDir()
	file := writeIn(t, dir, "checkbox.tui", `<tui>
  <screen id="main" focus="#dark">
    <checkbox id="dark" bind="settings.dark" label="Dark mode" on:change="dark_changed"/>
    <checkbox id="beta" label="Beta"/>
    <text>RUNAPP {saved}</text>
  </screen>
</tui>`)
	data := writeIn(t, dir, "checkbox.json", `{"settings": {"dark": true}, "saved": false}`)
	mock := writeIn(t, dir, "mock.json", `{"dark_changed": [{"path": "saved", "value": true}]}`)
	ev := filepath.Join(dir, "ev.ndjson")
	r := startRun(t, hostOpts{cols: 60, rows: 12}, file, "--data", data, "--mock", mock, "--events", ev, "--strip")
	if strings.Contains(r.screen.String(), "true") {
		t.Fatalf("the first frame already shows true; %s", runDescribe(r))
	}
	r.write(t, " ")
	// The diff rewrites only the cells that changed: "false" becomes "true ".
	if !r.waitFor(r.screen, "true", 5*slowdown*time.Second) {
		t.Fatalf("the mock was not applied; %s", runDescribe(r))
	}
	if !r.waitFor(r.screen, "dark_changed dark {} false", 5*slowdown*time.Second) {
		t.Fatalf("no strip line; %s", runDescribe(r))
	}
	r.write(t, "\x03")
	if code := r.exit(t, 5*slowdown*time.Second); code != 0 {
		t.Fatalf("ctrl+c exited %d; %s", code, runDescribe(r))
	}
	r.assertRestored(t)
	raw, err := os.ReadFile(ev)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"seq":1,"action":"dark_changed","source":"dark","keys":{},"value":false}` + "\n"; string(raw) != want {
		t.Errorf("--events:\n%s\nwant:\n%s", raw, want)
	}
}
