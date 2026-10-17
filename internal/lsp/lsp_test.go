package lsp

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// SPEC v0.4a test 123: a Go test drives the server over pipes with
// Content-Length framing.

func TestSession(t *testing.T) {
	c := startServer(t, Options{Version: "9.9.9"})
	r := c.call("initialize", map[string]any{"capabilities": map[string]any{}, "rootUri": nil})
	if r.Error != nil {
		t.Fatalf("initialize: %+v", r.Error)
	}
	var res struct {
		Capabilities map[string]any `json:"capabilities"`
		ServerInfo   map[string]any `json:"serverInfo"`
	}
	if err := json.Unmarshal(r.Result, &res); err != nil {
		t.Fatal(err)
	}
	sync, _ := res.Capabilities["textDocumentSync"].(map[string]any)
	if sync["openClose"] != true || sync["change"] != float64(1) {
		t.Errorf("textDocumentSync = %v, want openClose and Full (1)", sync)
	}
	for _, k := range []string{"completionProvider", "hoverProvider", "definitionProvider", "documentFormattingProvider"} {
		if res.Capabilities[k] == nil {
			t.Errorf("capability %s missing", k)
		}
	}
	for _, k := range []string{"renameProvider", "codeActionProvider", "semanticTokensProvider", "workspaceSymbolProvider"} {
		if res.Capabilities[k] != nil {
			t.Errorf("capability %s declared, not in 0.4a", k)
		}
	}
	if res.ServerInfo["name"] != "tuimark" || res.ServerInfo["version"] != "9.9.9" {
		t.Errorf("serverInfo = %v", res.ServerInfo)
	}
	c.notify("initialized", map[string]any{})
	if code := c.shutdown(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
}

const wideDoc = `<tui version="1">
  <screen id="s">
    <col title="日本😀"><bogus/></col>
  </screen>
</tui>
`

func TestDiagnosticsWideCharacterAndFix(t *testing.T) {
	dir := writeTree(t, nil)
	c := startServer(t, Options{})
	c.initialize()
	uri := c.open(filepath.Join(dir, "b.tui"), "tuimark", wideDoc)
	p := c.nextPublish(uri)
	if len(p.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want one V001", p.Diagnostics)
	}
	d := p.Diagnostics[0]
	// validate: error V001 b.tui:3:22 unknown tag <bogus>. Column 22 counts
	// code points, 21 of them before <bogus/>; the emoji takes two UTF-16
	// units, so the character is 22.
	if d.Code != "V001" || d.Message != "unknown tag <bogus>" || d.Severity != 1 || d.Source != "tuimark" {
		t.Errorf("diagnostic = %+v", d)
	}
	want := Range{Start: Position{2, 22}, End: Position{2, 28}}
	if d.Range != want {
		t.Errorf("range = %+v, want %+v (<bogus ends the token)", d.Range, want)
	}
	if p.Version == nil || *p.Version != 1 {
		t.Errorf("version = %v, want 1", p.Version)
	}
	// The fix republishes an empty list.
	c.change(uri, 2, strings.Replace(wideDoc, "<bogus/>", "", 1))
	p = c.nextPublish(uri)
	if len(p.Diagnostics) != 0 {
		t.Errorf("after the fix: %+v, want none", p.Diagnostics)
	}
	if code := c.shutdown(); code != 0 {
		t.Errorf("exit %d", code)
	}
}

func TestDiagnosticsInStylesheetGoToItsURI(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"c.tui":   `<tui version="1"><style src="t.tcss"/><screen id="s"><text>x</text></screen></tui>` + "\n",
		"t.tcss":  "screen { bogus-prop: 1; }\n",
		"u/d.tui": `<tui version="1"><style src="../t.tcss"/><screen id="s"><text>x</text></screen></tui>` + "\n",
	})
	c := startServer(t, Options{})
	c.initialize()
	docURI := c.open(filepath.Join(dir, "c.tui"), "tuimark", mustRead(t, filepath.Join(dir, "c.tui")))
	sheetURI := pathToURI(filepath.Join(dir, "t.tcss"))
	doc := c.nextPublish(docURI)
	if len(doc.Diagnostics) != 0 {
		t.Errorf("the document got %+v; a stylesheet's V003 is not its", doc.Diagnostics)
	}
	sheet := c.nextPublish(sheetURI)
	if len(sheet.Diagnostics) != 1 {
		t.Fatalf("stylesheet diagnostics = %+v, want one V003", sheet.Diagnostics)
	}
	d := sheet.Diagnostics[0]
	if d.Code != "V003" || d.Message != `unknown property "bogus-prop"` || d.Range.Start != (Position{0, 9}) || d.Range.End != (Position{0, 19}) {
		t.Errorf("diagnostic = %+v", d)
	}

	// The stylesheet is open too: its buffer is what is read, and a fix
	// there empties the list (both documents that load it are rechecked).
	c.open(filepath.Join(dir, "t.tcss"), "tcss", "screen { color: red; }\n")
	if p := c.nextPublish(sheetURI); len(p.Diagnostics) != 0 {
		t.Errorf("open buffer fixed it, still %+v", p.Diagnostics)
	}
	_ = c.shutdown()
}

