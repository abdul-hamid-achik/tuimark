package tuimark_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC 0.4d tests 264-269 through the Go API, in the places a text with
// runs sits in a frame: a list row (whose rows 0.4a caches), a tab, a
// modal, and a frame that is built again after the store changes. Under
// the tuimark_verify build tag every Dump below is also checked against a
// cold build, so a cache that kept a text's old runs would fail there.

const runsListDoc = `<tui>
  <style>
    .k { bold: true; }
    .w { color: red; }
    list > item > text > span.w { underline: true; }
  </style>
  <screen id="main">
    <list id="l" each="rows as m" key="m.id" height="4">
      <item><text id="row" runs="m.r"/></item>
    </list>
    <text id="plain">{n}</text>
  </screen>
</tui>
`

func runsData(t *testing.T, app *tuimark.App, json string) {
	t.Helper()
	bindJSON(t, app, "", json)
}

func dumpRows(t *testing.T, app *tuimark.App, cols, rows int) *tuimark.Dump {
	t.Helper()
	d, err := app.Dump(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// A list's rows keep their own runs, and a changed run reaches the next
// frame (the row cache of 0.4a is not used for a template with runs).
func TestRunsInListRowsFollowTheData(t *testing.T) {
	app := parseDoc(t, runsListDoc)
	runsData(t, app, `{"n": 1, "rows": [
		{"id": "a", "r": ["one ", {"text": "two", "class": "k"}]},
		{"id": "b", "r": [{"text": "three", "class": "w"}]}]}`)
	d := dumpRows(t, app, 12, 6)
	if want := []string{"one two     ", "three       "}; !reflect.DeepEqual(d.Grid[:2], want) {
		t.Fatalf("grid %q", d.Grid)
	}
	for _, d := range d.Errors {
		t.Errorf("diagnostic: %s", d)
	}

	// Another store member changes: the frame is built again from the same
	// rows and gives the same runs.
	if err := app.Set("n", 2); err != nil {
		t.Fatal(err)
	}
	if got := dumpRows(t, app, 12, 6).Grid[:2]; !reflect.DeepEqual(got, []string{"one two     ", "three       "}) {
		t.Errorf("after n: %q", got)
	}
	// A run's text and class change.
	if err := app.Set("rows", []any{
		map[string]any{"id": "a", "r": []any{"ONE ", map[string]any{"text": "TWO", "class": "w"}}},
		map[string]any{"id": "b", "r": "plain three"},
	}); err != nil {
		t.Fatal(err)
	}
	d = dumpRows(t, app, 12, 6)
	if got := d.Grid[:2]; !reflect.DeepEqual(got, []string{"ONE TWO     ", "plain three "}) {
		t.Errorf("after rows: %q", got)
	}
}

// The runs of a text sit in the same frame as ordinary texts, a modal, and
// an inactive tab that is activated later.
func TestRunsInModalAndTab(t *testing.T) {
	src := `<tui>
  <style>.k { bold: true; } modal { width: 14; height: 5; } modal text > span.k { color: red; }</style>
  <screen id="main">
    <tabs id="nav" bind="view">
      <tab id="one" label="one"><text id="a" runs="a"/></tab>
      <tab id="two" label="two"><text id="b" runs="b"/></tab>
    </tabs>
    <modal id="m" open="show"><text id="c" runs="c"/></modal>
  </screen>
</tui>
`
	app := parseDoc(t, src)
	runsData(t, app, `{"view": "one", "show": false,
		"a": ["tab ", {"text": "one", "class": "k"}],
		"b": [{"text": "tab two", "class": "k"}],
		"c": [{"text": "in the modal", "class": "k"}]}`)
	d := dumpRows(t, app, 20, 8)
	if got := strings.TrimRight(d.Grid[1], " "); got != "tab one" {
		t.Errorf("tab one: %q\n%s", got, strings.Join(d.Grid, "\n"))
	}
	if err := app.Set("view", "two"); err != nil {
		t.Fatal(err)
	}
	d = dumpRows(t, app, 20, 8)
	if got := strings.TrimRight(d.Grid[1], " "); got != "tab two" {
		t.Errorf("tab two: %q", got)
	}
	if err := app.Set("show", true); err != nil {
		t.Fatal(err)
	}
	d = dumpRows(t, app, 20, 8)
	if !strings.Contains(strings.Join(d.Grid, "\n"), "in the modal") {
		t.Errorf("modal:\n%s", strings.Join(d.Grid, "\n"))
	}
}

// Play drives the app with runs through the same engine as the CLI.
func TestRunsThroughPlay(t *testing.T) {
	app := parseDoc(t, `<tui><screen id="main"><input id="i" bind="q"/><text id="t" runs="r" style="wrap: wrap"/></screen></tui>`)
	runsData(t, app, `{"q": "", "r": ["hello ", {"text": "wide world", "class": "x"}]}`)
	res, err := app.Play(tuimark.PlayOptions{Cols: 8, Rows: 4}, "text:ab", "set:r=[\"changed\"]")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimRight(res.Dump.Grid[1], " "); got != "changed" {
		t.Errorf("after set: %q\n%s", got, strings.Join(res.Dump.Grid, "\n"))
	}
}
