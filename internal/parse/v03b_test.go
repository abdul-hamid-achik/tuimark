package parse_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC v0.3b §5.1, §6.2, §6.10.1, §6.10.4, §8.1, §8.3, §11.4, §13.1, §14:
// several <keymap> elements and <keymap when>, stick="bottom" on scroll,
// {path} counters in a tab's label/short, and focus= on modal. §21 test
// 91 (version gate) and the static parts of 96, 97, 98, 99, and 100.

func coreV3(body string) string {
	return "<tui version=\"3\">\n<screen>\n" + body + "\n</screen>\n</tui>"
}

// 91. Version gate: stick, a modal's focus, and a keymap's when are valid
// in version="3" and V002 (with the version="3" hint) in version="1" and
// version="2" documents alike; a tab's {path} label/short is V003 (with
// the hint) in version="2" and only the tag's V001 (with the version="2"
// hint) in version="1", since the tag itself is unknown there.
func TestVersionGateV3Items(t *testing.T) {
	for _, c := range []struct {
		name, body string
	}{
		{"stick", `<scroll id="s" stick="bottom"><text>x</text></scroll>`},
		{"modal focus", `<modal id="m" focus="#y"><button id="y" label="y"/></modal>`},
		{"keymap when handled elsewhere", ""},
	} {
		if c.body == "" {
			continue
		}
		t.Run(c.name+" v3 ok", func(t *testing.T) {
			doc := parse.Parse([]byte(coreV3(c.body)), "")
			if got := coreCodes(doc); got != "" {
				t.Errorf("codes %q, want none: %v", got, doc.Diags)
			}
		})
		for _, v := range []string{"1", "2"} {
			wrap := func(b string) string {
				return "<tui version=\"" + v + "\">\n<screen>\n" + b + "\n</screen>\n</tui>"
			}
			t.Run(c.name+" v"+v, func(t *testing.T) {
				doc := parse.Parse([]byte(wrap(c.body)), "")
				var found bool
				for _, d := range doc.Diags {
					if d.Code == "V002" && strings.HasSuffix(d.Msg, ir.VersionHintV3) {
						found = true
					}
				}
				if !found {
					t.Errorf("no V002 with the version=\"3\" hint in version=%q: %v", v, doc.Diags)
				}
			})
		}
	}
	// A <keymap when="SEL"> is V002 with the version="3" hint in
	// version="1" and version="2" documents, valid in version="3".
	for _, v := range []string{"1", "2"} {
		src := "<tui version=\"" + v + "\"><keymap when=\"#x:focus\"><bind keys=\"a\" action=\"go\"/></keymap><screen id=\"x\"/></tui>"
		doc := parse.Parse([]byte(src), "")
		var found bool
		for _, d := range doc.Diags {
			if d.Code == "V002" && strings.HasSuffix(d.Msg, ir.VersionHintV3) {
				found = true
			}
		}
		if !found {
			t.Errorf("keymap when v%s: no V002 with the version=\"3\" hint: %v", v, doc.Diags)
		}
	}
	v3 := parse.Parse([]byte(`<tui version="3"><keymap when="#x:focus"><bind keys="a" action="go"/></keymap><screen id="x"/></tui>`), "")
	if got := coreCodes(v3); got != "" {
		t.Errorf("keymap when v3: codes %q, want none: %v", got, v3.Diags)
	}
	// stick on axis="x" is V003, needs a y scroll axis.
	if d := parse.Parse([]byte(coreV3(`<scroll id="s" axis="x" stick="bottom"><text>x</text></scroll>`)), ""); coreCodes(d) != "V003" {
		t.Errorf("stick on axis=x: %q", coreCodes(d))
	}
	// A tab's {path} label: V003 with the hint in version="2", only the
	// tag's V001 with the version="2" hint in version="1" (the tag is
	// unknown there, so its attributes are not checked further).
	tabBody := `<tabs id="n"><tab id="a" label="{x}"/></tabs>`
	v2doc := parse.Parse([]byte(coreV2(tabBody)), "")
	var gotHint bool
	for _, d := range v2doc.Diags {
		if d.Code == "V003" && strings.HasSuffix(d.Msg, ir.VersionHintV3) {
			gotHint = true
		}
	}
	if !gotHint {
		t.Errorf("tab {path} label in v2: no V003 with the version=\"3\" hint: %v", v2doc.Diags)
	}
	v1doc := parse.Parse([]byte(coreV1(tabBody)), "")
	for _, d := range v1doc.Diags {
		if d.Code == "V003" {
			t.Errorf("tab {path} label in v1: unexpected V003 (the tag is unknown there): %v", v1doc.Diags)
		}
		if d.Code == "V001" && !strings.HasSuffix(d.Msg, ir.VersionHint) {
			t.Errorf("tab {path} label in v1: V001 without the version=\"2\" hint: %s", d)
		}
	}
	if !strings.Contains(coreCodes(v1doc), "V001") {
		t.Errorf("tab {path} label in v1: codes %q, want V001: %v", coreCodes(v1doc), v1doc.Diags)
	}
	// A label whose short= holds {path} follows the same rule.
	if d := parse.Parse([]byte(coreV2(`<tabs id="n"><tab id="a" label="a" short="{x}"/></tabs>`)), ""); !strings.Contains(coreCodes(d), "V003") {
		t.Errorf("short {path} in v2: %q", coreCodes(d))
	}
}