func TestStyleSrcWithDotDotAndOpenBuffer(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"t.tcss": "screen { color: red; }\n", // fine on disk
	})
	c := startServer(t, Options{})
	c.initialize()
	docPath := filepath.Join(dir, "u", "d.tui")
	docURI := c.open(docPath, "tuimark", `<tui version="1"><style src="../t.tcss"/><screen id="s"><text>x</text></screen></tui>`+"\n")
	// The stylesheet is open with a bad property that disk does not have.
	sheetPath := filepath.Join(dir, "t.tcss")
	sheetURI := c.open(sheetPath, "tcss", "screen { bogus-prop: 1; }\n")
	var got published
	for i := 0; i < 4 && len(got.Diagnostics) == 0; i++ {
		got = c.nextPublish(sheetURI)
	}
	if len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "V003" {
		t.Fatalf("the open buffer was not read through ..: %+v", got.Diagnostics)
	}
	_ = docURI
	_ = c.shutdown()
}

func TestSampleJSONEnablesBindChecks(t *testing.T) {
	const doc = `<tui version="1">
  <screen id="s">
    <text>{nope}</text>
  </screen>
</tui>
`
	with := writeTree(t, map[string]string{"app.tui": doc, "sample.json": `{"a": 1}`})
	without := writeTree(t, map[string]string{"app.tui": doc})

	c := startServer(t, Options{})
	c.initialize()
	uri := c.open(filepath.Join(with, "app.tui"), "tuimark", doc)
	p := c.nextPublish(uri)
	if len(p.Diagnostics) != 1 || p.Diagnostics[0].Code != "B003" || p.Diagnostics[0].Severity != 2 {
		t.Errorf("with sample.json: %+v, want one B003 warning", p.Diagnostics)
	}
	uri = c.open(filepath.Join(without, "app.tui"), "tuimark", doc)
	if p := c.nextPublish(uri); len(p.Diagnostics) != 0 {
		t.Errorf("without sample.json: %+v, want none", p.Diagnostics)
	}
	_ = c.shutdown()
}

func TestInvalidSampleJSONMeansNoData(t *testing.T) {
	const doc = `<tui version="1"><screen id="s"><text>{nope}</text></screen></tui>` + "\n"
	dir := writeTree(t, map[string]string{"app.tui": doc, "sample.json": `{not json`})
	c := startServer(t, Options{})
	c.initialize()
	uri := c.open(filepath.Join(dir, "app.tui"), "tuimark", doc)
	if p := c.nextPublish(uri); len(p.Diagnostics) != 0 {
		t.Errorf("%+v, want none", p.Diagnostics)
	}
	_ = c.shutdown()
}

