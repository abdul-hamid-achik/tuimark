package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

const (
	goodDoc = `<tui version="1"><screen id="s"><text>x</text></screen></tui>` + "\n"
	badDoc  = `<tui version="1"><screen id="s"><text>x</text><bogus/></screen></tui>` + "\n"
)

func hoverSync(c *client, uri string) {
	c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": Position{}})
}

// publishesOf drains what the server has sent and returns the
// publishDiagnostics for uri, in order.
func (c *client) publishesOf(uri string) []published {
	c.t.Helper()
	var out []published
	for {
		select {
		case m, ok := <-c.msgs:
			if !ok {
				return out
			}
			c.notes = append(c.notes, m)
		default:
			var rest []message
			for _, m := range c.notes {
				if m.Method == "textDocument/publishDiagnostics" {
					if p := decodePublished(c.t, m); p.URI == uri {
						out = append(out, p)
						continue
					}
				}
				rest = append(rest, m)
			}
			c.notes = rest
			return out
		}
	}
}

// An analysis that takes longer than a change does not publish its older
// result after the newer one's: the publishes of a URI are serialized and
// the stale one is dropped.
func TestSlowAnalysisDoesNotOverwriteANewerPublish(t *testing.T) {
	fc := &fakeClock{}
	srv := newServer(Options{})
	srv.afterFunc = fc.afterFunc
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	srv.analyze = func(sn *snapshot, path, text string) *analysis {
		if text == badDoc { // the slow one
			entered <- struct{}{}
			<-release
		}
		return analyze(sn, path, text)
	}
	c := startWith(t, srv)
	c.initialize()
	dir := writeTree(t, nil)
	uri := c.open(filepath.Join(dir, "a.tui"), "tuimark", badDoc)
	hoverSync(c, uri)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); fc.fire() }()
	<-entered // the first analysis is running, and held

	c.change(uri, 2, goodDoc)
	hoverSync(c, uri)
	go func() { defer wg.Done(); fc.fire() }()
	time.Sleep(100 * time.Millisecond) // the second publish waits for the first
	close(release)
	wg.Wait()
	hoverSync(c, uri)

	got := c.publishesOf(uri)
	if len(got) != 1 {
		t.Fatalf("%d publishes %+v, want the newer one only", len(got), got)
	}
	if got[0].Version == nil || *got[0].Version != 2 || len(got[0].Diagnostics) != 0 {
		t.Errorf("published %+v, want version 2 with no diagnostics", got[0])
	}
	srv.mu.Lock()
	last := srv.lastDiags[uri]
	srv.mu.Unlock()
	if len(last) != 0 {
		t.Errorf("the last diagnostics are %+v, want none", last)
	}
}

// A .tui and its stylesheet opened together publish the stylesheet once.
func TestDocumentAndStylesheetOpenedTogetherPublishTheStylesheetOnce(t *testing.T) {
	for name, opts := range map[string]Options{"fake clock": {}, "real clock": {Debounce: 20 * time.Millisecond}} {
		t.Run(name, func(t *testing.T) {
			var fc *fakeClock
			srv := newServer(opts)
			if name == "fake clock" {
				fc = &fakeClock{}
				srv.afterFunc = fc.afterFunc
			}
			c := startWith(t, srv)
			c.initialize()
			dir := writeTree(t, map[string]string{"theme.tcss": "screen { bogus-prop: 1; }\n"})
			doc := `<tui version="1"><style src="theme.tcss"/><screen id="s"><text>x</text></screen></tui>` + "\n"
			docURI := c.open(filepath.Join(dir, "a.tui"), "tuimark", doc)
			sheetURI := c.open(filepath.Join(dir, "theme.tcss"), "tcss", "screen { bogus-prop: 1; }\n")
			hoverSync(c, docURI)
			if fc != nil {
				for i := 0; i < 6 && fc.fire() > 0; i++ {
					hoverSync(c, docURI)
				}
				if got := c.publishesOf(sheetURI); len(got) != 1 {
					t.Errorf("the stylesheet was published %d times, want once: %+v", len(got), got)
				}
				return
			}
			c.nextPublish(docURI)
			c.nextPublish(sheetURI)
			c.noPublish(sheetURI, 400*time.Millisecond)
		})
	}
}

