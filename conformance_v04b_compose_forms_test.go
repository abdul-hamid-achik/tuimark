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

// F9's select on an element an include placed follows the same policy
// (SPEC v0.4b §6.18.6): its V012, V017, B001, and B012 name the include;
// V002 for on:click, a self-check, does not.
func TestSelectPlacedByAnInclude(t *testing.T) {
	const by = ", placed by <include> at app.tui:3:5"
	load := func(t *testing.T, frag string, data map[string]any) []string {
		t.Helper()
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
	// one returns the only diagnostic with prefix, failing unless there
	// is exactly one.
	one := func(t *testing.T, got []string, prefix string) string {
		t.Helper()
		var found []string
		for _, g := range got {
			if strings.HasPrefix(g, prefix) {
				found = append(found, g)
			}
		}
		if len(found) != 1 {
			t.Fatalf("want one diagnostic starting %q, got:\n%s", prefix, strings.Join(got, "\n"))
		}
		return found[0]
	}
	zones := []any{map[string]any{"id": "utc"}, map[string]any{"id": "lis"}}
	t.Run("select without id", func(t *testing.T) {
		got := load(t, `<select bind="tz" each="zones as z" key="z.id"/>`, map[string]any{"tz": nil, "zones": zones})
		if g := one(t, got, "V012 f.tui:2:1 /screen/select id= "); !strings.HasSuffix(g, by) {
			t.Errorf("V012 without the suffix: %s", g)
		}
	})
	t.Run("select without each", func(t *testing.T) {
		got := load(t, `<select id="s" bind="tz" key="z.id"/>`, map[string]any{"tz": nil})
		g := one(t, got, "V017 f.tui:2:1 /screen/select id=s ")
		if want := `V017 f.tui:2:1 /screen/select id=s <select> needs each="path as alias": its options are the elements of an array` + by; g != want {
			t.Errorf("got  %s\nwant %s", g, want)
		}
	})
	t.Run("select without key", func(t *testing.T) {
		got := load(t, `<select id="s" bind="tz" each="zones as z"/>`, map[string]any{"tz": nil, "zones": zones})
		g := one(t, got, "V017 f.tui:2:1 /screen/select id=s ")
		if want := `V017 f.tui:2:1 /screen/select id=s <select> needs key="path": bind holds the chosen option's key, which an index would not keep when the array changes` + by; g != want {
			t.Errorf("got  %s\nwant %s", g, want)
		}
	})
	t.Run("B012", func(t *testing.T) {
		got := load(t, `<select id="s" bind="tz" each="zones as z" key="z.id"/>`, map[string]any{"tz": "nope", "zones": zones})
		g := one(t, got, "B012 f.tui:2:1 /screen/select id=s ")
		if want := `B012 f.tui:2:1 /screen/select id=s bind="tz" holds "nope", which is no option's key; nothing is chosen, and a pick writes over it` + by; g != want {
			t.Errorf("got  %s\nwant %s", g, want)
		}
	})
	t.Run("B001", func(t *testing.T) {
		got := load(t, `<select id="s" bind="tz" each="zones as z" key="z.id"/>`, map[string]any{"tz": nil, "zones": 5})
		if g := one(t, got, "B001 f.tui:2:1 /screen/select id=s "); !strings.HasSuffix(g, by) {
			t.Errorf("B001 without the suffix: %s", g)
		}
	})
	t.Run("on:click is a self-check", func(t *testing.T) {
		got := load(t, `<select id="s" bind="tz" each="zones as z" key="z.id" on:click="go"/>`, map[string]any{"tz": nil, "zones": zones})
		if g := one(t, got, "V002 f.tui:2:55 /screen/select id=s "); strings.Contains(g, "placed by") {
			t.Errorf("a self-check has the suffix: %s", g)
		}
	})
	t.Run("the same select in the document", func(t *testing.T) {
		mem := fstest.MapFS{"app.tui": {Data: []byte(appWith(`<select id="s" bind="tz" each="zones as z" key="z.id"/>`))}}
		app, err := tuimark.LoadFS(mem, "app.tui")
		if err != nil {
			t.Fatal(err)
		}
		if err := app.Bind("", map[string]any{"tz": "nope", "zones": zones}); err != nil {
			t.Fatal(err)
		}
		got := diagTexts(app.Validate())
		if g := one(t, got, "B012 app.tui:3:5 /screen/select id=s "); strings.Contains(g, "placed by") {
			t.Errorf("a select the document writes has the suffix: %s", g)
		}
	})
}
