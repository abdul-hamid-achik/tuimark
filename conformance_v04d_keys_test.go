package tuimark_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4d §21 tests of phase D2, through the Go API (§18.1): test 279's
// Keyboard option and tests 277, 278, and 280 through Play. The CLI side
// is cmd/tuimark's conformance_v04d_keys_test.go.

const d2Doc = `<tui>
  <screen id="main">
    <input id="q" bind="q" on:change="ch" on:submit="sub"/>
    <hints id="h"/>
  </screen>
  <keymap>
    <bind keys="esc" action="e"/>
    <bind keys="shift+enter,alt+enter" action="nl" label="newline"/>
    <bind keys="ctrl+enter" action="go" label="send"/>
    <bind keys="f2" action="two"/>
  </keymap>
</tui>`

func d2App(t *testing.T, src string) *tuimark.App {
	t.Helper()
	app := parseDoc(t, src)
	bindJSON(t, app, "", `{"q": "ab"}`)
	return app
}

func d2Events(res *tuimark.PlayResult) []string {
	out := []string{}
	for _, e := range res.Events {
		v, _ := json.Marshal(e.Value)
		out = append(out, fmt.Sprintf("%d %s %s", e.Step, e.Action, v))
	}
	return out
}

// Test 279 through Play: Keyboard is "kitty" (also ""), or "legacy"; any
// other value is an error before any step; a key a legacy terminal cannot
// send is an error that applies no step.
func TestD2PlayKeyboard279(t *testing.T) {
	app := d2App(t, d2Doc)
	for _, kb := range []string{"", "kitty"} {
		if _, err := app.Play(tuimark.PlayOptions{Keyboard: kb, NoHandlers: true}, "shift+enter", "ctrl+enter"); err != nil {
			t.Errorf("Keyboard %q: %v", kb, err)
		}
	}
	_, err := app.Play(tuimark.PlayOptions{Keyboard: "other"}, "a")
	if err == nil || !strings.Contains(err.Error(), `Keyboard must be "kitty" or "legacy"`) {
		t.Errorf("Keyboard other: %v", err)
	}
	// An error applies nothing, not even step 0: the app still has the
	// value it had.
	res, err := app.Play(tuimark.PlayOptions{Keyboard: "legacy", NoHandlers: true}, "x", "shift+enter")
	if res != nil || err == nil || err.Error() != `step 2 (shift+enter): key "shift+enter" cannot be sent by a legacy terminal (--keyboard legacy)` {
		t.Errorf("legacy shift+enter: %v, %v", res, err)
	}
	if v, _ := app.Get("q"); v != "ab" {
		t.Errorf("a refused Play changed q to %v", v)
	}
	res, err = app.Play(tuimark.PlayOptions{Keyboard: "legacy", NoHandlers: true}, "alt+enter", "home", "delete", "f2", "esc")
	if err != nil {
		t.Fatal(err)
	}
	if got := d2Events(res); fmt.Sprint(got) != `[1 nl "ab" 3 ch "b" 4 two "b" 5 e "b"]` {
		t.Errorf("legacy events %v", got)
	}
}

// Test 279, pinned: a key step naming a 0.4d token is an error with the
// §5.1 message, and Keyboard is ignored.
func TestD2PlayPinned279(t *testing.T) {
	app := parseDoc(t, `<tui version="3"><screen id="s"><input id="q" on:change="ch"/></screen></tui>`)
	_, err := app.Play(tuimark.PlayOptions{}, "a", "delete")
	if err == nil || err.Error() != `step 2 (delete): key "delete" (requires a <tui> without version)` {
		t.Errorf("pinned delete: %v", err)
	}
	if _, err := app.Play(tuimark.PlayOptions{Keyboard: "legacy"}, "a", "esc", "ctrl+i"); err != nil {
		t.Errorf("pinned with Keyboard legacy: %v", err)
	}
}

// Test 280 through Play: <hints> shows the keys the session can send.
func TestD2PlayHints280(t *testing.T) {
	app := d2App(t, d2Doc)
	grid := func(kb string) string {
		res, err := app.Play(tuimark.PlayOptions{Keyboard: kb, NoHandlers: true, Cols: 60, Rows: 4})
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(res.Dump.Grid, "\n")
	}
	if g := grid(""); !strings.Contains(g, "shift+enter newline") || !strings.Contains(g, "ctrl+enter send") {
		t.Errorf("kitty hints:\n%s", g)
	}
	// Dump renders as a Kitty session, after a legacy Play too.
	if g := grid("legacy"); !strings.Contains(g, "alt+enter newline") || strings.Contains(g, "send") {
		t.Errorf("legacy hints:\n%s", g)
	}
	d, err := app.Dump(60, 4)
	if err != nil {
		t.Fatal(err)
	}
	if g := strings.Join(d.Grid, "\n"); !strings.Contains(g, "shift+enter newline") {
		t.Errorf("Dump after a legacy Play:\n%s", g)
	}
}
