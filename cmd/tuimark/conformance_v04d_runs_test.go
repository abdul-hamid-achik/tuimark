package main

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// SPEC v0.4d §21 tests 264-274 of phase D1 (rich text runs, the generated
// span, and wrap: pre-wrap), through the CLI. The fixtures are
// testdata/v04d/t<test>. Test 275 is the byte-identical comparison with
// tuimark 0.4.5 that the cmp3 and cmp5 scripts run over every example and
// golden document.

const runsFixtures = "../../testdata/v04d/"

// runsTUI is a current-language document with one text bound to runs.
func runsTUI(style, textAttrs string) string {
	return "<tui>\n  <style>" + style + "</style>\n  <screen id=\"main\">\n    <text id=\"t\" runs=\"r\" " + textAttrs + "/>\n  </screen>\n</tui>\n"
}

// dumpMap runs the dump command and returns its JSON.
func dumpMap(t *testing.T, args ...string) map[string]any {
	t.Helper()
	code, out, errw := runCLI(append([]string{"dump"}, append(args, "--format", "json")...)...)
	if code != 0 && code != 2 {
		t.Fatalf("dump %v: exit %d %s", args, code, errw)
	}
	return jsonMap(t, out)
}

// playMap runs the play command and returns its JSON.
func playMap(t *testing.T, args ...string) map[string]any {
	t.Helper()
	code, out, errw := runCLI(append([]string{"play"}, append(args, "--format", "json")...)...)
	if code != 0 && code != 2 {
		t.Fatalf("play %v: exit %d %s", args, code, errw)
	}
	return jsonMap(t, out)
}

func gridOfMap(m map[string]any) []string {
	var rows []string
	for _, r := range m["grid"].([]any) {
		rows = append(rows, r.(string))
	}
	return rows
}

// nodeOfMap is the dump node with id.
func nodeOfMap(t *testing.T, m map[string]any, id string) map[string]any {
	t.Helper()
	for _, n := range m["nodes"].([]any) {
		if nm := n.(map[string]any); nm["id"] == id {
			return nm
		}
	}
	t.Fatalf("no node %q in %v", id, m["nodes"])
	return nil
}

// textOfMap is the text member of node id, "" when it has none.
func textOfMap(t *testing.T, m map[string]any, id string) string {
	t.Helper()
	s, _ := nodeOfMap(t, m, id)["text"].(string)
	return s
}

// styleSpans is the styles of row y of a dump or play with --styles, as
// "x+w fg/bg attrs" strings.
func styleSpans(t *testing.T, m map[string]any, y int) []string {
	t.Helper()
	rows, ok := m["styles"].([]any)
	if !ok || y >= len(rows) {
		t.Fatalf("no styles row %d in %v", y, m["styles"])
	}
	var out []string
	for _, e := range rows[y].([]any) {
		s := e.(map[string]any)
		str := ""
		if a, ok := s["a"].([]any); ok {
			for _, x := range a {
				str += " " + x.(string)
			}
		}
		out = append(out, strings.TrimSpace(
			itoa(s["x"])+"+"+itoa(s["w"])+" "+s["fg"].(string)+"/"+s["bg"].(string)+str))
	}
	return out
}

func itoa(v any) string { return strconv.Itoa(int(v.(float64))) }

// 264. Runs: dump and styles.
func TestV04dRuns264DumpAndStyles(t *testing.T) {
	dir := runsFixtures + "t264/"
	m := dumpMap(t, dir+"app.tui", "--data", dir+"d.json", "--cols", "20", "--rows", "4", "--styles")
	if m["ok"] != true || len(m["errors"].([]any)) != 0 {
		t.Fatalf("ok %v errors %v", m["ok"], m["errors"])
	}
	wantNodes := []any{
		map[string]any{"id": "main", "tag": "screen", "x": 0.0, "y": 0.0, "w": 20.0, "h": 4.0},
		map[string]any{"id": "reply", "tag": "text", "x": 0.0, "y": 0.0, "w": 20.0, "h": 3.0, "text": "Use go build and then check the go.mod file"},
	}
	if !reflect.DeepEqual(m["nodes"], wantNodes) {
		t.Errorf("nodes: %v", m["nodes"])
	}
	sameLines(t, "grid", gridOfMap(m), []string{"Use go build and    ", "then check the      ", "go.mod file         ", "                    "})
	plain := "0+20 default/default"
	for y, want := range [][]string{
		{"0+4 default/default", "4+8 cyan/default bold", "12+8 default/default"},
		{plain},
		{"0+6 cyan/default bold italic", "6+14 default/default"},
		{plain},
	} {
		sameLines(t, "styles row", styleSpans(t, m, y), want)
	}
	// The grid and the node are what the joined text as the body gives
	// (the joining spaces at x 3 and x 12 take the style of the unstyled
	// runs they stand for).
	b := dumpMap(t, dir+"body.tui", "--cols", "20", "--rows", "4", "--styles")
	if !reflect.DeepEqual(b["nodes"], m["nodes"]) || !reflect.DeepEqual(b["grid"], m["grid"]) {
		t.Errorf("body variant differs:\n%v\n%v", b["nodes"], m["nodes"])
	}
}

