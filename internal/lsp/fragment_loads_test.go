package lsp

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Phase F4 review: an edit of a fragment that changes which files the
// documents reading it read republishes every file that change moves,
// the ones they no longer read included; finding the documents that read
// a fragment stays cheap in a directory of many documents; and a
// fragment no document reads resolves its own templates and ids.

// clockClient is a server on a fake clock and the function that fires
// its debounced publishes until none is left (each round may schedule
// more: a publish notes the files it moves).
func clockClient(t *testing.T) (*client, func(uri string)) {
	t.Helper()
	fc := &fakeClock{}
	srv := newServer(Options{})
	srv.afterFunc = fc.afterFunc
	c := startWith(t, srv)
	c.initialize()
	settle := func(uri string) {
		hover := func() {
			c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": Position{}})
		}
		hover()
		for i := 0; i < 8 && fc.fire() > 0; i++ {
			hover()
		}
	}
	return c, settle
}

// lastCodes is the diagnostics of the last publish of uri since the last
// call, and whether there was one.
func lastCodes(c *client, uri string) ([]string, bool) {
	ps := c.publishesOf(uri)
	if len(ps) == 0 {
		return nil, false
	}
	return codesAt(ps[len(ps)-1].Diagnostics), true
}

func sameCodes(got []string, want ...string) bool {
	return strings.Join(got, ",") == strings.Join(want, ",")
}

// app reads a.tui, which reads b.tui; b.tui's id repeats one of app's, so
// b.tui shows a V004. When a.tui drops its include of b.tui, b.tui is
// read by nothing, and is published again with its own check: whether or
// not app.tui is open.
func TestFragmentDroppingANestedIncludeRepublishesIt(t *testing.T) {
	files := map[string]string{
		"ui/app.tui":     "<tui>\n  <screen id=\"main\">\n    <text id=\"x\">top</text>\n    <include src=\"parts/a.tui\"/>\n  </screen>\n</tui>\n",
		"ui/parts/a.tui": "<fragment>\n  <text>a</text>\n  <include src=\"b.tui\"/>\n</fragment>\n",
		"ui/parts/b.tui": "<fragment>\n  <text id=\"x\">b</text>\n</fragment>\n",
	}
	for _, appOpen := range []bool{true, false} {
		dir := writeTree(t, files)
		c, settle := clockClient(t)
		if appOpen {
			c.open(filepath.Join(dir, "ui/app.tui"), "tuimark", files["ui/app.tui"])
		}
		aURI := c.open(filepath.Join(dir, "ui/parts/a.tui"), "tuimark", files["ui/parts/a.tui"])
		bURI := c.open(filepath.Join(dir, "ui/parts/b.tui"), "tuimark", files["ui/parts/b.tui"])
		settle(aURI)
		if got, ok := lastCodes(c, bURI); !ok || !sameCodes(got, "V004 1:2") {
			t.Fatalf("app open %v: b.tui first: %v %v, want [V004 1:2]", appOpen, got, ok)
		}
		c.change(aURI, 2, strings.Replace(files["ui/parts/a.tui"], `<include src="b.tui"/>`, "", 1))
		settle(aURI)
		if got, ok := lastCodes(c, bURI); !ok || len(got) != 0 {
			t.Errorf("app open %v: b.tui after a.tui dropped it: %v (published %v), want []", appOpen, got, ok)
		}
		// And back: a.tui reads it again.
		c.change(aURI, 3, files["ui/parts/a.tui"])
		settle(aURI)
		if got, ok := lastCodes(c, bURI); !ok || !sameCodes(got, "V004 1:2") {
			t.Errorf("app open %v: b.tui read again: %v (published %v), want [V004 1:2]", appOpen, got, ok)
		}
		_ = c.shutdown()
	}
}

