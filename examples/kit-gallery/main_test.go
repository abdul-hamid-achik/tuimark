package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// ---- the view: no diagnostics, at every size, in both themes ----

var sizes = [][2]int{{40, 24}, {80, 24}, {120, 40}}

// cleanAt asserts that the app, as it stands, has no diagnostic (warnings
// included) at every size under both themes.
func cleanAt(t *testing.T, ui *tuimark.App, label string) {
	t.Helper()
	for _, d := range ui.Validate() {
		t.Errorf("%s: validate: %s", label, d)
	}
	for _, theme := range []string{"dark", "light"} {
		if err := ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		for _, s := range sizes {
			d, err := ui.Dump(s[0], s[1])
			if err != nil {
				t.Fatalf("%s %s %dx%d: %v", label, theme, s[0], s[1], err)
			}
			if !d.OK || len(d.Errors) > 0 {
				t.Errorf("%s %s %dx%d: ok=%v errors=%v\n%s", label, theme, s[0], s[1], d.OK, d.Errors, strings.Join(d.Grid, "\n"))
			}
		}
	}
}

func TestValidateIsClean(t *testing.T) {
	for _, d := range newApp(t).ui.Validate() {
		t.Errorf("diagnostic: %s", d)
	}
}

// Inactive tabs are not laid out and closed modals paint nothing, so a clean
// first screen proves little: visit every tab and open every modal.
func TestEveryTabAndModalIsCleanAtEverySize(t *testing.T) {
	states := map[string][]string{
		"basics":           {"view=basics"},
		"forms":            {"view=forms"},
		"data":             {"view=data"},
		"overlays":         {"view=overlays"},
		"layouts":          {"view=layouts"},
		"confirm":          {"view=overlays", "confirm.open=true"},
		"palette":          {"view=overlays", "palette.open=true"},
		"palette-filtered": {"palette.open=true", "palette.query=roll"},
		"palette-empty":    {"palette.open=true", "palette.query=zzz"},
		"help":             {"help.open=true"},
		"toast-info":       {"toast.show=true", "toast.text=hello"},
		"toast-danger":     {"toast.show=true", "toast.text=hello", "toast.is_danger=true"},
		"form-invalid": {"view=forms", "forms.profile.name.error=Name is required.",
			"forms.profile.email.error=Enter a full address like name@host.example."},
	}
	names := make([]string, 0, len(states))
	for n := range states {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			a := newApp(t)
			for _, kv := range states[name] {
				path, val, _ := strings.Cut(kv, "=")
				var v any = val
				switch val {
				case "true":
					v = true
				case "false":
					v = false
				}
				if err := a.ui.Set(path, v); err != nil {
					t.Fatal(err)
				}
			}
			cleanAt(t, a.ui, name)
			// A state that opens a modal must really paint it.
			if sub, ok := rendered[name]; ok {
				if err := a.ui.Set("@theme", "dark"); err != nil {
					t.Fatal(err)
				}
				d, err := a.ui.Dump(120, 40)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(strings.Join(d.Grid, "\n"), sub) {
					t.Errorf("%s: %q is not on screen:\n%s", name, sub, strings.Join(d.Grid, "\n"))
				}
			}
		})
	}
}

// rendered is what each state that opens a modal or a toast must show.
var rendered = map[string]string{
	"confirm":          "permanently deletes",
	"palette":          "Command palette",
	"palette-filtered": "Roll back the last deploy",
	"palette-empty":    "No command matches",
	"help":             "close help",
	"toast-info":       "hello",
	"form-invalid":     "Enter a full address",
}

func TestEveryTabShowsItsContent(t *testing.T) {
	want := map[string][]string{
		"basics":   {"Badges", "Buttons", "Heads up", "Requests", "Progress"},
		"forms":    {"Profile", "Notify me by", "Shown on your profile."},
		"data":     {"Services · 5", "api-gateway", "build log · 6 lines", "No deployments yet"},
		"overlays": {"Dialogs", "Toasts", "Delete project…"},
		"layouts":  {"Incidents", "Billing errors", "owner: grace"},
	}
	for view, subs := range want {
		a := newApp(t)
		res := playAt(t, a, tuimark.PlayOptions{Cols: 120, Rows: 40}, `set:view="`+view+`"`)
		grid := gridOf(res)
		for _, s := range subs {
			if !strings.Contains(grid, s) {
				t.Errorf("%s: missing %q in\n%s", view, s, grid)
			}
		}
	}
}

