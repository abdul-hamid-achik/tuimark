package tuimark_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.2b §21 conformance tests of the 0.2b foundations: the
// version="2" gate (§5.1), the reserved Set path @theme (§18), class:NAME
// (§6.13), and version="1" compatibility (§13.2, test 69).

const versionHint = ` (requires version="2")`

func v1Screen(body string) string {
	return `<tui version="1"><screen id="main">` + body + `</screen></tui>`
}

func v1Keymap(rows string) string {
	return `<tui version="1"><keymap>` + rows + `</keymap><screen id="main"><list id="l" each="xs as x" key="x"><item><text>{x}</text></item></list></screen></tui>`
}

func v1Style(css string) string {
	return `<tui version="1"><style>` + css + `</style><screen id="main"><text>x</text></screen></tui>`
}

// gateCases are the §5.1 table, item by item, in a version="1" document,
// with the code each one gets, and the same item written validly in a
// version="2" document.
var gateCases = []struct {
	name, v1, code, v2 string
}{
	{"tag table", v1Screen(`<table id="t" each="xs as x" key="x"><column>{x}</column></table>`), "V001", ""},
	// A column outside a table is V016 in version="2" (SPEC §6.9.1): the
	// valid form there is inside one.
	{"tag column", v1Screen(`<column>x</column>`), "V001", `<tui version="2"><screen id="main"><table id="t" each="xs as x" key="x"><column>x</column></table></screen></tui>`},
	{"tag tabs", v1Screen(`<tabs id="n"><text>x</text></tabs>`), "V001", ""},
	{"tag tab", v1Screen(`<tab id="a" label="a"/>`), "V001", ""},
	{"tag sparkline", v1Screen(`<sparkline bind="xs"/>`), "V001", ""},
	{"tag hints", v1Screen(`<hints/>`), "V001", ""},
	{"attr mouse", `<tui version="1" mouse="true"><screen id="main"/></tui>`, "V002", ""},
	{"attr bind label", v1Keymap(`<bind keys="q" action="quit" label="quit"/>`), "V002", ""},
	{"attr bind keycap", v1Keymap(`<bind keys="q" action="quit" keycap="Q"/>`), "V002", ""},
	{"attr col each", v1Screen(`<col each="xs as x"><text>{x}</text></col>`), "V002", ""},
	{"attr row key", v1Screen(`<row key="x"/>`), "V002", ""},
	{"attr box each", v1Screen(`<box each="xs as x" key="x"/>`), "V002", ""},
	{"attr list checked", v1Screen(`<list id="l" each="xs as x" key="x" checked="m"><item><text>{x}</text></item></list>`), "V002", ""},
	{"attr list mark", v1Screen(`<list id="l" each="xs as x" key="x" checked="m" mark="▸"><item><text>{x}</text></item></list>`), "V002", ""},
	{"attr list on:change", v1Screen(`<list id="l" each="xs as x" key="x" checked="m" on:change="c"><item><text>{x}</text></item></list>`), "V002", ""},
	{"attr class:NAME", v1Screen(`<text class:hot="h">x</text>`), "V002", ""},
	{"value theme auto", `<tui version="1" theme="auto"><screen id="main"/></tui>`, "V003", ""},
	{"value layout grid", v1Style(`box { layout: grid; }`), "V003", ""},
	{"value layout grid in style attr", v1Screen(`<box style="layout: grid"/>`), "V003", ""},
	{"property grid-columns", v1Style(`box { grid-columns: 3; }`), "V003", ""},
	{"property grid-min-width", v1Style(`box { grid-min-width: 20; }`), "V003", ""},
	{"property scrollbar", v1Style(`scroll { scrollbar: auto; }`), "V003", ""},
	{"property bar", v1Style(`progress { bar: eighths; }`), "V003", ""},
	{"pseudo :checked", v1Style(`item:checked { bold: true; }`), "V003", ""},
	{"pseudo :focus-within", v1Style(`col:focus-within { bold: true; }`), "V003", ""},
	{"type table", v1Style(`table { bold: true; }`), "V003", ""},
	{"type column", v1Style(`column { bold: true; }`), "V003", ""},
	{"type tabs", v1Style(`tabs { bold: true; }`), "V003", ""},
	{"type tab", v1Style(`tab { bold: true; }`), "V003", ""},
	{"type sparkline", v1Style(`sparkline { bold: true; }`), "V003", ""},
	{"type hints", v1Style(`hints > row { bold: true; }`), "V003", ""},
	{"media theme", v1Style(`@media (theme: light) { text { bold: true; } }`), "V003", ""},
	{"when :checked", v1Keymap(`<bind keys="x" action="go" when="item:checked"/>`), "V003", ""},
	{"when :focus-within", v1Keymap(`<bind keys="x" action="go" when="screen:focus-within"/>`), "V003", ""},
	{"when type table", v1Keymap(`<bind keys="x" action="go" when="table"/>`), "V003", ""},
	{"action move-next", v1Keymap(`<bind keys="j" action="move-next"/>`), "V003", ""},
	{"action move-prev", v1Keymap(`<bind keys="k" action="move-prev"/>`), "V003", ""},
	{"action move-first", v1Keymap(`<bind keys="g" action="move-first"/>`), "V003", ""},
	{"action move-last", v1Keymap(`<bind keys="e" action="move-last"/>`), "V003", ""},
	{"action move-page-down", v1Keymap(`<bind keys="d" action="move-page-down"/>`), "V003", ""},
	{"action move-page-up", v1Keymap(`<bind keys="u" action="move-page-up"/>`), "V003", ""},
	{"action check-toggle", v1Keymap(`<bind keys="space" action="check-toggle"/>`), "V003", ""},
	{"action check-all", v1Keymap(`<bind keys="ctrl+a" action="check-all"/>`), "V003", ""},
	{"action check-none", v1Keymap(`<bind keys="ctrl+d" action="check-none"/>`), "V003", ""},
	{"action switch-to", v1Keymap(`<bind keys="2" action="switch-to" to="#l"/>`), "V003", ""},
}

