package main

// SPEC v0.4a §21 tests 116 to 118 (the CLI's parts): line bounds, the
// V005 definition, and `validate` without data. Test 119 is catalog_test.go
// and test 120 is new_test.go.

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/agentsdoc"
	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
)

// ── Test 116: line bounds ───────────────────────────────────────────────

// fd3Rig is a bridge with no app whose fd 3 and fd 4 are pipes this test
// holds the other ends of; readLoop runs on its own goroutine, as it does
// under `tuimark host`.
type fd3Rig struct {
	b    *hostBridge
	toB  *os.File // the parent's end of fd 3
	fd4  *os.File // the parent's end of fd 4
	done chan struct{}
}

func newFD3Rig(t *testing.T) *fd3Rig {
	t.Helper()
	r3, w3, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r4, w4, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	b := newHostBridge(nil, r3, w4, time.Second)
	g := &fd3Rig{b: b, toB: w3, fd4: r4, done: make(chan struct{})}
	go func() {
		b.readLoop()
		close(g.done)
	}()
	t.Cleanup(func() {
		_ = w3.Close()
		<-g.done
		b.out.finish(nil, 0)
		_ = w4.Close()
		_ = r4.Close()
		_ = r3.Close()
	})
	return g
}

// line returns the next fd 4 line.
func (g *fd3Rig) line(t *testing.T) string {
	t.Helper()
	_ = g.fd4.SetReadDeadline(time.Now().Add(20 * time.Second))
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		if _, err := g.fd4.Read(buf); err != nil {
			t.Fatalf("reading fd 4: %v (got %q)", err, sb.String())
		}
		if buf[0] == '\n' {
			return sb.String()
		}
		sb.WriteByte(buf[0])
	}
}

// writeLine writes n bytes of x and a newline to fd 3 from a goroutine
// (the pipe blocks until the bridge reads), and reports when it is done.
func (g *fd3Rig) writeLine(n int) <-chan error {
	done := make(chan error, 1)
	go func() {
		chunk := bytes.Repeat([]byte{'x'}, 1<<20)
		for n > 0 {
			k := min(n, len(chunk))
			if _, err := g.toB.Write(chunk[:k]); err != nil {
				done <- err
				return
			}
			n -= k
		}
		_, err := g.toB.Write([]byte("\n"))
		done <- err
	}()
	return done
}

