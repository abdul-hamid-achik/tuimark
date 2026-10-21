package tuimark_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4d §21 tests of phase D4 through the Go API (§18.3): 295 and 297
// through Play with Inline, 298 through RunWith and Play, and Commit
// outside an inline session. The CLI is cmd/tuimark's
// conformance_v04d_inline_test.go, and the bytes of Run are internal/host's
// terminal-emulator tests. The composer is an <input> in place of the
// textarea, which D3 brings.

const d4Doc = `<tui>
  <style>.good { color: green; }</style>
  <screen id="main" focus="#composer">
    <rule/>
    <textarea id="composer" bind="draft" placeholder="Message" on:submit="send"/>
    <text id="status">{status}</text>
  </screen>
  <template id="card" props="who">
    <text class="who">{$who}: {m.text}</text>
  </template>
</tui>`

func d4App(t *testing.T, src string) *tuimark.App {
	t.Helper()
	app := parseDoc(t, src)
	bindJSON(t, app, "", `{"draft": "", "status": "ready", "last": {"text": "done"}}`)
	return app
}

// Tests 295 and 297: Play with Inline records the commits the steps and the
// handlers make, in order, and the frames count the rows committed.
func TestD4PlayInline(t *testing.T) {
	app := d4App(t, d4Doc)
	app.On("send", func(ev tuimark.Event) error {
		return app.Commit(tuimark.Commit{Lines: []any{"sent: " + ev.Value.(string)}})
	})
	res, err := app.Play(tuimark.PlayOptions{Cols: 30, Rows: 24, Inline: true, Styles: true, Frames: true},
		`commit:{"lines": ["You: hello", [{"text": "ok", "class": "good"}]]}`, "text:abc", "enter")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"You: hello                    ", "ok                            ", "sent: abc                     "}
	if res.Scrollback == nil || !reflect.DeepEqual(res.Scrollback.Grid, want) {
		t.Fatalf("scrollback %+v", res.Scrollback)
	}
	if len(res.Scrollback.Styles) != 3 || res.Scrollback.Styles[1][0].FG != "green" {
		t.Errorf("styles %+v", res.Scrollback.Styles)
	}
	var counts []int
	for _, f := range res.Frames {
		counts = append(counts, *f.ScrollbackRows)
	}
	if !reflect.DeepEqual(counts, []int{0, 2, 2, 3}) {
		t.Errorf("scrollback_rows %v", counts)
	}
	if res.Dump.Rows != 3 || res.Dump.TerminalRows == nil || *res.Dump.TerminalRows != 24 {
		t.Errorf("dump rows %d terminal_rows %v", res.Dump.Rows, res.Dump.TerminalRows)
	}
	// It marshals as play --inline's scrollback member.
	b, _ := json.Marshal(res.Scrollback)
	if !strings.HasPrefix(string(b), `{"grid":["You: hello`) {
		t.Errorf("JSON %s", b)
	}
	// Without Inline, Scrollback is nil and a commit is the error.
	res, err = app.Play(tuimark.PlayOptions{NoHandlers: true})
	if err != nil || res.Scrollback != nil {
		t.Errorf("Play without Inline: %v %v", err, res.Scrollback)
	}
	if err := app.Commit(tuimark.Commit{Lines: []any{"x"}}); err == nil || err.Error() != "commit: not in inline mode" {
		t.Errorf("Commit outside a session: %v", err)
	}
	// The session left the app as it found it: a dump is the terminal's height.
	if d, err := app.Dump(30, 24); err != nil || d.Rows != 24 || d.TerminalRows != nil {
		t.Errorf("Dump after an inline Play: %v %+v", err, d)
	}
}

// Test 297: the errors of Commit, in Play.
func TestD4CommitErrors(t *testing.T) {
	for _, c := range []struct {
		c    tuimark.Commit
		want string
	}{
		{tuimark.Commit{}, "commit: give either lines or template"},
		{tuimark.Commit{Lines: []any{"a"}, Template: "card"}, "commit: give either lines or template"},
		{tuimark.Commit{Lines: []any{"a"}, Props: map[string]string{"who": "x"}}, "commit: with and props need a template"},
		{tuimark.Commit{Lines: []any{"a", 5}}, "commit: line 1 is a number, not a string or an array of runs"},
		{tuimark.Commit{Lines: []any{[]any{map[string]any{"class": "b"}}}}, `commit: line 0: run 0 has no string "text"`},
		{tuimark.Commit{Lines: []any{[]string{"a", "b"}, []any{map[string]any{"text": "x", "class": []string{"y"}}}}}, `commit: line 1: run 0 has a "class" that is not a list of class names`},
		{tuimark.Commit{Template: "nope"}, `commit: no <template id="nope">`},
		{tuimark.Commit{Template: "card", With: "nothing as m"}, `commit: with path "nothing" (alias m) is missing`},
		{tuimark.Commit{Template: "card", With: "last as m", Props: map[string]string{"whom": "C"}}, `commit: prop "whom" is not declared by <template id="card">`},
	} {
		var got error
		a2 := d4App(t, d4Doc)
		a2.On("send", func(tuimark.Event) error { got = a2.Commit(c.c); return nil })
		if _, err := a2.Play(tuimark.PlayOptions{Cols: 30, Rows: 24, Inline: true}, "text:a", "enter"); err != nil {
			t.Fatal(err)
		}
		if got == nil || got.Error() != c.want {
			t.Errorf("%+v: %v, want %s", c.c, got, c.want)
		}
	}
}

// Test 298: a document that writes version cannot run inline.
func TestD4Pinned(t *testing.T) {
	app := parseDoc(t, `<tui version="3"><screen id="s"><text>x</text></screen></tui>`)
	res, err := app.Play(tuimark.PlayOptions{Inline: true})
	if res != nil || err == nil || err.Error() != "tuimark: inline mode needs a <tui> without version" {
		t.Errorf("Play: %v, %v", res, err)
	}
	if err := app.RunWith(context.Background(), &strings.Builder{}, tuimark.RunOptions{Inline: true}); err == nil || err.Error() != "tuimark: inline mode needs a <tui> without version" {
		t.Errorf("RunWith: %v", err)
	}
}
