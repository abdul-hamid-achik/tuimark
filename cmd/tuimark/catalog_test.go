package main

// SPEC v0.4a §21 test 119: `tuimark catalog` validates against
// schema/catalog.v1.json, equals what the runtime's tables give, and agrees
// with what AGENTS.md's Reference lists, both ways.

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

const catalogSchema = "../../schema/catalog.v1.json"

func catalogOutput(t *testing.T) (raw string, c catalog.Catalog) {
	t.Helper()
	code, out, errw := runCLI("catalog")
	if code != 0 || errw != "" {
		t.Fatalf("catalog: exit %d, stderr %q", code, errw)
	}
	if err := json.Unmarshal([]byte(out), &c); err != nil {
		t.Fatalf("catalog output is not JSON: %v", err)
	}
	return out, c
}

func TestCatalogValidatesAgainstSchema(t *testing.T) {
	raw, c := catalogOutput(t)
	if errs := validateSchema(t, catalogSchema, decodeJSON(t, []byte(raw))); len(errs) != 0 {
		t.Fatalf("catalog output violates schema/catalog.v1.json:\n%s", strings.Join(errs, "\n"))
	}
	if c.Catalog != 1 || c.Tuimark != version {
		t.Errorf("catalog = %d, tuimark = %q (want 1 and %q)", c.Catalog, c.Tuimark, version)
	}
	if !reflect.DeepEqual(c.Versions, []int{1, 2, 3}) {
		t.Errorf("versions = %v", c.Versions)
	}
	// SPEC v0.4b §15.10: the current language is numbered 4 and is not a
	// version value.
	if c.Current != 4 {
		t.Errorf("current = %d, want 4", c.Current)
	}
	// A schema cannot check that a since is one of versions or current
	// (SPEC v0.4b decision 93): do it here, on the decoded JSON, so an item
	// of any shape is covered.
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			if s, ok := x["since"]; ok {
				n := int(s.(float64))
				found := n == c.Current
				for _, v := range c.Versions {
					found = found || v == n
				}
				if !found {
					t.Errorf("%s: since %d is not one of versions %v or current %d", path, n, c.Versions, c.Current)
				}
			}
			for k, e := range x {
				walk(path+"/"+k, e)
			}
		case []any:
			for i, e := range x {
				walk(path+"/"+string(rune('0'+i%10)), e)
			}
		}
	}
	walk("", decodeJSON(t, []byte(raw)))
}

// The schema rejects what 15.10 forbids and allows what it promises.
func TestCatalogSchemaShape(t *testing.T) {
	raw, _ := catalogOutput(t)
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	m["extra_member"] = "allowed" // additionalProperties is true at every level
	m["tags"].([]any)[0].(map[string]any)["extra"] = 1
	if errs := validateSchema(t, catalogSchema, m); len(errs) != 0 {
		t.Errorf("an added member was rejected: %v", errs)
	}
	m["catalog"] = float64(2)
	if errs := validateSchema(t, catalogSchema, m); len(errs) == 0 {
		t.Error("catalog: 2 passed the v1 schema")
	}
	m["catalog"] = float64(1)
	m["tags"].([]any)[0].(map[string]any)["since"] = float64(0)
	if errs := validateSchema(t, catalogSchema, m); len(errs) == 0 {
		t.Error("since: 0 passed the schema")
	}
	delete(m, "keys")
	if errs := validateSchema(t, catalogSchema, m); len(errs) == 0 {
		t.Error("a catalog without keys passed the schema")
	}
}

