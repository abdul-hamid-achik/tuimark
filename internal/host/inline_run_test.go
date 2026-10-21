package host

import (
	"context"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// inlineDoc is the screen of SPEC v0.4d test 294 // of the textarea (D3 brings the textarea), and a row that appears while
// more is set, which makes the region grow.
const inlineDoc = `<tui>
<style>.good { color: green; }</style>
<keymap>
  <bind keys="ctrl+q" action="quit"/>
</keymap>
<screen id="main" focus="#composer">
  <rule/>
  <textarea id="composer" bind="draft" placeholder="Message" on:submit="send"/>
  <text id="extra" if="more">extra row</text>
  <text id="status">{status}</text>
</screen>
<template id="card" props="who">
  <text class="who">{$who}: {m.text}</text>
</template>
</tui>`

// inlineRig runs inlineDoc inline through loopWith on a terminal emulator.
type inlineRig struct {
	t      *testing.T
	app    *App
	term   *vt
	in     *io.PipeWriter
	done   chan error
	cols   atomic.Int64
	rows   atomic.Int64
	resize chan struct{}
}

func startInline(t *testing.T, src string, cols, rows int, opts RunOptions, lg *runLog) *inlineRig {
	t.Helper()
	r := &inlineRig{t: t, app: doc(t, src), term: newVT(cols, rows), done: make(chan error, 1), resize: make(chan struct{}, 1)}
	r.cols.Store(int64(cols))
	r.rows.Store(int64(rows))
	_ = r.app.Bind("", map[string]any{"draft": "", "status": "ready", "more": false, "last": map[string]any{"text": "done"}})
	pr, pw := io.Pipe()
	r.in = pw
	opts.Inline = true
	// The shell's prompt is on the screen above the cursor.
	r.term.shell("$ ls\r\nfile\r\n$ ./app\r\n")
	go func() {
		r.done <- r.app.loopWith(context.Background(), pr, r.term, func() (int, int) { return int(r.cols.Load()), int(r.rows.Load()) }, r.resize, opts, lg)
	}()
	t.Cleanup(func() { pw.Close() })
	return r
}

func (r *inlineRig) send(s string) {
	r.t.Helper()
	if _, err := io.WriteString(r.in, s); err != nil {
		r.t.Fatal(err)
	}
}

// waitText waits until the emulated terminal, its scrollback then its
// screen, is want (trailing empty rows dropped).
func (r *inlineRig) waitText(want string) {
	r.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if r.term.text() == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	r.t.Fatalf("terminal:\n%s\nwant:\n%s", r.term.text(), want)
}

func (r *inlineRig) quit() error {
	r.t.Helper()
	r.send("\x11") // ctrl+q
	select {
	case err := <-r.done:
		return err
	case <-time.After(5 * time.Second):
		r.t.Fatal("the loop did not return")
	}
	return nil
}

const (
	ruleRow = "──────────────────────────────"
	prompt  = "$ ls\nfile\n$ ./app\n"
)

// SPEC 0.4d test 299 (the bytes): an inline session writes neither CSI
// ? 1049 h, CSI H, nor CSI 2 J; the shell's prompt above the region stays;
// the region grows by the row that appears and shrinks when it goes;
// quitting erases the region and leaves the cursor at the start of its
// first row. A row that appears (if="more") grows the region, and so does a
// line break in the textarea (TestInlineTextareaGrowsRun).
func TestInlineRegionRun(t *testing.T) {
	r := startInline(t, inlineDoc, 30, 10, RunOptions{}, nil)
	r.waitText(prompt + ruleRow + "\nMessage\nready")
	r.send("hi")
	r.waitText(prompt + ruleRow + "\nhi\nready")
	if err := r.app.Set("more", true); err != nil {
		t.Fatal(err)
	}
	r.waitText(prompt + ruleRow + "\nhi\nextra row\nready")
	if x, y := r.term.cursor(); x != 2 || y != 4 {
		t.Errorf("the caret is at %d,%d, want 2,4 (the composer's row)", x, y)
	}
	if err := r.app.Set("more", false); err != nil {
		t.Fatal(err)
	}
	r.waitText(prompt + ruleRow + "\nhi\nready")
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
	// The cursor is at the start of the row the region began on, now empty.
	if x, y := r.term.cursor(); x != 0 || y != 3 {
		t.Errorf("after the leave sequence the cursor is at %d,%d, want 0,3", x, y)
	}
	if got := r.term.text(); got != strings.TrimSuffix(prompt, "\n") {
		t.Errorf("after the leave sequence the terminal holds %q", got)
	}
	if f := r.term.forbiddenSeen(); len(f) > 0 {
		t.Errorf("an inline session wrote %v", f)
	}
	b := r.term.bytes()
	for _, s := range []string{"\x1b[?1049", "\x1b[H", "\x1b[2J"} {
		if strings.Contains(b, s) {
			t.Errorf("the session wrote %q", s)
		}
	}
	session := b[len("$ ls\r\nfile\r\n$ ./app\r\n"):]
	if !strings.HasPrefix(session, enterInline+"\r\x1b[J") {
		t.Errorf("the session starts with %q", session[:min(len(session), 40)])
	}
	if !strings.HasSuffix(b, "\x1b[1A\r\x1b[J"+leaveInline) {
		t.Errorf("the leave sequence is %q", b[max(0, len(b)-40):])
	}
}

// SPEC 0.4d test 299: a commit prints its rows above the region and the
// region is redrawn below them; a committed "You: hello" is written as
// itself, then CSI 0 m CR LF, with no trailing space; a Commit followed at
// once by Quit() leaves its rows above the cursor.
func TestInlineCommitRun(t *testing.T) {
	r := startInline(t, inlineDoc, 80, 10, RunOptions{}, nil)
	rule := strings.Repeat("─", 80)
	r.waitText(prompt + rule + "\nMessage\nready")
	if err := r.app.Commit(Commit{Lines: []any{"You: hello"}}); err != nil {
		t.Fatal(err)
	}
	r.waitText(prompt + "You: hello\n" + rule + "\nMessage\nready")
	if !strings.Contains(r.term.bytes(), "\x1b[J"+"You: hello\x1b[0m\r\n") {
		t.Errorf("the row is not written as CSI J, the text, CSI 0 m, CR LF: %q", r.term.bytes())
	}
	if err := r.app.Commit(Commit{Lines: []any{"bye"}}); err != nil {
		t.Fatal(err)
	}
	r.app.Quit()
	select {
	case err := <-r.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the loop did not return")
	}
	if got, want := r.term.text(), prompt+"You: hello\nbye"; got != want {
		t.Errorf("after the leave sequence:\n%s\nwant:\n%s", got, want)
	}
	if x, y := r.term.cursor(); x != 0 || y != 5 {
		t.Errorf("the cursor is at %d,%d, want the start of the empty row 0,5", x, y)
	}
}

// SPEC 0.4d test 299 and §18.3 "When it prints": a Commit made from a
// handler prints at once and returns after the print, before the frame
// that shows the handler's writes.
func TestInlineCommitFromHandler(t *testing.T) {
	r := startInline(t, inlineDoc, 30, 10, RunOptions{}, nil)
	var err1 atomic.Value
	r.app.On("send", func(ev Event) error {
		if err := r.app.Commit(Commit{Lines: []any{"sent: " + ev.Value.(string)}}); err != nil {
			err1.Store(err)
		}
		return r.app.Set("draft", "")
	})
	r.waitText(prompt + ruleRow + "\nMessage\nready")
	r.send("abc\r")
	r.waitText(prompt + "sent: abc\n" + ruleRow + "\nMessage\nready")
	if e := err1.Load(); e != nil {
		t.Fatal(e)
	}
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
}

// SPEC 0.4d test 297 (Go): a Commit from an Every function prints at once
// and returns nil; one from a Derive function returns "commit: inside a
// derive" and prints nothing.
func TestInlineCommitEveryAndDerive(t *testing.T) {
	r := startInline(t, inlineDoc, 30, 10, RunOptions{}, nil)
	var fired atomic.Int32
	var everyErr, deriveErr atomic.Value
	stop := r.app.Every(10*time.Millisecond, func(b *Batch) error {
		if fired.Add(1) == 1 {
			if err := r.app.Commit(Commit{Lines: []any{"tick"}}); err != nil {
				everyErr.Store(err)
			}
		}
		return nil
	})
	defer stop()
	if err := r.app.Derive("upper", func(get func(string) (any, bool)) (any, error) {
		if v, _ := get("status"); v == "again" {
			if err := r.app.Commit(Commit{Lines: []any{"from derive"}}); err != nil {
				deriveErr.Store(err)
			}
		}
		return "x", nil
	}); err != nil {
		t.Fatal(err)
	}
	r.waitText(prompt + "tick\n" + ruleRow + "\nMessage\nready")
	if e := everyErr.Load(); e != nil {
		t.Errorf("Commit from Every: %v", e)
	}
	if err := r.app.Set("status", "again"); err != nil { // runs the derive function
		t.Fatal(err)
	}
	e, _ := deriveErr.Load().(error)
	if e == nil || e.Error() != "commit: inside a derive" {
		t.Errorf("Commit from Derive: %v", e)
	}
	if strings.Contains(r.term.text(), "from derive") {
		t.Error("a commit from a derive was printed")
	}
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
}

// SPEC 0.4d test 299 (resize): a new size writes CSI N A (N the row of
// the region the cursor was on), CR, CSI J, and the region at the new
// width, and no CSI H or CSI 2 J; nothing of the old region is left after
// a widening.
func TestInlineResizeRun(t *testing.T) {
	r := startInline(t, inlineDoc, 30, 10, RunOptions{}, nil)
	r.waitText(prompt + ruleRow + "\nMessage\nready")
	r.send("hi")
	r.waitText(prompt + ruleRow + "\nhi\nready")
	before := len(r.term.bytes())
	r.term.mu.Lock()
	r.term.cols = 40 // the terminal is widened
	for i := range r.term.cells {
		r.term.cells[i] = append(r.term.cells[i], []rune(strings.Repeat(" ", 10))...)
	}
	r.term.mu.Unlock()
	r.cols.Store(40)
	r.resize <- struct{}{}
	wide := strings.Repeat("─", 40)
	r.waitText(prompt + wide + "\nhi\nready")
	got := r.term.bytes()[before:]
	if !strings.HasPrefix(got, "\x1b[1A\r\x1b[J") {
		t.Errorf("the resize starts with %q, want CSI 1 A, CR, CSI J", got[:min(len(got), 20)])
	}
	if f := r.term.forbiddenSeen(); len(f) > 0 {
		t.Errorf("a resize wrote %v", f)
	}
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
}

// SPEC 0.4d test 306 (the records the loop writes): an inline session's
// start record has inline true; each commit writes a commit record with
// line_count and rows, never the text.
func TestInlineLog(t *testing.T) {
	path := t.TempDir() + "/run.log"
	lg, err := openRunLog(path, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r := startInline(t, inlineDoc, 30, 10, RunOptions{}, lg)
	r.waitText(prompt + ruleRow + "\nMessage\nready")
	if err := r.app.Commit(Commit{Lines: []any{"secret one", "two\nlines"}}); err != nil {
		t.Fatal(err)
	}
	if err := r.app.Commit(Commit{Template: "card", With: "last as m", Props: map[string]string{"who": "C"}}); err != nil {
		t.Fatal(err)
	}
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
	recs := readLog(t, path)
	if recs[0]["ev"] != "start" || recs[0]["inline"] != true {
		t.Errorf("start record %v", recs[0])
	}
	var commits []map[string]any
	for _, rec := range recs {
		if rec["ev"] == "commit" {
			commits = append(commits, rec)
		}
	}
	if len(commits) != 2 || commits[0]["line_count"] != 2.0 || commits[0]["rows"] != 3.0 || commits[1]["line_count"] != 1.0 || commits[1]["rows"] != 1.0 {
		t.Errorf("commit records %v", commits)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "secret") {
		t.Error("the log holds the text of a commit")
	}
}

// SPEC 0.4d §26.15.1, decision 241: a document that writes version cannot
// run inline, and RunWith says so before it touches the terminal.
func TestInlinePinnedRunWith(t *testing.T) {
	a := doc(t, `<tui version="3"><screen id="s"><text>x</text></screen></tui>`)
	out := &syncBuf{}
	err := a.RunWith(context.Background(), out, RunOptions{Inline: true})
	if err == nil || err.Error() != "tuimark: inline mode needs a <tui> without version" {
		t.Errorf("RunWith = %v", err)
	}
	if out.String() != "" {
		t.Errorf("RunWith wrote %q", out.String())
	}
}

// SPEC 0.4d test 299: typing three lines with shift+enter grows the region
// by two rows, and sending them shrinks it again.
func TestInlineTextareaGrowsRun(t *testing.T) {
	r := startInline(t, inlineDoc, 30, 10, RunOptions{}, nil)
	r.waitText(prompt + ruleRow + "\nMessage\nready")
	shiftEnter := "\x1b[13;2u"
	r.send("one" + shiftEnter + "two" + shiftEnter + "three")
	r.waitText(prompt + ruleRow + "\none\ntwo\nthree\nready")
	if x, y := r.term.cursor(); x != 5 || y != 6 {
		t.Errorf("the caret is at %d,%d, want 5,6 (the last line of the composer)", x, y)
	}
	if f := r.term.forbiddenSeen(); len(f) > 0 {
		t.Errorf("an inline session wrote %v", f)
	}
	if err := r.app.Set("draft", ""); err != nil {
		t.Fatal(err)
	}
	r.waitText(prompt + ruleRow + "\nMessage\nready")
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
}