// 265. Runs: values and B013.
func TestV04dRuns265ValuesAndB013(t *testing.T) {
	app := writeTemp(t, "app.tui", runsTUI("", ""))
	const at = "app.tui" // diagnostics name the file as given, here its base
	body := writeTemp(t, "body.tui", "<tui><screen id=\"main\"><text id=\"t\">{r}</text></screen></tui>\n")
	data := func(json string) string { return writeTemp(t, "d.json", json) }
	dump := func(file, json string) map[string]any {
		return dumpMap(t, file, "--data", data(json), "--cols", "20", "--rows", "3")
	}

	// A TAB is four spaces in run text, and a {path} removes it (decision 208).
	m := dump(app, `{"r": "a\tb"}`)
	if got := textOfMap(t, m, "t"); got != "a    b" {
		t.Errorf("runs TAB: text %q", got)
	}
	if got := gridOfMap(m)[0]; !strings.HasPrefix(got, "a    b") {
		t.Errorf("runs TAB: row %q", got)
	}
	if len(m["errors"].([]any)) != 0 {
		t.Errorf("runs TAB: %v", m["errors"])
	}
	if got := textOfMap(t, dump(body, `{"r": "a\tb"}`), "t"); got != "ab" {
		t.Errorf("{path} TAB: text %q", got)
	}
	// null is no run, with no diagnostic.
	m = dump(app, `{"r": null}`)
	if _, has := nodeOfMap(t, m, "t")["text"]; has || len(m["errors"].([]any)) != 0 {
		t.Errorf("null: %v", m)
	}
	// ESC before a TAB is removed alone; ESC ( B is one escape sequence,
	// where a {path} keeps the B (decision 265).
	if got := textOfMap(t, dump(app, `{"r": "a\u001b\tb"}`), "t"); got != "a    b" {
		t.Errorf("ESC TAB: %q", got)
	}
	if got := textOfMap(t, dump(app, `{"r": "a\u001b(Bb"}`), "t"); got != "ab" {
		t.Errorf("ESC ( B in runs: %q", got)
	}
	if got := textOfMap(t, dump(body, `{"r": "a\u001b(Bb"}`), "t"); got != "aBb" {
		t.Errorf("ESC ( B in a body: %q", got)
	}

	// A missing path is B003.
	sameLines(t, "missing", vdiags(t, app, data(`{}`)), []string{
		`warning B003 ` + at + `:4:5 /screen/text id=t runs path "r" is missing`,
	})
	// A value of the wrong shape is B013.
	sameLines(t, "number", vdiags(t, app, data(`{"r": 5}`)), []string{
		`warning B013 ` + at + `:4:5 /screen/text id=t runs="r" holds a number, not an array of runs, a string, or null`,
	})
	mixed := `{"r": ["a", 3, {"class": "x"}, {"text": "b", "class": "x 1y"}, {"text": "c", "class": "x", "link": "u"}]}`
	sameLines(t, "mixed", vdiags(t, app, data(mixed)), []string{
		`warning B013 ` + at + `:4:5 /screen/text id=t runs="r": run 1 is a number, not a string or an object`,
	})
	if got := textOfMap(t, dump(app, mixed), "t"); got != "abc" {
		t.Errorf("mixed: text %q", got)
	}
	noText := `{"r": ["a", {"class": "x"}, {"text": "b", "class": "x 1y"}, {"text": "c", "class": "x", "link": "u"}]}`
	sameLines(t, "no text", vdiags(t, app, data(noText)), []string{
		`warning B013 ` + at + `:4:5 /screen/text id=t runs="r": run 1 has no string "text"`,
	})
	badClass := `{"r": ["a", {"text": "b", "class": "x 1y"}, {"text": "c", "class": "x", "link": "u"}]}`
	sameLines(t, "bad class", vdiags(t, app, data(badClass)), []string{
		`warning B013 ` + at + `:4:5 /screen/text id=t runs="r": run 1 has a "class" that is not a list of class names`,
	})
	sameLines(t, "boolean", vdiags(t, app, data(`{"r": true}`)), []string{
		`warning B013 ` + at + `:4:5 /screen/text id=t runs="r" holds a boolean, not an array of runs, a string, or null`,
	})
	sameLines(t, "object", vdiags(t, app, data(`{"r": {"text": "x"}}`)), []string{
		`warning B013 ` + at + `:4:5 /screen/text id=t runs="r" holds an object, not an array of runs, a string, or null`,
	})
	sameLines(t, "null element", vdiags(t, app, data(`{"r": [null]}`)), []string{
		`warning B013 ` + at + `:4:5 /screen/text id=t runs="r": run 0 is null, not a string or an object`,
	})

	// Once per element and frame: two rows of a list that are both bad give
	// one B013, about the first row.
	list := writeTemp(t, "list.tui", "<tui><screen id=\"main\"><list id=\"l\" each=\"rows as m\" key=\"m.id\"><item><text runs=\"m.r\"/></item></list></screen></tui>\n")
	two := data(`{"rows": [{"id": 1, "r": [5]}, {"id": 2, "r": [true]}]}`)
	got := vdiags(t, list, two)
	if len(got) != 1 || !strings.Contains(got[0], `B013`) || !strings.HasSuffix(got[0], `runs="m.r": run 0 is a number, not a string or an object`) {
		t.Errorf("list rows: %v", got)
	}

	// validate without --data gives none of these.
	sameLines(t, "no data", vdiags(t, app, ""), []string{})
}