func TestSampleJSONOpenBufferWins(t *testing.T) {
	const doc = `<tui version="1"><screen id="s"><text>{a}</text></screen></tui>` + "\n"
	dir := writeTree(t, map[string]string{"app.tui": doc, "sample.json": `{"a": 1}`})
	c := startServer(t, Options{})
	c.initialize()
	uri := c.open(filepath.Join(dir, "app.tui"), "tuimark", doc)
	if p := c.nextPublish(uri); len(p.Diagnostics) != 0 {
		t.Fatalf("%+v", p.Diagnostics)
	}
	// The editor edits sample.json without saving it: the buffer is the data.
	c.open(filepath.Join(dir, "sample.json"), "json", `{"b": 1}`)
	if p := c.nextPublish(uri); len(p.Diagnostics) != 1 || p.Diagnostics[0].Code != "B003" {
		t.Errorf("%+v, want B003 for {a} once the buffer lacks a", p.Diagnostics)
	}
	_ = c.shutdown()
}

func TestCompletionTagsAndKeywords(t *testing.T) {
	dir := writeTree(t, nil)
	c := startServer(t, Options{})
	c.initialize()
	const text = "<tui version=\"3\">\n  <screen id=\"s\">\n    <li\n    <box border=\"\"/>\n  </screen>\n</tui>\n"
	uri := c.open(filepath.Join(dir, "app.tui"), "tuimark", text)

	items := completionLabels(t, c, uri, Position{2, 7})
	if !items["list"] {
		t.Errorf("after <li: %v, want list", keys(items))
	}
	if items["col"] || items["screen"] {
		t.Errorf("after <li: %v, want only names starting with li", keys(items))
	}

	items = completionLabels(t, c, uri, Position{3, 17})
	for _, k := range []string{"none", "single", "double", "rounded", "thick"} {
		if !items[k] {
			t.Errorf("after border=\": %v, missing %s", keys(items), k)
		}
	}
	if items["bogus"] || items["box"] {
		t.Errorf("after border=\": %v, want only keywords", keys(items))
	}
	_ = c.shutdown()
}

