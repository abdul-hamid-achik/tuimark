package host

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// doc parses a document from source (styles resolve against ".").
func doc(t *testing.T, src string) *App {
	t.Helper()
	a, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// v1 wraps a screen body in a minimal v1 document.
func v1(body string) string {
	return `<tui version="1"><screen id="main">` + body + `</screen></tui>`
}

// inbox loads the SPEC §17 fixture with its sample data.
func inbox(t *testing.T) *App {
	t.Helper()
	a, err := Load("../../examples/inbox/app.tui")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../examples/inbox/sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if err := a.Bind("", v); err != nil {
		t.Fatal(err)
	}
	return a
}

func get(t *testing.T, a *App, path string) any {
	t.Helper()
	v, ok := a.Get(path)
	if !ok {
		t.Fatalf("store has no %q", path)
	}
	return v
}

func TestToJSON(t *testing.T) {
	type ticket struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Prio  int    `json:"prio,omitempty"`
	}
	cases := []struct {
		name string
		in   any
		want any
	}{
		{"nil", nil, nil},
		{"bool", true, true},
		{"string", "x", "x"},
		{"float64", 2.5, 2.5},
		{"int", 3, 3.0},
		{"int64", int64(-7), -7.0},
		{"uint8", uint8(200), 200.0},
		{"float32", float32(0.5), 0.5},
		{"struct", ticket{ID: "t-1", Title: "x"}, map[string]any{"id": "t-1", "title": "x"}},
		{"struct pointer", &ticket{ID: "t-2", Prio: 1}, map[string]any{"id": "t-2", "title": "", "prio": 1.0}},
		{"slice", []ticket{{ID: "a"}}, []any{map[string]any{"id": "a", "title": ""}}},
		{"empty slice", []string{}, []any{}},
		{"nil slice", []string(nil), nil},
		{"map", map[string]int{"n": 1}, map[string]any{"n": 1.0}},
		{"nested any", map[string]any{"a": []any{1, "b", nil}}, map[string]any{"a": []any{1.0, "b", nil}}},
		{"raw message", json.RawMessage(`{"k":[1]}`), map[string]any{"k": []any{1.0}}},
	}
	for _, c := range cases {
		got, err := ToJSON(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: ToJSON = %#v, want %#v", c.name, got, c.want)
		}
	}
	for name, in := range map[string]any{
		"chan":     make(chan int),
		"func":     func() {},
		"NaN":      math.NaN(),
		"+Inf":     math.Inf(1),
		"float32":  float32(math.Inf(-1)),
		"complex":  complex(1, 2),
		"int keys": map[bool]int{true: 1},
	} {
		if v, err := ToJSON(in); err == nil {
			t.Errorf("%s: ToJSON accepted it as %#v", name, v)
		}
	}
	// Coercion copies: later changes to the caller's value do not leak in.
	src := map[string]any{"a": []any{"x"}}
	a := doc(t, v1(""))
	if err := a.Bind("data", src); err != nil {
		t.Fatal(err)
	}
	src["a"].([]any)[0] = "changed"
	if got := get(t, a, "data.a.0"); got != "x" {
		t.Errorf("store aliases the caller's value: %v", got)
	}
}

func TestBindAndSetPaths(t *testing.T) {
	a := doc(t, v1(""))
	steps := []struct {
		path string
		v    any
		err  string
	}{
		{"folder", "deploy", ""},
		{"count", 3, ""},
		{"a.b.c", true, ""}, // creates objects
		{"tickets", []map[string]string{{"id": "t-1"}, {"id": "t-2"}}, ""},
		{"tickets.1.title", "second", ""}, // hosts may use numeric segments
		{"tickets.2.title", "x", "not an index of a 2-element array"},
		{"tickets.x", "x", "not an index"},
		{"folder.name", "x", "not an object"},
		{"a..b", 1, "bad path"},
		{".a", 1, "bad path"},
		{"a.", 1, "bad path"},
		{"", "scalar", "root value must be a JSON object"},
		{"", nil, "root value must be a JSON object"},
		{"bad", make(chan int), "not JSON-shaped"},
		{"selected_ticket", nil, ""},
		{"selected_ticket.title", "t", ""}, // null becomes an object on demand
	}
	for _, s := range steps {
		err := a.Bind(s.path, s.v)
		if s.err == "" && err != nil {
			t.Errorf("Bind(%q): %v", s.path, err)
		}
		if s.err != "" && (err == nil || !strings.Contains(err.Error(), s.err)) {
			t.Errorf("Bind(%q) error = %v, want %q", s.path, err, s.err)
		}
	}
	for path, want := range map[string]any{
		"folder":                "deploy",
		"count":                 3.0,
		"a.b.c":                 true,
		"tickets.0.id":          "t-1",
		"tickets.1.title":       "second",
		"selected_ticket.title": "t",
	} {
		if got := get(t, a, path); !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", path, got, want)
		}
	}
	for _, path := range []string{"nope", "tickets.5", "tickets.-1", "folder.x", "a.b.c.d"} {
		if v, ok := a.Get(path); ok {
			t.Errorf("Get(%q) = %v, want missing", path, v)
		}
	}
	if root, ok := a.Get(""); !ok || root.(map[string]any)["folder"] != "deploy" {
		t.Errorf("Get(\"\") = %v", root)
	}
	// "" replaces the whole store.
	if err := a.Bind("", map[string]any{"only": 1}); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Get("folder"); ok {
		t.Error("Bind(\"\", obj) replaces the store")
	}
}