// A line of 64 MiB − 1 bytes (the newline excluded) on fd 3 is processed:
// it is not a JSON object, so it gets the malformed-input error, and the
// session goes on to the next line. A line of 64 MiB ends the session with
// the §19.1 message and nothing of it is applied (test 116, host part; the
// pty test below adds the exit message and the restored terminal).
func TestHostFD3LineBound(t *testing.T) {
	g := newFD3Rig(t)
	if err := <-g.writeLine(maxFD3Line - 1); err != nil {
		t.Fatal(err)
	}
	if got, want := g.line(t), `{"type":"error","error":"not a JSON object"}`; got != want {
		t.Fatalf("a line of 64 MiB - 1 bytes: %.200s, want %s", got, want)
	}
	select {
	case s := <-g.b.stop:
		t.Fatalf("the session was stopped by a line under the bound: %+v", s)
	default:
	}
	// The same bridge still reads: a short line is answered.
	if _, err := g.toB.WriteString("not json\n"); err != nil {
		t.Fatal(err)
	}
	if got, want := g.line(t), `{"type":"error","error":"not a JSON object"}`; got != want {
		t.Fatalf("the line after: %s, want %s", got, want)
	}

	// 64 MiB exactly: the session ends.
	writeDone := g.writeLine(maxFD3Line)
	if got, want := g.line(t), `{"type":"error","error":"fd 3 line of 64 MiB or more"}`; got != want {
		t.Fatalf("a line of 64 MiB: %s, want %s", got, want)
	}
	select {
	case s := <-g.b.stop:
		if s.EOF || s.Err == nil || s.Err.Error() != "fd 3 line of 64 MiB or more" {
			t.Errorf("stop request = %+v, want the line-bound error", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no stop request after a line of 64 MiB")
	}
	// The reader stopped: it left the rest of the line unread, so the
	// writer may block on a full pipe; closing our end releases it.
	_ = g.toB.Close()
	<-writeDone
	select {
	case <-g.done:
	case <-time.After(5 * time.Second):
		t.Fatal("readLoop did not return after the bound")
	}
}

// The bound counts the newline out: an unterminated last line of 64 MiB − 1
// bytes at end of file is processed, then the session ends with eof.
func TestHostFD3UnterminatedLastLineUnderTheBound(t *testing.T) {
	g := newFD3Rig(t)
	go func() {
		_, _ = g.toB.Write(bytes.Repeat([]byte{'x'}, maxFD3Line-1))
		_ = g.toB.Close()
	}()
	if got, want := g.line(t), `{"type":"error","error":"not a JSON object"}`; got != want {
		t.Fatalf("got %.200s, want %s", got, want)
	}
	select {
	case s := <-g.b.stop:
		if !s.EOF {
			t.Errorf("stop = %+v, want eof", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no eof stop request")
	}
}

// tuimark mcp stops at a line of 64 MiB or more (§15.8 "End"): one of
// 64 MiB − 1 bytes gets an answer, and one of 64 MiB ends the server with
// exit 1 and a tuimark: line on stderr.
func TestMCPLineBound(t *testing.T) {
	const bound = 64 * 1024 * 1024
	run := func(n int) (code int, out, errw string) {
		in := io.MultiReader(bytes.NewReader(bytes.Repeat([]byte{'x'}, n)), strings.NewReader("\n"))
		var o, e bytes.Buffer
		code = runMCPServer(in, &o, &e)
		return code, o.String(), e.String()
	}
	code, out, errw := run(bound - 1)
	if code != 0 || errw != "" {
		t.Fatalf("a line of 64 MiB - 1 bytes: exit %d, stderr %q; want 0", code, errw)
	}
	if !strings.Contains(out, `"code":-32700`) || !strings.HasSuffix(out, "\n") {
		t.Errorf("a line of 64 MiB - 1 bytes got no answer: %.200s", out)
	}
	code, out, errw = run(bound)
	if code != 1 || !strings.HasPrefix(errw, "tuimark: ") || out != "" {
		t.Errorf("a line of 64 MiB: exit %d, stdout %.100q, stderr %q; want exit 1 and a tuimark: line", code, out, errw)
	}
}

// ── Test 117: V005 ──────────────────────────────────────────────────────

func TestV005Messages(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"foreign root", `<foo version="1"/>`, "document root must be <tui> (or the phase-0 alias <app>), got <foo>"},
		{"no screen", `<tui version="3"></tui>`, "document has no <screen>"},
	} {
		file := writeTemp(t, "doc.tui", c.src)
		code, out, _ := runCLI("validate", file)
		if code != 2 {
			t.Errorf("%s: exit %d, want 2", c.name, code)
		}
		if n := strings.Count(out, "V005"); n != 1 || !strings.Contains(out, c.want) {
			t.Errorf("%s: want one V005 with %q, got:\n%s", c.name, c.want, out)
		}
		if strings.Contains(out, "\nok") || strings.HasPrefix(out, "ok") {
			t.Errorf("%s: an invalid document printed ok:\n%s", c.name, out)
		}
	}
}

// The generated AGENTS.md's V005 row matches SPEC §14 (test 117), and the
// catalog carries the same row.
func TestV005RowMatchesSpec(t *testing.T) {
	const want = "not well-formed XML; or not a Tuimark document: a root element other than `<tui>` (or the phase-0 alias `<app>`), or a `<tui>` with no `<screen>`; a file whose root is `<fragment>` given as a document, except to `validate` and `fmt`"
	row := "| V005 | parse | " + want + " |"
	if !strings.Contains(agentsdoc.Markdown(), row) {
		t.Errorf("the briefing has no row %q", row)
	}
	if md := readAgents(t); !strings.Contains(md, row) {
		t.Errorf("AGENTS.md has no row %q", row)
	}
	var found bool
	for _, d := range catalog.Build("x").Diagnostics {
		if d.Code == "V005" {
			found = true
			if d.When != want || d.Pass != "parse" {
				t.Errorf("catalog V005 = %+v", d)
			}
		}
	}
	if !found {
		t.Error("the catalog has no V005")
	}
}

// ── Test 118: validate without data ─────────────────────────────────────

const validateDoc = `<tui version="1">
<screen id="s"><text>hello {name}</text></screen>
</tui>`

func TestValidateWithoutData(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "app.tui")
	data := filepath.Join(dir, "sample.json")
	writeFile(t, doc, validateDoc)
	writeFile(t, data, `{"name": "x"}`)

	code, out, errw := runCLI("validate", doc)
	if code != 0 || out != "ok (bindings not checked: no --data)\n" || errw != "" {
		t.Errorf("without --data: exit %d, stdout %q, stderr %q", code, out, errw)
	}
	code, out, errw = runCLI("validate", doc, "--data", data)
	if code != 0 || out != "ok\n" || errw != "" {
		t.Errorf("with --data: exit %d, stdout %q, stderr %q", code, out, errw)
	}

	// Warnings still print before the line; with data, the warning that
	// needs data (B003, a missing path) shows too.
	warn := filepath.Join(dir, "warn.tui")
	writeFile(t, warn, `<tui version="1"><screen id="s"><list id="l" each="rows as r"><item><text>{r}</text></item></list><text>{nope}</text></screen></tui>`)
	_, out, _ = runCLI("validate", warn)
	if !strings.Contains(out, "B006") || !strings.HasSuffix(out, "ok (bindings not checked: no --data)\n") {
		t.Errorf("warnings without --data:\n%s", out)
	}
	rowsData := filepath.Join(dir, "rows.json")
	writeFile(t, rowsData, `{"rows": ["a"]}`)
	_, out, _ = runCLI("validate", warn, "--data", rowsData)
	if !strings.Contains(out, "B003") || !strings.HasSuffix(out, "\nok\n") {
		t.Errorf("warnings with --data:\n%s", out)
	}

	// A document with errors prints no ok line, either way.
	bad := filepath.Join(dir, "bad.tui")
	writeFile(t, bad, `<tui version="1"><screen id="s"><nope/></screen></tui>`)
	for _, args := range [][]string{{"validate", bad}, {"validate", bad, "--data", data}} {
		code, out, _ := runCLI(args...)
		if code != 2 || strings.Contains(out, "ok") {
			t.Errorf("%v: exit %d, stdout %q", args, code, out)
		}
	}
}

func TestValidateJSONHoldsData(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "app.tui")
	data := filepath.Join(dir, "sample.json")
	writeFile(t, doc, validateDoc)
	writeFile(t, data, `{"name": "x"}`)

	keys := func(out string) []string {
		t.Helper()
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(out), &m); err != nil {
			t.Fatal(err)
		}
		var ks []string
		dec := json.NewDecoder(strings.NewReader(out))
		_, _ = dec.Token()
		for dec.More() {
			tok, _ := dec.Token()
			ks = append(ks, tok.(string))
			var skip json.RawMessage
			_ = dec.Decode(&skip)
		}
		return ks
	}

	_, out, _ := runCLI("validate", doc, "--json")
	if got := strings.Join(keys(out), ","); got != "actions,data,diagnostics,file,ok" {
		t.Errorf("members = %s, want actions,data,diagnostics,file,ok", got)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(out), &m)
	if v, present := m["data"]; !present || v != nil {
		t.Errorf(`without --data: "data" = %v (present %v), want null`, v, present)
	}
	_, out, _ = runCLI("validate", doc, "--data", data, "--json")
	m = nil
	_ = json.Unmarshal([]byte(out), &m)
	if m["data"] != data {
		t.Errorf(`with --data: "data" = %v, want %q (the argument as given)`, m["data"], data)
	}

	// The argument as given, not resolved: a relative path stays relative.
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()
	_, out, _ = runCLI("validate", "app.tui", "--data", "./sample.json", "--json")
	m = nil
	_ = json.Unmarshal([]byte(out), &m)
	if m["data"] != "./sample.json" {
		t.Errorf(`relative --data: "data" = %v, want "./sample.json"`, m["data"])
	}

	// tuimark_validate returns the same JSON, byte for byte.
	s := newMCPTestServer(t)
	defer s.closeAndWait()
	for _, args := range []struct {
		mcp map[string]any
		cli []string
	}{
		{map[string]any{"file": "app.tui"}, []string{"validate", "app.tui", "--json"}},
		{map[string]any{"file": "app.tui", "data": "sample.json"}, []string{"validate", "app.tui", "--data", "sample.json", "--json"}},
	} {
		res := s.toolCallResult(1, "tuimark_validate", args.mcp)
		content := res["content"].([]any)[0].(map[string]any)["text"].(string)
		_, want, _ := runCLI(args.cli...)
		if content != want {
			t.Errorf("tuimark_validate %v:\n%s\nwant\n%s", args.mcp, content, want)
		}
		if !strings.Contains(content, `"data":`) {
			t.Errorf("tuimark_validate has no data member:\n%s", content)
		}
	}
}

// `validate` checks the document under both themes for theme="auto", and
// the line is the same.
func TestValidateAutoThemeLine(t *testing.T) {
	file := writeTemp(t, "a.tui", `<tui version="2" theme="auto"><screen id="s"><text>x</text></screen></tui>`)
	_, out, _ := runCLI("validate", file)
	if out != "ok (bindings not checked: no --data)\n" {
		t.Errorf("got %q", out)
	}
}
