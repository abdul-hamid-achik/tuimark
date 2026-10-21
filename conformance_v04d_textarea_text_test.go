package tuimark_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4d §21 tests 289-292 of phase D3: wrapping, paste and a value
// the host writes, enter and the LF byte, mouse and disabled.

const taOne = `<tui><screen id="main" focus="#t"><textarea id="t" bind="d"/></screen></tui>`

// 289. Wrapping (decision 230): with 10 columns of content, the visual
// lines wrap at 9 and the caret stands where §6.19.4 says.
func TestD3Textarea289Wrapping(t *testing.T) {
	data := func(v string) string {
		b, _ := json.Marshal(map[string]string{"d": v})
		return string(b)
	}
	for _, c := range []struct {
		name  string
		value string
		steps []string
		rows  []string // the grid rows of the textarea
		caret string
	}{
		{"wrapped at 9", "abcdefghijkl", nil, []string{"abcdefghi ", "jkl       "}, "0,12@3,1"},
		{"full line", "abcdefghi", nil, []string{"abcdefghi "}, "0,9@9,0"},
		{"hanging spaces, the end of the first line", "ab         cd", []string{"left", "left", "left"}, []string{"ab        ", "cd        "}, "0,10@9,0"},
		{"hanging spaces, the start of the second line", "ab         cd", []string{"left", "left"}, []string{"ab        ", "cd        "}, "0,11@0,1"},
		{"an empty hard line", "a\n", nil, []string{"a         ", "          "}, "1,0@0,1"},
	} {
		res := taPlay(t, taOne, data(c.value), 10, 6, c.steps...)
		for i, want := range c.rows {
			if got := res.Dump.Grid[i]; got != want {
				t.Errorf("%s: row %d %q, want %q", c.name, i, got, want)
			}
		}
		n := taNode(t, res.Dump, "t")
		if n.H != len(c.rows) {
			t.Errorf("%s: %d rows high, want %d", c.name, n.H, len(c.rows))
		}
		// The caret's line is the hard line; its column the column in it.
		got := taCaret(t, res.Dump, "t")
		if got != c.caret {
			t.Errorf("%s: caret %s, want %s", c.name, got, c.caret)
		}
	}
}