// ---- every kit component, on its own ----

func componentFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("../../kit/components/*.tui")
	if err != nil || len(files) == 0 {
		t.Fatalf("no kit components found: %v", err)
	}
	return files
}

func loadComponent(t *testing.T, path string) *tuimark.App {
	t.Helper()
	ui, err := tuimark.Load(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	sample := strings.TrimSuffix(path, ".tui") + ".sample.json"
	if raw, err := os.ReadFile(sample); err == nil {
		var data map[string]any
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatalf("%s: %v", sample, err)
		}
		if err := ui.Bind("", data); err != nil {
			t.Fatalf("%s: %v", sample, err)
		}
	}
	return ui
}

func TestEveryKitComponentIsClean(t *testing.T) {
	files := componentFiles(t)
	if len(files) < 20 {
		t.Errorf("kit/components has %d documents, want at least 20", len(files))
	}
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".tui")
		t.Run(name, func(t *testing.T) { cleanAt(t, loadComponent(t, path), name) })
	}
}

// A component that binds data must ship the sample that binds it.
func TestEveryBindingComponentHasASample(t *testing.T) {
	for _, path := range componentFiles(t) {
		src, _ := os.ReadFile(path)
		_, err := os.Stat(strings.TrimSuffix(path, ".tui") + ".sample.json")
		if strings.Contains(string(src), "{") && strings.Contains(string(src), "=\"") {
			binds := regexp.MustCompile(`\{[a-z_.]+\}|bind="|if="|each="|class:[a-z-]+="`).MatchString(string(src))
			if binds && err != nil {
				t.Errorf("%s binds data but has no sample.json", path)
			}
		}
	}
}

var (
	classAttr  = regexp.MustCompile(`\bclass="([^"]*)"`)
	classGuard = regexp.MustCompile(`\bclass:([a-z_][a-z0-9_-]*)=`)
	sheetClass = regexp.MustCompile(`\.([a-z_][a-z0-9_-]*)`)
)

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
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		// Selectors only: drop comments and declaration blocks.
		src := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(string(raw), "")
		for _, sel := range regexp.MustCompile(`(?s)([^{}]+)\{`).FindAllStringSubmatch(src, -1) {
			for _, m := range sheetClass.FindAllStringSubmatch(sel[1], -1) {
				out[m[1]] = true
			}
		}
	}
	return out
}

// Every class a document writes is styled by the kit (or by the app's own
// sheet): a typo in a class name is otherwise silent.
func TestEveryClassIsStyled(t *testing.T) {
	kit := classesStyled(t, "../../kit/theme.tcss", "../../kit/kit.tcss")
	own := classesStyled(t, "../../kit/theme.tcss", "../../kit/kit.tcss", "app.tcss")
	check := func(path string, styled map[string]bool) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for c := range classesIn(string(src)) {
			if !styled[c] {
				t.Errorf("%s: class %q has no rule", path, c)
			}
		}
	}
	for _, path := range componentFiles(t) {
		check(path, kit)
	}
	check("app.tui", own)
}

// ---- the gallery's samples hold snapshots of derived values ----

// sample.json carries a literal snapshot of the derived values (found,
// detail) so that `tuimark validate`, `dump` and `run --mock` can show the
// gallery with no Go at all. The host's Derive recomputes them, so this test
// keeps the two from drifting.
func TestSampleSnapshotMatchesDerive(t *testing.T) {
	var sample map[string]any
	raw, err := os.ReadFile("sample.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatal(err)
	}
	a := newApp(t)
	for _, k := range []string{"found", "detail"} {
		got, _ := a.ui.Get(k)
		b, _ := json.Marshal(got)
		var norm any
		_ = json.Unmarshal(b, &norm)
		if !reflect.DeepEqual(norm, sample[k]) {
			if os.Getenv("UPDATE_SAMPLE") != "" {
				t.Logf("%s: %s", k, b)
			}
			t.Errorf("sample.json %q is stale: Derive gives %s", k, b)
		}
	}
}

