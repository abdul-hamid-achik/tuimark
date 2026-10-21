package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// SPEC 0.4d test 293 and §6.19.7 (TUIMARK_LOG), through a Run session on
// a fake terminal: the terminal's cursor goes to the caret's cell, the
// cell moves with a line break the decoder reads in every session, and an
// LF the textarea takes as a line break is logged as the key enter.
func TestRunTextareaCursorAndLog(t *testing.T) {
	const src = `<tui>
  <screen id="main" focus="#t">
    <textarea id="t" bind="text" placeholder="Message" on:change="chg"/>
    <text id="status">ready</text>
  </screen>
</tui>`
	path := filepath.Join(t.TempDir(), "run.log")
	lg, err := openRunLog(path, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r := startSession(t, src, &termSession{log: lg})
	// The empty textarea's caret is at its origin.
	r.waitOut(t, "\x1b[1;1H\x1b[?25h")
	r.send(t, "hi")
	r.waitOut(t, "\x1b[1;3H\x1b[?25h")
	// shift+enter as CSI 13 ; 2 u: the cursor is at column 0 of the next
	// row, and the status line moved down with the grown textarea.
	r.send(t, "\x1b[13;2u")
	r.waitOut(t, "\x1b[2;1H\x1b[?25h")
	if got := r.app.Frame(20, 6).Grid.Lines(); got[0] != "hi                  " || got[1] != "                    " || got[2] != "ready               " {
		t.Errorf("rows %q", got[:3])
	}
	// The LF byte is a line break here; the bound text sees it.
	r.send(t, "x\ny")
	r.waitOut(t, "\x1b[3;2H\x1b[?25h")
	if v, _ := r.app.Get("text"); v != "hi\nx\ny" {
		t.Errorf("text %q", v)
	}
	r.send(t, "\x03")
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	lg.end("quit")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	log := string(raw)
	if !strings.Contains(log, `{"ev":"key",`) || !strings.Contains(log, `"key":"enter"`) {
		t.Errorf("the LF is not logged as the key enter:\n%s", log)
	}
	if strings.Contains(log, "redacted") {
		t.Errorf("a textarea is never secret:\n%s", log)
	}
}

// SPEC 0.4d §6.19.2: the caret stays where the textarea's own edits put
// it and goes to the end of a value changed from outside; a write of the
// value it already holds is no change.
func TestTextareaCaretOnOutsideChange(t *testing.T) {
	a := doc(t, `<tui><screen id="main" focus="#t"><textarea id="t" bind="d"/></screen></tui>`)
	_ = a.Bind("", map[string]any{"d": "hello"})
	caret := func() int { return a.Frame(20, 4).ByID["t"].Cursor }
	if c := caret(); c != 5 {
		t.Fatalf("first caret %d, want the end", c)
	}
	a.HandleKey(Key{Name: "home"})
	if c := caret(); c != 0 {
		t.Fatalf("home: %d", c)
	}
	// The same value: no change.
	_ = a.Set("d", "hello")
	if c := caret(); c != 0 {
		t.Errorf("a write of the value it holds moved the caret to %d", c)
	}
	// Another value: the end of the new value.
	_ = a.Set("d", "a new value")
	if c := caret(); c != 11 {
		t.Errorf("an outside change put the caret at %d, want 11", c)
	}
	// A Derive recomputation and a Batch are outside changes too.
	a.HandleKey(Key{Name: "home"})
	if err := a.Batch(func(b *Batch) error { b.Set("d", "xy"); return nil }); err != nil {
		t.Fatal(err)
	}
	if c := caret(); c != 2 {
		t.Errorf("Batch: caret %d", c)
	}
	// An Append is an outside change too.
	a.HandleKey(Key{Name: "home"})
	if err := a.Append("d", "zz"); err != nil {
		t.Fatal(err)
	}
	if c := caret(); c != 4 {
		t.Errorf("Append: caret %d, want 4", c)
	}
	_ = a.Set("d", "xy")
	// The textarea's own edit is not an outside change.
	a.HandleKey(Key{Name: "home"})
	a.HandleKey(Key{Name: "x", Rune: 'x'})
	if v, _ := a.Get("d"); v != "xxy" {
		t.Fatalf("value %v", v)
	}
	if c := caret(); c != 1 {
		t.Errorf("after its own edit the caret is at %d, want 1", c)
	}
}

// A paste into a textarea keeps its line breaks and is one change; a
// paste into nothing focused is dropped (SPEC v0.4d §6.19.6).
func TestTextareaHandlePaste(t *testing.T) {
	a := doc(t, `<tui><screen id="main" focus="#t"><textarea id="t" on:change="chg"/></screen></tui>`)
	a.Frame(20, 4)
	evs := a.HandlePaste("a\r\nb\rc\td\x1b[31mred\x1b[0m\x00")
	if len(evs) != 1 || evs[0].Value != "a\nb\nc    dred" {
		t.Fatalf("events %+v", evs)
	}
	if evs := a.HandlePaste(""); len(evs) != 0 {
		t.Errorf("an empty paste: %+v", evs)
	}
	// Coalesced printable keys are one edit.
	keys := []Key{{Name: "o", Rune: 'o'}, {Name: "k", Rune: 'k'}, {Name: "enter"}}
	evs, n := a.HandleKeyRun(keys)
	if n != 2 || len(evs) != 1 || evs[0].Value != "a\nb\nc    dredok" {
		t.Errorf("run: n %d, events %+v", n, evs)
	}
}

// A textarea bound to a derived path changes nothing and fires no
// on:change (SPEC v0.4a §18.2).
func TestTextareaDerivedPathBlocks(t *testing.T) {
	a := doc(t, `<tui><screen id="main" focus="#t"><textarea id="t" bind="d" on:change="chg"/></screen></tui>`)
	if err := a.Derive("d", func(get func(string) (any, bool)) (any, error) { return "fixed", nil }); err != nil {
		t.Fatal(err)
	}
	a.Frame(20, 4)
	if evs := a.HandleKey(Key{Name: "x", Rune: 'x'}); len(evs) != 0 {
		t.Errorf("events %+v", evs)
	}
	if v, _ := a.Get("d"); v != "fixed" {
		t.Errorf("d = %v", v)
	}
}