// 290. Paste, and a value the host writes (decisions 234, 262, 265).
func TestD3Textarea290PasteAndHostValue(t *testing.T) {
	src := taSource(t, "t284/t.tui")
	empty := `{"draft": ""}`
	res := taPlay(t, src, empty, 30, 6, "paste:a\r\nb\rc\td")
	if got := taEvents(res); len(got) != 1 || got[0] != `typed composer {} "a\nb\nc    d"` {
		t.Errorf("paste: %q", got)
	}
	// The same paste into an input is one on:change, as in 0.4.5.
	in := `<tui><screen id="main" focus="#q"><input id="q" bind="q" on:change="ch"/></screen></tui>`
	res = taPlay(t, in, `{"q": ""}`, 30, 3, "paste:a\r\nb\rc\td")
	if got := taEvents(res); len(got) != 1 || got[0] != `ch q {} "a b c d"` {
		t.Errorf("input paste: %q", got)
	}
	// ESC is removed alone before the TAB, which becomes four spaces; an
	// input keeps §26.7's order.
	res = taPlay(t, src, empty, 30, 6, "paste:a\x1b\tb")
	if got := taEvents(res); len(got) != 1 || got[0] != `typed composer {} "a    b"` {
		t.Errorf("paste ESC TAB: %q", got)
	}
	// The string of the host-value half of the test, pasted: the same text.
	res = taPlay(t, src, empty, 30, 6, "paste:a\tb\x1bc")
	if got := taEvents(res); len(got) != 1 || got[0] != `typed composer {} "a    b"` {
		t.Errorf("paste a TAB b ESC c: %q", got)
	}
	res = taPlay(t, in, `{"q": ""}`, 30, 3, "paste:a\x1b\tb")
	if got := taEvents(res); len(got) != 1 || got[0] != `ch q {} "a"` {
		t.Errorf("input paste ESC TAB: %q", got)
	}

	// A value the host writes is read as run text.
	raw := strings.Replace(src, `<text id="status">ready</text>`, `<text id="status">ready</text><text id="raw">{draft}</text>`, 1)
	host := `{"draft": "a\tb\u001bc"}`
	app := btnApp(t, raw, host)
	d, err := app.Dump(30, 6)
	if err != nil {
		t.Fatal(err)
	}
	if n := taNode(t, d, "composer"); n.Text != "a    b" || taCaret(t, d, "composer") != "0,6@6,1" {
		t.Errorf("host value: text %q caret %s", n.Text, taCaret(t, d, "composer"))
	}
	if got := d.Grid[1]; got != "a    b                        " {
		t.Errorf("row %q", got)
	}
	// enter sends the text as read; the store, and {draft}, are untouched.
	res, err = app.Play(tuimark.PlayOptions{Cols: 30, Rows: 6, NoHandlers: true}, "enter")
	if err != nil {
		t.Fatal(err)
	}
	if got := taEvents(res); len(got) != 1 || got[0] != `send composer {} "a    b"` {
		t.Errorf("enter: %q", got)
	}
	if v, _ := app.Get("draft"); v != "a\tb\x1bc" {
		t.Errorf("Get after enter: %q", v)
	}
	if got := taNode(t, res.Dump, "raw").Text; got != "ab" {
		t.Errorf("{draft} after enter: %q, want ab (nothing was written)", got)
	}
	// An edit writes the normalized text with the edit applied.
	res, err = app.Play(tuimark.PlayOptions{Cols: 30, Rows: 6, NoHandlers: true}, "text:X")
	if err != nil {
		t.Fatal(err)
	}
	if got := taEvents(res); len(got) != 1 || got[0] != `typed composer {} "a    bX"` {
		t.Errorf("text:X: %q", got)
	}
	if v, _ := app.Get("draft"); v != "a    bX" {
		t.Errorf("Get after the edit: %q", v)
	}
	if got := taNode(t, res.Dump, "raw").Text; got != "a    bX" {
		t.Errorf("{draft} after the edit: %q", got)
	}
	// A key that changes nothing writes nothing.
	app = btnApp(t, raw, host)
	if _, err := app.Play(tuimark.PlayOptions{Cols: 30, Rows: 6, NoHandlers: true}, "home", "backspace"); err != nil {
		t.Fatal(err)
	}
	if v, _ := app.Get("draft"); v != "a\tb\x1bc" {
		t.Errorf("backspace at the start wrote %q", v)
	}
	// ESC before a TAB, and an escape sequence with an intermediate.
	for _, c := range []struct{ val, want string }{{"a\x1b\tb", "a    b"}, {"a\x1b(Bb", "ab"}} {
		b, _ := json.Marshal(map[string]string{"draft": c.val})
		d, err := btnApp(t, src, string(b)).Dump(30, 6)
		if err != nil {
			t.Fatal(err)
		}
		if got := taNode(t, d, "composer").Text; got != c.want {
			t.Errorf("host value %q: text %q, want %q", c.val, got, c.want)
		}
	}
	// Bound to an input, it keeps 0.4.5's rule: ab.
	d, err = btnApp(t, in, `{"q": "a\tb\u001bc"}`).Dump(30, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := taNode(t, d, "q").Text; got != "ab" {
		t.Errorf("input shows %q, want ab", got)
	}
}

// 291. enter without on:submit, and the LF byte (decisions 217 and 219).
func TestD3Textarea291EnterAndLF(t *testing.T) {
	src := taSource(t, "t284/t.tui")
	noSubmit := strings.Replace(src, ` on:submit="send"`, "", 1)
	res := taPlay(t, noSubmit, `{"draft": ""}`, 30, 6, "text:hello", "enter", "text:x")
	want := []string{`typed composer {} "hello"`, `typed composer {} "hello\n"`, `typed composer {} "hello\nx"`}
	sameStrings(t, "enter without on:submit", taEvents(res), want)

	// A text step with an LF: the LF ends the run of printable keys.
	res = taPlay(t, src, `{"draft": ""}`, 30, 6, "text:x\ny")
	sameStrings(t, "LF in a text step", taEvents(res), []string{`typed composer {} "x"`, `typed composer {} "x\n"`, `typed composer {} "x\ny"`})
	// An input with on:submit submits x and then types y, as in 0.4.4.
	in := `<tui><screen id="main" focus="#q"><input id="q" bind="q" on:change="ch" on:submit="sub"/></screen></tui>`
	res = taPlay(t, in, `{"q": ""}`, 30, 3, "text:x\ny")
	sameStrings(t, "input", taEvents(res), []string{`ch q {} "x"`, `sub q {} "x"`, `ch q {} "xy"`})

	// ctrl+j is a line break in the textarea, and enter elsewhere.
	res = taPlay(t, src, `{"draft": ""}`, 30, 6, "text:a", "ctrl+j", "text:b")
	sameStrings(t, "ctrl+j", taEvents(res), []string{`typed composer {} "a"`, `typed composer {} "a\n"`, `typed composer {} "a\nb"`})
	res = taPlay(t, in, `{"q": "z"}`, 30, 3, "ctrl+j")
	sameStrings(t, "ctrl+j in an input", taEvents(res), []string{`sub q {} "z"`})
	// ctrl+shift+j under Kitty drops its shift and ctrl: the LF byte.
	res = taPlay(t, src, `{"draft": ""}`, 30, 6, "text:a", "text:\x1b[106;6u", "text:b")
	sameStrings(t, "ctrl+shift+j", taEvents(res), []string{`typed composer {} "a"`, `typed composer {} "a\n"`, `typed composer {} "a\nb"`})
}

// 292. Mouse and disabled (decision 239).
func TestD3Textarea292MouseAndDisabled(t *testing.T) {
	two := `<tui mouse="true"><screen id="main" focus="#other"><input id="other"/><textarea id="t" bind="d" on:change="ch"/></screen></tui>`
	app := btnApp(t, two, `{"d": "hello"}`)
	// The caret is at the end of the value (col 5); a click focuses the
	// textarea and leaves it there.
	res, err := app.Play(tuimark.PlayOptions{Cols: 30, Rows: 4, NoHandlers: true}, "click:2,1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Dump.Focus == nil || *res.Dump.Focus != "t" {
		t.Errorf("focus %v", res.Dump.Focus)
	}
	if got := taCaret(t, res.Dump, "t"); got != "0,5@5,1" {
		t.Errorf("caret after the click: %s", got)
	}
	// Caret at the start first: home, then a click on cell (5, 1) leaves it.
	app = btnApp(t, strings.Replace(two, `focus="#other"`, `focus="#t"`, 1), `{"d": "hello"}`)
	res, err = app.Play(tuimark.PlayOptions{Cols: 30, Rows: 4, NoHandlers: true}, "home", "focus:other", "click:5,1")
	if err != nil {
		t.Fatal(err)
	}
	if got := taCaret(t, res.Dump, "t"); got != "0,0@0,1" {
		t.Errorf("caret after home and a click: %s", got)
	}

	// A disabled textarea is skipped by tab, consumes no key, and keeps its
	// value; set: still changes it.
	dis := `<tui><screen id="main" focus="#a"><input id="a"/><textarea id="t" bind="d" disabled="off" on:change="ch"/><input id="b"/></screen><keymap><bind keys="x" action="xx"/></keymap></tui>`
	app = btnApp(t, dis, `{"d": "keep", "off": true}`)
	res, err = app.Play(tuimark.PlayOptions{Cols: 30, Rows: 6, NoHandlers: true}, "tab")
	if err != nil {
		t.Fatal(err)
	}
	if res.Dump.Focus == nil || *res.Dump.Focus != "b" {
		t.Errorf("tab from a landed on %v, want b", res.Dump.Focus)
	}
	if n := taNode(t, res.Dump, "t"); n.Text != "keep" || n.Caret != nil {
		t.Errorf("disabled textarea: %+v", n)
	}
	res, err = app.Play(tuimark.PlayOptions{Cols: 30, Rows: 6, NoHandlers: true}, "set:d=\"new\"")
	if err != nil {
		t.Fatal(err)
	}
	if n := taNode(t, res.Dump, "t"); n.Text != "new" {
		t.Errorf("set: on a disabled textarea: %q", n.Text)
	}
	// Focus requests do not reach it either.
	res, err = app.Play(tuimark.PlayOptions{Cols: 30, Rows: 6, NoHandlers: true}, "focus:t", "text:z")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := app.Get("d"); v != "new" {
		t.Errorf("a disabled textarea took a key: %q", v)
	}
}