// The standalone palette and split documents carry the same kind of snapshot.
func TestComponentSnapshotsMatchTheirRule(t *testing.T) {
	a := newApp(t)
	// palette.sample.json: query "dep" over its five commands.
	raw, _ := os.ReadFile("../../kit/components/palette.sample.json")
	var pal map[string]any
	if err := json.Unmarshal(raw, &pal); err != nil {
		t.Fatal(err)
	}
	if err := a.ui.Batch(func(b *tuimark.Batch) error {
		return b.Set("palette.commands", pal["palette"].(map[string]any)["commands"])
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.ui.Set("palette.query", "dep"); err != nil {
		t.Fatal(err)
	}
	got, _ := a.ui.Get("found")
	b, _ := json.Marshal(got)
	want, _ := json.Marshal(pal["found"])
	if string(b) != string(want) {
		t.Errorf("palette.sample.json found is stale:\n got %s\nwant %s", b, want)
	}
}

// ---- actions: every one has a handler and a mock ----

func TestEveryActionHasAHandlerAndAMock(t *testing.T) {
	a := newApp(t)
	raw, err := os.ReadFile("mock.json")
	if err != nil {
		t.Fatal(err)
	}
	var mock map[string][]struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(raw, &mock); err != nil {
		t.Fatal(err)
	}
	named := map[string]bool{}
	for _, spec := range a.ui.Catalog() {
		if spec.Builtin {
			continue
		}
		named[spec.Name] = true
		if !spec.Registered {
			t.Errorf("action %q (from %v) has no handler", spec.Name, spec.Sources)
		}
		if _, ok := mock[spec.Name]; !ok {
			t.Errorf("action %q has no entry in mock.json", spec.Name)
		}
	}
	for name, writes := range mock {
		if !named[name] {
			t.Errorf("mock.json answers %q, which app.tui never names", name)
		}
		// Each mock is a batch of literal writes; they must all apply.
		fresh := newApp(t)
		if err := fresh.ui.Batch(func(b *tuimark.Batch) error {
			for _, w := range writes {
				if err := b.Set(w.Path, w.Value); err != nil {
					return fmt.Errorf("%s: %s: %w", name, w.Path, err)
				}
			}
			return nil
		}); err != nil {
			t.Error(err)
		}
	}
}

// ---- keys ----

func TestTabsSwitchByKeyAndByNumber(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "3")
	if got := getStr(t, a, "view"); got != "data" {
		t.Fatalf("after 3: view = %q", got)
	}
	if !strings.Contains(gridOf(res), "Services · 5") {
		t.Errorf("the data tab is not showing:\n%s", gridOf(res))
	}
	play(t, a, "]")
	if got := getStr(t, a, "view"); got != "overlays" {
		t.Errorf("after ]: view = %q", got)
	}
	play(t, a, "[", "[")
	if got := getStr(t, a, "view"); got != "forms" {
		t.Errorf("after [ [: view = %q", got)
	}
	play(t, a, "1")
	if got := getStr(t, a, "view"); got != "basics" {
		t.Errorf("after 1: view = %q", got)
	}
}

func TestConfirmDialogOpensAndCloses(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "4", "focus:#confirm__open", "enter")
	if !has(actions(res), "dialog_open") || !getBool(t, a, "confirm.open") {
		t.Fatalf("enter on the button should open the dialog; events %v", actions(res))
	}
	if focusOf(res) != "confirm__no" {
		t.Errorf("focus should start on the safe button, got %q", focusOf(res))
	}
	if !strings.Contains(gridOf(res), "permanently deletes") {
		t.Errorf("the dialog text is not showing:\n%s", gridOf(res))
	}
	// Enter on the focused Cancel keeps the project.
	res = play(t, a, "enter")
	if !has(actions(res), "dialog_cancel") || getBool(t, a, "confirm.open") {
		t.Errorf("enter on Cancel should close the dialog; events %v", actions(res))
	}
	if getStr(t, a, "toast.text") != "Kept the project" {
		t.Errorf("toast = %q", getStr(t, a, "toast.text"))
	}
	// y deletes; esc cancels.
	play(t, a, "focus:#confirm__open", "enter", "y")
	if getBool(t, a, "confirm.open") || getStr(t, a, "toast.text") != "Project deleted" || !getBool(t, a, "toast.is_ok") {
		t.Errorf("y should confirm: open=%v toast=%q", getBool(t, a, "confirm.open"), getStr(t, a, "toast.text"))
	}
	res = play(t, a, "focus:#confirm__open", "enter", "esc")
	if getBool(t, a, "confirm.open") || !has(actions(res), "dialog_cancel") {
		t.Errorf("esc should cancel; events %v", actions(res))
	}
}