func TestSetWakesTheLoop(t *testing.T) {
	a := doc(t, v1(""))
	if err := a.Bind("x", 1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.wake:
		t.Error("Bind must not request a redraw")
	default:
	}
	if err := a.Set("x", 2); err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.wake:
	default:
		t.Error("Set requests a redraw")
	}
	// Coalesced: a second Set before the loop drains does not block.
	_ = a.Set("x", 3)
	_ = a.Set("x", 4)
	if err := a.Set("x", math.NaN()); err == nil {
		t.Error("Set(NaN) accepted")
	}
}

func TestFormat(t *testing.T) {
	for _, c := range []struct {
		v    any
		want string
	}{
		{nil, ""},
		{"s", "s"},
		{true, "true"},
		{false, "false"},
		{3.0, "3"},
		{-2.0, "-2"},
		{2.5, "2.5"},
		{1e20, "1e+20"},
		{0.1, "0.1"},
		{[]any{1.0, "a"}, `[1,"a"]`},
		{map[string]any{"k": nil}, `{"k":null}`},
	} {
		if got := Format(c.v); got != c.want {
			t.Errorf("Format(%#v) = %q, want %q", c.v, got, c.want)
		}
	}
}

// SPEC §21 test 12: {folder} interpolates from sample.json.
func TestInterpolation(t *testing.T) {
	a := inbox(t)
	d := a.Dump(80, 24, false)
	if !d.OK {
		t.Fatal(d.Errors)
	}
	if !strings.HasPrefix(d.Grid[0], "mail  deploy") {
		t.Errorf("header = %q", d.Grid[0])
	}
	if !strings.HasSuffix(d.Grid[23], "3 tickets") || !strings.HasPrefix(d.Grid[23], "connected") {
		t.Errorf("status = %q", d.Grid[23])
	}

	b := doc(t, v1(`<col>
  <text>n={n} f={f} b={b} z={z} o={o} a={a}</text>
  <box border="single" title="T {n}"><text>x</text></box>
  <input id="q" placeholder="find {f}"/>
</col>`))
	_ = b.Bind("", map[string]any{"n": 3, "f": 2.5, "b": true, "z": nil, "o": map[string]any{"k": 1}, "a": []any{}})
	d = b.Dump(40, 6, false)
	if got := strings.TrimRight(d.Grid[0], " "); got != `n=3 f=2.5 b=true z= o={"k":1} a=[]` {
		t.Errorf("formats = %q", got)
	}
	if !strings.HasPrefix(d.Grid[1], "┌─ T 3 ─") {
		t.Errorf("title = %q", d.Grid[1])
	}
	if !strings.HasPrefix(d.Grid[4], "find 2.5") {
		t.Errorf("placeholder = %q", d.Grid[4])
	}
	if len(d.Errors) != 0 {
		t.Errorf("errors = %v", d.Errors)
	}
}

