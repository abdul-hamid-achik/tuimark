package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// SPEC v0.4b §24, phase F8: the CLI parts of tests 174-178, 186, 188,
// and 222 for the checkbox and single="true" (§6.14.1, §6.16). The Go API
// parts are the root package's conformance_v04b_forms_test.go; both read
// testdata/v04b/{174,177,178}.

const (
	cbFixture    = "../../testdata/v04b/174/checkbox.tui"
	cbJSON       = "../../testdata/v04b/174/checkbox.json"
	radioFixture = "../../testdata/v04b/177/radio.tui"
	radioTable   = "../../testdata/v04b/178/radio-table.tui"
	radioJSON    = "../../testdata/v04b/177/radio.json"
)

// 174: the text dump marks the focused, checked checkbox, and the JSON
// dump validates against schema/dump.v0.2.json, whose tag enum now holds
// checkbox.
func TestV04bCheckboxDumpCLI(t *testing.T) {
	code, out, errw := runCLI("dump", cbFixture, "--data", cbJSON, "--cols", "20", "--rows", "3")
	if code != 0 || !strings.Contains(out, "dark  checkbox  20x1 @(0,0) focus checked\n") || !strings.Contains(out, "beta  checkbox  20x1 @(0,1)\n") {
		t.Errorf("text dump: exit %d\n%s%s", code, out, errw)
	}
	for _, flags := range [][]string{nil, {"--styles"}, {"--cells"}} {
		args := append([]string{"dump", cbFixture, "--data", cbJSON, "--cols", "20", "--rows", "3", "--format", "json"}, flags...)
		code, out, errw = runCLI(args...)
		if code != 0 {
			t.Fatalf("dump %v: exit %d %s", flags, code, errw)
		}
		if errs := validateSchema(t, dumpV02Schema, decodeJSON(t, []byte(out))); len(errs) != 0 {
			t.Errorf("dump %v vs dump.v0.2.json: %v", flags, errs)
		}
	}
}

// 174 and 177: play's events, as the CLI prints them.
func TestV04bFormsPlayCLI(t *testing.T) {
	for _, c := range []struct {
		file, data, input, events string
	}{
		{cbFixture, cbJSON, "space", `[{"step":1,"action":"dark_changed","source":"dark","keys":{},"value":false}]`},
		{cbFixture, cbJSON, "enter", `[]`},
		{cbFixture, cbJSON, "tab space", `[]`},
		{radioFixture, radioJSON, "space", `[{"step":1,"action":"size_changed","source":"size","keys":{"it":"s"},"value":"s"}]`},
		{radioFixture, radioJSON, "x x", `[{"step":1,"action":"size_changed","source":"size","keys":{"it":"s"},"value":null}]`},
		{radioTable, radioJSON, "a", `[{"step":1,"action":"all_fallback","source":"size","keys":{"it":"s"},"value":null}]`},
	} {
		code, out, errw := runCLI("play", c.file, "--data", c.data, "--cols", "20", "--rows", "3", "--input", c.input, "--format", "json")
		if code != 0 {
			t.Fatalf("%s %q: exit %d %s", c.file, c.input, code, errw)
		}
		p := decodePlay(t, out)
		got, _ := json.Marshal(p.Events)
		if string(got) == "null" {
			got = []byte("[]")
		}
		if string(got) != c.events {
			t.Errorf("%s %q: events %s, want %s", c.file, c.input, got, c.events)
		}
	}
}

// 222: one read, through --script, sees the checkbox the space checked.
func TestV04bFormsOneReadCLI(t *testing.T) {
	doc := writeTemp(t, "oneread.tui", `<tui>
  <keymap>
    <bind keys="x" action="go" when="#c:checked"/>
  </keymap>
  <screen id="main" focus="#c">
    <checkbox id="c" label="C"/>
  </screen>
</tui>
`)
	script := writeTemp(t, "s.ndjson", `{"text":" x"}`+"\n")
	code, out, errw := runCLI("play", doc, "--script", script, "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d %s", code, errw)
	}
	got, _ := json.Marshal(decodePlay(t, out).Events)
	if want := `[{"step":1,"action":"go","source":"c","keys":{},"value":true}]`; string(got) != want {
		t.Errorf("events %s, want %s", got, want)
	}
}

