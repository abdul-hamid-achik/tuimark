package lsp

import (
	"path/filepath"
	"testing"
)

// A stylesheet a document stops loading and loads again is republished: it
// gets the document's diagnostics back (typing a <style src> passes through
// exactly this).
func TestStylesheetLoadedAgainIsRepublished(t *testing.T) {
	fc := &fakeClock{}
	srv := newServer(Options{})
	srv.afterFunc = fc.afterFunc
	c := startWith(t, srv)
	c.initialize()
	doc := func(src string) string {
		return `<tui version="1"><style src="` + src + `"/><screen id="s"><text>x</text></screen></tui>` + "\n"
	}
	dir := writeTree(t, map[string]string{
		"t.tcss": "screen { bogus-prop: 1; }\n",
		"x.tcss": "screen { color: red; }\n",
	})
	docURI := c.open(filepath.Join(dir, "a.tui"), "tuimark", doc("t.tcss"))
	sheetURI := pathToURI(filepath.Join(dir, "t.tcss"))
	sync := func() {
		c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": docURI}, "position": Position{}})
	}
	// fireAll runs the timers until none is left (a publish may schedule one).
	fireAll := func() {
		sync()
		for i := 0; i < 4 && fc.fire() > 0; i++ {
			sync()
		}
	}

	fireAll()
	c.nextPublish(docURI)
	if p := c.nextPublish(sheetURI); len(p.Diagnostics) != 1 {
		t.Fatalf("t.tcss loaded: %+v, want its V003", p.Diagnostics)
	}

	// The document names x.tcss instead: t.tcss is cleared.
	c.change(docURI, 2, doc("x.tcss"))
	fireAll()
	if p := c.nextPublish(sheetURI); len(p.Diagnostics) != 0 {
		t.Fatalf("t.tcss no longer loaded: %+v, want none", p.Diagnostics)
	}

	// And back: t.tcss was published before, and the old list does not have
	// it, so only the document's own change can say it is loaded again.
	c.change(docURI, 3, doc("t.tcss"))
	fireAll()
	p := c.nextPublish(sheetURI)
	if len(p.Diagnostics) != 1 || p.Diagnostics[0].Code != "V003" {
		t.Errorf("t.tcss loaded again: %+v, want its V003 back", p.Diagnostics)
	}
}
