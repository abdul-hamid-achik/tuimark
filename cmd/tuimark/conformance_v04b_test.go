package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/agentsdoc"
	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC v0.4b §24, phase F0: the CLI parts of tests 189, 209, and 130's IR
// bullet. The Go API parts are the root package's
// conformance_v04b_base_test.go.

// v04bKeymapK is test 209's document K; ATTRS is replaced by the <tui>
// attributes of each case.
const v04bKeymapK = `<tui ATTRS>
  <screen id="main">
    <input id="q" bind="q"/>
    <list id="l"><item><text>a</text></item></list>
  </screen>
  <keymap>
    <bind keys="ctrl+i" action="a"/>
    <bind keys="ctrl+j,x" action="b"/>
    <bind keys="ctrl+m,ctrl+i" action="c"/>
  </keymap>
</tui>`

func writeK(t *testing.T, attrs string) string {
	t.Helper()
	return writeTemp(t, "K.tui", strings.Replace(v04bKeymapK, "ATTRS", attrs, 1))
}

func decodePlay(t *testing.T, out string) dump.Play {
	t.Helper()
	var p dump.Play
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatalf("play output: %v\n%s", err, out)
	}
	return p
}

// 189. A dump of a document that writes version validates against the
// copy of schema/dump.v0.2.json frozen at 0.3a/0.4.1
// (testdata/dump.v0.2-0.3a.json), with every combination of --cells and
// --styles; the frozen 0.4.1 twins are entries of the golden manifest.
func TestV04bPinnedDumpsValidateAgainstFrozenSchema(t *testing.T) {
	type doc struct{ file, data string }
	docs := []doc{
		{"../../examples/inbox/app.tui", "../../examples/inbox/sample.json"},
		{"../../examples/monitor/studio.tui", "../../examples/monitor/sample.json"},
		{"../../specs/fixtures/v04b/twins/colrow.tui", ""},
		{"../../specs/fixtures/table.tui", "../../specs/fixtures/table.json"},
	}
	for _, v := range []string{"1", "2", "3"} {
		docs = append(docs, doc{writeK(t, `version="`+v+`"`), ""})
	}
	for _, d := range docs {
		for _, extra := range [][]string{nil, {"--cells"}, {"--styles"}, {"--cells", "--styles"}} {
			args := []string{"dump", d.file, "--format", "json", "--cols", "80", "--rows", "24"}
			if d.data != "" {
				args = append(args, "--data", d.data)
			}
			args = append(args, extra...)
			code, out, errw := runCLI(args...)
			if code != 0 {
				t.Fatalf("%v: exit %d: %s", args, code, errw)
			}
			if errs := validateSchema(t, dumpV02Frozen, decodeJSON(t, []byte(out))); len(errs) != 0 {
				t.Errorf("%v: %s", args, strings.Join(errs, "\n"))
			}
		}
	}
}

