package css

import (
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// CheckTokens statically reports V003 for every $token / var(--token)
// reference that is not defined by the theme (Themes[theme], falling back
// to "dark" the same way NewCascade does) or by a `:root { --token: ... }`
// block in any sheet, whatever its own @media. It checks every rule of
// every sheet, including a rule inside an @media block that does not match
// the current terminal size, plus inline: the style="" declarations of the
// document (parse.Document.InlineDecls), on every node, rendered or not.
//
// Cascade.Compute only resolves a color declaration's token once that
// declaration has matched a rendered node at the frame's size (SPEC
// §10.4), so a typo in a rule or a style="" that never renders at the
// sizes `validate` checks — a closed modal, a false if= branch, another
// screen, an @media block for a size not checked, a list-item template
// rendered only with --data — would pass silently. Because the theme is
// fixed for the whole document and every possible token definition is
// known once the sheets are parsed, that check does not need to wait for
// layout: it can run once, right after the sheets load. Each diagnostic
// has the file, position, and message the cascade gives the same
// declaration, so the two dedupe.
func CheckTokens(sheets []*Sheet, theme string, inline ...Decl) ir.Diags {
	known := KnownTokens(sheets, theme)
	var out ir.Diags
	seen := map[string]bool{}
	check := func(file string, d Decl) {
		name, ok := TokenRef(d.Value)
		if !ok || known[name] {
			return
		}
		key := fmt.Sprintf("%s:%d:%d:%s:$%s", file, d.Line, d.Col, d.Prop, name)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, ir.Diagnostic{
			Severity: ir.Error, Code: "V003", File: file, Line: d.Line, Col: d.Col,
			Msg: fmt.Sprintf("%s: unknown token $%s", d.Prop, name),
		})
	}
	for _, sh := range sheets {
		for _, r := range sh.Rules {
			if ruleIsRoot(r) {
				continue
			}
			for _, d := range r.Decls {
				check(sh.File, d)
			}
		}
	}
	for _, d := range inline {
		check(d.File, d)
	}
	return out.Sorted()
}

// KnownTokens is the set of token names a document can reference: the
// theme's (Themes[theme], "dark" when theme is empty or unknown) plus every
// `--token` defined in a :root rule of any sheet, whatever its @media.
func KnownTokens(sheets []*Sheet, theme string) map[string]bool {
	if theme == "" {
		theme = "dark"
	}
	base, ok := Themes[theme]
	if !ok {
		base = Themes["dark"]
	}
	known := make(map[string]bool, len(base))
	for k := range base {
		known[k] = true
	}
	for _, sh := range sheets {
		for _, r := range sh.Rules {
			if !ruleIsRoot(r) {
				continue
			}
			for _, d := range r.Decls {
				known[strings.TrimPrefix(d.Prop, "--")] = true
			}
		}
	}
	return known
}

func ruleIsRoot(r Rule) bool {
	for _, sel := range r.Selectors {
		if sel.Root {
			return true
		}
	}
	return false
}
