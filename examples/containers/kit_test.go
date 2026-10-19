package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The tests in this file hold the kit's conventions (kit/README.md) to this
// app's document and sheets, so a copied or renamed piece cannot drift from
// them. They mirror examples/kit-gallery/kit_test.go for one document.

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

var (
	classAttr  = regexp.MustCompile(`\bclass="([^"]*)"`)
	classGuard = regexp.MustCompile(`\bclass:([a-z_][a-z0-9_-]*)=`)
	sheetClass = regexp.MustCompile(`\.([a-z_][a-z0-9_-]*)`)
	tokenDef   = regexp.MustCompile(`--([a-z][a-z0-9-]*)\s*:`)
	tokenUse   = regexp.MustCompile(`\$([a-z][a-z0-9-]*)`)
	idAttr     = regexp.MustCompile(`\bid="([^"]*)"`)
	actionAttr = regexp.MustCompile(`\b(?:on:[a-z]+|action)="([^"]*)"`)
	idPattern  = regexp.MustCompile(`^[a-z][a-z0-9-]*(__[a-z][a-z0-9-]*)?$`)
	actionName = regexp.MustCompile(`^[a-z]+_[a-z]+$`)
)

// sheets are the stylesheets app.tui loads, in order.
func sheets() []string {
	return []string{"../../kit/theme.tcss", "../../kit/kit.tcss", "app.tcss"}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func stripComments(s string) string {
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(s, "")
}

// classesIn are the class names a document writes (class="a b" and the NAME of
// class:NAME guards).
func classesIn(src string) map[string]bool {
	out := map[string]bool{}
	for _, m := range classAttr.FindAllStringSubmatch(src, -1) {
		for _, c := range strings.Fields(m[1]) {
			out[c] = true
		}
	}
	for _, m := range classGuard.FindAllStringSubmatch(src, -1) {
		out[m[1]] = true
	}
	return out
}

// classesStyled are the class names a stylesheet has a selector for.
func classesStyled(t *testing.T, paths ...string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, p := range paths {
		src := stripComments(readFile(t, p))
		for _, sel := range regexp.MustCompile(`(?s)([^{}]+)\{`).FindAllStringSubmatch(src, -1) {
			for _, m := range sheetClass.FindAllStringSubmatch(sel[1], -1) {
				out[m[1]] = true
			}
		}
	}
	return out
}

// Every class the document writes is a k- class or a modifier, a modifier is
// written only next to a k- class, and every one has a rule (a typo in a class
// name is otherwise silent).
func TestClassNamesFollowTheConvention(t *testing.T) {
	src := readFile(t, "app.tui")
	for _, m := range classAttr.FindAllStringSubmatch(src, -1) {
		names := strings.Fields(m[1])
		hasK := false
		for _, c := range names {
			hasK = hasK || strings.HasPrefix(c, "k-")
		}
		for _, c := range names {
			switch {
			case strings.HasPrefix(c, "k-"):
			case modifiers[c]:
				if !hasK {
					t.Errorf("class %q stands alone in class=%q; a modifier goes with a k- class", c, m[1])
				}
			default:
				t.Errorf("class %q is neither a k- class nor a modifier: the app styles by id in app.tcss", c)
			}
		}
	}
	for _, tag := range regexp.MustCompile(`<[a-z]+\b[^>]*\bclass:[^>]*>`).FindAllString(src, -1) {
		if !regexp.MustCompile(`\bclass="[^"]*\bk-`).MatchString(tag) {
			t.Errorf("a class:NAME guard on a node with no k- class: %s", tag)
		}
		for _, g := range classGuard.FindAllStringSubmatch(tag, -1) {
			if !modifiers[g[1]] {
				t.Errorf("guard class:%s is not a modifier", g[1])
			}
		}
	}
	styled := classesStyled(t, sheets()...)
	for c := range classesIn(src) {
		if !styled[c] {
			t.Errorf("class %q has no rule", c)
		}
	}
}

// The app's own sheet styles by id and tag, never by a kit class or a bare
// modifier: it adds sizes and media rules, it does not restyle components.
func TestAppSheetStylesByIDOnly(t *testing.T) {
	src := stripComments(readFile(t, "app.tcss"))
	for _, sel := range regexp.MustCompile(`(?s)([^{}]+)\{`).FindAllStringSubmatch(src, -1) {
		text := strings.TrimSpace(sel[1])
		if strings.HasPrefix(text, "@media") {
			continue
		}
		for _, one := range strings.Split(text, ",") {
			if strings.Contains(one, ".") {
				t.Errorf("app.tcss selector %q names a class; style the app by id", strings.TrimSpace(one))
			}
		}
	}
	for _, p := range sheets()[1:] {
		for _, sel := range regexp.MustCompile(`(?s)([^{}]+)\{`).FindAllStringSubmatch(stripComments(readFile(t, p)), -1) {
			text := strings.TrimSpace(sel[1])
			if strings.HasPrefix(text, "@media") {
				continue
			}
			for _, one := range strings.Split(text, ",") {
				for _, compound := range regexp.MustCompile(`\s*>\s*|\s+`).Split(strings.TrimSpace(one), -1) {
					short, hasK := []string{}, false
					for _, m := range sheetClass.FindAllStringSubmatch(compound, -1) {
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
}

// Every token the sheets use is defined for both themes, and nothing outside
// theme.tcss holds a literal color.
func TestTokensAreDefinedAndNoLiteralColors(t *testing.T) {
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
	for _, p := range sheets() {
		for _, m := range tokenUse.FindAllStringSubmatch(stripComments(readFile(t, p)), -1) {
			if !dark[m[1]] || !light[m[1]] {
				t.Errorf("%s: $%s is not a token of both palettes", p, m[1])
			}
		}
	}
	lit := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	for _, p := range sheets()[1:] {
		if m := lit.FindString(stripComments(readFile(t, p))); m != "" {
			t.Errorf("%s holds a literal color %s; use a token", p, m)
		}
	}
}

// An id is `instance` or `instance__part`; an action is `component_verb`, and
// never a generic word.
func TestIDsAndActionsFollowTheConvention(t *testing.T) {
	builtins := map[string]bool{
		"quit": true, "focus": true, "move-next": true, "move-prev": true,
		"move-first": true, "move-last": true, "move-page-down": true,
		"move-page-up": true, "check-toggle": true, "check-all": true,
		"check-none": true, "switch-to": true,
	}
	generic := map[string]bool{
		"cancel": true, "open": true, "save": true, "dismiss": true, "delete": true,
		"skip": true, "search": true, "confirm": true, "close": true, "submit": true,
	}
	src := stripComments(readFile(t, "app.tui"))
	for _, m := range idAttr.FindAllStringSubmatch(src, -1) {
		if !idPattern.MatchString(m[1]) {
			t.Errorf("id %q is not `instance` or `instance__part`", m[1])
		}
	}
	for _, m := range actionAttr.FindAllStringSubmatch(src, -1) {
		for _, a := range strings.Split(m[1], ",") {
			a = strings.TrimSpace(a)
			if a == "" || builtins[a] || strings.Contains(a, "#") || strings.HasPrefix(a, ".") {
				continue
			}
			if generic[a] {
				t.Errorf("action %q is a generic name", a)
			}
			if !actionName.MatchString(a) {
				t.Errorf("action %q is not `component_verb`", a)
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

// Every overlay (a modal) names its parts `instance__part`, closes on esc,
// declares its keys in a keymap scoped to its own class, and has its size
// written by that class.
func TestOverlaysAreSelfContained(t *testing.T) {
	src := stripComments(readFile(t, "app.tui"))
	modals := regexp.MustCompile(`<modal\b[^>]*>`).FindAllString(src, -1)
	if len(modals) != 3 {
		t.Errorf("want the confirm, palette and help modals, found %d", len(modals))
	}
	for _, tag := range modals {
		id := idAttr.FindStringSubmatch(tag)
		if id == nil || !strings.Contains(id[1], "__") {
			t.Errorf("modal %s has no instance__part id", tag)
		}
		if !strings.Contains(tag, `on:escape="`) {
			t.Errorf("modal has no on:escape: %s", tag)
		}
		cls := classAttr.FindStringSubmatch(tag)
		if cls == nil {
			t.Errorf("modal has no class: %s", tag)
			continue
		}
		root := strings.Fields(cls[1])[0]
		if !strings.Contains(src, `<keymap when=".`+root+`">`) {
			t.Errorf("no <keymap when=\".%s\"> for the overlay", root)
		}
		if !ruleHas(t, root, "width", "height") {
			t.Errorf(".%s does not write a width and a height", root)
		}
	}
	// Overlay keymaps come before every other one: the first matching row wins.
	overlay := regexp.MustCompile(`<keymap when="\.k-`).FindAllStringIndex(src, -1)
	other := regexp.MustCompile(`<keymap( when="#[a-z_-]+")?>`).FindAllStringIndex(src, -1)
	if len(overlay) == 0 || len(other) == 0 {
		t.Fatal("no overlay keymaps or no other keymaps found")
	}
	if overlay[len(overlay)-1][0] > other[0][0] {
		t.Error("a non-overlay keymap comes before an overlay keymap")
	}
}

// No document writes a version; a tabs says whether it takes focus; a
// container that fills says so, by attribute or by its kit class.
func TestDocumentSurvivesTheNextCut(t *testing.T) {
	raw := readFile(t, "app.tui")
	if strings.Contains(raw, "version=") {
		t.Error("app.tui writes a version; the current language needs none")
	}
	body := stripComments(raw)
	tabs := regexp.MustCompile(`<tabs\b[^>]*>`).FindAllString(body, -1)
	if len(tabs) != 2 {
		t.Errorf("want the main and the nested tabs, found %d", len(tabs))
	}
	for _, tag := range tabs {
		if !strings.Contains(tag, "focusable=") {
			t.Errorf("tabs does not write focusable: %s", tag)
		}
	}
	for _, tag := range regexp.MustCompile(`<(row|col|box|scroll)\b[^>]*>`).FindAllString(body, -1) {
		if strings.Contains(tag, `width="`) && strings.Contains(tag, `height="`) {
			continue
		}
		sized := false
		if cls := classAttr.FindStringSubmatch(tag); cls != nil {
			for _, c := range strings.Fields(cls[1]) {
				if strings.HasPrefix(c, "k-") && ruleHas(t, c, "width", "height") {
					sized = true
				}
			}
		}
		if !sized {
			t.Errorf("container leaves its size to the default: %s", tag)
		}
	}
}

// The toast's text parts take no color of their own: they inherit it from the
// toast row, so a modifier on the row recolors them. Check it on the screen.
func TestToastTextTakesTheRowsTextColor(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		for _, level := range []string{"info", "ok", "warn", "danger"} {
			r := newRig(t)
			for k, v := range map[string]any{
				"@theme": theme, "toast.show": true, "toast.text": "toasted",
				"toast.is_ok": level == "ok", "toast.is_warn": level == "warn", "toast.is_danger": level == "danger",
			} {
				r.set(k, v)
			}
			res := r.playOpts(playStyles(100, 30))
			found := false
			for y, line := range res.Dump.Grid {
				x := strings.Index(line, "toasted")
				if x < 1 {
					continue
				}
				found = true
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