func TestPaletteFiltersAndRuns(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "ctrl+k")
	if !getBool(t, a, "palette.open") || focusOf(res) != "palette__q" {
		t.Fatalf("ctrl+k should open the palette on its input; open=%v focus=%q", getBool(t, a, "palette.open"), focusOf(res))
	}
	if !strings.Contains(gridOf(res), "Deploy to production") || !strings.Contains(gridOf(res), "Show the keys") {
		t.Errorf("all commands should list first:\n%s", gridOf(res))
	}
	res = play(t, a, "text:roll")
	grid := gridOf(res)
	if !strings.Contains(grid, "Roll back the last deploy") || strings.Contains(grid, "Deploy to production") {
		t.Errorf("typing should filter the list:\n%s", grid)
	}
	if n := len(listAt(a.ui.Get, "found.items")); n != 1 {
		t.Errorf("found has %d items, want 1", n)
	}
	// Enter runs the command: rollback toasts a warning.
	res = play(t, a, "enter")
	if getBool(t, a, "palette.open") || !has(actions(res), "palette_run") {
		t.Errorf("enter should run and close; events %v", actions(res))
	}
	if !getBool(t, a, "toast.is_warn") || getStr(t, a, "toast.text") != "Rolling back the last deploy" {
		t.Errorf("toast = %q warning=%v", getStr(t, a, "toast.text"), getBool(t, a, "toast.is_warn"))
	}
}

func TestPaletteArrowsMoveTheCursorAndNoMatchSaysSo(t *testing.T) {
	a := newApp(t)
	play(t, a, "ctrl+k", "down", "down")
	if got := getStr(t, a, "palette.cur"); got != "logs" {
		t.Errorf("two downs: cur = %q, want logs", got)
	}
	res := play(t, a, "enter")
	if getStr(t, a, "view") != "data" || getBool(t, a, "palette.open") {
		t.Errorf("running 'logs' should switch to the data tab: view=%q", getStr(t, a, "view"))
	}
	if !strings.Contains(gridOf(res), "build log") {
		t.Errorf("the log panel is not showing:\n%s", gridOf(res))
	}
	res = play(t, a, "ctrl+k", "text:zzz")
	if !strings.Contains(gridOf(res), "No command matches “zzz”") {
		t.Errorf("an empty result should say so:\n%s", gridOf(res))
	}
	play(t, a, "enter")
	if !getBool(t, a, "palette.open") {
		t.Error("enter with no match should leave the palette open")
	}
	play(t, a, "esc")
	if getBool(t, a, "palette.open") {
		t.Error("esc should close the palette")
	}
}

func TestHelpOverlayListsTheKeys(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "?")
	grid := gridOf(res)
	if !getBool(t, a, "help.open") {
		t.Fatal("? should open the help")
	}
	for _, want := range []string{"quit", "commands", "theme", "tabs"} {
		if !strings.Contains(grid, want) {
			t.Errorf("help is missing %q:\n%s", want, grid)
		}
	}
	play(t, a, "q")
	if getBool(t, a, "help.open") {
		t.Error("q should close the help")
	}
}

func TestFormValidatesAsYouType(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "2", "focus:#profile__email", "ctrl+u", "text:ada@")
	if got := getStr(t, a, "forms.profile.email.error"); got == "" {
		t.Fatalf("a half-typed address should be invalid; events %v", actions(res))
	}
	if !strings.Contains(gridOf(res), "Enter a full address") {
		t.Errorf("the error text is not showing:\n%s", gridOf(res))
	}
	if strings.Contains(gridOf(res), "We never share it.") {
		t.Error("the help line should give way to the error")
	}
	res = play(t, a, "text:x.io")
	if got := getStr(t, a, "forms.profile.email.error"); got != "" {
		t.Errorf("a full address should clear the error, got %q", got)
	}
	if !strings.Contains(gridOf(res), "We never share it.") {
		t.Errorf("the help line should come back:\n%s", gridOf(res))
	}
	// Enter in the input submits: the form is valid, so it toasts success.
	res = play(t, a, "enter")
	if !has(actions(res), "form_submit") || getStr(t, a, "toast.text") != "Profile saved" {
		t.Errorf("submit: events %v toast %q", actions(res), getStr(t, a, "toast.text"))
	}
}

