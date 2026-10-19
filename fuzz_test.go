package tuimark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"unicode/utf8"
)

// fuzzStore is the small fixed JSON store every fuzz input is rendered with.
const fuzzStore = `{"a":"x","b":true,"n":3,"view":"two","cur":2,"sel":[],
"rows":[{"id":1,"name":"one","ok":true},{"id":2,"name":"two","ok":false},{"id":3,"name":"three"}],
"hist":[1,5,null,3,9,2],"user":{"name":"u","tags":["p","q"]},"s":"héllo wörld 日本語 😀"}`

// fuzzSizes are the dump sizes every input is rendered at.
var fuzzSizes = [][2]int{{1, 1}, {40, 10}, {80, 24}, {200, 60}}

// fuzzSteps is a short interaction replayed after the dumps.
var fuzzSteps = []string{"tab", "down", "x", "enter", "tab", "space", "esc", "shift+tab", "set:a=\"hi\"", "text:abc", "right", "pgdn", "end"}

// fuzzSeeds returns the repository's real documents plus edge cases.
func fuzzSeeds(tb testing.TB) []string {
	var out []string
	for _, pat := range []string{
		"examples/*/*.tui", "examples/*/*/*.tui", "examples/*/*/*/*.tui",
		"kit/*.tui", "kit/*/*.tui", "testdata/*/*.tui", "testdata/*/*/*.tui", "testdata/*/*/*/*.tui",
	} {
		paths, _ := filepath.Glob(pat)
		for _, p := range paths {
			if b, err := os.ReadFile(p); err == nil && len(b) <= 8<<10 {
				out = append(out, string(b))
			}
		}
	}
	out = append(out,
		"",
		`<tui><screen/></tui>`,
		`<tui><screen><text>{s}</text><text wrap="wrap" style="width: 3">{s}</text></screen></tui>`,
		`<tui><screen><col><text>`+strings.Repeat("日本語😀é ", 40)+`</text></col></screen></tui>`,
		`<tui><screen>`+strings.Repeat("<box>", 200)+`<text>x</text>`+strings.Repeat("</box>", 200)+`</screen></tui>`,
		`<tui><screen><list id="l" each="rows as r" key="r.id" bind="cur" checked="sel" mark="✓"><item><text>{r.name}</text></item></list><input id="q" bind="a"/><button id="b" label="x"/></screen></tui>`,
		`<tui><screen><table id="t" each="rows as r" key="r.id" bind="cur"><column title="Name" width="1fr" priority="1">{r.name}</column><column title="Id" width="4">{r.id}</column></table><sparkline bind="hist" scale="s"/><sparkline bind="hist" scale="s" max="20"/></screen></tui>`,
		`<tui><keymap when="#q"><bind keys="ctrl+s" action="save" label="save"/></keymap><hints/><screen><tabs id="t" bind="view"><tab id="one" label="{a}"><input id="q"/></tab><tab id="two" label="two"><scroll id="log" stick="bottom"><text>a</text><text>b</text></scroll></tab></tabs><modal id="m" open="b" focus="#x"><input id="x"/></modal></screen></tui>`,
		`<tui><screen><box style="layout: grid; grid-columns: 3; gap: 1" each="rows as r"><text>{r.name}</text></box><progress value="50" style="bar: eighths"/></screen></tui>`,
		`<tui><screen><box style="width: 99999999999; height: 2fr; padding: 4 4 4 4; border: thick"><spacer style="flex: 1000000"/></box></screen></tui>`,
	)
	return out
}

// checkApp renders app at every fuzzSizes entry and replays fuzzSteps,
// returning the first broken invariant. It must not panic.
func checkApp(app *App) error {
	var store any
	if err := json.Unmarshal([]byte(fuzzStore), &store); err != nil {
		return err
	}
	if err := app.Bind("", store); err != nil {
		return err
	}
	for _, sz := range fuzzSizes {
		cols, rows := sz[0], sz[1]
		d, err := app.Dump(cols, rows)
		if err != nil {
			return fmt.Errorf("Dump(%d, %d): %v", cols, rows, err)
		}
		if err := checkGrid("Dump", d.Grid, d.Wide, cols, rows); err != nil {
			return err
		}
		if d.OK == hasError(d.Errors) {
			return fmt.Errorf("Dump(%d, %d): ok=%v with errors %v", cols, rows, d.OK, d.Errors)
		}
	}
	_ = app.Validate()
	for _, sz := range fuzzSizes {
		cols, rows := sz[0], sz[1]
		res, _ := app.Play(PlayOptions{Cols: cols, Rows: rows, Cells: true}, fuzzSteps...)
		if res == nil {
			// A step that cannot apply is a reported error, never a panic.
			continue
		}
		d := res.Dump
		if err := checkGrid("Play", d.Grid, d.Wide, d.Cols, d.Rows); err != nil {
			return err
		}
		if len(d.Cells) != d.Cols*d.Rows {
			return fmt.Errorf("Play(%d, %d): %d cells, want %d", cols, rows, len(d.Cells), d.Cols*d.Rows)
		}
	}
	return nil
}

