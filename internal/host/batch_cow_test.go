package host

import (
	"reflect"
	"strings"
	"testing"
)

// A Batch's trial copies only the containers along the paths it writes
// (cowSet) instead of the whole store, and never changes the store it
// started from: a failed batch, or one whose Derive fn panics, leaves
// every container of that store as it was, by identity and by content,
// and the list's cached rows, which hold its row maps, still show them.

const cowDoc = `<tui version="2"><screen id="s">
<text id="title">{cfg.title}</text>
<list id="l" each="rows as r" key="r.id" bind="cur"><item><text>{r.name}</text></item></list>
<tabs id="t" bind="view"><tab id="one" label="one"><text>first tab</text></tab><tab id="two" label="two"><input id="x"/></tab></tabs>
</screen></tui>`

func cowApp(t *testing.T) *App {
	t.Helper()
	a, err := Parse(strings.NewReader(cowDoc))
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]any, 6)
	for i := range rows {
		rows[i] = map[string]any{"id": float64(i), "name": "row " + string(rune('a'+i)), "tags": []any{"x", float64(i)}}
	}
	if err := a.Bind("", map[string]any{"rows": rows, "cur": 0.0, "cfg": map[string]any{"title": "T"}, "view": "one"}); err != nil {
		t.Fatal(err)
	}
	return a
}

// storeParts is the store's containers along the paths the tests write.
type storeParts struct {
	root, rows, row3, row4, tags3, cfg uintptr
}

func partsOf(t *testing.T, a *App) storeParts {
	t.Helper()
	root := a.store.(map[string]any)
	rows := root["rows"].([]any)
	row3 := rows[3].(map[string]any)
	return storeParts{
		root:  reflect.ValueOf(root).Pointer(),
		rows:  reflect.ValueOf(rows).Pointer(),
		row3:  reflect.ValueOf(row3).Pointer(),
		row4:  reflect.ValueOf(rows[4]).Pointer(),
		tags3: reflect.ValueOf(row3["tags"]).Pointer(),
		cfg:   reflect.ValueOf(root["cfg"]).Pointer(),
	}
}

func screenText(a *App) string {
	return strings.Join(a.Frame(40, 12).Dump(false).Grid, "\n")
}

func TestFailedBatchLeavesStoreContainersUntouched(t *testing.T) {
	a := cowApp(t)
	if s := screenText(a); !strings.Contains(s, "row d") || !strings.Contains(s, "first tab") {
		t.Fatalf("first frame:\n%s", s)
	}
	before, want := partsOf(t, a), deepCopyJSON(a.store)
	err := a.Batch(func(b *Batch) error {
		_ = b.Set("rows.3.name", "changed")
		_ = b.Set("rows.3.tags.1", "y")
		_ = b.Set("cfg.title", "new")
		_ = b.Set("@focus", "#x")         // activates tab two: writes view
		return b.Set("cfg.title.deep", 1) // cfg.title is a string by then
	})
	if err == nil {
		t.Fatal("the batch applied a write through a string")
	}
	if after := partsOf(t, a); after != before {
		t.Fatalf("the failed batch replaced store containers: %+v, want %+v", after, before)
	}
	if !reflect.DeepEqual(a.store, want) {
		t.Fatalf("the failed batch changed the store: %v", a.store)
	}
	if a.cow != nil {
		t.Fatal("the trial's cowSet outlived the batch")
	}
	if s := screenText(a); !strings.Contains(s, "row d") || strings.Contains(s, "changed") || !strings.Contains(s, "first tab") || !strings.Contains(s, "T") {
		t.Fatalf("the frame after a failed batch:\n%s", s)
	}
}

func TestPanickingDeriveBatchLeavesStoreContainersUntouched(t *testing.T) {
	a := cowApp(t)
	if err := a.Derive("count", func(get func(string) (any, bool)) (any, error) {
		if v, _ := get("rows.3.name"); v == "boom" {
			panic("boom")
		}
		rows, _ := get("rows")
		return float64(len(rows.([]any))), nil
	}); err != nil {
		t.Fatal(err)
	}
	screenText(a)
	before, want := partsOf(t, a), deepCopyJSON(a.store)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("the Derive fn's panic did not propagate")
			}
		}()
		_ = a.Batch(func(b *Batch) error {
			_ = b.Set("rows.3.tags.0", "z")
			_ = b.Set("cfg.title", "new")
			return b.Set("rows.3.name", "boom")
		})
	}()
	if after := partsOf(t, a); after != before {
		t.Fatalf("the panicking batch replaced store containers: %+v, want %+v", after, before)
	}
	if !reflect.DeepEqual(a.store, want) {
		t.Fatalf("the panicking batch changed the store: %v", a.store)
	}
	if a.cow != nil || a.deriveHold != 0 {
		t.Fatalf("the trial's state outlived the batch: cow %v, hold %d", a.cow, a.deriveHold)
	}
	if s := screenText(a); !strings.Contains(s, "row d") || strings.Contains(s, "boom") {
		t.Fatalf("the frame after a panicking batch:\n%s", s)
	}
}