// A save, or a watched file, refreshes the diagnostics of a stylesheet
// nobody has open.
func TestDidSaveRefreshesAClosedStylesheet(t *testing.T) {
	fc := &fakeClock{}
	srv := newServer(Options{})
	srv.afterFunc = fc.afterFunc
	c := startWith(t, srv)
	c.initialize()
	doc := `<tui version="1"><style src="theme.tcss"/><screen id="s"><text>x</text></screen></tui>` + "\n"
	dir := writeTree(t, map[string]string{"theme.tcss": "screen { bogus-prop: 1; }\n", "a.tui": doc})
	docURI := c.open(filepath.Join(dir, "a.tui"), "tuimark", doc)
	sheetURI := pathToURI(filepath.Join(dir, "theme.tcss"))
	fireAll := func() {
		hoverSync(c, docURI)
		for i := 0; i < 6 && fc.fire() > 0; i++ {
			hoverSync(c, docURI)
		}
	}
	fireAll()
	c.nextPublish(docURI)
	if p := c.nextPublish(sheetURI); len(p.Diagnostics) != 1 {
		t.Fatalf("the stylesheet on open: %+v, want its V003", p.Diagnostics)
	}

	// Fixed on disk (another editor, a checkout), then the document is saved.
	if err := os.WriteFile(filepath.Join(dir, "theme.tcss"), []byte("screen { color: red; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.notify("textDocument/didSave", map[string]any{"textDocument": map[string]any{"uri": docURI}})
	fireAll()
	if p := c.nextPublish(sheetURI); len(p.Diagnostics) != 0 {
		t.Errorf("after didSave: %+v, want none", p.Diagnostics)
	}

	// Broken again, reported by the watcher, with the document closed: only
	// the URI the watcher names says what changed.
	if err := os.WriteFile(filepath.Join(dir, "theme.tcss"), []byte("screen { bogus-prop: 1; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": docURI}})
	fireAll()
	c.publishesOf(sheetURI) // whatever the close sent
	c.notify("workspace/didChangeWatchedFiles", map[string]any{"changes": []map[string]any{{"uri": sheetURI, "type": 2}}})
	hoverSync(c, docURI)
	fireAll()
	got := c.publishesOf(sheetURI)
	if len(got) == 0 || len(got[len(got)-1].Diagnostics) != 1 {
		t.Errorf("after didChangeWatchedFiles: %+v, want the stylesheet republished with its V003", got)
	}
}

// A definition in an untitled buffer lands in that buffer, not in another
// untitled one.
func TestDefinitionInAnUntitledBufferStaysInIt(t *testing.T) {
	c := startServer(t, Options{})
	c.initialize()
	tui := "<tui version=\"3\">\n  <screen id=\"main\">\n    <button id=\"b\">ok</button>\n  </screen>\n  <keymap>\n    <bind keys=\"t\" action=\"focus\" to=\"#b\"/>\n  </keymap>\n</tui>\n"
	other := "<tui version=\"3\"><screen id=\"other\"><text>x</text></screen></tui>\n"
	open := func(uri, text string) {
		c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
			"uri": uri, "languageId": "tuimark", "version": 1, "text": text,
		}})
	}
	for i := 2; i <= 8; i++ {
		open("untitled:Untitled-"+itoa(i), other)
	}
	open("untitled:Untitled-1", tui)
	for i := 0; i < 30; i++ {
		r := c.call("textDocument/definition", map[string]any{
			"textDocument": map[string]any{"uri": "untitled:Untitled-1"}, "position": Position{5, 39},
		})
		var locs []location
		if err := json.Unmarshal(r.Result, &locs); err != nil || len(locs) != 1 || locs[0].URI != "untitled:Untitled-1" {
			t.Fatalf("definition #%d = %s, want one location in untitled:Untitled-1", i, r.Result)
		}
	}
	// Nothing matches a path that is empty.
	sn := &snapshot{byURI: map[string]*document{"untitled:a": {uri: "untitled:a"}}}
	if got := newServer(Options{}).uriOfPath(sn, nil, ""); got != "" {
		t.Errorf("uriOfPath of nothing = %q", got)
	}
}

// Closing a .tui drops its cached analysis (and the sample.json decoded in
// it).
func TestCloseDropsTheAnalysisCache(t *testing.T) {
	fc := &fakeClock{}
	srv := newServer(Options{})
	srv.afterFunc = fc.afterFunc
	c := startWith(t, srv)
	c.initialize()
	dir := writeTree(t, map[string]string{"sample.json": `{"name":"x"}`})
	path := filepath.Join(dir, "a.tui")
	uri := c.open(path, "tuimark", goodDoc)
	hoverSync(c, uri)
	fc.fire()
	c.nextPublish(uri)
	cached := func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		_, ok := srv.cache[path]
		return ok
	}
	if !cached() {
		t.Fatal("the analysis was not cached after a publish")
	}
	c.notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": uri}})
	hoverSync(c, uri)
	if cached() {
		t.Error("the analysis of a closed .tui is still cached")
	}
	fc.fire()
	if cached() {
		t.Error("the publish after the close cached it again")
	}
}

// A sample.json larger than the cap is read as absent.
func TestOversizedSampleIsAbsent(t *testing.T) {
	if testing.Short() {
		t.Skip("reads 64 MiB")
	}
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, sampleName))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(host.MaxReadBytes + 1); err != nil {
		t.Skipf("no sparse files: %v", err)
	}
	sn := &snapshot{open: map[string]string{}, byURI: map[string]*document{}}
	if _, ok := readSample(sn, filepath.Join(dir, "a.tui")); ok {
		t.Error("an oversized sample.json was read")
	}
}
