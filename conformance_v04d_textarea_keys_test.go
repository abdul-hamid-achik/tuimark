package tuimark_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4d §21 tests 286-288 of phase D3: the textarea's editing keys,
// its up and down, and its growing and scrolling, through the Go API.

// 286. Editing keys (decision 227): from "one two\nthree" with the caret at
// the end (line 1, col 5), each row of the table.
func TestD3Textarea286EditingKeys(t *testing.T) {
	src := taSource(t, "t284/t.tui")
	for _, c := range []struct {
		steps []string
		value string
		caret string // line,col
	}{
		{[]string{"backspace"}, "one two\nthre", "1,4"},
		{[]string{"home", "backspace"}, "one twothree", "0,7"},
		{[]string{"home", "delete"}, "one two\nhree", "1,0"},
		{[]string{"ctrl+u"}, "one two\n", "1,0"},
		{[]string{"up", "ctrl+k"}, "one t\nthree", "0,5"},
		{[]string{"up", "end", "ctrl+k"}, "one twothree", "0,7"},
		{[]string{"ctrl+w"}, "one two\n", "1,0"},
		{[]string{"home", "ctrl+w"}, "one twothree", "0,7"},
		{[]string{"alt+b", "alt+b"}, "one two\nthree", "0,4"},
		{[]string{"ctrl+a", "left", "alt+backspace"}, "one \nthree", "0,4"},
		{[]string{"home", "alt+f"}, "one two\nthree", "1,5"},
	} {
		app := btnApp(t, src, `{"draft": "one two\nthree"}`)
		res, err := app.Play(tuimark.PlayOptions{Cols: 30, Rows: 6, NoHandlers: true}, c.steps...)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := app.Get("draft")
		if got != c.value {
			t.Errorf("%v: value %q, want %q", c.steps, got, c.value)
		}
		if n := taNode(t, res.Dump, "composer"); n.Text != c.value {
			t.Errorf("%v: text %q, want %q", c.steps, n.Text, c.value)
		}
		cr := taNode(t, res.Dump, "composer").Caret
		if cr == nil || fmt.Sprintf("%d,%d", cr.Line, cr.Col) != c.caret {
			t.Errorf("%v: caret %+v, want %s", c.steps, cr, c.caret)
		}
	}
}

// 287. up and down fall through at the ends, move by visual lines, and
// pgup and pgdn are consumed only while the textarea scrolls (decision 228).
func TestD3Textarea287UpDownKeymap(t *testing.T) {
	src := taSource(t, "t284/t.tui")
	res := taPlay(t, src, `{"draft": "a\nb"}`, 30, 6, "up", "up", "down", "down")
	if got, want := taEvents(res), []string{`history_prev composer {} "a\nb"`, `history_next composer {} "a\nb"`}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("events %q, want %q", got, want)
	}
	// Step by step: up moves to line 0, down back to line 1.
	app := btnApp(t, src, `{"draft": "a\nb"}`)
	for i, c := range []struct{ step, caret string }{{"up", "0,1"}, {"up", "0,1"}, {"down", "1,1"}, {"down", "1,1"}} {
		res, err := app.Play(tuimark.PlayOptions{Cols: 30, Rows: 6, NoHandlers: true}, c.step)
		if err != nil {
			t.Fatal(err)
		}
		if got := taCaret(t, res.Dump, "composer"); !strings.HasPrefix(got, c.caret+"@") {
			t.Errorf("step %d (%s): caret %s, want %s", i, c.step, got, c.caret)
		}
	}
	// A value that wraps to two visual lines in one hard line: up moves
	// within the hard line first (9 columns of text per visual line at 10).
	wrap := btnApp(t, src, `{"draft": "abcdefghijkl"}`)
	res, err := wrap.Play(tuimark.PlayOptions{Cols: 10, Rows: 6, NoHandlers: true}, "up")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 0 {
		t.Errorf("up within a wrapped hard line fired %v", taEvents(res))
	}
	if got := taCaret(t, res.Dump, "composer"); got != "0,3@3,1" {
		t.Errorf("caret %s, want 0,3@3,1 (the goal column 3 on the first visual line)", got)
	}
	res, err = wrap.Play(tuimark.PlayOptions{Cols: 10, Rows: 6, NoHandlers: true}, "up")
	if err != nil || strings.Join(taEvents(res), "|") != `history_prev composer {} "abcdefghijkl"` {
		t.Errorf("second up: %v %v", err, taEvents(res))
	}
	// pgup with 3 visual lines and no scrolling is not consumed: a row
	// fires; with no row nothing does.
	three := `{"draft": "1\n2\n3"}`
	plain := taPlay(t, src, three, 30, 8, "pgup", "pgdn")
	if len(plain.Events) != 0 {
		t.Errorf("pgup without a row fired %v", taEvents(plain))
	}
	if n := taNode(t, plain.Dump, "composer"); taCaret(t, plain.Dump, "composer") != "2,1@1,3" || n.H != 3 {
		t.Errorf("pgup/pgdn moved the caret: %s", taCaret(t, plain.Dump, "composer"))
	}
	withRows := strings.Replace(src, `<bind keys="up" action="history_prev"/>`, `<bind keys="up" action="history_prev"/><bind keys="pgup" action="page_prev"/>`, 1)
	res = taPlay(t, withRows, three, 30, 8, "pgup")
	if got := taEvents(res); len(got) != 1 || got[0] != `page_prev composer {} "1\n2\n3"` {
		t.Errorf("pgup row: %q", got)
	}
}