// A batch that applies leaves the store it started from untouched too: it
// keeps copies of the containers it wrote, and shares every other one.
func TestAppliedBatchCopiesOnlyWrittenContainers(t *testing.T) {
	a := cowApp(t)
	screenText(a)
	before := partsOf(t, a)
	oldRoot := a.store.(map[string]any)
	oldRow3 := oldRoot["rows"].([]any)[3].(map[string]any)
	if err := a.Batch(func(b *Batch) error {
		_ = b.Set("rows.3.name", "changed")
		_ = b.Set("rows.3.name", "changed again") // the copy, written again
		return b.Set("rows.3.tags.1", "y")
	}); err != nil {
		t.Fatal(err)
	}
	after := partsOf(t, a)
	if after.root == before.root || after.rows == before.rows || after.row3 == before.row3 || after.tags3 == before.tags3 {
		t.Fatalf("the batch wrote a container of the store it started from: %+v vs %+v", after, before)
	}
	if after.row4 != before.row4 || after.cfg != before.cfg {
		t.Fatalf("the batch copied containers it did not write: %+v vs %+v", after, before)
	}
	if oldRow3["name"] != "row d" || oldRow3["tags"].([]any)[1] != 3.0 {
		t.Fatalf("the old row 3 was changed in place: %v", oldRow3)
	}
	if v, _ := a.Get("rows.3"); !reflect.DeepEqual(v, map[string]any{"id": 3.0, "name": "changed again", "tags": []any{"x", "y"}}) {
		t.Fatalf("rows.3 = %v", v)
	}
	if s := screenText(a); !strings.Contains(s, "changed again") || strings.Contains(s, "row d") {
		t.Fatalf("the frame after the batch:\n%s", s)
	}
	// A Set after the batch writes the batch's store in place, as before.
	if err := a.Set("rows.3.name", "set"); err != nil {
		t.Fatal(err)
	}
	if s := screenText(a); !strings.Contains(s, "set") {
		t.Fatalf("the frame after a Set:\n%s", s)
	}
}

// unassignIn through a cowSet copies the containers on its path and
// leaves the originals as they were; without one it deletes in place.
func TestUnassignInCopiesOnWrite(t *testing.T) {
	mk := func() map[string]any {
		return map[string]any{"a": []any{map[string]any{"b": map[string]any{"c": 1.0, "d": 2.0}}}, "e": map[string]any{"f": 1.0}}
	}
	orig := mk()
	cow := cowSet{}
	got := unassignIn(orig, "a.0.b.c", cow)
	if !reflect.DeepEqual(orig, mk()) {
		t.Fatalf("unassignIn changed the original: %v", orig)
	}
	want := mk()
	delete(want["a"].([]any)[0].(map[string]any)["b"].(map[string]any), "c")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unassignIn = %v, want %v", got, want)
	}
	if reflect.ValueOf(got.(map[string]any)["e"]).Pointer() != reflect.ValueOf(orig["e"]).Pointer() {
		t.Fatal("unassignIn copied a container off its path")
	}
	// Missing members, and a path through a scalar or past an array, are
	// no-ops that copy nothing.
	for _, p := range []string{"x.y", "a.5.b", "e.f.g", "a.0", "e.zz"} {
		n := len(cow)
		if out := unassignIn(orig, p, cow); reflect.ValueOf(out).Pointer() != reflect.ValueOf(orig).Pointer() || len(cow) != n {
			t.Fatalf("unassignIn(%q) copied something", p)
		}
	}
	inPlace := mk()
	if out := unassignIn(inPlace, "e.f", nil); reflect.ValueOf(out).Pointer() != reflect.ValueOf(inPlace).Pointer() {
		t.Fatal("unassignIn without a cowSet returned another root")
	}
	if _, ok := inPlace["e"].(map[string]any)["f"]; ok {
		t.Fatal("unassignIn without a cowSet did not delete in place")
	}
}