func TestSubmitFocusesTheFirstInvalidField(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "2", "focus:#profile__name", "ctrl+u", "focus:#profile__save", "enter")
	if getStr(t, a, "forms.profile.name.error") != "Name is required." {
		t.Fatalf("name_error = %q", getStr(t, a, "forms.profile.name.error"))
	}
	if focusOf(res) != "profile__name" {
		t.Errorf("focus = %q, want profile__name", focusOf(res))
	}
	if !getBool(t, a, "toast.is_danger") || !strings.Contains(getStr(t, a, "toast.text"), "1 field needs attention") {
		t.Errorf("toast = %q danger=%v", getStr(t, a, "toast.text"), getBool(t, a, "toast.is_danger"))
	}
}

func TestLogPanelFollowsAppendedLines(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "3", "focus:#log", "a", "a", "a")
	if n, _ := get(t, a, "log.count").(float64); n != 9 {
		t.Fatalf("log.count = %v, want 9", get(t, a, "log.count"))
	}
	// Line 9 is the last one appended; the panel sticks to the bottom.
	if !strings.Contains(gridOf(res), "12:00:09") {
		t.Errorf("the newest line should be in view:\n%s", gridOf(res))
	}
	if !strings.Contains(gridOf(res), "build log · 9 lines") {
		t.Errorf("the title should count the lines:\n%s", gridOf(res))
	}
}

func TestSplitDetailFollowsTheCursor(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "5", "focus:#items", "down")
	if got := getStr(t, a, "split.cur"); got != "i3" {
		t.Fatalf("split.cur = %q", got)
	}
	if got := getStr(t, a, "detail.title"); got != "Search index lag" {
		t.Errorf("detail.title = %q", got)
	}
	if !strings.Contains(gridOf(res), "Reindexed; lag is under a second.") || !strings.Contains(gridOf(res), "resolved") {
		t.Errorf("the detail pane did not follow:\n%s", gridOf(res))
	}
}

func TestBreadcrumbGoesUp(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "backspace")
	if !strings.Contains(gridOf(res), "Production › Services › billing") || strings.Contains(gridOf(res), "Settings") {
		t.Errorf("one level up:\n%s", gridOf(res))
	}
	play(t, a, "backspace", "backspace", "backspace")
	if n := len(listAt(a.ui.Get, "crumbs")); n != 1 {
		t.Errorf("the last crumb stays: %d left", n)
	}
}

func TestThemeToggleKey(t *testing.T) {
	a := newApp(t)
	play(t, a, "t")
	if getStr(t, a, "theme.name") != "light" {
		t.Fatalf("theme.name = %q", getStr(t, a, "theme.name"))
	}
	res := playAt(t, a, tuimark.PlayOptions{Cols: 100, Rows: 30, Styles: true}, "t")
	if getStr(t, a, "theme.name") != "dark" {
		t.Errorf("theme.name = %q after the second t", getStr(t, a, "theme.name"))
	}
	if res.Dump.Theme != "dark" {
		t.Errorf("dump theme = %q", res.Dump.Theme)
	}
}

// ---- focus is visible: a focused button changes its own cells ----

func TestFocusChangesTheButtonsCells(t *testing.T) {
	cellStyle := func(res *tuimark.PlayResult, id string) (fg, bg string, attrs []string) {
		for _, n := range res.Dump.Nodes {
			if n.ID == id {
				for _, sp := range res.Dump.Styles[n.Y] {
					if sp.X <= n.X+1 && n.X+1 < sp.X+sp.W {
						return sp.FG, sp.BG, sp.A
					}
				}
			}
		}
		t.Fatalf("no node %q", id)
		return
	}
	for _, theme := range []string{"dark", "light"} {
		a := newApp(t)
		if err := a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		opts := tuimark.PlayOptions{Cols: 100, Rows: 30, Styles: true}
		for _, id := range []string{"buttons__primary", "buttons__secondary", "buttons__danger", "buttons__ghost"} {
			idle := playAt(t, a, opts, "focus:#nav")
			focused := playAt(t, a, opts, "focus:#"+id)
			ifg, ibg, _ := cellStyle(idle, id)
			ffg, fbg, fa := cellStyle(focused, id)
			if ifg == ffg && ibg == fbg {
				t.Errorf("%s %s: focus does not change the colors (fg %s, bg %s)", theme, id, ifg, ibg)
			}
			if !has(fa, "bold") {
				t.Errorf("%s %s: focused label should be bold, got %v", theme, id, fa)
			}
		}
	}
}

