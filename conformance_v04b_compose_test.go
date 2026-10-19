package tuimark_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4b §24 conformance tests of composition, phase F1: <include> and
// <fragment> (tests 134-137). The parts the Go API observes are here; the
// CLI parts (validate's last line, dump's exit code, ir, the styles of a
// dump) are cmd/tuimark's conformance_v04b_compose_test.go. The fixtures
// are testdata/v04b/t134, t134ir, t136, and t137; test 135 writes its
// files from the strings below, since one document is read with a
// different src in each case.

// diagText is a diagnostic as "CODE FILE:LINE:COL PATH id=ID MSG".
func diagText(d tuimark.Diagnostic) string {
	return fmt.Sprintf("%s %s:%d:%d %s id=%s %s", d.Code, d.File, d.Line, d.Col, d.Path, d.ID, d.Msg)
}

func diagTexts(ds []tuimark.Diagnostic) []string {
	out := []string{}
	for _, d := range ds {
		out = append(out, diagText(d))
	}
	return out
}

func mustLoad(t *testing.T, path string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

// 134. The elements an include inserts take its place in the frame and the
// dump (no node has the tag include); diagnostics about them are in the
// file they are written in, named by their path relative to the document's
// directory, the same through Load and LoadFS; an include that LoadFS
// refuses is V006 and inserts nothing. (The ir bullet is cmd/tuimark's.)
func TestIncludePlacementAndFileNames(t *testing.T) {
	const (
		v003 = `V003 ../shared/foot.tui:2:16 /screen/text id=f `
		v002 = `V002 parts/body.tui:3:16 /screen/text id=b attribute "bogus" is not allowed on <text>`
	)
	check := func(t *testing.T, app *tuimark.App) {
		t.Helper()
		d, err := app.Dump(20, 6)
		if err != nil {
			t.Fatal(err)
		}
		// The screen, then the five texts at y 0 to 4.
		if len(d.Nodes) != 6 {
			t.Fatalf("%d nodes, want the screen and five texts", len(d.Nodes))
		}
		if n := d.Nodes[0]; n.Tag != "screen" || n.ID != "main" || n.X != 0 || n.Y != 0 || n.W != 20 || n.H != 6 {
			t.Errorf("screen: %+v", n)
		}
		for i, want := range []struct{ id, text string }{{"", "top"}, {"a", "alpha"}, {"b", "beta"}, {"", "bottom"}, {"f", "foot"}} {
			n := d.Nodes[i+1]
			if n.Tag != "text" || n.ID != want.id || n.Text != want.text || n.X != 0 || n.Y != i || n.W != 20 || n.H != 1 {
				t.Errorf("node %d: %+v, want text %q (id %q) at y=%d, w=20, h=1", i+1, n, want.text, want.id, i)
			}
		}
		for _, n := range d.Nodes {
			if n.Tag == "include" {
				t.Errorf("the dump has an include node: %+v", n)
			}
		}
		got := diagTexts(d.Errors)
		if len(got) != 2 || !strings.HasPrefix(got[0], v003) || got[1] != v002 {
			t.Errorf("errors:\n%s", strings.Join(got, "\n"))
		}
	}
	t.Run("Load", func(t *testing.T) { check(t, mustLoad(t, "testdata/v04b/t134/ui/app.tui")) })
	t.Run("LoadFS at the root", func(t *testing.T) {
		app, err := tuimark.LoadFS(os.DirFS("testdata/v04b/t134"), "ui/app.tui")
		if err != nil {
			t.Fatal(err)
		}
		check(t, app)
		viaLoad := diagTexts(mustLoad(t, "testdata/v04b/t134/ui/app.tui").Validate())
		if viaFS := diagTexts(app.Validate()); !reflect.DeepEqual(viaFS, viaLoad) {
			t.Errorf("Validate through LoadFS:\n%s\nthrough Load:\n%s", strings.Join(viaFS, "\n"), strings.Join(viaLoad, "\n"))
		}
	})
	t.Run("LoadFS rooted at ui", func(t *testing.T) {
		app, err := tuimark.LoadFS(os.DirFS("testdata/v04b/t134/ui"), "app.tui")
		if err != nil {
			t.Fatal(err)
		}
		d, err := app.Dump(20, 6)
		if err != nil {
			t.Fatal(err)
		}
		got := diagTexts(d.Errors)
		want := []string{
			`V006 app.tui:6:5 /screen/include id= include src="../shared/foot.tui" is outside the file system; root the fs.FS at a directory that holds both files, or copy the fragment inside it`,
			v002,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("errors:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
		for _, n := range d.Nodes {
			if n.ID == "f" {
				t.Errorf("the refused include inserted %+v", n)
			}
		}
	})
}

// screenIsEmpty checks that the dump holds the screen alone: an include
// that reports an error inserts nothing.
func screenIsEmpty(t *testing.T, app *tuimark.App) {
	t.Helper()
	d, err := app.Dump(20, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Nodes) != 1 {
		t.Errorf("%d nodes, want the empty screen alone", len(d.Nodes))
	}
}

// appWith is test 135's app.tui with line 3 replaced by body.
func appWith(body string) string {
	return "<tui>\n  <screen id=\"main\">\n    " + body + "\n  </screen>\n</tui>\n"
}

// 135. Include errors: the first check that fails is reported at the
// include, in the order of §6.18.2, and the include inserts nothing.
func TestIncludeErrors(t *testing.T) {
	files := map[string]string{
		"parts/a.tui": "<fragment>\n  <include src=\"b.tui\"/>\n</fragment>",
		"parts/b.tui": "<fragment>\n  <include src=\"a.tui\"/>\n</fragment>",
		"theme.tcss":  "text { bold: true; }\n",
		"bad.tui":     "<fragment><text>x</fragment>",
		"doc.tui":     "<tui><screen/></tui>",
		"v.tui":       "<fragment version=\"3\"/>",
	}
	const incl = `<include src="X"/>`
	type result struct {
		diags []string
		app   *tuimark.App
	}
	run := func(t *testing.T, body string, asFS bool) result {
		t.Helper()
		dir := t.TempDir()
		files["app.tui"] = appWith(body)
		for name, text := range files {
			p := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		var app *tuimark.App
		var err error
		if asFS {
			app, err = tuimark.LoadFS(os.DirFS(dir), "app.tui")
		} else {
			app, err = tuimark.Load(filepath.Join(dir, "app.tui"))
		}
		if err != nil {
			t.Fatal(err)
		}
		return result{diagTexts(app.Validate()), app}
	}
	for _, c := range []struct {
		name, x string
		want    []string
	}{
		{"cycle through two fragments", "parts/a.tui", []string{
			`V006 parts/b.tui:2:3 /screen/include id= include src="a.tui" is an include cycle: app.tui → parts/a.tui → parts/b.tui → parts/a.tui`}},
		{"the document itself", "app.tui", []string{
			`V006 app.tui:3:5 /screen/include id= include src="app.tui" is an include cycle: app.tui → app.tui`}},
		{"a stylesheet", "theme.tcss", []string{
			`V020 app.tui:3:5 /screen/include id= include src="theme.tcss" is a stylesheet: load it with <style src="theme.tcss">`}},
		{"not a fragment", "doc.tui", []string{
			`V020 app.tui:3:5 /screen/include id= include src="doc.tui" is not a fragment: its root is <tui>, and an included file's root is <fragment>`}},
		{"version on the root", "v.tui", []string{
			`V002 v.tui:1:11 / id= a <fragment> takes the language of the document that includes it; remove version`}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := run(t, strings.Replace(incl, "X", c.x, 1), false)
			if !reflect.DeepEqual(r.diags, c.want) {
				t.Errorf("diagnostics:\n%s\nwant\n%s", strings.Join(r.diags, "\n"), strings.Join(c.want, "\n"))
			}
			screenIsEmpty(t, r.app)
		})
	}
	t.Run("missing", func(t *testing.T) {
		r := run(t, strings.Replace(incl, "X", "missing.tui", 1), false)
		if len(r.diags) != 1 || !strings.HasPrefix(r.diags[0], `V006 app.tui:3:5 /screen/include id= include src="missing.tui" cannot be read: `) {
			t.Errorf("diagnostics: %v", r.diags)
		}
		screenIsEmpty(t, r.app)
	})
	t.Run("not well-formed", func(t *testing.T) {
		r := run(t, strings.Replace(incl, "X", "bad.tui", 1), false)
		if len(r.diags) != 1 || !strings.HasPrefix(r.diags[0], "V005 bad.tui:") {
			t.Errorf("diagnostics: %v", r.diags)
		}
		screenIsEmpty(t, r.app)
	})
	t.Run("no src", func(t *testing.T) {
		r := run(t, `<include/>`, false)
		want := []string{`V003 app.tui:3:5 /screen/include id= <include> needs src="FILE"`}
		if !reflect.DeepEqual(r.diags, want) {
			t.Errorf("diagnostics: %v, want %v", r.diags, want)
		}
		screenIsEmpty(t, r.app)
	})
	t.Run("outside the fs.FS", func(t *testing.T) {
		for _, x := range []string{"../x.tui", "/x.tui"} {
			r := run(t, strings.Replace(incl, "X", x, 1), true)
			want := []string{fmt.Sprintf(`V006 app.tui:3:5 /screen/include id= include src=%q is outside the file system; root the fs.FS at a directory that holds both files, or copy the fragment inside it`, x)}
			if !reflect.DeepEqual(r.diags, want) {
				t.Errorf("src=%q: %v, want %v", x, r.diags, want)
			}
			screenIsEmpty(t, r.app)
		}
	})
	t.Run("in an element without element children", func(t *testing.T) {
		// The rule of that element reports it; the file is not read.
		r := run(t, `<text><include src="missing.tui"/></text>`, false)
		if len(r.diags) != 1 || !strings.HasPrefix(r.diags[0], `V013 `) || !strings.HasSuffix(r.diags[0], `<text> cannot contain elements (found <include>)`) {
			t.Errorf("diagnostics: %v", r.diags)
		}
	})
	t.Run("a fragment outside the root", func(t *testing.T) {
		r := run(t, `<col><fragment/></col>`, false)
		if len(r.diags) != 1 || !strings.HasPrefix(r.diags[0], `V001 app.tui:3:10 `) || !strings.HasSuffix(r.diags[0], `<fragment> is only valid as the root of an included file`) {
			t.Errorf("diagnostics: %v", r.diags)
		}
	})
}

// 135 (last bullet, the Go API part): a file whose root is <fragment> is
// V005 when given as a document.
func TestFragmentIsNotADocument(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.tui")
	if err := os.WriteFile(p, []byte("<fragment>\n  <include src=\"b.tui\"/>\n</fragment>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := mustLoad(t, p)
	want := []string{`V005 a.tui:1:1 / id= a <fragment> is not a document: check it through the document that includes it`}
	if got := diagTexts(app.Validate()); !reflect.DeepEqual(got, want) {
		t.Errorf("Validate: %v, want %v", got, want)
	}
	if _, err := app.Dump(20, 4); err != nil {
		t.Fatal(err)
	}
}

// 136. A fragment holds the document's parts: a style, a keymap (with its
// own include of rows), and the screen.
func TestFragmentHoldsTheDocumentsParts(t *testing.T) {
	app := mustLoad(t, "testdata/v04b/t136/app.tui")
	if d := app.Validate(); len(d) != 0 {
		t.Fatalf("Validate: %v", diagTexts(d))
	}
	res, err := app.Play(tuimark.PlayOptions{Cols: 10, Rows: 2, NoHandlers: true}, "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 1 || res.Events[0].Step != 1 || res.Events[0].Action != "go" || res.Events[0].Source != "" || len(res.Events[0].Keys) != 0 || res.Events[0].Value != nil {
		t.Errorf("events: %+v", res.Events)
	}
	t.Run("a fragment's parts in a screen", func(t *testing.T) {
		got := diagTexts(mustLoad(t, "testdata/v04b/t136/misplaced.tui").Validate())
		const by = ", placed by <include> at misplaced.tui:3:5"
		want := []string{
			"V001 shell.tui:2:3 /screen/style id= <style> must be a direct child of <tui>" + by,
			"V001 shell.tui:3:3 /screen/keymap id= <keymap> must be a direct child of <tui>" + by,
			"V001 shell.tui:7:3 /screen/screen id=main <screen> must be a direct child of <tui>" + by,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("diagnostics:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	})
}

// 137. The same fragment included twice is not a cycle: a style it holds is
// applied once, and an id in it repeats (V004), reported at the include
// that placed the second occurrence.
func TestSameFragmentTwice(t *testing.T) {
	app := mustLoad(t, "testdata/v04b/t137/app.tui")
	got := diagTexts(app.Validate())
	// Test 137 expects the path /screen/include; §6.18.6 says the path of
	// the element, /screen/text. The runtime follows the test until the
	// draft decides (reported as an erratum).
	want := []string{
		`V004 app.tui:6:5 /screen/include id=r duplicate id "r" (first defined at row.tui:2:3, placed by <include> at app.tui:5:5)`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("diagnostics:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, d := range app.Validate() {
		if d.Code == "V006" {
			t.Errorf("V006: %s", diagText(d))
		}
	}
}

// Under Parse an include resolves against the working directory, and a
// fragment is named by its path relative to it; the document is `<input>`
// in a placement suffix (SPEC v0.4b §6.18.1, §6.18.6).
func TestIncludeUnderParse(t *testing.T) {
	app, err := tuimark.Parse(strings.NewReader(`<tui><include src="testdata/v04b/t136/shell.tui"/></tui>`))
	if err != nil {
		t.Fatal(err)
	}
	if d := app.Validate(); len(d) != 0 {
		t.Fatalf("Validate: %v", diagTexts(d))
	}
	res, err := app.Play(tuimark.PlayOptions{Cols: 10, Rows: 2, NoHandlers: true}, "x")
	if err != nil || len(res.Events) != 1 || res.Events[0].Action != "go" {
		t.Errorf("play: %v, events %+v", err, res.Events)
	}
	app, err = tuimark.Parse(strings.NewReader(`<tui><screen id="x"><include src="testdata/v04b/t136/shell.tui"/></screen></tui>`))
	if err != nil {
		t.Fatal(err)
	}
	got := diagTexts(app.Validate())
	want := `V001 testdata/v04b/t136/shell.tui:2:3 /screen/style id= <style> must be a direct child of <tui>, placed by <include> at <input>:1:21`
	if len(got) == 0 || got[0] != want {
		t.Errorf("diagnostics:\n%s\nwant first\n%s", strings.Join(got, "\n"), want)
	}
}

// LoadFS over an in-memory file system (the playground's): includes
// resolve inside it, against the including file.
func TestIncludeOverAMapFS(t *testing.T) {
	mem := fstest.MapFS{
		"app.tui":       {Data: []byte("<tui>\n  <include src=\"kit/shell.tui\"/>\n</tui>\n")},
		"kit/shell.tui": {Data: []byte("<fragment>\n  <screen id=\"s\"><include src=\"body.tui\"/></screen>\n</fragment>\n")},
		"kit/body.tui":  {Data: []byte("<fragment><text>hi</text></fragment>")},
	}
	app, err := tuimark.LoadFS(mem, "app.tui")
	if err != nil {
		t.Fatal(err)
	}
	if d := app.Validate(); len(d) != 0 {
		t.Fatalf("Validate: %v", diagTexts(d))
	}
	d, err := app.Dump(10, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Nodes) != 2 || d.Nodes[1].Text != "hi" {
		t.Errorf("nodes: %+v", d.Nodes)
	}
}

// The placement policy covers what the other 0.4b phases report on an
// element an include placed (SPEC v0.4b §6.18.6): F7's button label is a
// template, so its missing path is B003 with the suffix; F5's L009 (a
// layout warning) is in the fragment's file with the suffix.
func TestPlacementSuffixOnOtherPhasesDiagnostics(t *testing.T) {
	const doc = "<tui>\n <screen id=\"s\">\n  <include src=\"f.tui\"/>\n </screen>\n</tui>\n"
	load := func(frag string) *tuimark.App {
		mem := fstest.MapFS{
			"app.tui": {Data: []byte(doc)},
			"f.tui":   {Data: []byte(frag)},
		}
		app, err := tuimark.LoadFS(mem, "app.tui")
		if err != nil {
			t.Fatal(err)
		}
		return app
	}
	const by = ", placed by <include> at app.tui:3:3"
	app := load("<fragment>\n <button id=\"b\" label=\"{d.no}\"/>\n</fragment>\n")
	if err := app.Bind("", map[string]any{"d": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	want := []string{`B003 f.tui:2:2 /screen/button id=b bind path "d.no" is missing` + by}
	if got := diagTexts(app.Validate()); !reflect.DeepEqual(got, want) {
		t.Errorf("button label:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	app = load("<fragment>\n <col width=\"10\" height=\"2\"><text>a</text><text>b</text><text>c</text></col>\n</fragment>\n")
	got := diagTexts(app.Validate())
	if len(got) != 1 || !strings.HasPrefix(got[0], "L009 f.tui:2:2 /screen/col id= cuts text[2] on y") || !strings.HasSuffix(got[0], by) {
		t.Errorf("L009: %v", got)
	}
}