// 96. Several <keymap> elements: their rows form one keymap in document
// order, in every version; the top-level IR keymap array lists them in
// that order too.
func TestSeveralKeymaps(t *testing.T) {
	for _, v := range []string{"1", "2", "3"} {
		src := `<tui version="` + v + `"><keymap><bind keys="a" action="one"/></keymap><keymap><bind keys="b" action="two"/></keymap><keymap><bind keys="c" action="three"/></keymap><screen id="s"/></tui>`
		doc := parse.Parse([]byte(src), "")
		if len(doc.Diags) != 0 {
			t.Fatalf("v%s: %v", v, doc.Diags)
		}
		var actions []string
		for _, k := range doc.Keymap {
			actions = append(actions, k.Action)
		}
		if got := strings.Join(actions, " "); got != "one two three" {
			t.Errorf("v%s: keymap order %q, want \"one two three\"", v, got)
		}
		b, err := json.Marshal(parse.BuildIR(doc, map[string]string{}))
		if err != nil {
			t.Fatal(err)
		}
		i := strings.Index(string(b), `"one"`)
		j := strings.Index(string(b), `"two"`)
		k := strings.Index(string(b), `"three"`)
		if !(i >= 0 && i < j && j < k) {
			t.Errorf("v%s: IR keymap order not preserved: %s", v, b)
		}
	}
}

// 97. Keymap when: a bad selector on <keymap when> is one V003 at the
// <keymap>, not one per row; a missing id is one B005 at the <keymap>; a
// row with its own when uses that instead; when="" on a row fires
// everywhere even under a <keymap when>; the top-level IR keymap array
// carries each row's effective when, the root tree keeps the keymap
// node's and each bind node's own when as written; B007 fires against
// the effective when.
func TestKeymapWhen(t *testing.T) {
	// A bad selector: one V003, at the <keymap>, not one per row.
	src := `<tui version="3"><keymap when="#nope#also-bad"><bind keys="a" action="one"/><bind keys="b" action="two"/><bind keys="c" action="three"/></keymap><screen id="s"/></tui>`
	if doc := parse.Parse([]byte(src), ""); coreCodes(doc) != "V003" {
		t.Errorf("bad keymap when: codes %q, want one V003: %v", coreCodes(doc), doc.Diags)
	}
	// A <keymap when="#nope"> holding three rows reports one B005, at the
	// <keymap>.
	src = `<tui version="3"><keymap when="#nope"><bind keys="a" action="one"/><bind keys="b" action="two"/><bind keys="c" action="three"/></keymap><screen id="s"/></tui>`
	doc := parse.Parse([]byte(src), "")
	if got := coreCodes(doc); got != "B005" {
		t.Errorf("missing id in keymap when: codes %q, want one B005: %v", got, doc.Diags)
	}
	// A row's own when replaces the keymap's; when="" on a row fires
	// everywhere.
	src = `<tui version="3"><keymap when="#a:focus"><bind keys="x" action="own" when="#b:focus"/><bind keys="y" action="inherited"/><bind keys="z" action="everywhere" when=""/></keymap><screen id="s"><box id="a"/><box id="b"/></screen></tui>`
	doc = parse.Parse([]byte(src), "")
	if len(doc.Diags) != 0 {
		t.Fatalf("%v", doc.Diags)
	}
	own, inherited, everywhere := doc.Keymap[0], doc.Keymap[1], doc.Keymap[2]
	if own.WhenSel == nil || len(own.WhenSel.Parts) != 1 || own.WhenSel.Parts[0].ID != "b" {
		t.Errorf("own when: %+v", own)
	}
	if inherited.WhenSel == nil || len(inherited.WhenSel.Parts) != 1 || inherited.WhenSel.Parts[0].ID != "a" {
		t.Errorf("inherited when: %+v", inherited)
	}
	if everywhere.WhenSel != nil {
		t.Errorf("when=\"\" on a row: WhenSel %+v, want nil (matches everywhere)", everywhere.WhenSel)
	}
	// IR: the top-level keymap array's when is each row's effective when;
	// the root tree's keymap node keeps its own when in attrs, and so
	// does each bind node.
	b, err := json.Marshal(parse.BuildIR(doc, map[string]string{}))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`{"keys":"x","action":"own","when":"#b:focus"}`,
		`{"keys":"y","action":"inherited","when":"#a:focus"}`,
		`{"keys":"z","action":"everywhere"}`,
		`"kind":"keymap","classes":[],"attrs":{"when":"#a:focus"}`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("IR lacks %s:\n%s", want, s)
		}
	}
	// The everywhere row's own bind node in the tree has when="" as
	// written, not the inherited selector.
	if !strings.Contains(s, `"kind":"bind","classes":[],"attrs":{"action":"everywhere","keys":"z","when":""}`) {
		t.Errorf("IR bind node for the when=\"\" row:\n%s", s)
	}
	// B007: a move-next row without to, under a keymap when naming a
	// button, is incompatible (against the effective when).
	src = `<tui version="3"><keymap when="#b:focus"><bind keys="n" action="move-next"/></keymap><screen id="s"><button id="b" label="b"/></screen></tui>`
	doc = parse.Parse([]byte(src), "")
	if got := coreCodes(doc); got != "B007" {
		t.Errorf("B007 via inherited when: codes %q: %v", got, doc.Diags)
	}
}

