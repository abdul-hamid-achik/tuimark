package parse

import (
	"fmt"
	"strings"
	"testing"
)

// attrElement builds a one-element document of the given version in which
// attr="value" is on tag, or "" when the combination has no scaffold.
func attrElement(tag, attr, value string, version int) string {
	a := fmt.Sprintf(`%s=%q`, attr, value)
	switch {
	case tag == "tui":
		v := fmt.Sprintf("%d", version)
		attrs := `version="` + v + `"`
		if attr == "version" {
			attrs = a
		} else {
			attrs += " " + a
		}
		return `<tui ` + attrs + `><screen id="s"/></tui>`
	}
	body := ""
	switch tag {
	case "input":
		body = fmt.Sprintf(`<input id="i" %s/>`, a)
	case "modal":
		body = fmt.Sprintf(`<modal id="m" %s/>`, a)
	case "scroll":
		body = fmt.Sprintf(`<scroll id="sc" %s/>`, a)
	case "text":
		body = fmt.Sprintf(`<text %s>x</text>`, a)
	default:
		body = fmt.Sprintf(`<%s id="x" %s/>`, tag, a)
	}
	return fmt.Sprintf(`<tui version="%d"><screen id="s">%s</screen></tui>`, version, body)
}

// TestAttrKeywordsMatchTheParser checks the keyword table of `tuimark
// catalog` against the parser: every listed keyword is accepted from its
// version on and not before (SPEC v0.4a §15.10, test 119), and a value
// outside the list is not.
func TestAttrKeywordsMatchTheParser(t *testing.T) {
	cases := []struct{ tag, attr string }{
		{"box", "border"}, {"text", "wrap"}, {"box", "hidden"}, {"box", "disabled"},
		{"box", "focusable"}, {"input", "secret"}, {"modal", "open"}, {"tui", "mouse"},
		{"scroll", "axis"}, {"rule", "axis"}, {"tui", "theme"}, {"tui", "version"},
		{"hints", "scope"}, {"scroll", "stick"},
	}
	for _, c := range cases {
		kws := AttrKeywords(c.tag, c.attr)
		if len(kws) == 0 {
			t.Errorf("%s.%s has no keywords", c.tag, c.attr)
			continue
		}
		attrSince := attributeSince(c.tag, c.attr)
		for _, k := range kws {
			for v := 1; v <= 3; v++ {
				if c.attr == "version" {
					continue // the value is the version itself
				}
				doc := Parse([]byte(attrElement(c.tag, c.attr, k.Name, v)), "t.tui")
				errs := doc.Diags.HasErrors()
				want := v >= k.Since && v >= attrSince
				if want && errs {
					t.Errorf("%s.%s=%q at version %d: %v", c.tag, c.attr, k.Name, v, doc.Diags)
				}
				if !want && !errs {
					t.Errorf("%s.%s=%q accepted at version %d, before its since", c.tag, c.attr, k.Name, v)
				}
			}
		}
		if c.attr == "version" {
			for _, k := range kws {
				doc := Parse([]byte(attrElement("tui", "version", k.Name, 1)), "t.tui")
				if doc.Diags.HasErrors() {
					t.Errorf("version=%q: %v", k.Name, doc.Diags)
				}
			}
		}
		bad := Parse([]byte(attrElement(c.tag, c.attr, "no-such-value", 3)), "t.tui")
		if !bad.Diags.HasErrors() {
			t.Errorf("%s.%s=%q accepted", c.tag, c.attr, "no-such-value")
		}
	}
	if AttrKeywords("box", "id") != nil {
		t.Error("id has keywords")
	}
	if got := AttrKeywords("rule", "axis"); len(got) != 2 {
		t.Errorf("rule.axis = %v, want x and y only", got)
	}
}

// attributeSince is the lowest version whose tables list the attribute on
// the tag.
func attributeSince(tag, attr string) int {
	switch {
	case TagAttrs[tag][attr]:
		return 1
	case TagAttrsV2[tag][attr]:
		return 2
	case TagAttrsV3[tag][attr]:
		return 3
	case TagAttrsCurrent[tag][attr]:
		return 4
	}
	return 99
}

func TestAttrKeywordTablesNameRealAttributes(t *testing.T) {
	for key := range attrKeywords {
		tag, attr, split := strings.Cut(key, ".")
		if !split {
			tag, attr = "", key
		}
		found := false
		for _, tg := range sortedTags() {
			if (tag == "" || tag == tg) && attributeSince(tg, attr) < 99 {
				found = true
			}
		}
		if !found {
			t.Errorf("keyword table key %q names no attribute of a tag", key)
		}
	}
}

func sortedTags() []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range []map[string]map[string]bool{TagAttrs, TagAttrsV2, TagAttrsV3} {
		for tag := range m {
			if !seen[tag] {
				seen[tag] = true
				out = append(out, tag)
			}
		}
	}
	return out
}