// 288. Growing and scrolling (decisions 228 and 231-233).
func TestD3Textarea288GrowScroll(t *testing.T) {
	src := taSource(t, "t284/t.tui")
	ten := `set:draft="1\n2\n3\n4\n5\n6\n7\n8\n9\n10"`
	res := taPlay(t, src, `{"draft": ""}`, 30, 12, ten)
	n := taNode(t, res.Dump, "composer")
	if n.X != 0 || n.Y != 1 || n.W != 30 || n.H != 8 || *n.Scroll.Y != 2 || *n.Scroll.H != 10 {
		t.Errorf("composer %+v scroll %d/%d", n, *n.Scroll.Y, *n.Scroll.H)
	}
	if s := taNode(t, res.Dump, "status"); s.Y != 9 {
		t.Errorf("status at %d", s.Y)
	}
	if got := taCaret(t, res.Dump, "composer"); got != "9,2@2,8" {
		t.Errorf("caret %s", got)
	}
	// As a first frame, the least offset that shows the caret.
	first := taPlay(t, src, `{"draft": "1\n2\n3\n4\n5\n6\n7\n8\n9\n10"}`, 30, 12)
	if n := taNode(t, first.Dump, "composer"); *n.Scroll.Y != 2 || n.H != 8 {
		t.Errorf("first frame offset %d", *n.Scroll.Y)
	}
	app := btnApp(t, src, `{"draft": ""}`)
	steps := func(ss ...string) *tuimark.PlayResult {
		t.Helper()
		r, err := app.Play(tuimark.PlayOptions{Cols: 30, Rows: 12, NoHandlers: true}, ss...)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	steps(ten)
	r := steps("up", "up", "up", "up", "up", "up", "up")
	if n := taNode(t, r.Dump, "composer"); *n.Scroll.Y != 2 || taCaret(t, r.Dump, "composer") != "2,1@1,1" {
		t.Errorf("after 7 up: offset %d caret %s", *n.Scroll.Y, taCaret(t, r.Dump, "composer"))
	}
	// The goal column 2 (the end of "10") is kept by up: on lines of one
	// character the caret is at column 1.
	r = steps("up")
	if n := taNode(t, r.Dump, "composer"); *n.Scroll.Y != 1 || taCaret(t, r.Dump, "composer") != "1,1@1,1" {
		t.Errorf("after 8 up: offset %d caret %s", *n.Scroll.Y, taCaret(t, r.Dump, "composer"))
	}
	// The wheel moves the offset alone: the caret is above the box.
	mouse := strings.Replace(src, "<tui>", `<tui mouse="true">`, 1)
	mapp := btnApp(t, mouse, `{"draft": ""}`)
	all := []string{ten, "up", "up", "up", "up", "up", "up", "up", "up", "wheel-down:3,4"}
	mres, err := mapp.Play(tuimark.PlayOptions{Cols: 30, Rows: 12, NoHandlers: true}, all...)
	if err != nil {
		t.Fatal(err)
	}
	if n := taNode(t, mres.Dump, "composer"); *n.Scroll.Y != 2 {
		t.Errorf("wheel: offset %d, want 2", *n.Scroll.Y)
	}
	if got := taCaret(t, mres.Dump, "composer"); got != "1,1" {
		t.Errorf("caret above the box: %s, want 1,1 (no x or y)", got)
	}
	// The next edit brings the caret back into view.
	back, err := mapp.Play(tuimark.PlayOptions{Cols: 30, Rows: 12, NoHandlers: true}, "text:x")
	if err != nil {
		t.Fatal(err)
	}
	if n := taNode(t, back.Dump, "composer"); *n.Scroll.Y != 1 {
		t.Errorf("after typing: offset %d, want 1", *n.Scroll.Y)
	}

	// height, max-height, and a sibling list.
	sized := strings.Replace(src, `<textarea id="composer"`, `<textarea id="composer" style="height: 5"`, 1)
	r5 := taPlay(t, sized, `{"draft": "x"}`, 30, 12)
	if n := taNode(t, r5.Dump, "composer"); n.H != 5 || *n.Scroll.Y != 0 || *n.Scroll.H != 5 {
		t.Errorf("height 5: h %d scroll %d/%d", n.H, *n.Scroll.Y, *n.Scroll.H)
	}
	capped := strings.Replace(src, `<textarea id="composer"`, `<textarea id="composer" style="max-height: 3"`, 1)
	r3 := taPlay(t, capped, `{"draft": "1\n2\n3\n4\n5"}`, 30, 12)
	if n := taNode(t, r3.Dump, "composer"); n.H != 3 {
		t.Errorf("max-height 3: h %d", n.H)
	}
	list := `<tui><screen id="main" focus="#composer"><col><list id="l" each="rows as r" key="r"><item><text>{r}</text></item></list><textarea id="composer" bind="draft"/></col></screen></tui>`
	rows := make([]string, 50)
	for i := range rows {
		rows[i] = fmt.Sprintf("%d", i)
	}
	rj, _ := json.Marshal(map[string]any{"rows": rows, "draft": "1\n2\n3"})
	rl := taPlay(t, list, string(rj), 30, 12)
	if l, c := taNode(t, rl.Dump, "l"), taNode(t, rl.Dump, "composer"); c.H != 3 || l.H != 9 {
		t.Errorf("list %d rows, textarea %d rows: the list takes what the textarea leaves", l.H, c.H)
	}
}