// 98. stick="bottom": needs an id (V012), a y scroll axis (V003 on
// axis="x", checked above), exactly "bottom" (V003 otherwise), and is
// V002 inside a list <item> and inside a container each template.
func TestStickStructure(t *testing.T) {
	for _, c := range []struct {
		name, body, code string
	}{
		{"needs id", `<scroll stick="bottom"><text>x</text></scroll>`, "V012"},
		{"bad value", `<scroll id="s" stick="top"><text>x</text></scroll>`, "V003"},
		{"in a list item", `<list id="l" each="xs as x" key="x"><item><scroll id="s" stick="bottom"><text>{x}</text></scroll></item></list>`, "V002"},
		{"in an each template", `<col each="xs as x"><scroll id="s" stick="bottom"><text>{x}</text></scroll></col>`, "V002"},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := parse.Parse([]byte(coreV3(c.body)), "")
			if got := coreCodes(doc); !strings.Contains(got, c.code) {
				t.Errorf("codes %q, want to contain %q: %v", got, c.code, doc.Diags)
			}
		})
	}
}

// 99. Label counters: {path} is accepted in a tab's label and short in
// version="3" (checked above, TestVersionGateV3Items); this covers the
// remaining static rule, that mark stays literal text in every version,
// {path} included.
func TestTabMarkStaysLiteral(t *testing.T) {
	doc := parse.Parse([]byte(coreV3(`<tabs id="n" mark="{x}"><tab id="a" label="a"/></tabs>`)), "")
	if got := coreCodes(doc); got != "V003" {
		t.Errorf("codes %q, want V003 (mark is always literal text): %v", got, doc.Diags)
	}
}

// 100. Modal focus=: B005 for a target that does not exist, one inside a
// list item or an each template, and one outside the modal.
func TestModalFocusStructure(t *testing.T) {
	for _, c := range []struct {
		name, body string
	}{
		{"missing id", `<modal id="m" focus="#nope"><button id="y" label="y"/></modal>`},
		{"outside the modal", `<modal id="m" focus="#o"><button id="y" label="y"/></modal><button id="o" label="o"/>`},
		{"in a list item", `<modal id="m" focus="#r"><list id="l" each="xs as x" key="x"><item><text id="r">{x}</text></item></list></modal>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := parse.Parse([]byte(coreV3(c.body)), "")
			if got := coreCodes(doc); !strings.Contains(got, "B005") {
				t.Errorf("codes %q, want to contain B005: %v", got, doc.Diags)
			}
		})
	}
	// A valid focus= inside the modal: no diagnostics.
	doc := parse.Parse([]byte(coreV3(`<modal id="m" focus="#y"><button id="y" label="y"/><button id="n" label="n"/></modal>`)), "")
	if got := coreCodes(doc); got != "" {
		t.Errorf("codes %q, want none: %v", got, doc.Diags)
	}
}