// The command is a function of the build: same bytes twice, no flags, no
// arguments, and a keyword list is never null.
func TestCatalogCommand(t *testing.T) {
	a, _, _ := runCLI("catalog")
	b, out, _ := runCLI("catalog")
	_ = a
	if b != 0 {
		t.Fatalf("exit %d", b)
	}
	_, out2, _ := runCLI("catalog")
	if out != out2 {
		t.Error("catalog is not deterministic")
	}
	if strings.Contains(out, `"keywords": null`) {
		t.Error("a keywords member is null, not []")
	}
	if strings.Contains(out, `\u00`+`3c`) || !strings.Contains(out, "`<tui>`") {
		t.Error("the diagnostics' prose is HTML-escaped")
	}
	if !strings.HasSuffix(out, "}\n") {
		t.Error("the output does not end with a closing brace and newline")
	}
	for _, args := range [][]string{{"catalog", "--json"}, {"catalog", "extra"}} {
		code, out, _ := runCLI(args...)
		if code != 1 || out != "" {
			t.Errorf("%v: exit %d, stdout %q; want a usage error", args, code, out)
		}
	}
	t.Setenv("TUIMARK_THEME", "light")
	t.Setenv("TUIMARK_LOG", "/nonexistent/x")
	if _, out3, _ := runCLI("catalog"); out3 != out {
		t.Error("catalog reads the environment")
	}
}

