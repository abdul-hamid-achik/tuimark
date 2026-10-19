package tuimark_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/abdul-hamid-achik/tuimark"
)

// F8's form controls on an element an include placed follow the placement
// policy (SPEC v0.4b §6.18.6): what depends on where the element stands
// (V012, V016, B008) names the include; the self-checks (V002, V003) do
// not.
func TestFormControlsPlacedByAnInclude(t *testing.T) {
	const by = ", placed by <include> at app.tui:3:5"
	load := func(frag string, data map[string]any) []string {
		mem := fstest.MapFS{
			"app.tui": {Data: []byte(appWith(`<include src="f.tui"/>`))},
			"f.tui":   {Data: []byte("<fragment>\n" + frag + "\n</fragment>\n")},
		}
		app, err := tuimark.LoadFS(mem, "app.tui")
		if err != nil {
			t.Fatal(err)
		}
		if data != nil {
			if err := app.Bind("", data); err != nil {
				t.Fatal(err)
			}
		}
		return diagTexts(app.Validate())
	}
	has := func(t *testing.T, got []string, want string) {
		t.Helper()
		for _, g := range got {
			if g == want {
				return
			}
		}
		t.Errorf("diagnostics:\n%s\nwant\n%s", strings.Join(got, "\n"), want)
	}
	t.Run("checkbox without id", func(t *testing.T) {
		got := load(`<checkbox bind="on" label="x"/>`, map[string]any{"on": false})
		has(t, got, `V012 f.tui:2:1 /screen/checkbox id= <checkbox> is focusable and needs an id`+by)
	})
	t.Run("checkbox with children", func(t *testing.T) {
		got := load(`<checkbox id="c" bind="on" label="x"><text>a</text></checkbox>`, map[string]any{"on": false})
		has(t, got, `V016 f.tui:2:38 /screen/checkbox id=c <checkbox> takes no children (found <text>)`+by)
	})
	t.Run("checkbox with on:click is a self-check", func(t *testing.T) {
		got := load(`<checkbox id="c" bind="on" label="x" on:click="go"/>`, map[string]any{"on": false})
		found := false
		for _, g := range got {
			if strings.HasPrefix(g, "V002 ") && strings.Contains(g, `"on:click"`) {
				found = true
				if strings.Contains(g, "placed by") {
					t.Errorf("a self-check has the suffix: %s", g)
				}
			}
		}
		if !found {
			t.Errorf("no V002 for on:click: %v", got)
		}
	})
	t.Run("single B008", func(t *testing.T) {
		got := load(`<list id="l" each="zs as z" key="z" checked="pick" single="true"><item><text>{z}</text></item></list>`,
			map[string]any{"zs": []any{"a", "b"}, "pick": []any{"a"}})
		found := false
		for _, g := range got {
			if strings.HasPrefix(g, "B008 f.tui:2:1 /screen/list id=l ") {
				found = true
				if !strings.HasSuffix(g, by) {
					t.Errorf("B008 without the suffix: %s", g)
				}
			}
		}
		if !found {
			t.Errorf("no B008: %v", got)
		}
	})
}