func validateSrc(t *testing.T, src string) []tuimark.Diagnostic {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	return app.Validate()
}

// 39. The version gate: in a version="1" document every item of the §5.1
// table is its code with a message ending in (requires version="2"); the
// same items are valid in a version="2" document.
func TestVersionGate(t *testing.T) {
	for _, c := range gateCases {
		t.Run(c.name, func(t *testing.T) {
			diags := validateSrc(t, c.v1)
			found := false
			for _, d := range diags {
				if d.Code == c.code && strings.HasSuffix(d.Msg, versionHint) {
					found = true
				}
			}
			if !found {
				t.Fatalf("version=\"1\": want %s ending with %q, got %v", c.code, versionHint, diags)
			}
			// The bind checks depend on the data, which these documents do
			// not bind (validate without --data skips them, §14).
			v2 := c.v2
			if v2 == "" {
				v2 = strings.Replace(c.v1, `version="1"`, `version="2"`, 1)
			}
			for _, d := range validateSrc(t, v2) {
				if strings.HasPrefix(d.Code, "B") {
					continue
				}
				if strings.HasSuffix(d.Msg, versionHint) || d.Severity == "error" {
					t.Errorf("version=\"2\": %s", d)
				}
			}
		})
	}
}

// 39. version="3" is V003, and the document is checked as version="1":
// a version="2" tag in it is V001 with the version hint.
func TestVersionThreeIsReadAsVersionOne(t *testing.T) {
	diags := validateSrc(t, `<tui version="3"><screen id="main"><hints/></screen></tui>`)
	var codes []string
	for _, d := range diags {
		codes = append(codes, d.Code)
		if d.Code == "V003" && !strings.Contains(d.Msg, `version="3"`) {
			t.Errorf("V003 message %q", d.Msg)
		}
	}
	if strings.Join(codes, " ") != "V003 V001" {
		t.Fatalf("codes = %v (%v), want V003 then V001", codes, diags)
	}
	if !strings.HasSuffix(diags[1].Msg, versionHint) {
		t.Errorf("V001 message %q lacks the version hint", diags[1].Msg)
	}
}

