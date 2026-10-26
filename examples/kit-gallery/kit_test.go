package main

import (
	"bytes"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// The tests in this file hold the kit's conventions (kit/README.md) to the
// files themselves, so a copied or renamed piece cannot drift from them.
//
// A document is checked as the runtime reads it: every <include> is
// replaced by its fragment's content (expand), so the kit's templates,
// keymaps and stylesheet links count wherever a document includes kit.tui.

// modifiers are the short classes a component class takes as a second class
// on the same node. They are only ever written compound with a k- class, or
// as the variant of a <use>, which lands on its copy's k- root.
var modifiers = map[string]bool{
	"primary": true, "secondary": true, "danger": true, "ghost": true,
	"ok": true, "warn": true, "info": true, "accent": true,
	"good": true, "bad": true, "invalid": true, "current": true, "muted": true,
	// The kinds of a transcript block (k-block).
	"code": true, "head": true, "quote": true, "item": true,
}

// generated are the class names the runtime gives to nodes it generates; the
// kit styles them only under a component class.
var generated = map[string]bool{"tab-label": true, "hint-key": true, "hint-label": true}

// kitFragment is the kit itself, which every kit document includes.
const kitFragment = "../../kit/kit.tui"

// docs are every document the kit ships: the components and the gallery.
func docs(t *testing.T) []string {
	t.Helper()
	return append(componentFiles(t), "app.tui")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func sheets() []string {
	return []string{"../../kit/theme.tcss", "../../kit/kit.tcss", "app.tcss"}
}

func stripComments(s string) string {
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(s, "")
}

func stripXMLComments(s string) string {
	return regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(s, "")
}

// ---- includes ----

var (
	includeTag  = regexp.MustCompile(`<include\s+src="([^"]+)"\s*/>`)
	fragmentRE  = regexp.MustCompile(`(?s)^\s*<fragment>(.*)</fragment>\s*$`)
	templateRE  = regexp.MustCompile(`(?s)<template\s+id="([^"]+)"(?:\s+props="[^"]*")?\s*>(.*?)</template>`)
	useTag      = regexp.MustCompile(`<use\b[^>]*>`)
	anyTag      = regexp.MustCompile(`<([a-z]+)\b[^>]*>`)
	templateRef = regexp.MustCompile(`\btemplate="([^"]+)"`)
)

// fragmentBody is the content of a fragment file: what an <include> of it
// inserts, comments dropped.
func fragmentBody(t *testing.T, path string) string {
	t.Helper()
	m := fragmentRE.FindStringSubmatch(stripXMLComments(readFile(t, path)))
	if m == nil {
		t.Fatalf("%s: the root is not <fragment>", path)
	}
	return m[1]
}

// expand is path's markup, comments dropped, with every <include> replaced
// by its fragment's content, recursively, resolved against the file that
// holds it.
func expand(t *testing.T, path string) string {
	t.Helper()
	return expandSrc(t, path, stripXMLComments(readFile(t, path)), map[string]bool{})
}

func expandSrc(t *testing.T, path, src string, chain map[string]bool) string {
	t.Helper()
	abs, _ := filepath.Abs(path)
	if chain[abs] {
		t.Fatalf("%s: include cycle", path)
	}
	chain[abs] = true
	defer delete(chain, abs)
	return includeTag.ReplaceAllStringFunc(src, func(tag string) string {
		frag := filepath.Join(filepath.Dir(path), includeTag.FindStringSubmatch(tag)[1])
		return expandSrc(t, frag, fragmentBody(t, frag), chain)
	})
}

// sourcesOf is path and every fragment it includes, recursively, once each.
func sourcesOf(t *testing.T, path string) []string {
	t.Helper()
	seen := map[string]bool{}
	var out []string
	var walk func(string)
	walk = func(p string) {
		p = filepath.Clean(p)
		if seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
		for _, m := range includeTag.FindAllStringSubmatch(stripXMLComments(readFile(t, p)), -1) {
			walk(filepath.Join(filepath.Dir(p), m[1]))
		}
	}
	walk(path)
	return out
}

// allSources are every file the kit's documents are made of: each document
// and each fragment they include, once each.
func allSources(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	var out []string
	for _, d := range docs(t) {
		for _, p := range sourcesOf(t, d) {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// templates are kit.tui's templates: id to body.
func templates(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range templateRE.FindAllStringSubmatch(fragmentBody(t, kitFragment), -1) {
		out[m[1]] = m[2]
	}
	if len(out) == 0 {
		t.Fatal("kit.tui has no <template>")
	}
	return out
}

// rootTag is the first element of a template's body: the root of its copy.
func rootTag(body string) string {
	return anyTag.FindString(body)
}

// templateAt is the id of the <template> the offset i of src falls in, or "".
func templateAt(src string, i int) string {
	for _, m := range templateRE.FindAllStringSubmatchIndex(src, -1) {
		if m[0] <= i && i < m[1] {
			return src[m[2]:m[3]]
		}
	}
	return ""
}

// inTemplate reports whether the offset i of src falls inside a <template>.
func inTemplate(src string, i int) bool { return templateAt(src, i) != "" }

func classesOf(tag string) []string {
	if m := regexp.MustCompile(`\bclass="([^"]*)"`).FindStringSubmatch(tag); m != nil {
		return strings.Fields(m[1])
	}
	return nil
}

// ---- classes ----

// Every class a document writes is a k- class or a modifier; a modifier is
// written only next to a k- class on the same node, or as a <use>'s
// variant, which lands on a template root that has a k- class.
func TestClassNamesFollowTheConvention(t *testing.T) {
	tpl := templates(t)
	for _, path := range docs(t) {
		src := expand(t, path)
		for _, tag := range anyTag.FindAllString(src, -1) {
			names := classesOf(tag)
			if strings.HasPrefix(tag, "<use") {
				for _, c := range names {
					if !modifiers[c] {
						t.Errorf("%s: a <use> takes a variant (a modifier), not %q: %s", path, c, tag)
					}
				}
				if len(names) == 0 {
					continue
				}
				ref := templateRef.FindStringSubmatch(tag)
				if ref == nil || tpl[ref[1]] == "" {
					t.Errorf("%s: a <use> with a variant names no kit template: %s", path, tag)
					continue
				}
				if !hasK(classesOf(rootTag(tpl[ref[1]]))) {
					t.Errorf("%s: the variant of %s lands on a root with no k- class", path, tag)
				}
				continue
			}
			hasKClass := hasK(names)
			for _, c := range names {
				switch {
				case strings.HasPrefix(c, "k-"):
				case modifiers[c]:
					if !hasKClass {
						t.Errorf("%s: class %q stands alone in %s; a modifier goes with a k- class", path, c, tag)
					}
				default:
					t.Errorf("%s: class %q is neither a k- class nor a modifier", path, c)
				}
			}
		}
		// A class:NAME guard adds a modifier; the node must carry a k- class.
		for _, tag := range regexp.MustCompile(`<[a-z]+\b[^>]*\bclass:[^>]*>`).FindAllString(src, -1) {
			if !regexp.MustCompile(`\bclass="[^"]*\bk-`).MatchString(tag) {
				t.Errorf("%s: a class:NAME guard on a node with no k- class: %s", path, tag)
			}
			for _, g := range classGuard.FindAllStringSubmatch(tag, -1) {
				if !modifiers[g[1]] {
					t.Errorf("%s: guard class:%s is not a modifier", path, g[1])
				}
			}
		}
	}
}

func hasK(names []string) bool {
	for _, c := range names {
		if strings.HasPrefix(c, "k-") {
			return true
		}
	}
	return false
}

// No rule targets a bare modifier or a bare utility: in every selector, each
// compound that names a short class also names a k- class (the runtime's own
// generated classes are allowed after a `>` under a component).
func TestNoRuleTargetsABareModifier(t *testing.T) {
	for _, p := range sheets() {
		src := stripComments(readFile(t, p))
		for _, sel := range regexp.MustCompile(`(?s)([^{}]+)\{`).FindAllStringSubmatch(src, -1) {
			text := strings.TrimSpace(sel[1])
			if strings.HasPrefix(text, "@media") {
				continue
			}
			for _, one := range strings.Split(text, ",") {
				for _, compound := range regexp.MustCompile(`\s*>\s*|\s+`).Split(strings.TrimSpace(one), -1) {
					classes := sheetClass.FindAllStringSubmatch(compound, -1)
					short, hasK := []string{}, false
					for _, m := range classes {
						switch {
						case strings.HasPrefix(m[1], "k-"):
							hasK = true
						case generated[m[1]]:
						default:
							short = append(short, m[1])
						}
					}
					if len(short) > 0 && !hasK {
						t.Errorf("%s: selector %q targets %v with no k- class on the same node", p, strings.TrimSpace(one), short)
					}
				}
			}
		}
	}
	// The runtime's generated classes are styled only under a component.
	for _, p := range sheets() {
		src := stripComments(readFile(t, p))
		for _, sel := range regexp.MustCompile(`(?s)([^{}]+)\{`).FindAllStringSubmatch(src, -1) {
			for _, one := range strings.Split(sel[1], ",") {
				one = strings.TrimSpace(one)
				for g := range generated {
					if strings.Contains(one, "."+g) && !strings.Contains(one, ">") {
						t.Errorf("%s: selector %q styles the generated class %q outside a component", p, one, g)
					}
				}
			}
		}
	}
}

// Every k- class the kit sheet defines is used by a document or by a
// template of the kit, so a rename cannot leave a dead rule behind.
func TestEveryKitClassIsUsed(t *testing.T) {
	defined := classesStyled(t, "../../kit/theme.tcss", "../../kit/kit.tcss")
	used := map[string]bool{}
	for _, path := range docs(t) {
		for c := range classesIn(expand(t, path)) {
			used[c] = true
		}
	}
	// Utilities and layout helpers that a document may or may not need.
	optional := map[string]bool{
		"k-strong": true, "k-em": true, "k-kbd": true, "k-code": true, "k-section": true,
		"k-text-accent": true, "k-text-ok": true, "k-text-warn": true,
		"k-text-danger": true, "k-text-info": true, "k-subtitle": true,
	}
	var dead []string
	for c := range defined {
		if (strings.HasPrefix(c, "k-") || modifiers[c]) && !used[c] && !optional[c] {
			dead = append(dead, c)
		}
	}
	sort.Strings(dead)
	if len(dead) > 0 {
		t.Errorf("kit classes with a rule but no use: %v", dead)
	}
}

// ---- tokens ----

var tokenDef = regexp.MustCompile(`--([a-z][a-z0-9-]*)\s*:`)
var tokenUse = regexp.MustCompile(`\$([a-z][a-z0-9-]*)`)

func TestTokens(t *testing.T) {
	theme := stripComments(readFile(t, "../../kit/theme.tcss"))
	i := strings.Index(theme, "@media (theme: light)")
	if i < 0 {
		t.Fatal("theme.tcss has no light palette")
	}
	dark, light := map[string]bool{}, map[string]bool{}
	for _, m := range tokenDef.FindAllStringSubmatch(theme[:i], -1) {
		dark[m[1]] = true
	}
	for _, m := range tokenDef.FindAllStringSubmatch(theme[i:], -1) {
		light[m[1]] = true
	}
	// The runtime's required tokens, which the kit keeps under the same names.
	for _, r := range []string{"bg", "fg", "muted", "accent", "ok", "warn", "danger", "border", "focus", "panel"} {
		if !dark[r] || !light[r] {
			t.Errorf("required runtime token %q is not defined in both palettes", r)
		}
	}
	for tok := range dark {
		if !light[tok] {
			t.Errorf("token %q has no light value", tok)
		}
	}
	for tok := range light {
		if !dark[tok] {
			t.Errorf("token %q has no dark value", tok)
		}
	}
	used := map[string]bool{}
	for _, p := range sheets() {
		for _, m := range tokenUse.FindAllStringSubmatch(stripComments(readFile(t, p)), -1) {
			used[m[1]] = true
			if !dark[m[1]] {
				t.Errorf("%s: $%s is not a defined token", p, m[1])
			}
		}
	}
	for tok := range dark {
		if !used[tok] {
			t.Errorf("token %q is defined but nothing uses it", tok)
		}
	}
	// Nothing outside theme.tcss holds a literal color.
	lit := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	for _, p := range sheets()[1:] {
		if m := lit.FindString(stripComments(readFile(t, p))); m != "" {
			t.Errorf("%s holds a literal color %s; use a token", p, m)
		}
	}
}

// colorDef matches a color token's declaration in theme.tcss.
var colorDef = regexp.MustCompile(`--([a-z][a-z0-9-]*)\s*:\s*(#[0-9a-fA-F]{6})\b`)

// TestPaletteContrast holds both palettes to WCAG AA: every text token reads
// at 4.5:1 on each surface, every on-* token at 4.5:1 on the fill it sits on,
// and fg, muted and subtle stay a visible step apart on bg (1.1:1).
func TestPaletteContrast(t *testing.T) {
	theme := stripComments(readFile(t, "../../kit/theme.tcss"))
	i := strings.Index(theme, "@media (theme: light)")
	if i < 0 {
		t.Fatal("theme.tcss has no light palette")
	}
	text := []string{"fg", "muted", "subtle", "accent", "ok", "warn", "danger", "info", "code"}
	surfaces := []string{"bg", "panel", "raised", "sel"}
	fills := [][2]string{
		{"bg", "fg"}, {"on-accent", "accent"}, {"on-accent", "info"}, {"on-accent", "focus"},
		{"on-ok", "ok"}, {"on-warn", "warn"}, {"on-danger", "danger"},
	}
	for _, p := range []struct{ name, src string }{{"dark", theme[:i]}, {"light", theme[i:]}} {
		c := map[string]string{}
		for _, m := range colorDef.FindAllStringSubmatch(p.src, -1) {
			c[m[1]] = strings.ToLower(m[2])
		}
		check := func(a, b string, min float64) {
			if c[a] == "" || c[b] == "" {
				t.Errorf("%s palette: --%s or --%s is not defined", p.name, a, b)
				return
			}
			if r := contrastRatio(c[a], c[b]); r < min {
				t.Errorf("%s palette: --%s on --%s is %.2f:1, under %.1f:1", p.name, a, b, r, min)
			}
		}
		for _, tok := range text {
			for _, s := range surfaces {
				check(tok, s, 4.5)
			}
		}
		for _, f := range fills {
			check(f[0], f[1], 4.5)
		}
		if c["bg"] != "" && c["muted"] != "" && c["subtle"] != "" {
			fgOverMuted := contrastRatio(c["fg"], c["bg"]) / contrastRatio(c["muted"], c["bg"])
			mutedOverSubtle := contrastRatio(c["muted"], c["bg"]) / contrastRatio(c["subtle"], c["bg"])
			if fgOverMuted < 1.1 || mutedOverSubtle < 1.1 {
				t.Errorf("%s palette: fg/muted is %.2f and muted/subtle %.2f on bg; each needs 1.1", p.name, fgOverMuted, mutedOverSubtle)
			}
		}
	}
}

// relLuminance is the WCAG relative luminance of a #rrggbb color.
func relLuminance(hex string) float64 {
	var lin [3]float64
	for k := range lin {
		v, _ := strconv.ParseInt(hex[1+2*k:3+2*k], 16, 0)
		c := float64(v) / 255
		if c <= 0.03928 {
			lin[k] = c / 12.92
		} else {
			lin[k] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*lin[0] + 0.7152*lin[1] + 0.0722*lin[2]
}

// contrastRatio is the WCAG contrast ratio of two #rrggbb colors.
func contrastRatio(a, b string) float64 {
	la, lb := relLuminance(a), relLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// ---- ids and actions ----

var (
	idAttr     = regexp.MustCompile(`\bid="([^"]*)"`)
	actionAttr = regexp.MustCompile(`\b(?:on:[a-z]+|action)="([^"]*)"`)
	idPattern  = regexp.MustCompile(`^[a-z][a-z0-9-]*(__[a-z][a-z0-9-]*)?$`)
	partID     = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	actionName = regexp.MustCompile(`^[a-z]+_[a-z]+$`)
)

// builtins are the runtime's own actions; they keep their names.
var builtins = map[string]bool{
	"quit": true, "focus": true, "move-next": true, "move-prev": true,
	"move-first": true, "move-last": true, "move-page-down": true,
	"move-page-up": true, "check-toggle": true, "check-all": true,
	"check-none": true, "switch-to": true,
}

// A written id is `instance` or `instance__part`, and an id in a template is
// a bare part: its <use> gives the instance (U__part). An action is
// `component_verb`, and never a generic word.
func TestIDsAndActionsFollowTheConvention(t *testing.T) {
	generic := map[string]bool{
		"cancel": true, "open": true, "save": true, "dismiss": true, "delete": true,
		"skip": true, "search": true, "confirm": true, "close": true, "submit": true,
	}
	for _, path := range allSources(t) {
		src := stripXMLComments(readFile(t, path))
		for _, m := range idAttr.FindAllStringSubmatchIndex(src, -1) {
			id := src[m[2]:m[3]]
			tag := anyTag.FindString(src[strings.LastIndex(src[:m[0]], "<"):])
			switch {
			case strings.HasPrefix(tag, "<template"):
				if !strings.HasPrefix(id, "k-") || !partID.MatchString(id) {
					t.Errorf("%s: template id %q is not k-NAME", path, id)
				}
			case inTemplate(src, m[0]):
				if !partID.MatchString(id) {
					t.Errorf("%s: id %q in a template is not a bare part; its <use> gives the instance", path, id)
				}
			case !idPattern.MatchString(id):
				t.Errorf("%s: id %q is not `instance` or `instance__part`", path, id)
			}
		}
		for _, m := range actionAttr.FindAllStringSubmatch(src, -1) {
			for _, a := range strings.Split(m[1], ",") {
				a = strings.TrimSpace(a)
				if a == "" || builtins[a] || strings.Contains(a, "#") || strings.HasPrefix(a, ".") {
					continue
				}
				if generic[a] {
					t.Errorf("%s: action %q is a generic name", path, a)
				}
				if !actionName.MatchString(a) {
					t.Errorf("%s: action %q is not `component_verb`", path, a)
				}
			}
		}
	}
}

// Every overlay (a modal) has parts named `instance__part` (a template's
// modal writes the part, and its use the instance), closes on esc through
// on:escape, writes its size in its class, and declares its keys in a
// keymap scoped to its own class. A template's overlay is checked in each
// document that uses the template: its keymap is the kit's when its rows
// name no id, and the document's otherwise (a row's `to` names the copy).
func TestOverlaysAreSelfContained(t *testing.T) {
	modal := regexp.MustCompile(`<modal\b[^>]*>`)
	for _, path := range docs(t) {
		src := expand(t, path)
		for _, loc := range modal.FindAllStringIndex(src, -1) {
			tag := src[loc[0]:loc[1]]
			tpl := templateAt(src, loc[0])
			if tpl != "" && !regexp.MustCompile(`<use\b[^>]*\btemplate="`+tpl+`"`).MatchString(src) {
				continue
			}
			id := idAttr.FindStringSubmatch(tag)
			switch {
			case id == nil:
				t.Errorf("%s: modal %s has no id", path, tag)
			case tpl != "":
				if !partID.MatchString(id[1]) {
					t.Errorf("%s: modal %s in a template should write only its part", path, tag)
				}
			case !strings.Contains(id[1], "__"):
				t.Errorf("%s: modal %s has no instance__part id", path, tag)
			}
			if !strings.Contains(tag, `on:escape="`) {
				t.Errorf("%s: modal has no on:escape: %s", path, tag)
			}
			cls := classesOf(tag)
			if len(cls) == 0 {
				t.Errorf("%s: modal has no class: %s", path, tag)
				continue
			}
			root := cls[0]
			if !strings.Contains(src, `<keymap when=".`+root+`">`) {
				t.Errorf("%s: no <keymap when=\".%s\"> for the overlay", path, root)
			}
			// The class writes the overlay's size.
			if !ruleHas(t, root, "width", "height") {
				t.Errorf("%s: .%s does not write a width and a height", path, root)
			}
		}
	}
}

// ruleHas reports whether the first rule of kit.tcss for exactly `.class`
// declares every property.
func ruleHas(t *testing.T, class string, props ...string) bool {
	t.Helper()
	src := stripComments(readFile(t, "../../kit/kit.tcss"))
	m := regexp.MustCompile(`(?m)^\.` + regexp.QuoteMeta(class) + `\s*\{([^}]*)\}`).FindStringSubmatch(src)
	if m == nil {
		return false
	}
	for _, p := range props {
		if !regexp.MustCompile(`(^|[;\s])` + p + `\s*:`).MatchString(m[1]) {
			return false
		}
	}
	return true
}

// sized reports whether an element writes its width and height: both as
// attributes, or both in the first rule of one of its k- classes.
func sized(t *testing.T, tag string) bool {
	t.Helper()
	if strings.Contains(tag, `width="`) && strings.Contains(tag, `height="`) {
		return true
	}
	for _, c := range classesOf(tag) {
		if strings.HasPrefix(c, "k-") && ruleHas(t, c, "width", "height") {
			return true
		}
	}
	return false
}

// ---- surviving the next language cut ----

// No document writes a version; a tabs says whether it takes focus; a
// container that fills says so (by attribute or by its kit class), in a
// document and in the templates it includes.
func TestDocumentsSurviveTheNextCut(t *testing.T) {
	container := regexp.MustCompile(`<(row|col|box|scroll)\b[^>]*>`)
	for _, path := range docs(t) {
		body := expand(t, path)
		if strings.Contains(readFile(t, path), "version=") || strings.Contains(body, "version=") {
			t.Errorf("%s writes a version; the current language needs none", path)
		}
		for _, tag := range regexp.MustCompile(`<tabs\b[^>]*>`).FindAllString(body, -1) {
			if !strings.Contains(tag, "focusable=") {
				t.Errorf("%s: tabs does not write focusable: %s", path, tag)
			}
		}
		for _, tag := range container.FindAllString(body, -1) {
			if !sized(t, tag) {
				t.Errorf("%s: container leaves its size to the default: %s", path, tag)
			}
		}
	}
}

// ---- the kit fragment and its templates ----

// Every template is named k-NAME, its root writes its own size (so a copy is
// the same size in a screen, a tab, or a col), it is shown in use by
// kit/components/NAME.tui, and the gallery uses it.
func TestKitTemplates(t *testing.T) {
	gallery := expand(t, "app.tui")
	for id, body := range templates(t) {
		root := rootTag(body)
		if root == "" {
			t.Errorf("template %s is empty", id)
			continue
		}
		if !sized(t, root) {
			t.Errorf("template %s: its root %s does not write a width and a height", id, root)
		}
		example := "../../kit/components/" + strings.TrimPrefix(id, "k-") + ".tui"
		if _, err := os.Stat(example); err != nil {
			t.Errorf("template %s has no example %s", id, example)
		} else if !strings.Contains(expand(t, example), `template="`+id+`"`) {
			t.Errorf("%s does not use the template %s", example, id)
		}
		if !strings.Contains(gallery, `template="`+id+`"`) {
			t.Errorf("the gallery does not use the template %s", id)
		}
	}
	// The kit's keymaps land in every document that includes it, so they
	// name classes only: a row that named a copy's id would be B005 in a
	// document without that copy.
	for _, km := range regexp.MustCompile(`(?s)<keymap\b[^>]*>.*?</keymap>`).FindAllString(fragmentBody(t, kitFragment), -1) {
		if strings.Contains(km, "#") {
			t.Errorf("kit.tui: a keymap names an id: %s", km)
		}
	}
}

// Kit v2: every template takes the prop `style` and writes it on its root, a
// copy having no id of its own to size it by; a prop it declares is read in
// its body (an unread prop is a typo that validates clean); and a template has
// at most one <slot/>.
func TestTemplatesTakeProps(t *testing.T) {
	declared := regexp.MustCompile(`(?s)<template\s+id="([^"]+)"(?:\s+props="([^"]*)")?\s*>(.*?)</template>`)
	n := 0
	for _, m := range declared.FindAllStringSubmatch(fragmentBody(t, kitFragment), -1) {
		id, props, body := m[1], strings.Fields(m[2]), m[3]
		n++
		if !slices.Contains(props, "style") {
			t.Errorf("template %s: no `style` prop", id)
		}
		if root := rootTag(body); !strings.Contains(root, `style="{$style}"`) {
			t.Errorf("template %s: its root does not write style=\"{$style}\": %s", id, root)
		}
		for _, p := range props {
			if !strings.Contains(body, "{$"+p+"}") {
				t.Errorf("template %s: the prop %q is declared and never read", id, p)
			}
		}
		if got := len(regexp.MustCompile(`<slot\s*/>`).FindAllString(body, -1)); got > 1 {
			t.Errorf("template %s: %d slots, a template has one", id, got)
		}
	}
	if n < 14 {
		t.Errorf("kit.tui has %d templates, want at least 14", n)
	}
}

// The kit's overlay keys have no label (nor keycap): an overlay names its
// own way out. The confirm dialog's buttons name its choices, and y and n
// work while it is open; the help shows "? close help" at its top, as
// markup. So a <hints> never lists them, the help's own list included, and
// a document that includes the kit without using an overlay shows none of
// them.
func TestOverlayKeysAreNotHints(t *testing.T) {
	for _, class := range []string{"k-dialog", "k-help"} {
		kms := regexp.MustCompile(`(?s)<keymap\b[^>]*when="\.`+class+`"[^>]*>.*?</keymap>`).FindAllString(fragmentBody(t, kitFragment), -1)
		if len(kms) != 1 {
			t.Fatalf("kit.tui: want one <keymap when=\".%s\">, got %d", class, len(kms))
		}
		if strings.Contains(kms[0], "label=") || strings.Contains(kms[0], "keycap=") {
			t.Errorf("kit.tui: a %s key is a hint: %s", class, kms[0])
		}
	}
	// A document that includes the kit, has a labeled row of its own, and
	// lists every labeled row, with no help overlay: only its own row shows.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.tui"), []byte(`<tui>
  <include src="`+filepath.ToSlash(mustAbs(t, kitFragment))+`"/>
  <keymap><bind keys="x" action="do_it" label="do it"/></keymap>
  <screen><hints class="k-hints" scope="all"/></screen>
</tui>
`), 0o644); err != nil {
		t.Fatal(err)
	}
	bare, err := tuimark.Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := bare.Dump(60, 2)
	if err != nil {
		t.Fatal(err)
	}
	if row := strings.TrimSpace(d.Grid[0]); row != "x do it" {
		t.Errorf("a document without overlays lists the kit's keys: got %q, want %q", row, "x do it")
	}
	for _, path := range []string{"../../kit/components/help.tui", "app.tui"} {
		ui, err := tuimark.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := ui.Set("help", map[string]any{"open": true, "title": "Keys"}); err != nil {
			t.Fatal(err)
		}
		d, err := ui.Dump(120, 30)
		if err != nil {
			t.Fatal(err)
		}
		grid := strings.Join(d.Grid, "\n")
		if n := strings.Count(grid, "? close help"); n != 1 {
			t.Errorf("%s: the help shows \"? close help\" %d times, want once (its own line):\n%s", path, n, grid)
		}
		for _, hint := range []string{"y confirm", "n cancel"} {
			if strings.Contains(grid, hint) {
				t.Errorf("%s: the help lists the dialog's %q:\n%s", path, hint, grid)
			}
		}
	}
}

// The kit is a fragment: `tuimark validate` checks it on its own, with or
// without data, and reports nothing; any other command refuses it as a
// document.
func TestKitFragmentValidatesOnItsOwn(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the CLI")
	}
	cli := filepath.Join(t.TempDir(), "tuimark")
	build := exec.Command("go", "build", "-o", cli, "./cmd/tuimark")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/tuimark: %v\n%s", err, out)
	}
	for _, args := range [][]string{
		{"validate", kitFragment},
		{"validate", kitFragment, "--data", "sample.json"},
	} {
		out, code := runCLI(t, cli, args...)
		if code != 0 || out != "ok (fragment checked on its own)\n" {
			t.Errorf("tuimark %s: exit %d, %q", strings.Join(args, " "), code, out)
		}
	}
	out, code := runCLI(t, cli, "dump", kitFragment)
	if code != 2 || !strings.Contains(out, "V005") || !strings.Contains(out, "a <fragment> is not a document") {
		t.Errorf("tuimark dump %s: exit %d, %q", kitFragment, code, out)
	}
}

func runCLI(t *testing.T, cli string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(cli, args...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return out.String(), 0
	case errors.As(err, &ee):
		return out.String(), ee.ExitCode()
	}
	t.Fatalf("tuimark %s: %v", strings.Join(args, " "), err)
	return "", -1
}

// ---- contracts ----

// Every component example starts with its contract. It names the kit class
// it shows or, for a template, the template, which it then uses.
func TestEveryComponentStartsWithItsContract(t *testing.T) {
	head := regexp.MustCompile(`^<!-- (k-[a-z-]+) · Contract: .+ · Actions: .+ · Keys: .+ · Requires: .+ -->\n<tui`)
	kit := classesStyled(t, "../../kit/kit.tcss")
	tpl := templates(t)
	for _, path := range componentFiles(t) {
		m := head.FindStringSubmatch(readFile(t, path))
		if m == nil {
			t.Errorf("%s: no `k-NAME · Contract: … · Actions: … · Keys: … · Requires: …` header", path)
			continue
		}
		switch {
		case tpl[m[1]] != "":
			if !strings.Contains(readFile(t, path), `template="`+m[1]+`"`) {
				t.Errorf("%s: the header names the template %s, which the document does not use", path, m[1])
			}
		case !kit[m[1]]:
			t.Errorf("%s: the header names %s, which is neither a kit template nor a class kit.tcss styles", path, m[1])
		}
		if !strings.Contains(readFile(t, path), `<include src="../kit.tui"/>`) {
			t.Errorf("%s does not include the kit", path)
		}
		first := strings.SplitN(readFile(t, path), "\n", 2)[0]
		first = strings.TrimSuffix(strings.TrimPrefix(first, "<!--"), "-->")
		if strings.Contains(first, "--") {
			t.Errorf("%s: an XML comment cannot hold two hyphens in a row", path)
		}
	}
}

// Overlay keymaps come before global ones, in the order the runtime reads
// the document (the kit's, from the include, first): the first matching
// row wins.
func TestOverlayKeymapsComeFirst(t *testing.T) {
	for _, path := range docs(t) {
		src := expand(t, path)
		overlay := regexp.MustCompile(`<keymap when="\.k-`).FindAllStringIndex(src, -1)
		global := regexp.MustCompile(`<keymap( when="#[a-z_-]+")?>`).FindAllStringIndex(src, -1)
		if len(overlay) == 0 || len(global) == 0 {
			continue
		}
		if overlay[len(overlay)-1][0] > global[0][0] {
			t.Errorf("%s: a global keymap comes before an overlay keymap", filepath.Base(path))
		}
	}
}

// The toast's text parts take no color of their own: they inherit it from the
// toast row, so a modifier on the row recolors them. Check it on the screen.
func TestToastTextTakesTheRowsTextColor(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		for _, level := range []string{"info", "ok", "warn", "danger"} {
			a := newApp(t)
			for k, v := range map[string]any{
				"@theme": theme, "toast.show": true, "toast.text": "toasted",
				"toast.is_ok": level == "ok", "toast.is_warn": level == "warn", "toast.is_danger": level == "danger",
			} {
				if err := a.ui.Set(k, v); err != nil {
					t.Fatal(err)
				}
			}
			res := playAt(t, a, tuimark.PlayOptions{Cols: 100, Rows: 30, Styles: true})
			found := false
			for y, line := range res.Dump.Grid {
				x := strings.Index(line, "toasted")
				if x < 1 {
					continue
				}
				found = true
				// The text, like the row around it, is drawn in the token for
				// text on that fill.
				on := map[string]string{"info": "on-accent", "ok": "on-ok", "warn": "on-warn", "danger": "on-danger"}[level]
				want := tokenValue(t, theme, on)
				var textFG, padFG string
				for _, sp := range res.Dump.Styles[y] {
					if sp.X <= x-1 && x-1 < sp.X+sp.W {
						padFG = sp.FG
					}
					if sp.X <= x && x < sp.X+sp.W {
						textFG = sp.FG
					}
				}
				if textFG != want || padFG != want {
					t.Errorf("%s %s: toast text fg %s, toast row fg %s, want %s ($%s)", theme, level, textFG, padFG, want, on)
				}
			}
			if !found {
				t.Errorf("%s %s: the toast is not on screen", theme, level)
			}
		}
	}
}

// tokenValue is the value of a token in a palette of theme.tcss.
func tokenValue(t *testing.T, theme, name string) string {
	t.Helper()
	src := stripComments(readFile(t, "../../kit/theme.tcss"))
	i := strings.Index(src, "@media (theme: light)")
	if theme == "light" {
		src = src[i:]
	} else {
		src = src[:i]
	}
	m := regexp.MustCompile(`--` + regexp.QuoteMeta(name) + `:\s*(#[0-9a-fA-F]+)`).FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("no token %s in the %s palette", name, theme)
	}
	return strings.ToLower(m[1])
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