func TestInputBorderShowsFocus(t *testing.T) {
	a := newApp(t)
	opts := tuimark.PlayOptions{Cols: 100, Rows: 30, Styles: true}
	borderFG := func(res *tuimark.PlayResult, id string) string {
		for _, n := range res.Dump.Nodes {
			if n.ID == id {
				for _, sp := range res.Dump.Styles[n.Y] {
					if sp.X <= n.X && n.X < sp.X+sp.W {
						return sp.FG
					}
				}
			}
		}
		t.Fatalf("no node %q", id)
		return ""
	}
	idle := playAt(t, a, opts, "2", "focus:#profile__save")
	focused := playAt(t, a, opts, "focus:#profile__name")
	if borderFG(idle, "profile__name") == borderFG(focused, "profile__name") {
		t.Errorf("the input border keeps its color when focused: %s", borderFG(idle, "profile__name"))
	}
	// An invalid input keeps a danger border.
	if err := a.ui.Set("forms.profile.name.error", "Name is required."); err != nil {
		t.Fatal(err)
	}
	invalid := playAt(t, a, opts, "focus:#profile__save")
	if borderFG(invalid, "profile__name") == borderFG(idle, "profile__name") {
		t.Errorf("an invalid input should have a different border color")
	}
}

// ---- the clock ----

func TestTickClearsTheToastAndTheStatusLine(t *testing.T) {
	a := newApp(t)
	play(t, a, "4", "focus:#toast__ok", "enter", "focus:#buttons__primary")
	if !getBool(t, a, "toast.show") {
		t.Fatal("the toast should show")
	}
	for i := 0; i < toastTTL-1; i++ {
		if err := a.ui.Batch(a.tick); err != nil {
			t.Fatal(err)
		}
		if !getBool(t, a, "toast.show") {
			t.Fatalf("the toast cleared after %d ticks, want %d", i+1, toastTTL)
		}
	}
	if err := a.ui.Batch(a.tick); err != nil {
		t.Fatal(err)
	}
	if getBool(t, a, "toast.show") {
		t.Errorf("the toast should be gone after %d ticks", toastTTL)
	}
	// The status line goes back to idle after its own, longer time.
	if err := a.say(a.ui, "pressed something"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < statusTTL; i++ {
		if getStr(t, a, "status.text") == idle {
			t.Fatalf("status went idle after %d ticks, want %d", i, statusTTL)
		}
		if err := a.ui.Batch(a.tick); err != nil {
			t.Fatal(err)
		}
	}
	if getStr(t, a, "status.text") != idle {
		t.Errorf("status = %q, want %q", getStr(t, a, "status.text"), idle)
	}
}

func TestEscDismissesTheToast(t *testing.T) {
	a := newApp(t)
	play(t, a, "4", "focus:#toast__info", "enter", "focus:#buttons__primary")
	if !getBool(t, a, "toast.show") {
		t.Fatal("the toast should show")
	}
	play(t, a, "esc")
	if getBool(t, a, "toast.show") {
		t.Error("esc should dismiss the toast")
	}
}

func TestEveryToastLevelPaints(t *testing.T) {
	for _, level := range []string{"info", "ok", "warn", "danger"} {
		a := newApp(t)
		res := play(t, a, "4", "focus:#toast__"+level, "enter")
		if !getBool(t, a, "toast.show") {
			t.Fatalf("%s: no toast", level)
		}
		want := map[string]bool{"ok": level == "ok", "warn": level == "warn", "danger": level == "danger"}
		for k, w := range want {
			if got := getBool(t, a, "toast.is_"+k); got != w {
				t.Errorf("%s: toast.is_%s = %v", level, k, got)
			}
		}
		if !strings.Contains(gridOf(res), getStr(t, a, "toast.text")) {
			t.Errorf("%s: the toast text is not on screen:\n%s", level, gridOf(res))
		}
	}
}