// 39. A stylesheet has no version of its own: one .tcss loaded by a
// version="1" and a version="2" document reports the §5.1 V003s only in
// the version="1" one.
func TestSharedStylesheetFollowsTheDocument(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "shared.tcss"), "box { layout: grid; grid-columns: 2; }\ncol:focus-within { bold: true; }\n@media (theme: light) { :root { --accent: #c8102e; } }\n")
	doc := `<tui version="%s"><style src="shared.tcss"/><screen id="main"><box><text>x</text></box></screen></tui>`
	for _, v := range []string{"1", "2"} {
		p := filepath.Join(dir, "v"+v+".tui")
		mustWrite(t, p, strings.Replace(doc, "%s", v, 1))
		app, err := tuimark.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, d := range app.Validate() {
			if d.Code == "V003" && strings.HasSuffix(d.Msg, versionHint) && d.File == "shared.tcss" {
				n++
			} else if d.Severity == "error" {
				t.Errorf("version=%s: unexpected %s", v, d)
			}
		}
		want := map[string]int{"1": 4, "2": 0}[v]
		if n != want {
			t.Errorf("version=%s: %d hinted V003s in shared.tcss, want %d", v, n, want)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 43. The reserved Set path @theme: "dark", "light", or "auto" in both
// versions; anything else is an error and changes nothing; it never
// reaches the store.
func TestSetThemePath(t *testing.T) {
	for _, v := range []string{"1", "2"} {
		app, err := tuimark.Parse(strings.NewReader(`<tui version="` + v + `"><screen id="main"><text>{x}</text></screen></tui>`))
		if err != nil {
			t.Fatal(err)
		}
		for _, ok := range []string{"dark", "light", "auto"} {
			if err := app.Set("@theme", ok); err != nil {
				t.Errorf("version=%s: Set(@theme, %q) = %v", v, ok, err)
			}
			if err := app.Bind("@theme", ok); err != nil {
				t.Errorf("version=%s: Bind(@theme, %q) = %v", v, ok, err)
			}
		}
		for _, bad := range []any{"blue", "Light", "", 1, nil, true} {
			if err := app.Set("@theme", bad); err == nil {
				t.Errorf("version=%s: Set(@theme, %v) accepted", v, bad)
			}
		}
		// @theme is not a store member: {x} still resolves nothing.
		d, err := app.Dump(10, 1)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(d.Grid[0]) != "" {
			t.Errorf("version=%s: @theme leaked into the store: %q", v, d.Grid[0])
		}
	}
}

// 63 / 69. A version="1" document with class attributes dumps no classes
// member, while a version="2" one lists its classes, then its truthy
// guards, without repeats.
func TestClassesOnlyInVersionTwoDumps(t *testing.T) {
	src := `<tui version="%s"><screen id="main"><text id="t" class="a b">x</text></screen></tui>`
	for _, c := range []struct {
		v    string
		want string
	}{{"1", ""}, {"2", `"classes":["a","b"]`}} {
		app, err := tuimark.Parse(strings.NewReader(strings.Replace(src, "%s", c.v, 1)))
		if err != nil {
			t.Fatal(err)
		}
		d, err := app.Dump(10, 1)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(d.Nodes)
		if c.want == "" && strings.Contains(string(b), "classes") {
			t.Errorf("version=1 dump has classes: %s", b)
		}
		if c.want != "" && !strings.Contains(string(b), c.want) {
			t.Errorf("version=2 dump nodes %s, want %s", b, c.want)
		}
	}
	guards := `<tui version="2"><screen id="main"><text id="t" class="a live" class:live="on" class:stale="off" class:hot="!off" class:gone="missing">x</text></screen></tui>`
	app, err := tuimark.Parse(strings.NewReader(guards))
	if err != nil {
		t.Fatal(err)
	}
	_ = app.Bind("", map[string]any{"on": true, "off": false})
	d, err := app.Dump(10, 1)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range d.Nodes {
		if n.ID == "t" {
			got = n.Classes
		}
	}
	if strings.Join(got, " ") != "a live hot" {
		t.Errorf("classes = %v, want [a live hot] (class names, truthy guards in order, no repeats)", got)
	}
	b002 := 0
	for _, e := range d.Errors {
		if e.Code == "B002" && strings.Contains(e.Msg, "class:gone") {
			b002++
		}
	}
	if b002 != 1 {
		t.Errorf("a missing guard path gives one B002, got %v", d.Errors)
	}
}

// 63. class:NAME diagnostics: V015 for a bad NAME or guard, V002 for an
// uppercase NAME, V005 for a repeated NAME, and none of the tags that
// take no class:NAME accept it.
func TestClassGuardDiagnostics(t *testing.T) {
	for _, c := range []struct {
		name, body, code string
	}{
		{"digit name", `<text class:1x="a">x</text>`, "V015"},
		{"guard with a space", `<text class:a="a b">x</text>`, "V015"},
		{"guard with braces", `<text class:a="{a}">x</text>`, "V015"},
		{"uppercase name", `<text class:Hot="a">x</text>`, "V002"},
		{"uppercase prefix", `<text Class:hot="a">x</text>`, "V002"},
		{"repeated name", `<text class:a="x" class:a="y">x</text>`, "V005"},
	} {
		t.Run(c.name, func(t *testing.T) {
			diags := validateSrc(t, `<tui version="2"><screen id="main">`+c.body+`</screen></tui>`)
			if len(diags) == 0 || diags[0].Code != c.code {
				t.Fatalf("want %s, got %v", c.code, diags)
			}
		})
	}
	diags := validateSrc(t, `<tui version="2" class:a="x"><keymap class:b="y"><bind keys="q" action="quit" class:c="z"/></keymap><screen id="main"/></tui>`)
	if n := len(diags); n != 3 {
		t.Fatalf("class:NAME on tui, keymap, bind: %v", diags)
	}
	for _, d := range diags {
		if d.Code != "V002" {
			t.Errorf("%s", d)
		}
	}
}

// 69. Every existing golden passes `tuimark test` byte for byte (also
// asserted by cmd/tuimark); here: the §17 inbox is still version="1" and
// its JSON goldens hold no member 0.2b added.
func TestVersionOneGoldensHaveNo02bMembers(t *testing.T) {
	src, err := os.ReadFile("examples/inbox/app.tui")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `<tui version="1"`) {
		t.Fatal("examples/inbox/app.tui is no longer version=\"1\"")
	}
	for _, g := range []string{"inbox/80x24.json", "inbox/120x24.json", "spike/80x24.json", "unicode/80x24.json"} {
		b, err := os.ReadFile(filepath.Join("testdata/golden", g))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), `"classes"`) || strings.Contains(string(b), `"checked"`) {
			t.Errorf("%s holds a 0.2b member", g)
		}
	}
}