// A stylesheet a fragment loads gets the diagnostics of the documents
// that read the fragment, wherever they are (app.tui is in the parent
// directory, open or not); when the fragment drops its <style src>, the
// stylesheet is published again, with nothing.
func TestFragmentDroppingAStylesheetRepublishesIt(t *testing.T) {
	files := map[string]string{
		"ui/app.tui":        "<tui>\n  <include src=\"parts/kit.tui\"/>\n  <screen id=\"main\">\n    <text>top</text>\n  </screen>\n</tui>\n",
		"ui/parts/kit.tui":  "<fragment>\n  <style src=\"kit.tcss\"/>\n</fragment>\n",
		"ui/parts/kit.tcss": "screen { bogus-prop: 1; }\n",
	}
	for _, appOpen := range []bool{true, false} {
		dir := writeTree(t, files)
		c, settle := clockClient(t)
		if appOpen {
			c.open(filepath.Join(dir, "ui/app.tui"), "tuimark", files["ui/app.tui"])
		}
		kitURI := c.open(filepath.Join(dir, "ui/parts/kit.tui"), "tuimark", files["ui/parts/kit.tui"])
		sheetURI := c.open(filepath.Join(dir, "ui/parts/kit.tcss"), "tcss", files["ui/parts/kit.tcss"])
		settle(kitURI)
		if got, ok := lastCodes(c, sheetURI); !ok || !sameCodes(got, "V003 0:9") {
			t.Fatalf("app open %v: kit.tcss first: %v %v, want [V003 0:9]", appOpen, got, ok)
		}
		c.change(kitURI, 2, "<fragment>\n</fragment>\n")
		settle(kitURI)
		if got, ok := lastCodes(c, sheetURI); !ok || len(got) != 0 {
			t.Errorf("app open %v: kit.tcss after kit.tui dropped it: %v (published %v), want []", appOpen, got, ok)
		}
		_ = c.shutdown()
	}
}

// A fragment opened with a text that differs from the file changes what
// the document reading it reads, though nothing open had read the
// fragment's buffer: the stylesheet only the file named is dropped by
// app.tui's publish, and published again with nothing.
func TestOpeningAFragmentRepublishesWhatItsReaderDrops(t *testing.T) {
	files := map[string]string{
		"ui/app.tui":        "<tui>\n  <include src=\"parts/kit.tui\"/>\n  <screen id=\"main\">\n    <text>top</text>\n  </screen>\n</tui>\n",
		"ui/parts/kit.tui":  "<fragment>\n  <style src=\"kit.tcss\"/>\n</fragment>\n",
		"ui/parts/kit.tcss": "screen { bogus-prop: 1; }\n",
	}
	dir := writeTree(t, files)
	c, settle := clockClient(t)
	appURI := c.open(filepath.Join(dir, "ui/app.tui"), "tuimark", files["ui/app.tui"])
	sheetURI := c.open(filepath.Join(dir, "ui/parts/kit.tcss"), "tcss", files["ui/parts/kit.tcss"])
	settle(appURI)
	if got, ok := lastCodes(c, sheetURI); !ok || !sameCodes(got, "V003 0:9") {
		t.Fatalf("kit.tcss first: %v %v, want [V003 0:9]", got, ok)
	}
	c.open(filepath.Join(dir, "ui/parts/kit.tui"), "tuimark", "<fragment>\n</fragment>\n")
	settle(appURI)
	if got, ok := lastCodes(c, sheetURI); !ok || len(got) != 0 {
		t.Errorf("kit.tcss once kit.tui's buffer drops it: %v (published %v), want []", got, ok)
	}
	_ = c.shutdown()
}

// A fragment's diagnostics, routed from every document that reads it,
// are told apart as validate tells them apart, by what its cut drops too
// (F3's diagnostic identity): two that read the same once cut stay two,
// and the same one from two documents is one.
func TestFragmentDiagnosticsKeepWhatACutMerged(t *testing.T) {
	long := strings.Repeat("a", 70)
	app := "<tui>\n  <screen id=\"main\">\n    <include src=\"kit.tui\"/>\n  </screen>\n</tui>\n"
	kit := "<fragment>\n  <template id=\"t\"><text>{f." + long + "1} {f." + long + "2}</text></template>\n" +
		"  <use id=\"u\" template=\"t\" with=\"x as f\"/>\n</fragment>\n"
	dir := writeTree(t, map[string]string{"one.tui": app, "two.tui": app, "kit.tui": kit, "sample.json": `{"x": {}}`})
	c := startServer(t, Options{})
	c.initialize()
	p := c.nextPublish(c.open(filepath.Join(dir, "kit.tui"), "tuimark", kit))
	var b003 []lspDiagnostic
	for _, d := range p.Diagnostics {
		if d.Code == "B003" {
			b003 = append(b003, d)
		}
	}
	if len(b003) != 2 || b003[0].Message != b003[1].Message {
		t.Errorf("kit.tui: %d B003 %+v, want 2 that read the same", len(b003), b003)
	}
	_ = c.shutdown()
}

