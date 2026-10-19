package main

import (
	"github.com/abdul-hamid-achik/tuimark"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The tests in this file hold the kit's conventions (kit/README.md) to the
// files themselves, so a copied or renamed piece cannot drift from them.

// modifiers are the short classes a component class takes as a second class
// on the same node. They are only ever written compound with a k- class.
var modifiers = map[string]bool{
	"primary": true, "secondary": true, "danger": true, "ghost": true,
	"ok": true, "warn": true, "info": true, "accent": true,
	"good": true, "bad": true, "invalid": true, "current": true, "muted": true,
}

// generated are the class names the runtime gives to nodes it generates; the
// kit styles them only under a component class.
var generated = map[string]bool{"tab-label": true, "hint-key": true, "hint-label": true}

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

// Every class a document writes is a k- class, a modifier, or (for the
// app's own sheet) nothing else; a modifier is written only next to a k-
// class on the same node.
func TestClassNamesFollowTheConvention(t *testing.T) {
	for _, path := range docs(t) {
		src := readFile(t, path)
		for _, m := range classAttr.FindAllStringSubmatch(src, -1) {
			names := strings.Fields(m[1])
			hasK := false
			for _, c := range names {
				if strings.HasPrefix(c, "k-") {
					hasK = true
				}
			}
			for _, c := range names {
				switch {
				case strings.HasPrefix(c, "k-"):
				case modifiers[c]:
					if !hasK {
						t.Errorf("%s: class %q stands alone in class=%q; a modifier goes with a k- class", path, c, m[1])
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

// Every k- class the kit sheet defines is used by a document, so a rename
// cannot leave a dead rule behind.
func TestEveryKitClassIsUsed(t *testing.T) {
	defined := classesStyled(t, "../../kit/theme.tcss", "../../kit/kit.tcss")
	used := map[string]bool{}
	for _, path := range docs(t) {
		for c := range classesIn(readFile(t, path)) {
			used[c] = true
		}
	}
	// Utilities and layout helpers that a document may or may not need.
	optional := map[string]bool{
		"k-strong": true, "k-kbd": true, "k-code": true, "k-section": true,
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

// ---- ids and actions ----

var (
	idAttr     = regexp.MustCompile(`\bid="([^"]*)"`)
	actionAttr = regexp.MustCompile(`\b(?:on:[a-z]+|action)="([^"]*)"`)
	idPattern  = regexp.MustCompile(`^[a-z][a-z0-9-]*(__[a-z][a-z0-9-]*)?$`)
	actionName = regexp.MustCompile(`^[a-z]+_[a-z]+$`)
)

// builtins are the runtime's own actions; they keep their names.
var builtins = map[string]bool{
	"quit": true, "focus": true, "move-next": true, "move-prev": true,
	"move-first": true, "move-last": true, "move-page-down": true,
	"move-page-up": true, "check-toggle": true, "check-all": true,
	"check-none": true, "switch-to": true,
}

// An id is `instance` or `instance__part`; an action is `component_verb`,
// and never a generic word.
func TestIDsAndActionsFollowTheConvention(t *testing.T) {
	generic := map[string]bool{
		"cancel": true, "open": true, "save": true, "dismiss": true, "delete": true,
		"skip": true, "search": true, "confirm": true, "close": true, "submit": true,
	}
	for _, path := range docs(t) {
		src := stripComments(readFile(t, path))
		for _, m := range idAttr.FindAllStringSubmatch(src, -1) {
			if !idPattern.MatchString(m[1]) {
				t.Errorf("%s: id %q is not `instance` or `instance__part`", path, m[1])
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

// Every overlay (a modal) names its own parts `instance__part`, closes on
// esc, and declares its keys in a keymap scoped to its own class.
func TestOverlaysAreSelfContained(t *testing.T) {
	modal := regexp.MustCompile(`<modal\b[^>]*>`)
	for _, path := range docs(t) {
		src := stripComments(readFile(t, path))
		for _, tag := range modal.FindAllString(src, -1) {
			id := idAttr.FindStringSubmatch(tag)
			if id == nil || !strings.Contains(id[1], "__") {
				t.Errorf("%s: modal %s has no instance__part id", path, tag)
			}
			if !strings.Contains(tag, `on:escape="`) {
				t.Errorf("%s: modal has no on:escape: %s", path, tag)
			}
			cls := regexp.MustCompile(`\bclass="([^"]*)"`).FindStringSubmatch(tag)
			if cls == nil {
				t.Errorf("%s: modal has no class: %s", path, tag)
				continue
			}
			root := strings.Fields(cls[1])[0]
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

// ---- surviving the next language cut ----

// No document writes a version; a tabs says whether it takes focus; a
// container that fills says so (by attribute or by its kit class); every
// document starts with its contract.
func TestDocumentsSurviveTheNextCut(t *testing.T) {
	container := regexp.MustCompile(`<(row|col|box|scroll)\b[^>]*>`)
	for _, path := range docs(t) {
		src := readFile(t, path)
		if strings.Contains(src, "version=") {
			t.Errorf("%s writes a version; the current language needs none", path)
		}
		body := stripComments(src)
		for _, tag := range regexp.MustCompile(`<tabs\b[^>]*>`).FindAllString(body, -1) {
			if !strings.Contains(tag, "focusable=") {
				t.Errorf("%s: tabs does not write focusable: %s", path, tag)
			}
		}
		for _, tag := range container.FindAllString(body, -1) {
			if strings.Contains(tag, `width="`) && strings.Contains(tag, `height="`) {
				continue
			}
			sized := false
			if cls := regexp.MustCompile(`\bclass="([^"]*)"`).FindStringSubmatch(tag); cls != nil {
				for _, c := range strings.Fields(cls[1]) {
					if strings.HasPrefix(c, "k-") && ruleHas(t, c, "width", "height") {
						sized = true
					}
				}
			}
			if !sized {
				t.Errorf("%s: container leaves its size to the default: %s", path, tag)
			}
		}
	}
}

func TestEveryComponentStartsWithItsContract(t *testing.T) {
	head := regexp.MustCompile(`^<!-- (k-[a-z-]+) · Contract: .+ · Actions: .+ · Keys: .+ · Requires: .+ -->\n<tui`)
	kit := classesStyled(t, "../../kit/kit.tcss")
	for _, path := range componentFiles(t) {
		m := head.FindStringSubmatch(readFile(t, path))
		if m == nil {
			t.Errorf("%s: no `k-NAME · Contract: … · Actions: … · Keys: … · Requires: …` header", path)
			continue
		}
		if !kit[m[1]] {
			t.Errorf("%s: the header names %s, which kit.tcss does not style", path, m[1])
		}
		first := strings.SplitN(readFile(t, path), "\n", 2)[0]
		first = strings.TrimSuffix(strings.TrimPrefix(first, "<!--"), "-->")
		if strings.Contains(first, "--") {
			t.Errorf("%s: an XML comment cannot hold two hyphens in a row", path)
		}
	}
}

// Overlay keymaps come before global ones: the first matching row wins.
func TestOverlayKeymapsComeFirst(t *testing.T) {
	for _, path := range docs(t) {
		src := stripComments(readFile(t, path))
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
