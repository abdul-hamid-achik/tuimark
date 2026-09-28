package tuimark_test

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.3 §21 test 82: Play state. Two Play calls continue from each
// other (focus, list cursor, input text). A handler that writes with Set
// from another goroutine after returning is shown by a following
// Play(opts) with no steps. Dump between the calls changes nothing.

const playStateDoc = `<tui version="1">
  <screen focus="#q">
    <col>
      <input id="q" bind="query"/>
      <list id="l" each="rows as r" key="r" bind="sel">
        <item><text>{r}</text></item>
      </list>
    </col>
  </screen>
</tui>`

func TestPlayStateAcrossCalls(t *testing.T) {
	app, err := tuimark.Parse(strings.NewReader(playStateDoc))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", map[string]any{"rows": []any{"a", "b", "c"}, "query": "", "sel": nil}); err != nil {
		t.Fatal(err)
	}
	opts := tuimark.PlayOptions{Cols: 40, Rows: 10}

	// First call: type "hi" into the focused input, then move to the
	// list and down once.
	res1, err := app.Play(opts, "text:hi", "tab", "down")
	if err != nil {
		t.Fatal(err)
	}
	focus1 := res1.Dump.Focus
	if focus1 == nil || *focus1 != "l" {
		t.Fatalf("focus after call 1 = %v, want l", focus1)
	}

	// A Dump between the calls changes nothing (it is a snapshot).
	dBetween, err := app.Dump(40, 10)
	if err != nil {
		t.Fatal(err)
	}
	if dBetween.Focus == nil || *dBetween.Focus != "l" {
		t.Fatalf("Dump between calls moved focus: %v", dBetween.Focus)
	}

	// Second call continues from the first: the input still holds "hi",
	// the list cursor is still on row 1 (moving down again lands on row
	// 2), and focus is still on the list.
	res2, err := app.Play(opts, "down")
	if err != nil {
		t.Fatal(err)
	}
	if res2.Dump.Focus == nil || *res2.Dump.Focus != "l" {
		t.Fatalf("focus after call 2 = %v, want l (continued from call 1)", res2.Dump.Focus)
	}
	// The selected row is the <item> wrapper (its Key is the row's each
	// key, "c" for the third row); the row's own <text> is a separate,
	// unselected child node.
	var selectedKey string
	for _, n := range res2.Dump.Nodes {
		if n.Tag == "item" && n.Selected {
			selectedKey = n.Key
		}
	}
	if selectedKey != "c" {
		t.Fatalf("selected row after two downs from row 0 = %q, want c (state continued across calls)", selectedKey)
	}
	if v, ok := app.Get("query"); !ok || v != "hi" {
		t.Fatalf("query = (%v, %v), want (hi, true): the input's text continued across calls", v, ok)
	}

	// A handler that writes with Set from another goroutine after
	// returning is shown by a following Play(opts) with no steps: an
	// empty Play settles the app (step 0), which is exactly what "from
	// another goroutine" needs, since nothing else redraws it headless.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = app.Set("query", "async")
	}()
	<-done
	res3, err := app.Play(opts)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range res3.Dump.Nodes {
		if n.ID == "" && n.Text == "async" {
			found = true
		}
	}
	// The input itself shows the async value as its Text (id "q").
	for _, n := range res3.Dump.Nodes {
		if n.ID == "q" && n.Text == "async" {
			found = true
		}
	}
	if !found {
		t.Errorf("an empty Play did not show the async write: nodes=%+v", res3.Dump.Nodes)
	}
}