// The catalog equals what the runtime's tables give, item for item.
func TestCatalogEqualsTheTables(t *testing.T) {
	_, c := catalogOutput(t)

	// Tags and their attributes, with the table each came from.
	wantTags := map[string]int{}
	for _, k := range ir.Kinds {
		wantTags[k] = 1
	}
	for _, k := range ir.KindsV2 {
		wantTags[k] = 2
	}
	for _, k := range ir.KindsCurrent {
		wantTags[k] = catalog.Current
	}
	gotTags := map[string]catalog.Tag{}
	var names []string
	for _, tg := range c.Tags {
		gotTags[tg.Name] = tg
		names = append(names, tg.Name)
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("tags are not alphabetical: %v", names)
	}
	if len(gotTags) != len(wantTags) {
		t.Errorf("%d tags, want %d", len(gotTags), len(wantTags))
	}
	for name, since := range wantTags {
		tg, ok := gotTags[name]
		if !ok || tg.Since != since {
			t.Errorf("tag %s: %+v, want since %d", name, tg, since)
			continue
		}
		want := map[string]int{}
		for a := range parse.TagAttrs[name] {
			want[a] = 1
		}
		for a := range parse.TagAttrsV2[name] {
			if _, ok := want[a]; !ok {
				want[a] = 2
			}
		}
		for a := range parse.TagAttrsV3[name] {
			if _, ok := want[a]; !ok {
				want[a] = 3
			}
		}
		for a := range parse.TagAttrsCurrent[name] {
			if _, ok := want[a]; !ok {
				want[a] = catalog.Current
			}
		}
		if parse.TakesClassGuard(name) {
			// version="2" vocabulary, as new as a current-language tag
			// (SPEC v0.4b §15.10).
			want[catalog.ClassGuardAttr] = max(2, since)
		}
		got := map[string]int{}
		var anames []string
		for _, a := range tg.Attrs {
			got[a.Name] = a.Since
			anames = append(anames, a.Name)
			if a.Keywords == nil {
				t.Errorf("%s.%s: keywords is null", name, a.Name)
			}
			if a.Name != catalog.ClassGuardAttr {
				var kw []string
				for _, k := range a.Keywords {
					kw = append(kw, k.Name)
				}
				var wk []string
				for _, k := range parse.AttrKeywords(name, a.Name) {
					wk = append(wk, k.Name)
				}
				if !reflect.DeepEqual(append([]string(nil), kw...), append([]string(nil), wk...)) && (len(kw) != 0 || len(wk) != 0) {
					t.Errorf("%s.%s keywords %v, want %v", name, a.Name, kw, wk)
				}
			}
		}
		if !sort.StringsAreSorted(anames) {
			t.Errorf("%s: attributes are not alphabetical: %v", name, anames)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s attrs = %v, want %v", name, got, want)
		}
	}

	// Properties.
	pnames := css.PropertyNames()
	sort.Strings(pnames)
	if len(c.Properties) != len(pnames) {
		t.Fatalf("%d properties, want %d", len(c.Properties), len(pnames))
	}
	for i, p := range c.Properties {
		if p.Name != pnames[i] || p.Since != css.PropertySince(p.Name) || p.Syntax != css.PropertyValues(p.Name) {
			t.Errorf("property %d = %+v, want %s/%d/%q", i, p, pnames[i], css.PropertySince(pnames[i]), css.PropertyValues(pnames[i]))
		}
		if len(p.Keywords) != len(css.PropertyKeywords(p.Name)) {
			t.Errorf("property %s keywords = %v", p.Name, p.Keywords)
		}
	}

	// Pseudo-classes and media features.
	if len(c.PseudoClasses) != len(css.PseudoClasses()) {
		t.Errorf("pseudo-classes = %v", c.PseudoClasses)
	}
	for i, k := range css.PseudoClasses() {
		if c.PseudoClasses[i].Name != k.Name || c.PseudoClasses[i].Since != k.Since {
			t.Errorf("pseudo-class %d = %+v, want %+v", i, c.PseudoClasses[i], k)
		}
	}
	if len(c.Media) != len(css.MediaFeatures()) {
		t.Errorf("media = %v", c.Media)
	}
	for i, f := range css.MediaFeatures() {
		if c.Media[i].Name != f.Name || c.Media[i].Since != f.Since || len(c.Media[i].Keywords) != len(f.Keywords) {
			t.Errorf("media %d = %+v, want %+v", i, c.Media[i], f)
		}
	}

	// Keys.
	if !reflect.DeepEqual(c.Keys.Named, ir.NamedKeysAll) || c.Keys.Ctrl != "a-z" || !c.Keys.Printable ||
		!reflect.DeepEqual(c.Keys.NeverMatch, []string{"ctrl+i", "ctrl+j", "ctrl+m"}) ||
		c.Keys.RejectedSince != 4 {
		t.Errorf("keys = %+v", c.Keys)
	}
	// SPEC v0.4d §15.10: named gains the tokens of the current language in
	// the order of §8.1, named_since maps each to 4, and alt is the
	// characters alt+ takes.
	if c.Keys.Alt != "a-z A-Z 0-9" || c.Keys.AltSince != 4 || len(c.Keys.NamedSince) != len(ir.NamedKeysCurrent) {
		t.Errorf("keys alt = %q since %d, named_since = %v", c.Keys.Alt, c.Keys.AltSince, c.Keys.NamedSince)
	}
	for _, k := range ir.NamedKeysAll {
		if !ir.ValidKeyLang(k, ir.LangCurrent) {
			t.Errorf("named key %q is not a valid key", k)
		}
		since, current := c.Keys.NamedSince[k]
		if current != !ir.ValidKey(k) || (current && since != 4) {
			t.Errorf("named key %q: named_since %d (present %v), valid in 0.4.4: %v", k, since, current, ir.ValidKey(k))
		}
	}
	for _, k := range ir.NamedKeys {
		if !ir.ValidKey(k) {
			t.Errorf("named key %q is not a valid key", k)
		}
	}

	// Actions.
	var anames []string
	for _, a := range c.Actions {
		anames = append(anames, a.Name)
		if !a.Builtin {
			t.Errorf("action %s is not builtin", a.Name)
		}
	}
	wantActions := append([]string{"focus", "quit"}, ir.BuiltinActionsV2...)
	if !reflect.DeepEqual(anames, wantActions) {
		t.Errorf("actions = %v, want %v", anames, wantActions)
	}

	// Diagnostics: all of them, in the order of the table.
	if len(c.Diagnostics) < 30 {
		t.Errorf("%d diagnostics", len(c.Diagnostics))
	}
	for i := 1; i < len(c.Diagnostics); i++ {
		if c.Diagnostics[i].Code == c.Diagnostics[i-1].Code {
			t.Errorf("duplicate code %s", c.Diagnostics[i].Code)
		}
	}
}

