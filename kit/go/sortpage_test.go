package kit

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// serviceTable is kit/components/table.tui with its sample moved to "all",
// so SortPage can derive the "services" array the table reads.
func serviceTable(t *testing.T, size int) (*tuimark.App, *SortPage) {
	t.Helper()
	app := loadSample(t, "table")
	all, _ := app.Get("services")
	if err := app.Bind("", map[string]any{"all": all, "cur": "api", "count": 5.0}); err != nil {
		t.Fatal(err)
	}
	sp, err := NewSortPage(app, SortPageOptions{
		Source: "all", Out: "services", Meta: "svc_meta", State: "svc_state", Size: size,
		Columns: []Column{{"name", "SERVICE"}, {"state", "STATE"}, {"rps", "REQ/S"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return app, sp
}

// order is the services in the order they appear on the screen.
func order(t *testing.T, app *tuimark.App) []string {
	t.Helper()
	text := screen(t, app)
	type at struct {
		name string
		pos  int
	}
	var found []at
	for _, name := range []string{"api-gateway", "auth", "billing", "mailer", "search"} {
		if i := strings.Index(text, name); i >= 0 {
			found = append(found, at{name, i})
		}
	}
	for i := range found {
		for j := i + 1; j < len(found); j++ {
			if found[j].pos < found[i].pos {
				found[i], found[j] = found[j], found[i]
			}
		}
	}
	out := []string{}
	for _, f := range found {
		out = append(out, f.name)
	}
	return out
}

func TestSortPageDrivesTheTable(t *testing.T) {
	app, sp := serviceTable(t, 50)
	if got := order(t, app); !equal(got, []string{"api-gateway", "auth", "billing", "search", "mailer"}) {
		// the sample's own order is api, auth, billing, search, mailer
		t.Fatalf("source order on screen: %v", got)
	}
	if err := sp.Cycle("name"); err != nil { // ascending
		t.Fatal(err)
	}
	if got := order(t, app); !equal(got, []string{"api-gateway", "auth", "billing", "mailer", "search"}) {
		t.Errorf("ascending: %v", got)
	}
	_ = sp.Cycle("name") // descending
	if got := order(t, app); !equal(got, []string{"search", "mailer", "billing", "auth", "api-gateway"}) {
		t.Errorf("descending: %v", got)
	}
	_ = sp.Cycle("name") // back to the source order
	if got := order(t, app); got[0] != "api-gateway" || got[3] != "search" {
		t.Errorf("unsorted: %v", got)
	}
}

func TestSortPagePages(t *testing.T) {
	app, sp := serviceTable(t, 2)
	if got := order(t, app); !equal(got, []string{"api-gateway", "auth"}) {
		t.Fatalf("page 1: %v", got)
	}
	if label := getStr(t, app, "svc_meta.label"); label != "1–2 of 5" {
		t.Errorf("label = %q", label)
	}
	_ = sp.NextPage()
	if got := order(t, app); !equal(got, []string{"billing", "search"}) {
		t.Errorf("page 2: %v", got)
	}
	_ = sp.NextPage()
	_ = sp.NextPage() // stops at the last page
	if got := order(t, app); !equal(got, []string{"mailer"}) {
		t.Errorf("page 3: %v", got)
	}
	if label := getStr(t, app, "svc_meta.label"); label != "5–5 of 5" {
		t.Errorf("label = %q", label)
	}
	if getBool(t, app, "svc_meta.has_next") || !getBool(t, app, "svc_meta.has_prev") {
		t.Error("the last page has a previous page and no next")
	}
	// Sorting goes back to page 1; a bigger page shows everything.
	_ = sp.Cycle("name")
	if mustGet(t, app, "svc_meta.page") != 1.0 {
		t.Errorf("page after sorting = %v", mustGet(t, app, "svc_meta.page"))
	}
	_ = sp.SetSize(10)
	if got := order(t, app); len(got) != 5 {
		t.Errorf("size 10: %v", got)
	}
	_ = sp.SetPage(9) // clamped
	if mustGet(t, app, "svc_meta.page") != 1.0 {
		t.Errorf("page 9 of 1 = %v", mustGet(t, app, "svc_meta.page"))
	}
}

func TestSortPageTitlesCarryTheArrow(t *testing.T) {
	app, sp := serviceTable(t, 50)
	title := func(col string) string { return getStr(t, app, "svc_meta.titles."+col) }
	if title("name") != "SERVICE" {
		t.Errorf("idle title = %q", title("name"))
	}
	_ = sp.Cycle("name")
	if title("name") != "SERVICE ▲" || title("state") != "STATE" {
		t.Errorf("ascending titles: %q %q", title("name"), title("state"))
	}
	_ = sp.Cycle("name")
	if title("name") != "SERVICE ▼" {
		t.Errorf("descending title = %q", title("name"))
	}
	_ = sp.Cycle("state") // another column starts ascending
	if title("name") != "SERVICE" || title("state") != "STATE ▲" {
		t.Errorf("after switching: %q %q", title("name"), title("state"))
	}
	if err := sp.Cycle("nope"); err != nil {
		t.Errorf("an unknown column is ignored, not an error: %v", err)
	}
	if title("state") != "STATE ▲" {
		t.Error("an unknown column must not change the sort")
	}
}

// The order is cached between writes; it must be redone when what it was
// computed from changes, and must follow rows that changed in other fields.
func TestSortCacheFollowsTheSource(t *testing.T) {
	app, sp := serviceTable(t, 50)
	_ = sp.Cycle("name")
	all := mustGet(t, app, "all").([]any)
	// A change in a field other than the sort key: the order stays, the row updates.
	all[0].(map[string]any)["state"] = "stopped"
	_ = app.Set("all", all)
	if got := order(t, app); !equal(got, []string{"api-gateway", "auth", "billing", "mailer", "search"}) {
		t.Fatalf("after an unrelated field changed: %v", got)
	}
	// A change in the key: sorted again.
	all[0].(map[string]any)["name"] = "zzz-last"
	_ = app.Set("all", all)
	text := screen(t, app)
	if strings.Index(text, "zzz-last") < strings.Index(text, "search") {
		t.Errorf("a renamed row should move:\n%s", text)
	}
	// A row added.
	all = append(all, map[string]any{"id": "aaa", "name": "aaa-first", "state": "running", "ok": true, "region": "x", "rps": "1", "p95": "1"})
	_ = app.Set("all", all)
	if got := order(t, app); len(got) > 0 && got[0] == "api-gateway" {
		t.Errorf("an added row sorts in: %v", got)
	}
	if total := mustGet(t, app, "svc_meta.total"); total != 6.0 {
		t.Errorf("total = %v", total)
	}
}

func TestSortRowsKinds(t *testing.T) {
	r := []any{
		map[string]any{"id": "a", "v": 10.0},
		map[string]any{"id": "b", "v": 9.0},
		map[string]any{"id": "c"}, // no value: always last
		map[string]any{"id": "d", "v": 100.0},
	}
	if got := ids(SortRows(r, "v", false)); !equal(got, []string{"b", "a", "d", "c"}) {
		t.Errorf("numeric ascending: %v", got)
	}
	if got := ids(SortRows(r, "v", true)); !equal(got, []string{"d", "a", "b", "c"}) {
		t.Errorf("numeric descending: %v", got)
	}
	text := []any{
		map[string]any{"id": "1", "s": "banana"}, map[string]any{"id": "2", "s": "Apple"}, map[string]any{"id": "3", "s": "cherry"},
	}
	if got := ids(SortRows(text, "s", false)); !equal(got, []string{"2", "1", "3"}) {
		t.Errorf("text ignores case: %v", got)
	}
	if got := SortRows(text, "", false); ids(got)[0] != "1" || &got[0] == &text[0] {
		t.Error("no key keeps the order, in a copy")
	}
}

func TestPageLabel(t *testing.T) {
	for _, c := range []struct {
		page, size, total int
		want              string
	}{
		{1, 50, 340, "1–50 of 340"}, {7, 50, 340, "301–340 of 340"}, {1, 50, 0, "0 of 0"}, {1, 50, 3, "1–3 of 3"},
	} {
		if got := PageLabel(c.page, c.size, c.total); got != c.want {
			t.Errorf("PageLabel(%d,%d,%d) = %q, want %q", c.page, c.size, c.total, got, c.want)
		}
	}
}
