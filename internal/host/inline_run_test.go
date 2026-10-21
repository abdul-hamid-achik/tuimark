package host

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// inlineDoc is the screen of SPEC v0.4d test 294 with an <input> in place
// of the textarea (D3 brings the textarea), and a row that appears while
// more is set, which makes the region grow.
const inlineDoc = `<tui>
<style>.good { color: green; }</style>
<keymap>
  <bind keys="ctrl+q" action="quit"/>
</keymap>
<screen id="main" focus="#composer">
  <rule/>
  <input id="composer" bind="draft" placeholder="Message" on:submit="send"/>
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

// waitScreen waits until the emulated screen, after its scrollback, is
// text (trailing empty rows dropped).
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

func TestInlineSmoke(t *testing.T) {
	r := startInline(t, inlineDoc, 30, 10, RunOptions{}, nil)
	r.waitText("$ ls\nfile\n$ ./app\n──────────────────────────────\nMessage\nready")
	r.send("hi")
	r.waitText("$ ls\nfile\n$ ./app\n──────────────────────────────\nhi\nready")
	t.Logf("%q", r.term.bytes())
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%q", r.term.bytes())
	t.Logf("%q", r.term.text())
	if f := r.term.forbiddenSeen(); len(f) > 0 {
		t.Errorf("forbidden: %v", f)
	}
	if !strings.Contains(r.term.text(), "$ ./app") {
		t.Error("the prompt was lost")
	}
}
