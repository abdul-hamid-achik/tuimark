package host

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// staleDoc puts the select of SPEC v0.4b §6.17.7 in a two-row column
// whose texts appear with more, so a host write pushes the select out of
// view and step 7a closes its open list (decision 66), above a hints row
// whose q row the open list hides.
const staleDoc = `<tui>
  <keymap>
    <bind keys="tab" action="next" label="next"/>
    <bind keys="q" action="quit" label="quit"/>
  </keymap>
  <screen id="main" focus="#tz">
    <col height="2" style="overflow: hidden">
      <text if="more">one</text>
      <text if="more">two</text>
      <select id="tz" bind="tz" each="zones as z" key="z.id" label="{z.name}"/>
    </col>
    <checkbox id="c" label="C"/>
    <hints/>
  </screen>
</tui>`

func staleData() map[string]any {
	zones := []any{
		map[string]any{"id": "utc", "name": "UTC"},
		map[string]any{"id": "lis", "name": "Lisbon"},
	}
	return map[string]any{"tz": "lis", "more": false, "zones": zones}
}

// When step 7a closes the list, the frame Run shows is built again with
// it closed: the hints row is the one a frame built with the list closed
// from the start has, not the open list's, also when no input follows the
// host write.
func TestOptionsClosedAt7aRebuildTheFrame(t *testing.T) {
	const hints = "tab next  q quit    "
	cold := doc(t, staleDoc)
	data := staleData()
	data["more"] = true
	if err := cold.Bind("", data); err != nil {
		t.Fatal(err)
	}
	if got := cold.Frame(20, 8).Grid.Lines()[3]; got != hints {
		t.Fatalf("cold frame: hints %q, want %q", got, hints)
	}

	a := doc(t, staleDoc)
	if err := a.Bind("", staleData()); err != nil {
		t.Fatal(err)
	}
	r := startTool(t, a, 20, 8, HostHooks{}, nil)
	r.waitOut(t, "Lisbon")
	r.send(t, "\r")
	waitUntil(t, "the list to open", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.opts.id == "tz"
	})
	if err := a.Set("more", true); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the list to close", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.opts.id == "" && a.last != nil && strings.HasPrefix(a.last.Grid.Lines()[0], "one")
	})
	if got := strings.Split(screenOf(a), "\n")[3]; got != hints {
		t.Errorf("live frame after the close: hints %q, want %q", got, hints)
	}
	r.in.Close()
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
}

// Only a list row or an each template makes a select nested (shown with
// its placeholder alone, SPEC v0.4b §6.8 item 12); an alias scope by
// itself does not.
func TestSelectNestingIsTheRowNotTheScope(t *testing.T) {
	a := doc(t, `<tui><screen id="main"><select id="tz" each="zones as z" key="z" placeholder="zone"/></screen></tui>`)
	if err := a.Bind("", map[string]any{"zones": []any{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	n := a.doc.Screens[0].Children[0]
	if n.Kind != "select" {
		t.Fatalf("first child is %s", n.Kind)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	fb := &builder{a: a, seen: map[string]bool{}, byID: map[string]*layout.Box{}, rc: &renderCtx{tabDone: map[string]bool{}}}
	sc := &scope{alias: "d", value: map[string]any{"x": 1}}
	if b := fb.inflate(n, nil, sc, false); b == nil || b.Rows != 2 || b.Placeholder != "zone" {
		t.Errorf("under an alias scope: %v", boxSummary(b))
	}
	if b := fb.inflate(n, nil, sc, true); b == nil || b.Rows != 0 || b.Placeholder != "zone" {
		t.Errorf("in a row: %v", boxSummary(b))
	}
}

func boxSummary(b *layout.Box) any {
	if b == nil {
		return nil
	}
	return struct {
		Rows        int
		Placeholder string
	}{b.Rows, b.Placeholder}
}