func TestMissingPaths(t *testing.T) {
	a := doc(t, v1(`<col><text>[{missing}]</text><box if="gone"><text>x</text></box><progress bind="pct"/></col>`))
	d := a.Dump(20, 3, false)
	if !d.OK || !strings.HasPrefix(d.Grid[0], "[]") {
		t.Errorf("missing {path} renders empty: %q ok=%v", d.Grid[0], d.OK)
	}
	got := map[string]string{}
	for _, e := range d.Errors {
		got[e.Code] = e.Severity
	}
	if got["B003"] != ir.Warning || got["B002"] != ir.Warning || len(d.Errors) != 3 {
		t.Errorf("errors = %v", d.Errors)
	}
	// --strict upgrades B003 to an error.
	a.SetStrict(true)
	d = a.Dump(20, 3, false)
	if d.OK {
		t.Errorf("strict: %v", d.Errors)
	}
	// Every bind= reports a missing path the same way: list, input,
	// progress, and modal.
	for _, body := range []string{
		`<list id="l" each="rows as r" key="r" bind="sel"><item><text>{r}</text></item></list>`,
		`<input id="q" bind="sel"/>`,
		`<progress bind="sel"/>`,
		`<modal id="m" bind="sel"/>`,
	} {
		m := doc(t, v1(body))
		_ = m.Bind("rows", []any{"a"})
		d := m.Dump(10, 3, false)
		if len(d.Errors) != 1 || d.Errors[0].Code != "B003" || !d.OK {
			t.Errorf("%s: %v", body, d.Errors)
		}
		m.SetStrict(true)
		if d := m.Dump(10, 3, false); d.OK {
			t.Errorf("%s: --strict makes B003 an error", body)
		}
	}
	// Control characters in data are stripped from the grid.
	b := doc(t, v1(`<text>{x}</text>`))
	_ = b.Bind("x", "a\x1b[31mb\x07c")
	if d := b.Dump(10, 1, false); d.Grid[0] != "abc       " {
		t.Errorf("data with ANSI = %q", d.Grid[0])
	}
	// A progress bound to a non-number warns.
	c := doc(t, v1(`<progress bind="p"/>`))
	_ = c.Bind("p", "high")
	if d := c.Dump(10, 1, false); len(d.Errors) != 1 || !strings.Contains(d.Errors[0].Msg, "want a number") {
		t.Errorf("progress bind = %v", d.Errors)
	}
}