// needs_to is true exactly for the built-in action whose row is V003
// without a `to` (a test of the table the catalog cannot derive).
func TestCatalogNeedsToMatchesTheParser(t *testing.T) {
	_, c := catalogOutput(t)
	for _, a := range c.Actions {
		doc := `<tui version="3"><keymap><bind keys="x" action="` + a.Name + `"/></keymap><screen id="s"/></tui>`
		d := parse.Parse([]byte(doc), "t.tui")
		gotNeeds := false
		for _, dg := range d.Diags {
			if dg.Code == "V003" && strings.Contains(dg.Msg, "to") {
				gotNeeds = true
			}
		}
		if gotNeeds != a.NeedsTo {
			t.Errorf("action %s: needs_to = %v, but the parser says %v (%v)", a.Name, a.NeedsTo, gotNeeds, d.Diags)
		}
	}
}

// ── AGENTS.md ───────────────────────────────────────────────────────────

func readAgents(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// bullets parses "- `name`: rest" lines of a section into name -> rest, in
// order.
func bullets(lines []string) (names []string, rest map[string]string) {
	rest = map[string]string{}
	for _, l := range lines {
		if !strings.HasPrefix(l, "- `") {
			continue
		}
		end := strings.Index(l[3:], "`")
		if end < 0 || !strings.HasPrefix(l[3+end:], "`: ") {
			continue
		}
		name := l[3 : 3+end]
		names = append(names, name)
		rest[name] = l[3+end+3:]
	}
	return
}

// section returns the lines between the heading and the next heading.
func section(md, heading string) []string {
	lines := strings.Split(md, "\n")
	var out []string
	in := false
	for _, l := range lines {
		if strings.HasPrefix(l, "#") {
			if in {
				break
			}
			in = l == heading
			continue
		}
		if in {
			out = append(out, l)
		}
	}
	return out
}

func TestCatalogAgreesWithAGENTS(t *testing.T) {
	_, c := catalogOutput(t)
	if problems := compareToAgents(c, readAgents(t)); len(problems) != 0 {
		t.Errorf("the catalog and AGENTS.md disagree:\n%s", strings.Join(problems, "\n"))
	}
}

// compareToAgents checks the catalog against the Reference of AGENTS.md
// both ways (SPEC v0.4a §21 test 119): every tag, attribute, property, key,
// action, and code AGENTS.md lists is in the catalog with the version
// AGENTS.md gives it, and nothing else is. It returns the disagreements.
func compareToAgents(c catalog.Catalog, md string) (problems []string) {
	errorf := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	// "### Attributes per tag": the version="1" list, then what version="2"
	// and version="3" add.
	lines := section(md, "### Attributes per tag")
	var v1, v2, v3, v4 []string
	cur := &v1
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "Only in a `<tui version=\"2\">`"):
			cur = &v2
		case strings.HasPrefix(l, "Only in a `<tui version=\"3\">`"):
			cur = &v3
		case strings.HasPrefix(l, "Only in a `<tui>` without `version`"):
			// SPEC v0.4b §5.1, §15.10: the current language's additions.
			cur = &v4
		default:
			*cur = append(*cur, l)
		}
	}
	want := map[string]map[string]int{}
	tagSince := map[string]int{}
	add := func(list []string, since int) {
		names, rest := bullets(list)
		for _, tag := range names {
			if tag == "item" && strings.HasPrefix(rest[tag], "class only") {
				continue // "`item` inside a `table`": a context rule
			}
			if strings.HasPrefix(rest[tag], "`class:NAME`") {
				continue // the class:NAME bullet
			}
			if _, ok := tagSince[tag]; !ok {
				tagSince[tag] = since
			}
			if want[tag] == nil {
				want[tag] = map[string]int{}
			}
			if rest[tag] == "(no attributes)" {
				continue
			}
			for _, a := range strings.Split(rest[tag], ", ") {
				if _, ok := want[tag][a]; !ok {
					want[tag][a] = max(since, tagSince[tag])
				}
			}
		}
	}
	add(v1, 1)
	add(v2, 2)
	add(v3, 3)
	add(v4, c.Current)
	if len(want) == 0 {
		errorf("no attribute bullets found in AGENTS.md")
		return
	}
	got := map[string]map[string]int{}
	for _, tg := range c.Tags {
		if tagSince[tg.Name] != tg.Since {
			errorf("tag %s: catalog since %d, AGENTS.md says %d", tg.Name, tg.Since, tagSince[tg.Name])
		}
		got[tg.Name] = map[string]int{}
		for _, a := range tg.Attrs {
			if a.Name == catalog.ClassGuardAttr {
				// AGENTS.md's class:NAME bullet is version="2"; on a tag of
				// the current language it is as new as the tag.
				if want := max(2, tg.Since); a.Since != want {
					errorf("%s: class:NAME since %d, AGENTS.md says %d", tg.Name, a.Since, want)
				}
				continue
			}
			got[tg.Name][a.Name] = a.Since
		}
	}
	if !reflect.DeepEqual(got, want) {
		for tag := range want {
			if !reflect.DeepEqual(got[tag], want[tag]) {
				errorf("tag %s: catalog %v, AGENTS.md %v", tag, got[tag], want[tag])
			}
		}
		for tag := range got {
			if want[tag] == nil {
				errorf("tag %s is in the catalog but not in AGENTS.md", tag)
			}
		}
	}
	// class:NAME is on every tag but tui, style, keymap, bind, and the
	// composition tags, the slot included (SPEC v0.4b §6.15, v0.4c §6.18.10).
	for _, tg := range c.Tags {
		has := false
		for _, a := range tg.Attrs {
			has = has || a.Name == catalog.ClassGuardAttr
		}
		if skip := map[string]bool{"tui": true, "style": true, "keymap": true, "bind": true, "include": true, "fragment": true, "template": true, "use": true, "slot": true}[tg.Name]; has == skip {
			errorf("tag %s: class:NAME present = %v", tg.Name, has)
		}
	}

	// "### CSS properties".
	pnames, prest := bullets(section(md, "### CSS properties"))
	if len(pnames) != len(c.Properties) {
		errorf("AGENTS.md lists %d properties, the catalog %d", len(pnames), len(c.Properties))
	}
	for i, p := range c.Properties {
		if i >= len(pnames) || pnames[i] != p.Name {
			errorf("property %d: catalog %s, AGENTS.md %v", i, p.Name, pnames)
			break
		}
		rest := prest[p.Name]
		since := 1
		switch {
		case strings.HasSuffix(rest, ` (version="2" stylesheets only)`):
			since = 2
			rest = strings.TrimSuffix(rest, ` (version="2" stylesheets only)`)
		case strings.HasSuffix(rest, ` (version="3" stylesheets only)`):
			since = 3
			rest = strings.TrimSuffix(rest, ` (version="3" stylesheets only)`)
		}
		if p.Since != since || p.Syntax != rest {
			errorf("property %s: catalog %d %q, AGENTS.md %d %q", p.Name, p.Since, p.Syntax, since, rest)
		}
	}

	// "### Key tokens".
	var named string
	for _, l := range section(md, "### Key tokens") {
		if strings.HasPrefix(l, "Named: ") {
			named = strings.TrimPrefix(l, "Named: ")
		}
		if strings.HasPrefix(l, "Also valid:") {
			for _, k := range c.Keys.NeverMatch {
				if !strings.Contains(l, "`"+k+"`") {
					errorf("AGENTS.md does not name %s as a key that never matches", k)
				}
			}
			if !strings.Contains(l, "`ctrl+<a-z>`") {
				errorf("AGENTS.md does not list ctrl+<a-z>")
			}
			// SPEC v0.4b test 209: the paragraph says the three are V003 in
			// a <tui> without version and accepted in a pinned one.
			if c.Keys.RejectedSince != 0 && (!strings.Contains(l, "V003") || !strings.Contains(l, "`<tui>` without `version`") || !strings.Contains(l, "accepts them, never matching")) {
				errorf("AGENTS.md does not say the never-matching keys are V003 in a <tui> without version and accepted in a pinned one")
			}
		}
	}
	if named != strings.Join(c.Keys.Named, ", ") {
		errorf("named keys: catalog %v, AGENTS.md %q", c.Keys.Named, named)
	}

	// "### Diagnostic codes": the table, row for row.
	var rows [][3]string
	for _, l := range section(md, "### Diagnostic codes") {
		if !strings.HasPrefix(l, "| ") || strings.HasPrefix(l, "| Code") || strings.HasPrefix(l, "|---") {
			continue
		}
		parts := strings.SplitN(strings.TrimSuffix(strings.TrimPrefix(l, "| "), " |"), " | ", 3)
		rows = append(rows, [3]string{parts[0], parts[1], parts[2]})
	}
	if len(rows) != len(c.Diagnostics) {
		errorf("AGENTS.md lists %d codes, the catalog %d", len(rows), len(c.Diagnostics))
		return
	}
	for i, d := range c.Diagnostics {
		if rows[i] != [3]string{d.Code, d.Pass, d.When} {
			errorf("code %d: catalog %v, AGENTS.md %v", i, d, rows[i])
		}
	}

	// Built-in actions: "Built-in actions: focus, quit" and the hyphenated
	// ones the version="2" bullet lists.
	var builtins, hyphenated string
	for _, l := range strings.Split(md, "\n") {
		if strings.HasPrefix(l, "Built-in actions: ") {
			builtins = strings.TrimPrefix(l, "Built-in actions: ")
		}
		if i := strings.Index(l, "the hyphenated built-in actions ("); i >= 0 {
			rest := l[i+len("the hyphenated built-in actions ("):]
			hyphenated = rest[:strings.Index(rest, ")")]
		}
	}
	var gotBase, gotHyph []string
	for _, a := range c.Actions {
		if a.Since == 1 {
			gotBase = append(gotBase, a.Name)
		} else {
			gotHyph = append(gotHyph, a.Name)
		}
	}
	if builtins != strings.Join(gotBase, ", ") || hyphenated != strings.Join(gotHyph, ", ") {
		errorf("actions: catalog %v + %v, AGENTS.md %q + %q", gotBase, gotHyph, builtins, hyphenated)
	}
	return
}

