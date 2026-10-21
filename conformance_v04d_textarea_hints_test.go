package tuimark_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4d §21 tests 312 and 314 of phase D3: the word moves of a
// textarea on left and right, and the hints' second pass (§18 step 6a).

// 312, the textarea part: ctrl+left (CSI 1;5 D) puts the caret where alt+b
// does; the input halves of the test are phase D2's.
func TestD3Textarea312WordMoveOnLeft(t *testing.T) {
	src := taSource(t, "t284/t.tui")
	draft := `{"draft": "one two\nthree"}`
	app := btnApp(t, src, draft)
	step := func(s string) string {
		t.Helper()
		res, err := app.Play(tuimark.PlayOptions{Cols: 30, Rows: 6, NoHandlers: true}, s)
		if err != nil {
			t.Fatal(err)
		}
		return taCaret(t, res.Dump, "composer")
	}
	if got := step(`text:` + "\x1b[1;5D"); !strings.HasPrefix(got, "1,0@") {
		t.Errorf("ctrl+left: caret %s, want 1,0", got)
	}
	if got := step(`text:` + "\x1b[1;5D"); !strings.HasPrefix(got, "0,4@") {
		t.Errorf("ctrl+left again: caret %s, want 0,4", got)
	}
	// alt+right and ctrl+right are alt+f.
	if got := step(`text:` + "\x1b[1;3C"); !strings.HasPrefix(got, "0,7@") {
		t.Errorf("alt+right: caret %s, want 0,7", got)
	}
	if got := step(`text:` + "\x1b[1;5C"); !strings.HasPrefix(got, "1,5@") {
		t.Errorf("ctrl+right: caret %s, want 1,5", got)
	}
	// A plain left moves one cluster, and the keymap still sees left.
	if got := step(`text:` + "\x1b[D"); !strings.HasPrefix(got, "1,4@") {
		t.Errorf("left: caret %s, want 1,4", got)
	}
	// ctrl with a character that is not a letter gives no key: nothing is
	// typed (decision 263).
	app = btnApp(t, src, draft)
	res, err := app.Play(tuimark.PlayOptions{Cols: 30, Rows: 6, NoHandlers: true}, "text:\x1b[32;5u", "text:\x1b[51;5u", "text:\x1b[27;5;47~")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := app.Get("draft"); v != "one two\nthree" || len(res.Events) != 0 {
		t.Errorf("ctrl+space and friends changed %q / fired %v", v, taEvents(res))
	}
}

// itemsOf is the hint items of a dump: the texts of the hints node's row
// descendants, from the grid row of the textarea.
func itemsOf(d *tuimark.Dump) string {
	return strings.Join(strings.Fields(d.Grid[0]), " ")
}

// 314. Hints and a focused textarea (decision 259).
func TestD3Textarea314HintsSecondPass(t *testing.T) {
	src := taSource(t, "t314/h.tui")
	dump := func(draft string) string {
		t.Helper()
		b, _ := json.Marshal(map[string]string{"draft": draft})
		d, err := btnApp(t, src, string(b)).Dump(60, 3)
		if err != nil {
			t.Fatal(err)
		}
		return itemsOf(d)
	}
	// The caret is on the textarea's only visual line: up goes on to the
	// keymap.
	if got := dump(""); got != "Message up history ctrl+enter send" {
		t.Errorf("empty draft: %q", got)
	}
	// The caret is at the end of the last of two visual lines: up would
	// move the caret.
	if got := dump("a\nb"); got != "a ctrl+enter send" {
		t.Errorf("two lines: %q", got)
	}

	// A session: text:a and shift+enter hide up; one more up shows it.
	app := btnApp(t, src, `{"draft": ""}`)
	play := func(step string) *tuimark.Dump {
		t.Helper()
		res, err := app.Play(tuimark.PlayOptions{Cols: 60, Rows: 3, NoHandlers: true}, step)
		if err != nil {
			t.Fatal(err)
		}
		return res.Dump
	}
	fresh := func() string {
		t.Helper()
		v, _ := app.Get("draft")
		return dump(v.(string))
	}
	d := play("text:a")
	if got := itemsOf(d); got != "a up history ctrl+enter send" || got != fresh() {
		t.Errorf("after text:a: %q (fresh %q)", got, fresh())
	}
	d = play("shift+enter")
	if got := itemsOf(d); strings.Contains(got, "up history") || !strings.Contains(got, "ctrl+enter send") {
		t.Errorf("after shift+enter: %q", got)
	}
	if a, b := itemsOf(d), fresh(); a != b {
		t.Errorf("after shift+enter the session shows %q, a fresh app %q", a, b)
	}
	d = play("up")
	if got := itemsOf(d); !strings.Contains(got, "up history") {
		t.Errorf("after the up: %q", got)
	}
	// The caret lives outside the store: a fresh app puts it at the end.
	if a, b := itemsOf(d), fresh(); a == b {
		t.Errorf("after the last up the session and a fresh app agree: %q", a)
	}
}
