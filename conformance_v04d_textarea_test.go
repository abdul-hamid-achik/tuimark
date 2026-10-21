package tuimark_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4d §21 tests of phase D3, the textarea (§6.19), through the Go
// API (§18.1): tests 284-293 and 314, with the textarea parts of 287, 291,
// and 312. The fixtures are testdata/v04d/t<test>; t.tui of test 284 is the
// document most of them use. The CLI side (dump, validate, ir, catalog,
// the language server) is cmd/tuimark's conformance_v04d_textarea_test.go.

const taFixtures = "testdata/v04d/"

func taSource(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(taFixtures + rel)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// taPlay plays steps on a fresh app of src with data at cols×rows.
func taPlay(t *testing.T, src, data string, cols, rows int, steps ...string) *tuimark.PlayResult {
	t.Helper()
	res, err := btnApp(t, src, data).Play(tuimark.PlayOptions{Cols: cols, Rows: rows, NoHandlers: true}, steps...)
	if err != nil {
		t.Fatalf("%v: %v", steps, err)
	}
	return res
}

// taEvents is the events of a play, as "ACTION SOURCE VALUE" lines.
func taEvents(res *tuimark.PlayResult) []string {
	out := []string{}
	for _, e := range res.Events {
		v, _ := json.Marshal(e.Value)
		k, _ := json.Marshal(e.Keys)
		out = append(out, fmt.Sprintf("%s %s %s %s", e.Action, e.Source, k, v))
	}
	return out
}

func taNode(t *testing.T, d *tuimark.Dump, id string) *tuimark.DumpNode {
	t.Helper()
	n := dumpNode(d, id)
	if n == nil {
		t.Fatalf("no node #%s", id)
	}
	return n
}

// taCaret is the caret of node #id as "line,col" or "line,col@x,y".
func taCaret(t *testing.T, d *tuimark.Dump, id string) string {
	t.Helper()
	c := taNode(t, d, id).Caret
	if c == nil {
		return "none"
	}
	if c.X == nil || c.Y == nil {
		return fmt.Sprintf("%d,%d", c.Line, c.Col)
	}
	return fmt.Sprintf("%d,%d@%d,%d", c.Line, c.Col, *c.X, *c.Y)
}

func taGrid(d *tuimark.Dump) string { return strings.Join(d.Grid, "\n") }

func sameStrings(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s:\n got %q\nwant %q", what, got, want)
	}
}