// Two fragments read side by side by a closed app.tui: an id a.tui gains
// makes b.tui's the second one, a V004 on b.tui, and losing it again
// clears it.
func TestFragmentEditRepublishesItsSiblings(t *testing.T) {
	files := map[string]string{
		"ui/app.tui":     "<tui>\n  <screen id=\"main\">\n    <include src=\"parts/a.tui\"/>\n    <include src=\"parts/b.tui\"/>\n  </screen>\n</tui>\n",
		"ui/parts/a.tui": "<fragment>\n  <text>a</text>\n</fragment>\n",
		"ui/parts/b.tui": "<fragment>\n  <text id=\"x\">b</text>\n</fragment>\n",
	}
	dir := writeTree(t, files)
	c, settle := clockClient(t)
	aURI := c.open(filepath.Join(dir, "ui/parts/a.tui"), "tuimark", files["ui/parts/a.tui"])
	bURI := c.open(filepath.Join(dir, "ui/parts/b.tui"), "tuimark", files["ui/parts/b.tui"])
	settle(aURI)
	if got, ok := lastCodes(c, bURI); !ok || len(got) != 0 {
		t.Fatalf("b.tui first: %v %v, want []", got, ok)
	}
	c.change(aURI, 2, "<fragment>\n  <text id=\"x\">a</text>\n</fragment>\n")
	settle(aURI)
	if got, ok := lastCodes(c, bURI); !ok || !sameCodes(got, "V004 1:2") {
		t.Errorf("b.tui after a.tui took its id: %v (published %v), want [V004 1:2]", got, ok)
	}
	c.change(aURI, 3, files["ui/parts/a.tui"])
	settle(aURI)
	if got, ok := lastCodes(c, bURI); !ok || len(got) != 0 {
		t.Errorf("b.tui after a.tui gave it back: %v (published %v), want []", got, ok)
	}
	_ = c.shutdown()
}

// An edit of a fragment beside many documents that read other fragments
// checks only the document that reads it: the others are told apart by
// their includes, not by an analysis each (which lays a document out at
// three widths). The publish costs far less than analyzing them would.
func TestFragmentIncludersAreFoundCheaply(t *testing.T) {
	const n = 120
	files := map[string]string{
		"frag.tui":   "<fragment>\n  <text>frag</text>\n</fragment>\n",
		"other.tui":  "<fragment>\n  <text>other</text>\n</fragment>\n",
		"reader.tui": "<tui>\n  <screen id=\"main\">\n    <include src=\"frag.tui\"/>\n  </screen>\n</tui>\n",
	}
	var body strings.Builder
	for k := 0; k < 40; k++ {
		fmt.Fprintf(&body, "    <row><text id=\"t%d\">line %d {x}</text><input id=\"i%d\" bind=\"v\"/></row>\n", k, k, k)
	}
	doc := "<tui>\n  <screen id=\"main\">\n    <include src=\"other.tui\"/>\n" + body.String() + "  </screen>\n</tui>\n"
	for i := 0; i < n; i++ {
		files[fmt.Sprintf("doc%03d.tui", i)] = doc
	}
	dir := writeTree(t, files)
	srv := newServer(Options{})
	var analyses atomic.Int64
	srv.analyze = func(sn *snapshot, path, text string) *analysis {
		analyses.Add(1)
		return analyze(sn, path, text)
	}
	frag := filepath.Join(dir, "frag.tui")
	var publish time.Duration
	for gen := 1; gen <= 3; gen++ {
		text := strings.Replace(files["frag.tui"], "frag", fmt.Sprintf("frag %d", gen), 1)
		d := &document{uri: pathToURI(frag), path: frag, kind: kindTUI, text: text}
		sn := &snapshot{gen: gen, byURI: map[string]*document{d.uri: d}, open: map[string]string{frag: text}}
		analyses.Store(0)
		start := time.Now()
		own := srv.analysisOf(sn, frag, text)
		_, loads := srv.fragmentDiags(sn, frag, text, own)
		publish = max(publish, time.Since(start))
		// The fragment itself and reader.tui, nothing else.
		if got := analyses.Load(); got != 2 {
			t.Errorf("gen %d: %d analyses, want 2 (the fragment and reader.tui)", gen, got)
		}
		if len(loads) != 0 {
			t.Errorf("gen %d: loads %v, want none (reader.tui reads only frag.tui)", gen, loads)
		}
	}
	// One document's analysis, against which the publish is measured.
	sn := &snapshot{gen: 99, byURI: map[string]*document{}, open: map[string]string{}}
	start := time.Now()
	analyze(sn, filepath.Join(dir, "doc000.tui"), doc)
	one := time.Since(start)
	if publish > one*n/4+50*time.Millisecond {
		t.Errorf("a fragment's publish took %v; one analysis takes %v, and %d documents beside it read other fragments", publish, one, n)
	}
}