// 175 and 185's validate bullet: B011 depends on the data, so validate
// without --data reports none; B003 follows --strict.
func TestV04bCheckboxValidateCLI(t *testing.T) {
	code, out, _ := runCLI("validate", cbFixture)
	if code != 0 || strings.TrimSpace(out) != "ok (bindings not checked: no --data)" {
		t.Errorf("no data: exit %d, %q", code, out)
	}
	code, out, _ = runCLI("validate", cbFixture, "--data", cbJSON)
	if code != 0 || strings.TrimSpace(out) != "ok" {
		t.Errorf("--data: exit %d, %q", code, out)
	}
	yes := writeTemp(t, "yes.json", `{"settings": {"dark": "yes"}}`)
	code, out, _ = runCLI("validate", cbFixture, "--data", yes)
	if code != 0 || !strings.Contains(out, "warning B011 checkbox.tui:3:5 /screen/checkbox") || strings.Count(out, "B011") != 1 {
		t.Errorf("B011: exit %d, %q", code, out)
	}
	none := writeTemp(t, "none.json", `{}`)
	code, out, _ = runCLI("validate", cbFixture, "--data", none, "--strict")
	if code != 2 || !strings.Contains(out, `error B003 checkbox.tui:3:5 /screen/checkbox bind path "settings.dark" is missing`) {
		t.Errorf("--strict: exit %d, %q", code, out)
	}
	arr := writeTemp(t, "arr.json", `{"size": ["m"], "sizes": []}`)
	code, out, _ = runCLI("validate", radioFixture, "--data", arr)
	if code != 2 || !strings.Contains(out, "error B008 radio.tui:8:5 /screen/list") {
		t.Errorf("B008: exit %d, %q", code, out)
	}
}

// 188's catalog bullet: the checkbox and its attributes have since 4,
// single on list and table has since 4 with true/false since 4, and B011
// follows B010.
func TestV04bFormsCatalog(t *testing.T) {
	_, c := catalogOutput(t)
	seen := map[string]bool{}
	for _, tg := range c.Tags {
		switch tg.Name {
		case "checkbox":
			seen[tg.Name] = true
			if tg.Since != 4 {
				t.Errorf("checkbox since %d", tg.Since)
			}
			names := []string{}
			for _, a := range tg.Attrs {
				names = append(names, a.Name)
				if a.Since != 4 {
					t.Errorf("checkbox.%s since %d", a.Name, a.Since)
				}
			}
			for _, want := range []string{"bind", "label", "on:change", "id", "class:NAME"} {
				if !strings.Contains(" "+strings.Join(names, " ")+" ", " "+want+" ") {
					t.Errorf("checkbox lacks %s: %v", want, names)
				}
			}
			if strings.Contains(" "+strings.Join(names, " ")+" ", " on:click ") {
				t.Errorf("checkbox takes on:click: %v", names)
			}
		case "list", "table":
			for _, a := range tg.Attrs {
				if a.Name != "single" {
					continue
				}
				seen[tg.Name] = true
				kw, _ := json.Marshal(a.Keywords)
				if a.Since != 4 || string(kw) != `[{"name":"true","since":4},{"name":"false","since":4}]` {
					t.Errorf("%s.single: since %d, keywords %s", tg.Name, a.Since, kw)
				}
			}
		}
	}
	if len(seen) != 3 {
		t.Errorf("seen %v", seen)
	}
	codes := []string{}
	for _, d := range c.Diagnostics {
		codes = append(codes, d.Code)
	}
	if j := strings.Join(codes, " "); !strings.Contains(j, "B009 B010 B011 ") {
		t.Errorf("diagnostics order: %s", j)
	}
}

// 130's IR bullet for the form controls: IR 0.4 holds the checkbox kind
// and keeps label and single in attrs; fmt keeps a checkbox as written.
func TestV04bFormsIRAndFmt(t *testing.T) {
	doc := writeTemp(t, "f.tui", `<tui>
  <screen id="s">
    <checkbox id="c" bind="on" label="Hi {who}"/>
    <list id="l" each="xs as x" key="x" checked="pick" single="true">
      <item>
        <text>{x}</text>
      </item>
    </list>
  </screen>
</tui>
`)
	code, out, errw := runCLI("ir", doc)
	if code != 0 {
		t.Fatalf("ir: exit %d %s", code, errw)
	}
	if errs := validateSchema(t, irV04Schema, decodeJSON(t, []byte(out))); len(errs) != 0 {
		t.Errorf("IR vs ir.v0.4.json: %v", errs)
	}
	for _, want := range []string{`"kind": "checkbox"`, `"label": "Hi {who}"`, `"single": "true"`, `"bind": "on"`} {
		if !strings.Contains(out, want) {
			t.Errorf("IR lacks %s:\n%s", want, out)
		}
	}
	code, out, errw = runCLI("fmt", doc)
	if code != 0 || !strings.Contains(out, `<checkbox id="c" bind="on" label="Hi {who}"/>`) || !strings.Contains(out, `single="true"`) {
		t.Errorf("fmt: exit %d\n%s%s", code, out, errw)
	}
}