// 266. Runs: the body.
func TestV04dRuns266Body(t *testing.T) {
	doc := func(inner string) string {
		return writeTemp(t, "app.tui", "<tui><screen id=\"main\"><text runs=\"r\">"+inner+"</text></screen></tui>\n")
	}
	got := vdiags(t, doc("hello"), "")
	if len(got) != 1 || !strings.HasPrefix(got[0], "error V013 ") || !strings.HasSuffix(got[0], ":1:24 /screen/text id= <text runs> takes no body text: its text comes from the runs") {
		t.Errorf("hello: %v", got)
	}
	sameLines(t, "whitespace", vdiags(t, doc("  "), ""), []string{})
	got = vdiags(t, doc("<text>a</text>"), "")
	if len(got) != 1 || !strings.HasPrefix(got[0], "error V013 ") || !strings.HasSuffix(got[0], "<text> cannot contain elements (found <text>)") {
		t.Errorf("element child: %v", got)
	}
	// The body is ignored: the text comes from the runs alone.
	file := doc("hello")
	d := writeTemp(t, "d.json", `{"r": "from runs"}`)
	if got := textOfMap(t, dumpMap(t, file, "--data", d, "--cols", "20", "--rows", "2"), "main"); got != "" {
		t.Errorf("screen text %q", got)
	}
	m := dumpMap(t, file, "--data", d, "--cols", "20", "--rows", "2")
	if g := gridOfMap(m)[0]; !strings.HasPrefix(g, "from runs") {
		t.Errorf("row %q", g)
	}
}