func completionLabels(t *testing.T, c *client, uri string, pos Position) map[string]bool {
	t.Helper()
	r := c.call("textDocument/completion", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": pos})
	if r.Error != nil {
		t.Fatalf("completion: %+v", r.Error)
	}
	var list completionList
	if err := json.Unmarshal(r.Result, &list); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, it := range list.Items {
		out[it.Label] = true
	}
	return out
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestHoverTCSSProperty(t *testing.T) {
	dir := writeTree(t, nil)
	c := startServer(t, Options{})
	c.initialize()
	const text = "screen {\n  wrap: wrap;\n}\n"
	uri := c.open(filepath.Join(dir, "t.tcss"), "tcss", text)
	r := c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": Position{1, 3}})
	var h hoverResult
	if err := json.Unmarshal(r.Result, &h); err != nil {
		t.Fatalf("%v: %s", err, r.Result)
	}
	if !strings.Contains(h.Contents.Value, css.PropertyValues("wrap")) || !strings.Contains(h.Contents.Value, "wrap") {
		t.Errorf("hover = %q, want the catalog's syntax %q", h.Contents.Value, css.PropertyValues("wrap"))
	}
	// Nothing to say about a brace.
	r = c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": Position{0, 7}})
	if string(r.Result) != "null" {
		t.Errorf("hover on a brace = %s, want null", r.Result)
	}
	_ = c.shutdown()
}

func TestDefinitionOfIDInStylesheet(t *testing.T) {
	const doc = `<tui version="1">
  <style src="t.tcss"/>
  <screen id="s">
    <list id="rows" each="items as it" key="it.id"><item><text>{it.name}</text></item></list>
  </screen>
</tui>
`
	dir := writeTree(t, map[string]string{"app.tui": doc, "t.tcss": "#rows { height: 3; }\n.x { bold: true; }\n"})
	c := startServer(t, Options{})
	c.initialize()
	sheet := c.open(filepath.Join(dir, "t.tcss"), "tcss", mustRead(t, filepath.Join(dir, "t.tcss")))
	r := c.call("textDocument/definition", map[string]any{"textDocument": map[string]any{"uri": sheet}, "position": Position{0, 2}})
	var locs []location
	if err := json.Unmarshal(r.Result, &locs); err != nil {
		t.Fatalf("%v: %s", err, r.Result)
	}
	want := location{URI: pathToURI(filepath.Join(dir, "app.tui")), Range: Range{Start: Position{3, 4}, End: Position{3, 9}}}
	if len(locs) != 1 || locs[0] != want {
		t.Errorf("definition = %+v, want %+v", locs, want)
	}
	_ = c.shutdown()
}

func TestFormatting(t *testing.T) {
	dir := writeTree(t, nil)
	c := startServer(t, Options{})
	c.initialize()
	messy := "<tui version=\"1\">\n<screen   id=\"s\"><text>héllo 😀</text></screen>\n</tui>"
	want, err := parse.Format([]byte(messy))
	if err != nil {
		t.Fatal(err)
	}
	uri := c.open(filepath.Join(dir, "a.tui"), "tuimark", messy)
	var edits []textEdit
	r := c.call("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": uri}, "options": map[string]any{"tabSize": 2}})
	if err := json.Unmarshal(r.Result, &edits); err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 || edits[0].NewText != want {
		t.Fatalf("edits = %+v, want one edit with fmt's output %q", edits, want)
	}
	// The edit covers the whole text: 3 lines, the last without a newline.
	if edits[0].Range.Start != (Position{}) || edits[0].Range.End != (Position{2, 6}) {
		t.Errorf("range = %+v", edits[0].Range)
	}
	// Canonical text: no edit.
	canon := c.open(filepath.Join(dir, "canon.tui"), "tuimark", want)
	r = c.call("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": canon}, "options": map[string]any{}})
	if string(r.Result) != "[]" {
		t.Errorf("canonical text: %s, want []", r.Result)
	}
	// V005: no edit.
	broken := c.open(filepath.Join(dir, "broken.tui"), "tuimark", "<tui version=\"1\"><screen></tui>")
	r = c.call("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": broken}, "options": map[string]any{}})
	if string(r.Result) != "[]" {
		t.Errorf("V005 document: %s, want []", r.Result)
	}
	// A .tcss has no formatter.
	sheet := c.open(filepath.Join(dir, "t.tcss"), "tcss", "screen{color:red}")
	r = c.call("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": sheet}, "options": map[string]any{}})
	if string(r.Result) != "[]" {
		t.Errorf(".tcss: %s, want []", r.Result)
	}
	_ = c.shutdown()
}

// --- transport -------------------------------------------------------------

func TestNonJSONBodyIsParseError(t *testing.T) {
	c := startServer(t, Options{})
	c.sendRaw(frame([]byte("this is not json")))
	m := <-c.msgs
	if m.Error == nil || m.Error.Code != -32700 || string(m.ID) != "null" {
		t.Fatalf("got %+v, want -32700 with id null", m)
	}
	// The server goes on.
	c.initialize()
	if code := c.shutdown(); code != 0 {
		t.Errorf("exit %d", code)
	}
}