// SPEC v0.4b §24, phase F9: the CLI parts of tests 179-188, 220, and 222
// for the <select> (§6.17); the Go API parts are the root package's
// conformance_v04b_forms_test.go. Both read testdata/v04b/179.
const (
	selFixture = "../../testdata/v04b/179/select.tui"
	selJSON    = "../../testdata/v04b/179/select.json"
)

// 179 and 180: the closed select's dump and the open list's play output,
// with --styles and --cells too, validate against schema/dump.v0.2.json,
// whose tag enum now holds select; the open list's cells belong to tz.
func TestV04bSelectDumpCLI(t *testing.T) {
	code, out, errw := runCLI("dump", selFixture, "--data", selJSON, "--cols", "20", "--rows", "6")
	if code != 0 || !strings.Contains(out, "tz    select    20x1 @(0,0) focus\n") {
		t.Errorf("text dump: exit %d\n%s%s", code, out, errw)
	}
	base := []string{selFixture, "--data", selJSON, "--cols", "20", "--rows", "6", "--format", "json"}
	for _, args := range [][]string{
		append([]string{"dump"}, base...),
		append([]string{"dump", "--styles", "--cells"}, base...),
		append([]string{"play", "--input", "enter", "--styles", "--cells"}, base...),
		append([]string{"play", "--input", "enter down", "--frames"}, base...),
	} {
		code, out, errw := runCLI(args...)
		if code != 0 {
			t.Fatalf("%v: exit %d %s", args, code, errw)
		}
		if errs := validateSchema(t, dumpV02Schema, decodeJSON(t, []byte(out))); len(errs) != 0 {
			t.Errorf("%v vs dump.v0.2.json: %v", args, errs)
		}
		if args[0] != "play" || args[2] != "enter" {
			continue
		}
		p := decodePlay(t, out)
		n := 0
		for _, c := range p.Cells {
			if c.Y >= 1 && c.Y <= 5 {
				n++
				if c.ID != "tz" {
					t.Errorf("cell (%d,%d) belongs to %q", c.X, c.Y, c.ID)
				}
			}
		}
		if n != 100 {
			t.Errorf("%d cells under the list", n)
		}
	}
}

// 185's validate bullet: B012 depends on the data, so validate without
// --data reports none.
func TestV04bSelectValidateCLI(t *testing.T) {
	code, out, _ := runCLI("validate", selFixture)
	if code != 0 || strings.TrimSpace(out) != "ok (bindings not checked: no --data)" {
		t.Errorf("no data: exit %d, %q", code, out)
	}
	code, out, _ = runCLI("validate", selFixture, "--data", selJSON)
	if code != 0 || strings.TrimSpace(out) != "ok" {
		t.Errorf("--data: exit %d, %q", code, out)
	}
	nope := writeTemp(t, "nope.json", `{"tz": "nope", "zones": [{"id": "utc", "name": "UTC"}]}`)
	code, out, _ = runCLI("validate", selFixture, "--data", nope)
	const want = `warning B012 select.tui:6:5 /screen/select bind="tz" holds "nope", which is no option's key; nothing is chosen, and a pick writes over it`
	if code != 0 || !strings.Contains(out, want) || strings.Count(out, "B012") != 1 {
		t.Errorf("B012: exit %d, %q", code, out)
	}
}

// 222's select bullet: one read, through --script, opens the list, moves
// its cursor, and picks.
func TestV04bSelectOneReadCLI(t *testing.T) {
	script := writeTemp(t, "s.ndjson", `{"text":"\r\u001b[B\r"}`+"\n")
	code, out, errw := runCLI("play", selFixture, "--data", selJSON, "--cols", "20", "--rows", "6", "--script", script, "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d %s", code, errw)
	}
	p := decodePlay(t, out)
	got, _ := json.Marshal(p.Events)
	if want := `[{"step":1,"action":"tz_changed","source":"tz","keys":{"z":"tok"},"value":"tok"}]`; string(got) != want {
		t.Errorf("events %s, want %s", got, want)
	}
	if p.Grid[0] != "Tokyo              ▾" {
		t.Errorf("grid[0] %q", p.Grid[0])
	}
}