// The 0.4.1 twins of plan §5 (tests 189, 129, and the pinned twins of
// 156-170 and 221) are frozen entries of the manifest, written by tuimark
// 0.4.1, so --update never rewrites them; every manifest entry named
// "*-041" is one of them, and each pins its document.
func TestV04bTwinsAreFrozen(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/golden/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		Name   string `json:"name"`
		File   string `json:"file"`
		Frozen bool   `json:"frozen"`
		JSON   []string
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	const tw = "specs/fixtures/v04b/twins/"
	want := map[string]string{
		"inbox-041":               "examples/inbox/app.tui",
		"monitor-041":             "examples/monitor/studio.tui",
		"colrow-041":              tw + "colrow.tui",
		"t156-hints-col-041":      tw + "hints-col.tui",
		"t157-hints-row-041":      tw + "hints-row.tui",
		"t158-confirm-041":        tw + "confirm.tui",
		"t159-help-041":           tw + "help.tui",
		"t160-esc-041":            tw + "esc.tui",
		"t162-esc-q-041":          tw + "esc.tui",
		"t162-esc-x-041":          tw + "esc.tui",
		"t162-esc-o-041":          tw + "esc.tui",
		"t162-esc-ctrlc-041":      tw + "esc.tui",
		"t163-esc-041":            tw + "esc.tui",
		"t164-tabs-041":           tw + "tabs.tui",
		"t164-tabs-focusable-041": tw + "tabs-focusable.tui",
		"t165-panes-041":          tw + "panes.tui",
		"t166-stack-041":          tw + "stack.tui",
		"t167-pct-041":            tw + "pct.tui",
		"t168-starve-d30-041":     tw + "starve.tui",
		"t168-starve-d3-041":      tw + "starve.tui",
		"t168-starve-app-d30-041": tw + "starve-app.tui",
		"t168-starve-app-d3-041":  tw + "starve-app.tui",
		"t169-twopane-041":        tw + "twopane.tui",
		"t170-freeze-041":         tw + "freeze.tui",
		"t221-field-041":          tw + "field.tui",
		"t221-field-typed-041":    tw + "field.tui",
	}
	for _, e := range entries {
		file, ok := want[e.Name]
		if !ok {
			if strings.HasSuffix(e.Name, "-041") {
				t.Errorf("%s: a twin this test does not list", e.Name)
			}
			continue
		}
		delete(want, e.Name)
		if !e.Frozen || e.File != file || len(e.JSON) == 0 {
			t.Errorf("%s: %+v, want frozen, file %s, and JSON sizes", e.Name, e, file)
		}
		for _, size := range e.JSON {
			if _, err := os.Stat(filepath.Join("../../testdata/golden", e.Name, size+".json")); err != nil {
				t.Errorf("%s: %v", e.Name, err)
			}
		}
		if doc := parse.Parse(mustRead(t, "../../"+e.File), e.File); doc.Lang.V4() {
			t.Errorf("%s: %s is not pinned", e.Name, e.File)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing twins: %v", want)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// 189's "same diagnostics": validate --json of each pinned example prints,
// byte for byte, what tuimark 0.4.1 printed (testdata/validate-*-041.json,
// written by v0.4.1 from this directory with the same arguments).
func TestV04bPinnedValidateAsIn041(t *testing.T) {
	for _, c := range []struct{ golden, file, data string }{
		{"testdata/validate-inbox-041.json", "../../examples/inbox/app.tui", "../../examples/inbox/sample.json"},
		{"testdata/validate-monitor-041.json", "../../examples/monitor/studio.tui", "../../examples/monitor/sample.json"},
	} {
		code, out, errw := runCLI("validate", c.file, "--data", c.data, "--json")
		if code != 0 {
			t.Fatalf("%s: exit %d: %s", c.file, code, errw)
		}
		if want := string(mustRead(t, c.golden)); out != want {
			t.Errorf("%s: validate --json differs from 0.4.1\ngot:\n%s\nwant:\n%s", c.file, out, want)
		}
	}
}

// 209, through the CLI. The current language: validate --catalog reports
// the four V003 in order and exits 2; play still runs the other tokens.
// Pinned: validate is ok, and play behaves as in 0.4.1. The three steps
// are accepted against both.
func TestV04bNeverMatchKeysCLI(t *testing.T) {
	cat := writeTemp(t, "C.json", `["a","b","c"]`)
	k := writeK(t, "")
	code, out, errw := runCLI("validate", k, "--catalog", cat)
	want := []string{
		`error V003 K.tui:7:5 /keymap/bind key "ctrl+i" never reaches a keymap: it arrives as tab (write tab)`,
		`error V003 K.tui:8:5 /keymap/bind key "ctrl+j" never reaches a keymap: it arrives as enter (write enter)`,
		`error V003 K.tui:9:5 /keymap/bind key "ctrl+m" never reaches a keymap: it arrives as enter (write enter)`,
		`error V003 K.tui:9:5 /keymap/bind key "ctrl+i" never reaches a keymap: it arrives as tab (write tab)`,
	}
	if code != 2 || strings.TrimRight(out, "\n") != strings.Join(want, "\n") {
		t.Errorf("current validate: exit %d\n%s%s", code, out, errw)
	}
	code, out, errw = runCLI("play", k, "--input", "tab x", "--format", "json")
	p := decodePlay(t, out)
	if code != 2 || len(p.Events) != 1 || p.Events[0].Step != 2 || p.Events[0].Action != "b" || p.Events[0].Source != "l" || p.Focus == nil || *p.Focus != "l" {
		t.Errorf("current play tab x: exit %d, events %+v, focus %v: %s", code, p.Events, p.Focus, errw)
	}
	if code, _, errw := runCLI("play", k, "--input", "ctrl+i ctrl+j ctrl+m", "--format", "json"); code != 2 {
		t.Errorf("current play ctrl+i ctrl+j ctrl+m: exit %d (want 2, for the document's errors): %s", code, errw)
	}

	for _, v := range []string{"1", "2", "3"} {
		k := writeK(t, `version="`+v+`"`)
		if code, out, errw := runCLI("validate", k, "--catalog", cat); code != 0 || out != "ok (bindings not checked: no --data)\n" {
			t.Errorf("version=%s validate: exit %d %q %s", v, code, out, errw)
		}
		code, out, errw := runCLI("play", k, "--input", "ctrl+i x", "--format", "json", "--frames")
		p := decodePlay(t, out)
		if code != 0 || len(p.Frames) < 2 || p.Frames[1].Dump.Focus == nil || *p.Frames[1].Dump.Focus != "l" ||
			len(p.Events) != 1 || p.Events[0].Step != 2 || p.Events[0].Action != "b" {
			t.Errorf("version=%s play ctrl+i x: exit %d, events %+v: %s", v, code, p.Events, errw)
		}
		code, out, errw = runCLI("play", k, "--input", "ctrl+i ctrl+j ctrl+m", "--format", "json", "--frames")
		p = decodePlay(t, out)
		if code != 0 || p.Frames[1].Dump.Focus == nil || *p.Frames[1].Dump.Focus != "l" || len(p.Events) != 0 {
			t.Errorf("version=%s play ctrl+i ctrl+j ctrl+m: exit %d, events %+v: %s", v, code, p.Events, errw)
		}
	}
}

// 130's IR bullet: a <tui> without version prints IR 0.4, which validates
// against schema/ir.v0.4.json; version="3" still prints 0.3.
func TestV04bIRVersion(t *testing.T) {
	cur := writeTemp(t, "a.tui", `<tui><screen id="s"><text>a</text></screen></tui>`)
	code, out, errw := runCLI("ir", cur)
	if code != 0 || !strings.Contains(out, `"version": "0.4"`) {
		t.Fatalf("ir: exit %d %s%s", code, out, errw)
	}
	if errs := validateSchema(t, irV04Schema, decodeJSON(t, []byte(out))); len(errs) != 0 {
		t.Errorf("IR vs ir.v0.4.json: %v", errs)
	}
	pinned := writeTemp(t, "b.tui", `<tui version="3"><screen id="s"><text>a</text></screen></tui>`)
	if code, out, _ := runCLI("ir", pinned); code != 0 || !strings.Contains(out, `"version": "0.3"`) {
		t.Errorf("version=3 ir: exit %d %s", code, out)
	}
}

// The "Only in a <tui> without version" attribute section of AGENTS.md is
// written once a phase fills parse.TagAttrsCurrent, and the catalog
// cross-check of test 119 reads it back with since current. The test
// replaces the tables for its duration, so it does not depend on the
// vocabulary the phases have filled.
func TestV04bCurrentAttrsSectionRoundTrips(t *testing.T) {
	kinds, attrs := ir.KindsCurrent, parse.TagAttrsCurrent
	t.Cleanup(func() { ir.KindsCurrent, parse.TagAttrsCurrent = kinds, attrs })
	ir.KindsCurrent = []string{"checkbox"}
	parse.TagAttrsCurrent = map[string]map[string]bool{"checkbox": {"id": true, "label": true}, "list": {"single": true}}
	md := agentsdoc.RepoMarkdown()
	for _, s := range []string{"Only in a `<tui>` without `version`", "- `checkbox`: id, label\n", "- `list`: single\n"} {
		if !strings.Contains(md, s) {
			t.Errorf("AGENTS.md lacks %q", s)
		}
	}
	if problems := compareToAgents(catalog.Build(version), md); len(problems) != 0 {
		t.Errorf("the catalog and AGENTS.md disagree:\n%s", strings.Join(problems, "\n"))
	}
}
