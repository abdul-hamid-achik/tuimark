package host

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// SPEC v0.2b §18 step 1: a tabs' tab nodes are created, but not their
// content (only the active tab's content is inflated, in step 3), so
// nothing inside a tab is resolved; and a table's column and row
// templates are templates, never inflated as they stand.
func TestStepOneInflatesTabNodesOnly(t *testing.T) {
	a := doc(t, `<tui version="2"><screen id="s">
<tabs id="n"><tab id="a" label="a"><text>{missing.one}</text></tab><tab id="b" label="b" class:hot="missing.two"/></tabs>
<table id="t" each="rows as r" key="r"><item class:x="r.x"/><column>{r.y}</column></table>
</screen></tui>`)
	_ = a.Bind("rows", []any{})
	f := a.Frame(20, 4)
	var bind []string
	for _, d := range f.Diags {
		bind = append(bind, d.String())
	}
	joined := strings.Join(bind, "\n")
	if strings.Contains(joined, "missing.one") || strings.Contains(joined, "r.x") || strings.Contains(joined, "r.y") {
		t.Errorf("tab content or table templates were inflated:\n%s", joined)
	}
	if !strings.Contains(joined, "missing.two") {
		t.Errorf("a tab node's own guard is evaluated in step 1:\n%s", joined)
	}
	tabs := f.ByID["n"]
	if tabs == nil || len(tabs.Children) != 2 || len(tabs.Children[0].Children) != 0 {
		t.Fatalf("tabs %+v", tabs)
	}
}

// SPEC v0.2b §10.1, §18 step 2: while a tab's own style is computed,
// :empty and :focus-within never match it; as a parent in a selector they
// match normally.
func TestTabOwnStyleIgnoresEmptyAndFocusWithin(t *testing.T) {
	a := doc(t, `<tui version="2"><style>tab:empty { display: none; } tab:focus-within { bold: true; } tabs > tab { italic: true; }</style>
<screen id="s"><tabs id="n"><tab id="a" label="a"/></tabs></screen></tui>`)
	f := a.Frame(10, 2)
	tabs := f.ByID["n"]
	if tabs == nil || len(tabs.Children) != 1 {
		t.Fatal("tab:empty dropped the tab while its own style was computed")
	}
	tab := tabs.Children[0]
	if tab.Style.Bold || !tab.Style.Italic {
		t.Errorf("tab style %+v", tab.Style)
	}
	if !tab.HasPseudo("empty") || subjectOf(tab).HasPseudo("empty") {
		t.Error(":empty must hold for the tab everywhere but its own cascade")
	}
}

// SPEC v0.2b §15.7 / §8.5 steps 2-3: the hit node of a cell descends from
// the owner through anonymous children; a cell in a modal starts in the
// modal; an id repeated in list rows picks the last node containing the
// cell; layout paths index same-tag siblings without ids.
func TestHitNodeAndLayoutPath(t *testing.T) {
	a := doc(t, `<tui version="2"><screen id="s">
<list id="l" each="xs as x" key="x"><item><row><text id="nm">{x}</text><text>.</text></row></item></list>
<text>tail</text><text>end</text>
<modal id="m" open="true" style="width: 10; height: 3"><text>in modal</text></modal>
</screen></tui>`)
	_ = a.Bind("xs", []any{"a", "b", "c"})
	f := a.Frame(20, 8)
	check := func(x, y int, wantTag, wantPath string) {
		t.Helper()
		b := HitNode(f, x, y)
		if b == nil {
			t.Fatalf("(%d,%d): no hit node", x, y)
		}
		if b.Tag != wantTag || LayoutPath(f, b) != wantPath {
			t.Errorf("(%d,%d): %s %s, want %s %s", x, y, b.Tag, LayoutPath(f, b), wantTag, wantPath)
		}
	}
	check(0, 1, "text", "/screen#s/list#l/item[1]/row/text#nm")
	check(1, 0, "text", "/screen#s/list#l/item[0]/row/text[1]")
	check(0, 3, "text", "/screen#s/text[0]")
	check(0, 4, "text", "/screen#s/text[1]")
	m := f.ByID["m"]
	check(m.Content.X, m.Content.Y, "text", "/screen#s/modal#m/text")
	check(m.X, m.Y, "modal", "/screen#s/modal#m")
	check(19, 7, "screen", "/screen#s")
}

// SPEC v0.2b §21 test 72 (part): Set("@theme") and store Sets from
// goroutines while a session renders are race-free (run with -race).
func TestSetThemeWhileRendering(t *testing.T) {
	r := startSession(t, autoDoc, &termSession{})
	r.next(t)
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_ = r.app.Set("@theme", []string{"dark", "light", "auto"}[(g+i)%3])
				_ = r.app.Set("text", strings.Repeat("x", i%5))
				_ = r.app.Dump(20, 3, false)
			}
		}(g)
	}
	wg.Wait()
	time.Sleep(20 * time.Millisecond)
	r.send(t, "\x03")
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
}

// SPEC v0.2b §8.4: a hyphenated built-in is built in (Catalog lists it
// with Builtin: true and it is never B004); this build does not run the
// built-ins yet, so a row naming one never matches and the key goes on to
// the next row, as for a row whose target is missing — it is never
// dispatched as a host action.
func TestVersionTwoBuiltinsNeverDispatchAsHostActions(t *testing.T) {
	a := doc(t, `<tui version="2"><keymap><bind keys="j" action="move-next"/><bind keys="j" action="later"/><bind keys="k" action="check-all"/></keymap>
<screen id="s"><text>x</text></screen></tui>`)
	a.Frame(10, 2)
	evs := a.HandleKey(Key{Name: "j", Rune: 'j'})
	if len(evs) != 1 || evs[0].Action != "later" {
		t.Errorf("j fired %+v, want the next row", evs)
	}
	if evs := a.HandleKey(Key{Name: "k", Rune: 'k'}); len(evs) != 0 {
		t.Errorf("k fired %+v", evs)
	}
	for _, s := range a.Catalog() {
		if (s.Name == "move-next" || s.Name == "check-all") != s.Builtin && s.Name != "later" {
			t.Errorf("catalog %+v", s)
		}
	}
	if d := a.CheckCatalog(map[string]bool{"later": true}); len(d) != 0 {
		t.Errorf("B004 for a built-in: %v", d)
	}
}
