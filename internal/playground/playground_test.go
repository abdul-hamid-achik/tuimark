package playground

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

const doc = `<tui>
  <style src="theme.tcss"/>
  <screen id="s" focus="#name">
    <col>
      <text>Hello {user}</text>
      <text if="saved">saved</text>
      <input id="name" bind="draft" on:submit="save"/>
      <list id="l" each="items as it" key="it.id" bind="cur">
        <item><text>{it.title}</text></item>
      </list>
    </col>
  </screen>
</tui>`

const sample = `{"user":"ada","draft":"","saved":false,"cur":"a","items":[{"id":"a","title":"first"},{"id":"b","title":"second"}]}`

func files() map[string]string {
	return map[string]string{
		"app.tui":     doc,
		"theme.tcss":  `text { color: red; }`,
		"sample.json": sample,
		"mock.json":   `{"save":[{"path":"saved","value":true}]}`,
	}
}

func js(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// call runs a method and decodes its result into a generic object.
func call(t *testing.T, m *Manager, method string, args ...string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(m.Call(method, args...)), &out); err != nil {
		t.Fatalf("%s: not JSON: %v", method, err)
	}
	return out
}

func mustOpen(t *testing.T, m *Manager, f map[string]string, opts string) (int, map[string]any) {
	t.Helper()
	r := call(t, m, "open", js(t, f), opts)
	if e, bad := r["error"]; bad {
		t.Fatalf("open: %v", e)
	}
	return int(r["session"].(float64)), r
}

func grid(r map[string]any) string {
	lines := r["dump"].(map[string]any)["grid"].([]any)
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.(string))
		b.WriteByte('\n')
	}
	return b.String()
}

func TestOpenRenderStepValidateClose(t *testing.T) {
	m := NewManager()
	id, r := mustOpen(t, m, files(), "")
	if g := grid(r); !strings.Contains(g, "Hello ada") || !strings.Contains(g, "first") {
		t.Fatalf("first frame:\n%s", g)
	}
	if d := r["dump"].(map[string]any); d["cols"] != 80.0 || d["rows"] != 24.0 {
		t.Fatalf("default size = %vx%v", d["cols"], d["rows"])
	}
	if _, ok := r["dump"].(map[string]any)["styles"]; !ok {
		t.Fatal("dump has no styles")
	}
	if ev, ok := r["events"].([]any); !ok || len(ev) != 0 {
		t.Fatalf("events = %v, want []", r["events"])
	}

	// Typing and submitting fires the action once and the mock writes.
	s := call(t, m, "step", js(t, id), js(t, []string{"text:hi", "enter"}))
	if e := s["error"]; e != nil {
		t.Fatal(e)
	}
	evs := s["events"].([]any)
	if len(evs) != 1 || evs[0].(map[string]any)["action"] != "save" || evs[0].(map[string]any)["value"] != "hi" {
		t.Fatalf("events = %v", evs)
	}
	if !strings.Contains(grid(s), "saved") {
		t.Fatalf("the mock write did not show:\n%s", grid(s))
	}

	// A single string is one step; the size and the typed text persist.
	s = call(t, m, "step", js(t, id), `"down"`, `{"cols":60,"rows":10}`)
	if d := s["dump"].(map[string]any); d["cols"] != 60.0 || d["rows"] != 10.0 {
		t.Fatalf("size = %vx%v", d["cols"], d["rows"])
	}
	s = call(t, m, "render", js(t, id))
	if d := s["dump"].(map[string]any); d["cols"] != 60.0 {
		t.Fatalf("render forgot the size: %v", d["cols"])
	}
	// A resize: step changes the size the next call starts from.
	call(t, m, "step", js(t, id), `"resize:50x8"`)
	s = call(t, m, "render", js(t, id))
	if d := s["dump"].(map[string]any); d["cols"] != 50.0 || d["rows"] != 8.0 {
		t.Fatalf("after resize: %vx%v", d["cols"], d["rows"])
	}

	// Theme through opts.
	s = call(t, m, "render", js(t, id), `{"theme":"light"}`)
	if th := s["dump"].(map[string]any)["theme"]; th != "light" {
		t.Fatalf("theme = %v", th)
	}
	if e := call(t, m, "render", js(t, id), `{"theme":"auto"}`)["error"]; e == nil {
		t.Fatal("theme auto accepted")
	}

	v := call(t, m, "validate", js(t, id))
	if v["ok"] != true || v["file"] != "app.tui" || v["data"] != "sample.json" {
		t.Fatalf("validate = %v", v)
	}
	if diags, ok := v["diagnostics"].([]any); !ok || len(diags) != 0 {
		t.Fatalf("diagnostics = %v", v["diagnostics"])
	}

	if r := call(t, m, "close", js(t, id)); r["error"] != nil {
		t.Fatal(r["error"])
	}
	if e := call(t, m, "render", js(t, id))["error"]; e != "no such session" {
		t.Fatalf("after close: %v", e)
	}
}