// The comparison has teeth: a table change without its other side fails it.
func TestCatalogComparisonDetectsDrift(t *testing.T) {
	_, c := catalogOutput(t)
	md := readAgents(t)
	if p := compareToAgents(c, md); len(p) != 0 {
		t.Fatalf("baseline disagrees: %v", p)
	}
	mutations := []struct {
		name string
		do   func(c *catalog.Catalog, md string) (catalog.Catalog, string)
	}{
		{"a new attribute in the catalog only", func(c *catalog.Catalog, md string) (catalog.Catalog, string) {
			c.Tags[0].Attrs = append(c.Tags[0].Attrs, catalog.Attr{Name: "zzz", Since: 1, Keywords: []catalog.Item{}})
			return *c, md
		}},
		{"a property gate changed in AGENTS.md only", func(c *catalog.Catalog, md string) (catalog.Catalog, string) {
			return *c, strings.Replace(md, " (version=\"2\" stylesheets only)", "", 1)
		}},
		{"a property since changed in the catalog only", func(c *catalog.Catalog, md string) (catalog.Catalog, string) {
			c.Properties[0].Since = 3
			return *c, md
		}},
		{"a code row changed in AGENTS.md only", func(c *catalog.Catalog, md string) (catalog.Catalog, string) {
			return *c, strings.Replace(md, "| V006 | parse |", "| V006 | layout |", 1)
		}},
		{"a key dropped from the catalog", func(c *catalog.Catalog, md string) (catalog.Catalog, string) {
			c.Keys.Named = c.Keys.Named[1:]
			return *c, md
		}},
		{"an action dropped from the catalog", func(c *catalog.Catalog, md string) (catalog.Catalog, string) {
			c.Actions = c.Actions[:len(c.Actions)-1]
			return *c, md
		}},
		{"a tag dropped from the catalog", func(c *catalog.Catalog, md string) (catalog.Catalog, string) {
			c.Tags = c.Tags[1:]
			return *c, md
		}},
	}
	for _, m := range mutations {
		c2, md2 := m.do(&c, md)
		if p := compareToAgents(c2, md2); len(p) == 0 {
			t.Errorf("%s: the comparison found no difference", m.name)
		}
		_, c = catalogOutput(t) // each mutation starts from a fresh catalog
	}
}