func checkGrid(what string, grid []string, wide bool, cols, rows int) error {
	if len(grid) != rows {
		return fmt.Errorf("%s %dx%d: %d grid rows", what, cols, rows, len(grid))
	}
	for y, row := range grid {
		if !utf8.ValidString(row) {
			return fmt.Errorf("%s %dx%d: row %d is not valid UTF-8: %q", what, cols, rows, y, row)
		}
		// A wide cluster breaks the one-rune-per-column rule (the dump says
		// so with "wide"); otherwise every row has exactly cols runes.
		if !wide && utf8.RuneCountInString(row) != cols {
			return fmt.Errorf("%s %dx%d: row %d has %d runes: %q", what, cols, rows, y, utf8.RuneCountInString(row), row)
		}
		if strings.IndexFunc(row, func(r rune) bool { return r < 0x20 || (r >= 0x7f && r <= 0x9f) }) >= 0 {
			return fmt.Errorf("%s %dx%d: control character in row %d: %q", what, cols, rows, y, row)
		}
	}
	return nil
}

func hasError(ds []Diagnostic) bool {
	for _, d := range ds {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

// checkWithDeadline runs checkApp and fails the test when it returns an
// error, panics, or takes longer than d: a hang is a bug the fuzz engine
// would otherwise only see as a stuck worker.
func checkWithDeadline(t *testing.T, app *App, d time.Duration) {
	t.Helper()
	type result struct {
		err   error
		panic any
	}
	done := make(chan result, 1)
	go func() {
		var r result
		defer func() {
			if p := recover(); p != nil {
				r.panic = fmt.Sprintf("%v\n%s", p, debug.Stack())
			}
			done <- r
		}()
		r.err = checkApp(app)
	}()
	select {
	case r := <-done:
		if r.panic != nil {
			t.Fatalf("panic: %v", r.panic)
		}
		if r.err != nil {
			t.Fatal(r.err)
		}
	case <-time.After(d):
		t.Fatalf("no result after %v: the input hangs", d)
	}
}

// FuzzDocument parses any document, then dumps it at four sizes, validates
// it, and replays a few keys. Invariants: no panic, no hang, every grid has
// exactly rows lines of cols cells (see "wide"), ok matches the errors.
func FuzzDocument(f *testing.F) {
	for _, s := range fuzzSeeds(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 8<<10 {
			t.Skip()
		}
		app, err := Parse(strings.NewReader(src))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		checkWithDeadline(t, app, 20*time.Second)
	})
}

// fuzzSheetDoc is a rich fixed document whose only variable is the
// stylesheet it loads.
const fuzzSheetDoc = `<tui><style src="f.tcss"/><keymap><bind keys="x" action="a" label="x"/></keymap><screen id="s">
<col id="c" class="a"><row><text id="t" class="a b">{s}</text><button id="b" label="go"/><input id="q" bind="a"/></row>
<list id="l" each="rows as r" key="r.id" bind="cur"><item class="b"><text>{r.name}</text></item></list>
<table id="tb" each="rows as r" key="r.id"><column title="N" class="a">{r.name}</column><column title="I">{r.id}</column></table>
<tabs id="tabs"><tab id="t1" label="one"><progress value="40"/></tab><tab id="t2" label="two"><scroll id="sc"><text>a</text></scroll></tab></tabs>
<sparkline bind="hist"/><hints/><rule/><spacer/></col>
<modal id="m" open="b"><box title="m"><button id="mb" label="ok"/></box></modal></screen></tui>`

// FuzzStylesheet loads a fixed rich document with any stylesheet and checks
// the same invariants as FuzzDocument, so the cascade, layout and paint see
// every property value the stylesheet grammar lets through.
func FuzzStylesheet(f *testing.F) {
	for _, pat := range []string{"examples/*/*.tcss", "examples/*/*/*.tcss", "kit/*.tcss", "testdata/*/*.tcss", "testdata/*/*/*.tcss"} {
		paths, _ := filepath.Glob(pat)
		for _, p := range paths {
			if b, err := os.ReadFile(p); err == nil && len(b) <= 8<<10 {
				f.Add(string(b))
			}
		}
	}
	for _, s := range []string{
		"", "text { color: red; background: #123; bold: true; reverse: true }",
		"* { width: 0; height: 0 } col { padding: 4; margin: 4; gap: 4; border: double }",
		"col { layout: grid; grid-columns: 12; grid-min-width: 1; gap: 4 } text { width: 50%; flex: 3 }",
		"text { width: 99999999999999999999; max-width: 1; min-width: 99999999 } list { height: 1fr; overflow: scroll; scrollbar: auto }",
		"table { dock: left; width: 3 } column { width: 100%; max-width: 1fr } tabs { dock: bottom } hints { layout: column; gap: 4 }",
		"text { wrap: truncate-middle; content-align: end } box { dock: top; height: 1fr } box { dock: top }",
		"@media (max-cols: 20) { * { display: none } } @media (theme: light) { :root { bg: #fff } } :root { --x: red } text { color: $x }",
		"modal { width: 1; height: 1; border: none } progress { bar: eighths; width: 1fr } sparkline { width: 0; height: 9 }",
		"scroll, list, table { padding: 1 1 1 1; border: thick; overflow: hidden } item { margin: 1 }",
		"text { visibility: hidden } col { display: none } col { display: flex } row { justify: space-between; align: center }",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, css string) {
		if len(css) > 8<<10 {
			t.Skip()
		}
		fsys := fstest.MapFS{
			"app.tui": {Data: []byte(fuzzSheetDoc)},
			"f.tcss":  {Data: []byte(css)},
		}
		app, err := LoadFS(fsys, "app.tui")
		if err != nil {
			t.Fatalf("LoadFS: %v", err)
		}
		checkWithDeadline(t, app, 20*time.Second)
	})
}