// 284. Typing, line breaks, submitting: the first dump, the events, and
// the layout that grows (decisions 226, 229, 237).
func TestD3Textarea284TypingBreaksSubmit(t *testing.T) {
	src := taSource(t, "t284/t.tui")
	res := taPlay(t, src, `{"draft": ""}`, 30, 6)
	d := res.Dump
	if n := taNode(t, d, "composer"); n.X != 0 || n.Y != 1 || n.W != 30 || n.H != 1 || !n.Focused || n.Text != "" {
		t.Errorf("composer: %+v", n)
	}
	if n := taNode(t, d, "composer"); n.Scroll == nil || *n.Scroll.Y != 0 || *n.Scroll.H != 1 {
		t.Errorf("scroll: %+v", n.Scroll)
	}
	if got := taCaret(t, d, "composer"); got != "0,0@0,1" {
		t.Errorf("caret %s", got)
	}
	if n := taNode(t, d, "status"); n.Y != 2 {
		t.Errorf("status at %d", n.Y)
	}
	if got := d.Grid[1]; got != "Message"+strings.Repeat(" ", 23) {
		t.Errorf("row 1 %q", got)
	}
	// The placeholder is painted dim.
	styled, err := btnApp(t, src, `{"draft": ""}`).Play(tuimark.PlayOptions{Cols: 30, Rows: 6, NoHandlers: true, Styles: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := spansJSON(t, styled.Dump, 1); !strings.Contains(got, `"a":["dim"]`) {
		t.Errorf("placeholder spans %s", got)
	}

	steps := []string{"text:hello", "shift+enter", "text:world", "enter"}
	want := []string{
		`typed composer {} "hello"`,
		`typed composer {} "hello\n"`,
		`typed composer {} "hello\nworld"`,
		`send composer {} "hello\nworld"`,
	}
	res = taPlay(t, src, `{"draft": ""}`, 30, 6, steps...)
	sameStrings(t, "events", taEvents(res), want)
	d = res.Dump
	if n := taNode(t, d, "composer"); n.Y != 1 || n.H != 2 || n.W != 30 || n.Text != "hello\nworld" {
		t.Errorf("composer: %+v", n)
	}
	if got := taCaret(t, d, "composer"); got != "1,5@5,2" {
		t.Errorf("caret %s", got)
	}
	if n := taNode(t, d, "status"); n.Y != 3 {
		t.Errorf("status moved to %d, want 3", n.Y)
	}
	// alt+enter, and an LF in a text step, are line breaks too.
	for _, nl := range []string{"alt+enter", "text:\n"} {
		res = taPlay(t, src, `{"draft": ""}`, 30, 6, "text:hello", nl, "text:world", "enter")
		sameStrings(t, nl, taEvents(res), want)
	}
}

// 285. Diagnostics (decisions 225 and 240).
func TestD3Textarea285Diagnostics(t *testing.T) {
	doc := func(body, tail string) string {
		return `<tui><screen id="main">` + body + `</screen>` + tail + `</tui>`
	}
	diags := func(body, tail string) []tuimark.Diagnostic {
		t.Helper()
		app, err := tuimark.Parse(strings.NewReader(doc(body, tail)))
		if err != nil {
			t.Fatal(err)
		}
		return app.Validate()
	}
	check := func(name, body, tail, code, msg string) {
		t.Helper()
		for _, d := range diags(body, tail) {
			if d.Code == code && d.Msg == msg {
				return
			}
		}
		t.Errorf("%s: want %s %q in %v", name, code, msg, diagLines(diags(body, tail)))
	}
	check("no id", `<textarea bind="d"/>`, "", "V012", "<textarea> is focusable and needs an id")
	check("secret", `<textarea id="t" secret="true"/>`, "", "V002", `attribute "secret" is not allowed on <textarea>`)
	check("text", `<textarea id="t">hi</textarea>`, "", "V013", "text is not allowed directly inside <textarea>; its text is the value at its bind path")
	check("element", `<textarea id="t"><text>a</text></textarea>`, "", "V013", "<textarea> cannot contain elements (found <text>)")
	for _, d := range diags("<textarea id=\"t\">\n  </textarea>", "") {
		if d.Code == "V013" {
			t.Errorf("whitespace is no text: %s", d.Msg)
		}
	}

	// In a list <item> and in a container each template: the messages an
	// input gets there, with <textarea>, and no V012.
	inputMsgs := func(tag string) []string {
		var out []string
		for _, body := range []string{
			`<list id="l" each="xs as x" key="x"><item><` + tag + `/></item></list>`,
			`<col each="xs as x" key="x"><` + tag + `/></col>`,
		} {
			for _, d := range diags(body, "") {
				if d.Code == "V001" || d.Code == "V012" {
					out = append(out, d.Code+" "+strings.ReplaceAll(strings.ReplaceAll(d.Msg, "<"+tag+">", "<TAG>"), "<"+tag+"/>", "<TAG/>"))
				}
			}
		}
		return out
	}
	got, want := inputMsgs("textarea"), inputMsgs("input")
	if len(want) != 2 || strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("a textarea in a list item and an each template:\n got %q\nwant %q", got, want)
	}

	check("move-next", `<textarea id="t"/>`, `<keymap><bind keys="n" action="move-next" to="#t"/></keymap>`, "B007",
		`action="move-next": its target #t (from to="#t") is a <textarea>, whose caret and offset only its own keys and the wheel move (want a list, a table, a tabs, or a viewport)`)
	check("check-toggle", `<textarea id="t"/>`, `<keymap><bind keys="n" action="check-toggle" to="#t"/></keymap>`, "B007",
		`action="check-toggle": its target #t (from to="#t") is a <textarea>, not a list or table with checked= (the array of checked row keys)`)
	// A when that is exactly #id:focus fixes the target too.
	check("when", `<textarea id="t"/>`, `<keymap><bind keys="n" action="move-page-up" when="#t:focus"/></keymap>`, "B007",
		`action="move-page-up": its target #t (from when="#t:focus") is a <textarea>, whose caret and offset only its own keys and the wheel move (want a list, a table, a tabs, or a viewport)`)
}