// 152, the orphan bullet's other half (SPEC v0.4b §15.12): a fragment no
// document reads resolves its own templates and ids, and the copies its
// uses make, as a document that read it would give them; completion in
// template= offers its templates, and those of the files it includes.
func TestOrphanFragmentResolvesItsOwn(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"lib.tui": "<fragment>\n  <template id=\"shared\">\n    <text id=\"s\">s</text>\n  </template>\n</fragment>\n",
	})
	c := startServer(t, Options{})
	c.initialize()
	text := "<fragment>\n  <include src=\"lib.tui\"/>\n  <template id=\"field\">\n    <input id=\"in\" bind=\"f\"/>\n  </template>\n" +
		"  <use id=\"name\" template=\"field\"/>\n  <use template=\"shared\"/>\n  <text id=\"top\">t</text>\n" +
		"  <keymap>\n    <bind keys=\"y\" action=\"focus\" to=\"#top\"/>\n    <bind keys=\"z\" action=\"focus\" to=\"#name__in\"/>\n  </keymap>\n</fragment>\n"
	path := filepath.Join(dir, "kit.tui")
	uri := c.open(path, "tuimark", text)
	kitURI, libURI := pathToURI(path), pathToURI(filepath.Join(dir, "lib.tui"))
	for _, w := range []struct {
		mark   string
		skip   int
		uri    string
		line   int
		char   int
		reason string
	}{
		{`template="field"`, 11, kitURI, 2, 2, "its own template"},
		{`template="shared"`, 11, libURI, 1, 2, "a template its include brings"},
		{`#top`, 2, kitURI, 7, 2, "its own id"},
		{`#name__in`, 2, kitURI, 3, 4, "a copy's scoped id"},
	} {
		locs := definitionAt(t, c, uri, text, w.mark, w.skip)
		if len(locs) != 1 || locs[0].URI != w.uri || locs[0].Range.Start.Line != w.line || locs[0].Range.Start.Character != w.char {
			t.Errorf("definition of %s (%s): %+v, want %s %d:%d", w.mark, w.reason, locs, w.uri, w.line, w.char)
		}
	}
	marked := strings.Replace(text, `template="field"`, `template="|"`, 1)
	typed := strings.Replace(marked, "|", "", 1)
	c.change(uri, 2, typed)
	got := completionLabels(t, c, uri, positionAt(typed, strings.IndexByte(marked, '|')))
	if !got["field"] || !got["shared"] {
		t.Errorf("completion in template=: %v, want field and shared", keys(got))
	}
	marked = strings.Replace(text, `to="#top"`, `to="#|"`, 1)
	typed = strings.Replace(marked, "|", "", 1)
	c.change(uri, 3, typed)
	got = completionLabels(t, c, uri, positionAt(typed, strings.IndexByte(marked, '|')))
	if !got["#top"] || !got["#name__in"] {
		t.Errorf("completion in to=: %v, want #top and #name__in", keys(got))
	}
	_ = c.shutdown()
}