func TestDefaultsAndNames(t *testing.T) {
	m := NewManager()
	// No sample.json and no mock.json: no data, so bindings are not
	// checked and validate says data is null.
	f := map[string]string{"x/doc.tui": `<tui><screen id="s"><text>{nope}</text></screen></tui>`}
	id, r := mustOpen(t, m, f, "")
	if !strings.Contains(grid(r), "") {
		t.Fatal("no grid")
	}
	v := call(t, m, "validate", js(t, id))
	if v["data"] != nil || v["file"] != "x/doc.tui" {
		t.Fatalf("validate = %v", v)
	}
	for _, d := range v["diagnostics"].([]any) {
		if d.(map[string]any)["code"] == "B003" {
			t.Fatal("B003 reported without data")
		}
	}
	// NAME.sample.json beside the entry, then sample.json.
	f["x/doc.sample.json"] = `{"nope":"yes"}`
	_, r = mustOpen(t, m, f, "")
	if !strings.Contains(grid(r), "yes") {
		t.Fatalf("doc.sample.json not bound:\n%s", grid(r))
	}
	// Leading ./ and / are dropped.
	_, r = mustOpen(t, m, map[string]string{"./app.tui": doc, "/sample.json": sample, "theme.tcss": ""}, "")
	if !strings.Contains(grid(r), "Hello ada") {
		t.Fatalf("./ and / keys:\n%s", grid(r))
	}
}

func TestOpenErrors(t *testing.T) {
	m := NewManager()
	bad := map[string]struct {
		files, opts, want string
	}{
		"no files":      {`{}`, ``, "no files"},
		"not an object": {`[1]`, ``, "files"},
		"climbing key":  {`{"../a.tui":"x"}`, ``, "valid relative path"},
		"two tui":       {`{"a.tui":"<tui/>","b.tui":"<tui/>"}`, ``, "set opts.entry"},
		"missing entry": {`{"a.tui":"<tui/>"}`, `{"entry":"b.tui"}`, `entry "b.tui" is not in files`},
		"missing data":  {`{"a.tui":"<tui/>"}`, `{"data":"d.json"}`, `data "d.json" is not in files`},
		"bad data":      {`{"a.tui":"<tui/>","sample.json":"{"}`, ``, "sample.json"},
		"bad theme":     {`{"a.tui":"<tui/>"}`, `{"theme":"neon"}`, "dark or light"},
		"bad size":      {`{"a.tui":"<tui/>"}`, `{"cols":5000}`, ""},
	}
	for name, c := range bad {
		out := call(t, m, "open", c.files, c.opts)
		e, _ := out["error"].(string)
		if e == "" || !strings.Contains(e, c.want) {
			t.Errorf("%s: error = %q, want it to contain %q", name, e, c.want)
		}
	}
	if len(m.sessions) != 0 {
		t.Fatalf("%d sessions left behind by failed opens", len(m.sessions))
	}
	if e := call(t, m, "nope")["error"]; e == nil {
		t.Fatal("unknown method accepted")
	}
	if e := call(t, m, "render", `"x"`)["error"]; e == nil {
		t.Fatal("non-numeric session accepted")
	}
	// A document with errors still opens; the errors are in the dump.
	_, r := mustOpen(t, m, map[string]string{"app.tui": `<tui><screen id="s"><blink/></screen></tui>`}, "")
	if r["dump"].(map[string]any)["ok"] != false {
		t.Fatal("dump.ok for a bad document")
	}
	// Not well-formed XML opens too.
	_, r = mustOpen(t, m, map[string]string{"app.tui": `<tui><screen id="s"><text>hi</screen></tui>`}, "")
	if r["dump"].(map[string]any)["ok"] != false {
		t.Fatal("dump.ok for broken XML")
	}
}

func TestMock(t *testing.T) {
	m := NewManager()
	f := files()
	for _, c := range []struct{ mock, want string }{
		{`{"nope":[]}`, `"nope" is not an action`},
		{`{"save":{}}`, `want an array`},
		{`{"save":[{"path":"a"}]}`, `needs a string "path" and a "value"`},
		{`[]`, `want one JSON object`},
	} {
		f["mock.json"] = c.mock
		e, _ := call(t, m, "open", js(t, f))["error"].(string)
		if !strings.Contains(e, c.want) {
			t.Errorf("mock %s: error %q, want %q", c.mock, e, c.want)
		}
	}
	// A failing write is reported, not fatal: the call goes on.
	f["mock.json"] = `{"save":[{"path":"user.deep","value":1},{"path":"saved","value":true}]}`
	id, _ := mustOpen(t, m, f, "")
	s := call(t, m, "step", js(t, id), js(t, []string{"text:x", "enter"}))
	if s["error"] != nil {
		t.Fatalf("a failed mock batch ended the call: %v", s["error"])
	}
	if len(s["events"].([]any)) != 1 {
		t.Fatalf("events = %v", s["events"])
	}
	if me, _ := s["mockErrors"].([]any); len(me) != 1 || !strings.HasPrefix(me[0].(string), "save: ") {
		t.Fatalf("mockErrors = %v", s["mockErrors"])
	}
}