// 188's catalog bullet for the select: the tag and each of its
// attributes have since 4, it takes no on:click, and B012 follows B011.
func TestV04bSelectCatalog(t *testing.T) {
	_, c := catalogOutput(t)
	found := false
	for _, tg := range c.Tags {
		if tg.Name != "select" {
			continue
		}
		found = true
		if tg.Since != 4 {
			t.Errorf("select since %d", tg.Since)
		}
		names := []string{}
		for _, a := range tg.Attrs {
			names = append(names, a.Name)
			if a.Since != 4 {
				t.Errorf("select.%s since %d", a.Name, a.Since)
			}
		}
		joined := " " + strings.Join(names, " ") + " "
		for _, want := range []string{"bind", "each", "key", "label", "placeholder", "on:change", "id", "class:NAME"} {
			if !strings.Contains(joined, " "+want+" ") {
				t.Errorf("select lacks %s: %v", want, names)
			}
		}
		for _, no := range []string{"on:click", "checked", "single"} {
			if strings.Contains(joined, " "+no+" ") {
				t.Errorf("select takes %s: %v", no, names)
			}
		}
	}
	if !found {
		t.Error("no select tag")
	}
	codes := []string{}
	for _, d := range c.Diagnostics {
		codes = append(codes, d.Code)
	}
	if j := strings.Join(codes, " "); !strings.Contains(j, "B010 B011 B012 ") {
		t.Errorf("diagnostics order: %s", j)
	}
}

// 130's IR bullet for the select: IR 0.4 holds the select kind and keeps
// its attributes; fmt keeps it as written.
func TestV04bSelectIRAndFmt(t *testing.T) {
	doc := writeTemp(t, "s.tui", `<tui>
  <screen id="s">
    <select id="tz" bind="tz" each="zones as z" key="z.id" label="{z.name}" placeholder="zone"/>
  </screen>
</tui>
`)
	code, out, errw := runCLI("ir", doc)
	if code != 0 {
		t.Fatalf("ir: exit %d %s", code, errw)
	}
	if errs := validateSchema(t, irV04Schema, decodeJSON(t, []byte(out))); len(errs) != 0 {
		t.Errorf("IR vs ir.v0.4.json: %v", errs)
	}
	for _, want := range []string{`"kind": "select"`, `"label": "{z.name}"`, `"placeholder": "zone"`, `"each": "zones as z"`, `"key": "z.id"`} {
		if !strings.Contains(out, want) {
			t.Errorf("IR lacks %s:\n%s", want, out)
		}
	}
	code, out, errw = runCLI("fmt", doc)
	if code != 0 || !strings.Contains(out, `<select id="tz" bind="tz" each="zones as z" key="z.id" label="{z.name}" placeholder="zone"/>`) {
		t.Errorf("fmt: exit %d\n%s%s", code, out, errw)
	}
}

// 220's inspect bullets: the built-in focus rule sets reverse on the
// focused button, and an author rule overrides it.
func TestV04bFocusLookInspectCLI(t *testing.T) {
	const (
		fixture = "../../testdata/v04b/220/focus.tui"
		data    = "../../testdata/v04b/220/focus.json"
	)
	type rule struct {
		File        string `json:"file"`
		Selector    string `json:"selector"`
		Specificity int    `json:"specificity"`
	}
	type entry struct {
		Prop       string  `json:"prop"`
		Value      any     `json:"value"`
		Origin     string  `json:"origin"`
		Rule       *rule   `json:"rule"`
		Overridden []entry `json:"overridden"`
	}
	reverse := func(file string) entry {
		t.Helper()
		code, out, errw := runCLI("inspect", file, "--data", data, "--cols", "20", "--rows", "4", "--id", "b", "--json")
		if code != 0 {
			t.Fatalf("inspect: exit %d %s", code, errw)
		}
		var res struct {
			Style []entry `json:"style"`
		}
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatal(err)
		}
		for _, e := range res.Style {
			if e.Prop == "reverse" {
				return e
			}
		}
		t.Fatal("no reverse entry")
		return entry{}
	}
	ua := rule{File: "<ua>", Selector: "button:focus, checkbox:focus, select:focus", Specificity: 11}
	e := reverse(fixture)
	if e.Value != "true" || e.Origin != "ua" || e.Rule == nil || *e.Rule != ua || e.Overridden == nil || len(e.Overridden) != 0 {
		t.Errorf("built-in: %+v", e)
	}
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	src := strings.Replace(string(raw), "<tui>", `<tui><style>.k-btn:focus { reverse: false; bold: true; }</style>`, 1)
	src = strings.Replace(src, `<button id="b"`, `<button id="b" class="k-btn"`, 1)
	e = reverse(writeTemp(t, "focus.tui", src))
	if e.Value != "false" || e.Origin != "author" || len(e.Overridden) != 1 {
		t.Fatalf("author: %+v", e)
	}
	if o := e.Overridden[0]; o.Origin != "ua" || o.Rule == nil || *o.Rule != ua {
		t.Errorf("author, overridden: %+v", o)
	}
}