func TestStyleIncludes(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("a.tcss", "#t { bold: true; }")
	write("bad.tcss", "#t { colour: red; }")
	cases := []struct {
		name, styles, code, msg string
	}{
		{"self include", `<style src="app.tui"/>`, "V006", "includes a .tui document"},
		{"other tui", `<style src="other.tui"/>`, "V006", "includes a .tui document"},
		{"twice", `<style src="a.tcss"/><style src="./a.tcss"/>`, "V006", "included twice"},
		{"unreadable", `<style src="missing.tcss"/>`, "V006", "cannot be read"},
		{"sheet error", `<style src="bad.tcss"/>`, "V003", `unknown property "colour"`},
		{"inline error", "<style>\n#t { bogus: 1; }\n</style>", "V003", `unknown property "bogus"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := write("app.tui", `<tui version="1">`+c.styles+`<screen><text id="t">x</text></screen></tui>`)
			a, err := Load(p)
			if err != nil {
				t.Fatal(err)
			}
			var found *ir.Diagnostic
			for _, d := range a.Validate() {
				if d.Code == c.code {
					d := d
					found = &d
				}
			}
			if found == nil {
				t.Fatalf("want %s, got %v", c.code, a.Validate())
			}
			if !strings.Contains(found.Msg, c.msg) || found.Severity != ir.Error {
				t.Errorf("diag = %+v", found)
			}
		})
	}
	// Inline sheet diagnostics point into the .tui file.
	p := write("app.tui", "<tui version=\"1\">\n<style>\n#t { bogus: 1; }\n</style>\n<screen><text id=\"t\">x</text></screen></tui>")
	a, _ := Load(p)
	for _, d := range a.Validate() {
		if d.Code == "V003" && (d.File != "app.tui" || d.Line != 3 || d.Col != 6) {
			t.Errorf("inline sheet diag at %s:%d:%d", d.File, d.Line, d.Col)
		}
	}
	// Files lists the document and its stylesheets (for --watch).
	p = write("app.tui", `<tui version="1"><style src="a.tcss"/><screen/></tui>`)
	a, _ = Load(p)
	if f := a.Files(); len(f) != 2 || f[0] != p || f[1] != filepath.Join(dir, "a.tcss") {
		t.Errorf("Files() = %v", f)
	}
	if len(a.Sheets()) != 1 || a.Document() == nil {
		t.Error("Sheets/Document")
	}
	if _, err := Load(filepath.Join(dir, "nope.tui")); err == nil {
		t.Error("Load of a missing file is an I/O error")
	}
}

func TestCatalogAndValidate(t *testing.T) {
	a := inbox(t)
	a.On("open", func(Event) error { return nil })
	cat := a.Catalog()
	var names []string
	for _, s := range cat {
		names = append(names, s.Name)
	}
	if strings.Join(names, " ") != "focus open quit search" {
		t.Fatalf("catalog = %+v", cat)
	}
	byName := map[string]ActionSpec{}
	for _, s := range cat {
		byName[s.Name] = s
	}
	if o := byName["open"]; !o.Registered || o.Builtin || strings.Join(o.Sources, "; ") != "keys enter; #inbox on:select" {
		t.Errorf("open = %+v", o)
	}
	if q := byName["quit"]; !q.Builtin || q.Registered {
		t.Errorf("quit = %+v", q)
	}
	b4 := a.CheckCatalog(map[string]bool{"open": true})
	if len(b4) != 1 || b4[0].Code != "B004" || b4[0].Severity != ir.Warning || !strings.Contains(b4[0].Msg, `"search"`) {
		t.Errorf("CheckCatalog = %v", b4)
	}
	if len(a.CheckCatalog(map[string]bool{"open": true, "search": true})) != 0 {
		t.Error("builtins are always in the catalog")
	}
	if diags := a.Validate(); len(diags) != 0 {
		t.Errorf("inbox validates clean: %v", diags)
	}
	// Validate checks 40/80/120 columns and does not disturb runtime state.
	c := doc(t, v1(`<row><box id="a" width="100"/><box id="b" width="30"/></row><input id="q"/>`))
	c.Frame(200, 24)
	_ = c.Set("@focus", "#q")
	before := c.Focus()
	diags := c.Validate()
	if len(diags) != 3 || diags[0].Code != "L003" || !strings.Contains(diags[0].Msg, "has 40") || !strings.Contains(diags[2].Msg, "has 120") {
		t.Errorf("Validate = %v (one L003 per breakpoint)", diags)
	}
	if c.Focus() != before {
		t.Error("Validate changed focus")
	}
	if len(c.Static()) != 0 {
		t.Errorf("Static = %v", c.Static())
	}
}

func TestParsedFailuresStillDump(t *testing.T) {
	for _, src := range []string{`<app><box></app>`, `<html/>`, `<tui version="1"/>`} {
		a := doc(t, src)
		d := a.Dump(10, 3, false)
		if d.OK || len(d.Grid) != 3 || d.Grid[0] != "          " || len(d.Nodes) != 0 {
			t.Errorf("%s: %+v", src, d)
		}
		if txt := dump.Text(d); !strings.Contains(txt, "=== errors ===\nerror V005") {
			t.Errorf("%s: text dump %q", src, txt)
		}
	}
}

// Validate renders at 40/80/120 columns without disturbing the live
// viewport offsets.
func TestValidateKeepsViewportState(t *testing.T) {
	a := doc(t, v1(`<list id="l" each="rows as r" key="r" style="height: 3"><item><text>{r}</text></item></list><scroll id="s" focusable="true" style="height: 1fr"><text>1
2
3
4
5
6
7
8</text></scroll>`))
	_ = a.Bind("rows", []any{"a", "b", "c", "d", "e", "f", "g", "h"})
	a.Frame(30, 8)
	for i := 0; i < 5; i++ {
		a.HandleKey(Key{Name: "down"})
		a.Frame(30, 8)
	}
	_ = a.Set("@focus", "#s")
	f := a.Frame(30, 8)
	a.HandleKey(Key{Name: "pgdn"})
	f = a.Frame(30, 8)
	listOff, scrollOff := f.ByID["l"].ScrollY, f.ByID["s"].ScrollY
	if listOff != 3 || scrollOff != 3 {
		t.Fatalf("setup: list offset %d scroll offset %d", listOff, scrollOff)
	}
	_ = a.Validate()
	f = a.Frame(30, 8)
	if f.ByID["l"].ScrollY != listOff || f.ByID["s"].ScrollY != scrollOff || a.Focus() != "s" {
		t.Errorf("after Validate: list %d scroll %d focus %q", f.ByID["l"].ScrollY, f.ByID["s"].ScrollY, a.Focus())
	}
}