func TestStepLimitsAndPartialFailure(t *testing.T) {
	m := NewManager()
	id, _ := mustOpen(t, m, files(), "")
	big := "paste:" + strings.Repeat("x", MaxPaste+1)
	if e, _ := call(t, m, "step", js(t, id), js(t, []string{big}))["error"].(string); !strings.Contains(e, "paste") {
		t.Fatalf("oversized paste: %q", e)
	}
	ok := "paste:" + strings.Repeat("x", MaxPaste)
	if e := call(t, m, "step", js(t, id), js(t, []string{ok}))["error"]; e != nil {
		t.Fatalf("paste at the cap: %v", e)
	}
	// An unparseable step is an error and applies nothing.
	if e := call(t, m, "step", js(t, id), `["down","wat:1"]`)["error"]; e == nil {
		t.Fatal("bad step accepted")
	}
	// A step that cannot be applied is a StepError with the frame so far.
	s := call(t, m, "step", js(t, id), `["click:500,500"]`)
	if s["error"] != nil || s["stepError"] == nil || s["dump"] == nil {
		t.Fatalf("click outside the grid: %v", s)
	}
	if e := call(t, m, "step", js(t, id), `{"a":1}`)["error"]; e == nil {
		t.Fatal("steps object accepted")
	}
}

func TestReloadKeepsState(t *testing.T) {
	m := NewManager()
	id, _ := mustOpen(t, m, files(), "")
	call(t, m, "step", js(t, id), js(t, []string{"text:abc", "tab", "down"}))
	f := files()
	f["app.tui"] = strings.Replace(doc, "Hello {user}", "Howdy {user}", 1)
	r := call(t, m, "reload", js(t, id), js(t, f))
	if r["error"] != nil {
		t.Fatal(r["error"])
	}
	g := grid(r)
	if !strings.Contains(g, "Howdy ada") || !strings.Contains(g, "abc") {
		t.Fatalf("reload lost the edit or the typed text:\n%s", g)
	}
	if r["dump"].(map[string]any)["focus"] != "l" {
		t.Fatalf("focus = %v, want the list", r["dump"].(map[string]any)["focus"])
	}
	// A bad reload leaves the session as it was.
	if e := call(t, m, "reload", js(t, id), `{"other.tui":"<tui/>","app.tui":"<tui/>","sample.json":"{"}`)["error"]; e == nil {
		t.Fatal("reload with bad data accepted")
	}
	if g := grid(call(t, m, "render", js(t, id))); !strings.Contains(g, "Howdy ada") {
		t.Fatalf("a failed reload changed the session:\n%s", g)
	}
	if e := call(t, m, "reload", `99`, js(t, f))["error"]; e != "no such session" {
		t.Fatalf("reload of a missing session: %v", e)
	}
}

func TestPanicIsRecovered(t *testing.T) {
	m := NewManager()
	m.sessions[7] = &session{cols: 80, rows: 24} // no app: Play panics
	r := call(t, m, "render", `7`)
	if e, _ := r["error"].(string); !strings.Contains(e, "internal error") {
		t.Fatalf("panic result = %v", r)
	}
	if _, still := m.sessions[7]; still {
		t.Fatal("the session that panicked was kept")
	}
	// The manager still works.
	mustOpen(t, m, files(), "")
}

func TestSessionCap(t *testing.T) {
	m := NewManager()
	for i := 0; i < maxSessions; i++ {
		mustOpen(t, m, map[string]string{"app.tui": `<tui><screen id="s"><text>x</text></screen></tui>`}, "")
	}
	e, _ := call(t, m, "open", `{"app.tui":"<tui/>"}`)["error"].(string)
	if !strings.Contains(e, "too many") {
		t.Fatalf("open past the cap: %q", e)
	}
}

func TestMemFS(t *testing.T) {
	m := memFS{"a/b.txt": "hi", "a/c/d.txt": "yo", "z.txt": "zz"}
	if err := fstest.TestFS(m, "a/b.txt", "a/c/d.txt", "z.txt"); err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(m, "a/b.txt")
	if err != nil || string(b) != "hi" {
		t.Fatalf("ReadFile = %q, %v", b, err)
	}
	if _, err := fs.ReadFile(m, "../x"); err == nil {
		t.Fatal("climbing path read")
	}
	if _, err := fs.ReadFile(m, "missing"); err == nil {
		t.Fatal("missing file read")
	}
}