// 267. Runs: paths and aliases.
func TestV04dRuns267PathsAndAliases(t *testing.T) {
	dir := runsFixtures + "t267/"
	m := dumpMap(t, dir+"app.tui", "--data", dir+"d.json", "--cols", "20", "--rows", "5")
	sameLines(t, "grid", gridOfMap(m), []string{"one two             ", "three               ", "last                ", "                    ", "                    "})
	sameLines(t, "validate", vdiags(t, dir+"app.tui", dir+"d.json"), []string{})

	// A missing last.r, through the with alias.
	d := writeTemp(t, "d.json", `{"msgs": [], "last": {}}`)
	got := vdiags(t, dir+"app.tui", d)
	if len(got) != 1 || !strings.Contains(got[0], "warning B003 ") || !strings.HasSuffix(got[0], `runs path "m.r" is missing (last.r via <use template="t">)`) {
		t.Errorf("missing last.r: %v", got)
	}

	// runs="{r}" is not a path.
	bad := writeTemp(t, "bad.tui", "<tui><screen id=\"main\"><text runs=\"{r}\"/></screen></tui>\n")
	got = vdiags(t, bad, "")
	if len(got) != 1 || !strings.HasPrefix(got[0], "error V003 ") || !strings.HasSuffix(got[0], `runs="{r}" is not a path`) {
		t.Errorf("{r}: %v", got)
	}
	// In a template's body, a {$p} in it is the V003 of a path place.
	tpl := writeTemp(t, "tpl.tui", "<tui><template id=\"t\" props=\"p\"><text runs=\"{$p}\"/></template><screen id=\"main\"><use template=\"t\" p=\"x\"/></screen></tui>\n")
	got = vdiags(t, tpl, "")
	found := false
	for _, g := range got {
		if strings.HasPrefix(g, "error V003 ") && strings.HasSuffix(g, `{$p} cannot stand in runs: a prop is text; pass a path with with="…"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("{$p}: %v", got)
	}
}

// 268. The span's cascade.
func TestV04dRuns268SpanCascade(t *testing.T) {
	dir := runsFixtures + "t268/"
	sameLines(t, "validate", vdiags(t, dir+"app.tui", dir+"d.json"), []string{})
	m := dumpMap(t, dir+"app.tui", "--data", dir+"d.json", "--cols", "20", "--rows", "3", "--styles")
	sameLines(t, "styles", styleSpans(t, m, 1), []string{"0+1 red/default bold", "1+1 red/default", "2+18 default/default"})
	if w := nodeOfMap(t, m, "t")["w"]; w != 20.0 {
		t.Errorf("w = %v: width: 10 on the span must change nothing", w)
	}
	// After focus:#t both cells are also underlined, neither is italic.
	p := playMap(t, dir+"app.tui", "--data", dir+"d.json", "--cols", "20", "--rows", "3", "--input", "focus:#t", "--styles")
	sameLines(t, "focused", styleSpans(t, p, 1), []string{"0+1 red/default bold underline", "1+1 red/default underline", "2+18 default/default"})

	// The SPEC's document has no input before the text, so the text takes
	// the initial focus and is underlined from the first frame, and
	// focus:#t changes nothing (an erratum of test 268, reported; the
	// fixture above adds an input to see the unfocused state).
	literal := writeTemp(t, "literal.tui", strings.Replace(string(mustRead(t, dir+"app.tui")), "    <input id=\"i\"/>\n", "", 1))
	m = dumpMap(t, literal, "--data", dir+"d.json", "--cols", "20", "--rows", "2", "--styles")
	sameLines(t, "literal 268, no focus step", styleSpans(t, m, 0), []string{"0+1 red/default bold underline", "1+1 red/default underline", "2+18 default/default"})

	// background: blue on the text: x is white on blue, the cells after the
	// text are blue.
	bg := writeTemp(t, "bg.tui", runsTUI(".b { color: white; } text { background: blue; }", ""))
	d := writeTemp(t, "d.json", `{"r": [{"text": "x", "class": "b"}, "y"]}`)
	m = dumpMap(t, bg, "--data", d, "--cols", "6", "--rows", "1", "--styles")
	sameLines(t, "background", styleSpans(t, m, 0), []string{"0+1 white/blue", "1+5 default/blue"})

	// theme="auto" and a token: cyan in the dark render, blue in the light one.
	tok := writeTemp(t, "tok.tui", "<tui theme=\"auto\">\n  <style>.b { color: $accent; }</style>\n  <screen id=\"main\">\n    <text id=\"t\" runs=\"r\"/>\n  </screen>\n</tui>\n")
	m = dumpMap(t, tok, "--data", d, "--cols", "6", "--rows", "1", "--styles")
	sameLines(t, "dark", styleSpans(t, m, 0)[:1], []string{"0+1 cyan/default"})
	m = dumpMap(t, tok, "--data", d, "--cols", "6", "--rows", "1", "--styles", "--theme", "light")
	sameLines(t, "light", styleSpans(t, m, 0)[:1], []string{"0+1 blue/default"})

	// inspect --id gives the text and no run.
	code, out, errw := runCLI("inspect", dir+"app.tui", "--data", dir+"d.json", "--cols", "10", "--rows", "2", "--id", "t", "--json")
	if code != 0 {
		t.Fatalf("inspect --id: %d %s", code, errw)
	}
	if _, has := jsonMap(t, out)["run"]; has {
		t.Error("inspect --id t has a run")
	}
}

// 269. Runs: truncating.
func TestV04dRuns269Truncating(t *testing.T) {
	d := writeTemp(t, "d.json", `{"r": [{"text": "abc", "class": "b"}, "defgh"]}`)
	for _, c := range []struct {
		wrap string
		row  string
		want []string
	}{
		{"truncate", "abcde…", []string{"0+3 default/default bold", "3+3 default/default"}},
		{"truncate-middle", "abc…gh", []string{"0+3 default/default bold", "3+3 default/default"}},
	} {
		app := writeTemp(t, "app.tui", runsTUI(".b { bold: true; } text { wrap: "+c.wrap+"; width: 6; }", ""))
		m := dumpMap(t, app, "--data", d, "--cols", "6", "--rows", "1", "--styles")
		sameLines(t, c.wrap+" grid", gridOfMap(m), []string{c.row})
		sameLines(t, c.wrap+" styles", styleSpans(t, m, 0), c.want)
	}
}

// 270. pre-wrap.
func TestV04dPreWrap270(t *testing.T) {
	dir := runsFixtures + "t270/"
	code := dir + "code.json"
	m := dumpMap(t, dir+"c.tui", "--data", code, "--cols", "12", "--rows", "6")
	wantText := "def f(x):\n    return  [1,   2,\n            3]"
	wantNode := map[string]any{"id": "c", "tag": "text", "x": 0.0, "y": 0.0, "w": 12.0, "h": 5.0, "text": wantText}
	if got := nodeOfMap(t, m, "c"); !reflect.DeepEqual(got, wantNode) {
		t.Errorf("node: %v", got)
	}
	sameLines(t, "grid", gridOfMap(m), []string{"def f(x):   ", "    return  ", "[1,   2,    ", "            ", "3]          ", "            "})

	// wrap: wrap, as with tuimark 0.4.4.
	wrap := variant(t, dir+"c.tui", "wrap: pre-wrap", "wrap: wrap")
	m = dumpMap(t, wrap, "--data", code, "--cols", "12", "--rows", "6")
	if h := nodeOfMap(t, m, "c")["h"]; h != 4.0 {
		t.Errorf("wrap: h = %v", h)
	}
	sameLines(t, "wrap grid", gridOfMap(m), []string{"def f(x):   ", "return [1,  ", "2,          ", "3]          ", "            ", "            "})
	// nowrap written: h 3, no diagnostic.
	nowrap := variant(t, dir+"c.tui", "wrap: pre-wrap", "wrap: nowrap")
	m = dumpMap(t, nowrap, "--data", code, "--cols", "12", "--rows", "6")
	if h := nodeOfMap(t, m, "c")["h"]; h != 3.0 {
		t.Errorf("nowrap: h = %v", h)
	}
	sameLines(t, "nowrap grid", gridOfMap(m), []string{"def f(x):   ", "    return  ", "            ", "            ", "            ", "            "})
	if len(m["errors"].([]any)) != 0 {
		t.Errorf("nowrap written: %v", m["errors"])
	}

	// Runs with pre-wrap: "  a" and a bold "  b" paint "  a  b", cells 3-5 bold.
	app := writeTemp(t, "app.tui", runsTUI(".b { bold: true; }", `style="wrap: pre-wrap"`))
	d := writeTemp(t, "d.json", `{"r": ["  a", {"text": "  b", "class": "b"}]}`)
	m = dumpMap(t, app, "--data", d, "--cols", "10", "--rows", "1", "--styles")
	if g := gridOfMap(m)[0]; g != "  a  b    " {
		t.Errorf("runs pre-wrap row %q", g)
	}
	sameLines(t, "runs pre-wrap styles", styleSpans(t, m, 0), []string{"0+3 default/default", "3+3 default/default bold", "6+4 default/default"})

	// An empty text is 0 lines, and the node after it takes its row.
	two := writeTemp(t, "two.tui", "<tui><screen id=\"main\"><text id=\"t\" runs=\"r\" style=\"wrap: pre-wrap\"/><text id=\"n\">next</text></screen></tui>\n")
	for _, v := range []string{`{"r": null}`, `{"r": ""}`} {
		m = dumpMap(t, two, "--data", writeTemp(t, "d.json", v), "--cols", "10", "--rows", "2")
		if h := nodeOfMap(t, m, "t")["h"]; h != 0.0 {
			t.Errorf("%s: h = %v", v, h)
		}
		if y := nodeOfMap(t, m, "n")["y"]; y != 0.0 {
			t.Errorf("%s: next at y %v", v, y)
		}
	}

	// Alignment leaves out the hanging spaces of a line.
	al := writeTemp(t, "al.tui", "<tui><screen id=\"main\"><text id=\"c\" style=\"wrap: pre-wrap; content-align: end; width: 4\">{code}</text></screen></tui>\n")
	m = dumpMap(t, al, "--data", writeTemp(t, "d.json", `{"code": "ab      cd"}`), "--cols", "6", "--rows", "2")
	sameLines(t, "aligned", gridOfMap(m), []string{"  ab  ", "  cd  "})
}

// 271. L008 and pre-wrap.
func TestV04dPreWrap271L008(t *testing.T) {
	dir := runsFixtures + "t270/"
	code := dir + "code.json"
	warn := func(file string, rows string) []string {
		m := dumpMap(t, file, "--data", code, "--cols", "12", "--rows", rows)
		out := []string{}
		for _, e := range m["errors"].([]any) {
			d := e.(map[string]any)
			out = append(out, d["code"].(string)+" "+d["msg"].(string))
		}
		return out
	}
	plain := variant(t, dir+"c.tui", ` style="wrap: pre-wrap"`, "")
	sameLines(t, "no style", warn(plain, "6"), []string{"L008 text is 20 columns wide but its content box has 12; set wrap: truncate, or write wrap: nowrap to keep the cut"})
	sameLines(t, "pre-wrap", warn(dir+"c.tui", "6"), []string{})
	short := variant(t, dir+"c.tui", `wrap: pre-wrap`, `wrap: pre-wrap; height: 3`)
	sameLines(t, "pre-wrap, height 3", warn(short, "6"), []string{"L008 text has 5 lines but its content box has 3 rows; give it room, or write overflow: hidden to keep the cut"})
}

// 272. inspect on a run.
func TestV04dRuns272Inspect(t *testing.T) {
	dir := runsFixtures + "t264/"
	inspect := func(args ...string) map[string]any {
		code, out, errw := runCLI(append([]string{"inspect", dir + "app.tui", "--data", dir + "d.json", "--cols", "20", "--rows", "4", "--json"}, args...)...)
		if code != 0 {
			t.Fatalf("inspect %v: exit %d %s", args, code, errw)
		}
		return jsonMap(t, out)
	}
	style := func(run map[string]any, prop string) map[string]any {
		for _, e := range run["style"].([]any) {
			if em := e.(map[string]any); em["prop"] == prop {
				return em
			}
		}
		t.Fatalf("no %s entry in %v", prop, run["style"])
		return nil
	}
	run := inspect("--at", "5,0")["run"].(map[string]any)
	if run["index"] != 1.0 || run["text"] != "go build" || !reflect.DeepEqual(run["classes"], []any{"code"}) {
		t.Errorf("run at 5,0: %v", run)
	}
	color := style(run, "color")
	if color["value"] != "cyan" || color["origin"] != "author" || color["rule"].(map[string]any)["selector"] != ".code" {
		t.Errorf("color: %v", color)
	}
	bold := style(run, "bold")
	if bold["value"] != "true" || bold["rule"].(map[string]any)["selector"] != ".code" {
		t.Errorf("bold: %v", bold)
	}
	if len(run["style"].([]any)) != 7 {
		t.Errorf("style has %d entries, want the 7 of a span", len(run["style"].([]any)))
	}
	run = inspect("--at", "0,2")["run"].(map[string]any)
	if run["index"] != 3.0 || run["text"] != "go.mod" || !reflect.DeepEqual(run["classes"], []any{"code", "i"}) {
		t.Errorf("run at 0,2: %v", run)
	}
	if rule := style(run, "italic")["rule"].(map[string]any); rule["selector"] != "text > span.i" {
		t.Errorf("italic rule: %v", rule)
	}
	// After the text, and on an empty row, no run; so for --id. (The SPEC
	// writes 15,0 for "after the text", but x 15 of row 0 is the d of
	// "and": run 2. The cell after the text on row 0 is 16,0.)
	for _, at := range []string{"16,0", "19,0", "15,1", "0,3"} {
		if _, has := inspect("--at", at)["run"]; has && at != "15,1" {
			t.Errorf("--at %s has a run", at)
		}
	}
	if r := inspect("--at", "15,0")["run"].(map[string]any); r["index"] != 2.0 {
		t.Errorf("--at 15,0: %v", r)
	}
	if _, has := inspect("--id", "reply")["run"]; has {
		t.Error("--id reply has a run")
	}
	// The text output names the run.
	code, out, _ := runCLI("inspect", dir+"app.tui", "--data", dir+"d.json", "--cols", "20", "--rows", "4", "--at", "5,0")
	if code != 0 || !strings.Contains(out, "run 1 \"go build\" .code\n") {
		t.Errorf("text output: %s", out)
	}
}

// 273, the runs part. IR keeps runs in attrs, as written.
func TestV04dRuns273IR(t *testing.T) {
	dir := runsFixtures + "t264/"
	root := irRoot(t, dir+"app.tui")
	screen := kids(t, root)[1]
	reply := kids(t, screen)[0]
	if reply["kind"] != "text" || reply["id"] != "reply" || !reflect.DeepEqual(attrsOf(reply), map[string]any{"runs": "msg.runs"}) {
		t.Errorf("reply node: %v", reply)
	}
}

// 274, the runs, span, and pre-wrap parts. The textarea and key items of
// the same document belong to other phases and keep 0.4.5's messages here.
func TestV04dPinned274(t *testing.T) {
	file := runsFixtures + "t274/pin.tui"
	got := vdiags(t, file, "")
	want := map[int]string{
		0: `selector "text > span.code": type "span" (requires a <tui> without version)`,
		3: `attribute "runs" on <text> (requires a <tui> without version)`,
		4: `wrap: value "pre-wrap" (want wrap | nowrap | truncate | truncate-start | truncate-middle) (requires a <tui> without version)`,
		5: `unknown tag <span>; use <text>`,
	}
	if len(got) != 9 {
		t.Fatalf("%d diagnostics: %v", len(got), got)
	}
	positions := map[int]string{0: ":2:10 ", 3: ":5:11 ", 4: ":5:20 ", 5: ":6:5 "}
	for i, msg := range want {
		if !strings.HasSuffix(got[i], msg) || !strings.Contains(got[i], positions[i]) || !strings.HasPrefix(got[i], "error V00") {
			t.Errorf("diagnostic %d = %s, want ...%s at%s", i+1, got[i], msg, positions[i])
		}
	}
	// With version="2", the wrap message lists wrap | nowrap | truncate.
	v2 := writeTemp(t, "v2.tui", "<tui version=\"2\"><screen id=\"main\"><text wrap=\"pre-wrap\">x</text></screen></tui>\n")
	got = vdiags(t, v2, "")
	if len(got) != 1 || !strings.HasSuffix(got[0], `wrap: value "pre-wrap" (want wrap | nowrap | truncate) (requires a <tui> without version)`) {
		t.Errorf("version 2: %v", got)
	}
	// In the current language span and pre-wrap are accepted in a
	// stylesheet and a document (a written <span> is still V001).
	cur := writeTemp(t, "cur.tui", "<tui><style>text > span.code { bold: true; } text { wrap: pre-wrap; }</style><screen id=\"main\"><text>x</text><span>y</span></screen></tui>\n")
	got = vdiags(t, cur, "")
	if len(got) != 1 || !strings.HasSuffix(got[0], `unknown tag <span>; spans are generated from <text runs="…">`) {
		t.Errorf("current language: %v", got)
	}
}
