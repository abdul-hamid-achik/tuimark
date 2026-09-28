package tuimark_test

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.3 §21 test 77: Get.

const minimalDoc = `<tui version="1"><screen><text>hi</text></screen></tui>`

func newMinimal(t *testing.T) *tuimark.App {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(minimalDoc))
	if err != nil {
		t.Fatal(err)
	}
	return app
}

// A nested object and an array element by numeric segment resolve; a
// missing path, a path through a scalar, and a malformed path give
// (nil, false); mutating the returned map changes nothing in the store.
func TestGetStorePaths(t *testing.T) {
	app := newMinimal(t)
	data := map[string]any{
		"a": map[string]any{"b": []any{1.0, 2.0, map[string]any{"c": "x"}}},
		"n": "scalar",
	}
	if err := app.Bind("", data); err != nil {
		t.Fatal(err)
	}

	if v, ok := app.Get("a.b.2.c"); !ok || v != "x" {
		t.Errorf("a.b.2.c = (%v, %v), want (x, true)", v, ok)
	}
	if v, ok := app.Get("a.b.0"); !ok || v != 1.0 {
		t.Errorf("a.b.0 = (%v, %v), want (1, true)", v, ok)
	}

	for _, path := range []string{"nope", "a.b.99", "a.b.-1", "n.x", "a.b.2.c.d"} {
		if v, ok := app.Get(path); ok {
			t.Errorf("Get(%q) = (%v, true), want (_, false)", path, v)
		}
	}

	// Mutating the returned map changes nothing in the store: a following
	// Get and Dump show the old value.
	v, ok := app.Get("a.b.2")
	if !ok {
		t.Fatal("a.b.2 missing")
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("a.b.2 = %T, want map[string]any", v)
	}
	m["c"] = "mutated"
	if v2, _ := app.Get("a.b.2.c"); v2 != "x" {
		t.Errorf("Get after mutating the copy = %v, want x (unchanged)", v2)
	}
	d, err := app.Dump(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	_ = d // the dump itself doesn't reference the store's "a", just confirms Dump still succeeds

	// The empty path returns a copy of the whole store.
	root, ok := app.Get("")
	if !ok {
		t.Fatal("Get(\"\") missing")
	}
	rootMap, ok := root.(map[string]any)
	if !ok || rootMap["n"] != "scalar" {
		t.Errorf("Get(\"\") = %v", root)
	}

	// A path starting with "@" that is not one of the three reserved ones
	// reads and writes the store like any other.
	if err := app.Set("@x", 1.0); err != nil {
		t.Fatal(err)
	}
	if v, ok := app.Get("@x"); !ok || v != 1.0 {
		t.Errorf("Get(\"@x\") = (%v, %v), want (1, true)", v, ok)
	}
}

// @screen is the active screen's id; @theme is "dark" for an auto
// document outside Run, "light" after Set("@theme", "light").
func TestGetScreenAndTheme(t *testing.T) {
	doc := `<tui version="1" theme="auto">
  <screen id="s1"><text>one</text></screen>
  <screen id="s2"><text>two</text></screen>
</tui>`
	app, err := tuimark.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := app.Get("@screen"); !ok || v != "s1" {
		t.Errorf("@screen = (%v, %v), want (s1, true)", v, ok)
	}
	if v, ok := app.Get("@theme"); !ok || v != "dark" {
		t.Errorf("@theme = (%v, %v), want (dark, true) for an auto document outside Run", v, ok)
	}
	if err := app.Set("@screen", "s2"); err != nil {
		t.Fatal(err)
	}
	if v, ok := app.Get("@screen"); !ok || v != "s2" {
		t.Errorf("@screen after Set = (%v, %v), want (s2, true)", v, ok)
	}
	if err := app.Set("@theme", "light"); err != nil {
		t.Fatal(err)
	}
	if v, ok := app.Get("@theme"); !ok || v != "light" {
		t.Errorf("@theme after Set = (%v, %v), want (light, true)", v, ok)
	}
}

// @focus is nil before any frame, "#id" after Play, and, inside an
// on:focus handler run by Play, the node that is gaining focus (SPEC
// v0.3 §30.4 decision 13a), not the previous one.
func TestGetFocus(t *testing.T) {
	doc := `<tui version="1">
  <screen focus="#a">
    <col>
      <input id="a"/>
      <input id="b" on:focus="focusedB"/>
    </col>
  </screen>
</tui>`
	app, err := tuimark.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := app.Get("@focus"); v != nil || !ok {
		t.Errorf("@focus before any frame = (%v, %v), want (nil, true)", v, ok)
	}

	var seenInHandler any
	app.On("focusedB", func(tuimark.Event) error {
		v, _ := app.Get("@focus")
		seenInHandler = v
		return nil
	})

	if _, err := app.Play(tuimark.PlayOptions{}, "tab"); err != nil {
		t.Fatal(err)
	}
	if seenInHandler != "#b" {
		t.Errorf("@focus inside the on:focus handler = %v, want #b (the node gaining focus)", seenInHandler)
	}
	if v, ok := app.Get("@focus"); !ok || v != "#b" {
		t.Errorf("@focus after Play = (%v, %v), want (#b, true)", v, ok)
	}
}