func TestInvalidRequests(t *testing.T) {
	c := startServer(t, Options{})
	c.sendRaw(frame([]byte(`[1,2]`)))
	if m := <-c.msgs; m.Error == nil || m.Error.Code != -32600 || string(m.ID) != "null" {
		t.Errorf("batch array: %+v", m)
	}
	c.sendRaw(frame([]byte(`{"jsonrpc":"2.0","id":7}`)))
	if m := <-c.msgs; m.Error == nil || m.Error.Code != -32600 || string(m.ID) != "7" {
		t.Errorf("no method: %+v", m)
	}
	// A request before initialize.
	if m := c.call("textDocument/hover", map[string]any{}); m.Error == nil || m.Error.Code != -32002 {
		t.Errorf("before initialize: %+v", m)
	}
	c.initialize()
	// An id is echoed as written, whatever float64 would make of it.
	c.sendRaw(frame([]byte(`{"jsonrpc":"2.0","id":12345678901234567890,"method":"nope"}`)))
	m := <-c.msgs
	if m.Error == nil || m.Error.Code != -32601 || string(m.ID) != "12345678901234567890" {
		t.Errorf("unknown method: %+v", m)
	}
	// A response from the client is ignored, an unknown notification too.
	c.sendRaw(frame([]byte(`{"jsonrpc":"2.0","id":1,"result":null}`)))
	c.notify("$/setTrace", map[string]any{})
	c.notify("nope", nil)
	if code := c.shutdown(); code != 0 {
		t.Errorf("exit %d", code)
	}
	// After shutdown, a request is invalid; this one raced `exit`, so use a
	// fresh server.
	c = startServer(t, Options{})
	c.initialize()
	c.call("shutdown", nil)
	if m := c.call("textDocument/hover", map[string]any{}); m.Error == nil || m.Error.Code != -32600 {
		t.Errorf("after shutdown: %+v", m)
	}
	c.notify("exit", nil)
	if code := c.waitExit(); code != 0 {
		t.Errorf("exit %d", code)
	}
}

func TestMalformedHeaderExits1(t *testing.T) {
	for _, header := range []string{
		"Content-Length: abc\r\n\r\n",
		"Content-Length: -1\r\n\r\n",
		"Content-Length: 99999999999\r\n\r\n",
		"not a header\r\n\r\n",
		"Content-Type: application/json\r\n\r\n", // no Content-Length
	} {
		c := startServer(t, Options{})
		c.sendRaw([]byte(header))
		if code := c.waitExit(); code != 1 {
			t.Errorf("%q: exit %d, want 1", header, code)
		}
		if !strings.HasPrefix(c.stderr.String(), "tuimark: ") {
			t.Errorf("%q: stderr %q", header, c.stderr.String())
		}
	}
}

func TestExitCodes(t *testing.T) {
	// shutdown then exit: 0.
	c := startServer(t, Options{})
	c.initialize()
	if code := c.shutdown(); code != 0 {
		t.Errorf("shutdown, exit: %d, want 0", code)
	}
	// exit alone: 1.
	c = startServer(t, Options{})
	c.initialize()
	c.notify("exit", nil)
	if code := c.waitExit(); code != 1 {
		t.Errorf("exit alone: %d, want 1", code)
	}
	// End of input after shutdown: 0, without it: 1.
	c = startServer(t, Options{})
	c.initialize()
	c.call("shutdown", nil)
	c.in.Close()
	if code := c.waitExit(); code != 0 {
		t.Errorf("EOF after shutdown: %d, want 0", code)
	}
	c = startServer(t, Options{})
	c.initialize()
	c.in.Close()
	if code := c.waitExit(); code != 1 {
		t.Errorf("EOF alone: %d, want 1", code)
	}
}

func TestHeaderLeniency(t *testing.T) {
	// Header names are case-insensitive and Content-Type is ignored.
	msg := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	br := bufio.NewReader(strings.NewReader("content-length: " + itoa(len(msg)) + "\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n" + msg))
	body, err := readMessage(br)
	if err != nil || string(body) != msg {
		t.Fatalf("%q, %v", body, err)
	}
	if _, err := readMessage(br); err != io.EOF {
		t.Errorf("after the last message: %v, want EOF", err)
	}
	// A body cut short is the end of input.
	if _, err := readMessage(bufio.NewReader(strings.NewReader("Content-Length: 10\r\n\r\nabc"))); err != io.EOF {
		t.Errorf("short body: %v", err)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestDefaultDebounceIs150ms(t *testing.T) {
	if DefaultDebounce != 150*time.Millisecond {
		t.Fatalf("DefaultDebounce = %v", DefaultDebounce)
	}
	if s := newServer(Options{}); s.debounce != 150*time.Millisecond {
		t.Errorf("a server's debounce = %v", s.debounce)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
