package tuimark_test

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4d §21 tests of phase D5a-1 through the Go API (§18.3): test 302
// (Append) and test 303's Batch and Play parts. The wire side (fd 3, ready)
// is cmd/tuimark's conformance_v04d_host_test.go; the TypeScript client is
// sdk/ts.

const d5Doc = `<tui>
  <screen id="a"><text>{s}</text></screen>
  <screen id="b"><text>{n}</text></screen>
</tui>`

func d5App(t *testing.T) *tuimark.App {
	t.Helper()
	app := parseDoc(t, d5Doc)
	bindJSON(t, app, "", `{"s": "ab", "a": [1], "n": 3, "o": {}, "z": null}`)
	return app
}

func mustGet(t *testing.T, app *tuimark.App, path string, want any) {
	t.Helper()
	got, ok := app.Get(path)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("Get(%q) = %#v, %v; want %#v", path, got, ok, want)
	}
}

func wantErr(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Errorf("error %v, want %q", err, want)
	}
}

// SPEC 0.4d test 302: the three successes, then every error with its text,
// each changing nothing.
func TestD5Append302(t *testing.T) {
	app := d5App(t)
	if err := app.Append("s", "c"); err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "s", "abc")
	if err := app.Append("a", []any{2.0, 3.0}); err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "a", []any{1.0, []any{2.0, 3.0}})
	if err := app.Append("a", nil); err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "a", []any{1.0, []any{2.0, 3.0}, nil})

	before, _ := app.Get("")
	cases := []struct {
		path string
		v    any
		want string
	}{
		{"missing", "x", `append "missing": "missing" is missing`},
		{"n", 1, `append "n": "n" holds a number, not a string or an array`},
		{"o", 1, `append "o": "o" holds an object, not a string or an array`},
		{"z", "x", `append "z": "z" holds null, not a string or an array`},
		{"s", 5, `append "s": "s" holds a string, and the value is a number, not a string`},
		{"s", nil, `append "s": "s" holds a string, and the value is null, not a string`},
		{"s", []any{"x"}, `append "s": "s" holds a string, and the value is an array, not a string`},
		{"s", map[string]any{}, `append "s": "s" holds a string, and the value is an object, not a string`},
		{"s", true, `append "s": "s" holds a string, and the value is a boolean, not a string`},
		{"", 1, `append "": the root is not a string or an array`},
		{"@focus", "x", `append "@focus": "@focus" is a reserved path`},
		{"@screen", "x", `append "@screen": "@screen" is a reserved path`},
		{"@theme", "x", `append "@theme": "@theme" is a reserved path`},
		{"a..b", "x", `tuimark: bad path "a..b"`},
		{"a.", "x", `tuimark: bad path "a."`},
	}
	for _, c := range cases {
		wantErr(t, app.Append(c.path, c.v), c.want)
	}
	if after, _ := app.Get(""); !reflect.DeepEqual(before, after) {
		t.Errorf("a failed Append changed the store: %v -> %v", before, after)
	}
	// A value that is not JSON-shaped is Set's error.
	if err := app.Append("a", func() {}); err == nil || !strings.Contains(err.Error(), "not JSON-shaped") {
		t.Errorf("a func value: %v", err)
	}
	// An element of an array reached by index.
	bindJSON(t, app, "", `{"m": {"log": ["x"], "t": "hi"}, "rows": [{"t": "a"}]}`)
	if err := app.Append("m.log", "y"); err != nil {
		t.Fatal(err)
	}
	if err := app.Append("rows.0.t", "b"); err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "m.log", []any{"x", "y"})
	mustGet(t, app, "rows.0.t", "ab")
}

// SPEC 0.4d test 302: on a derived path the error is Set's, and a Derive
// over an array sees the appended element.
func TestD5AppendDerive302(t *testing.T) {
	app := d5App(t)
	err := app.Derive("count", func(get func(string) (any, bool)) (any, error) {
		v, _ := get("a")
		arr, _ := v.([]any)
		return len(arr), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "count", 1.0)
	if err := app.Append("a", "two"); err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "count", 2.0)
	wantErr(t, app.Append("count", 1), `tuimark: "count" is a derived path (Derive): only its function writes it`)
	setErr := app.Set("count", 1)
	if setErr == nil || setErr.Error() != `tuimark: "count" is a derived path (Derive): only its function writes it` {
		t.Fatalf("Set on a derived path: %v", setErr)
	}
	mustGet(t, app, "count", 2.0)

	// A derived path inside an object: the error names the path inside it.
	if err := app.Derive("view.rows", func(get func(string) (any, bool)) (any, error) {
		v, _ := get("a")
		return v, nil
	}); err != nil {
		t.Fatal(err)
	}
	wantErr(t, app.Append("view.rows", 1), `tuimark: "view.rows" is a derived path (Derive): only its function writes it`)
	mustGet(t, app, "view.rows", []any{1.0, "two"})
	// A path inside a derived path is Set's error for it, which names the
	// derived path (the SPEC quotes only the first form).
	insideErr := `tuimark: "view.rows.x" is inside the derived path "view.rows" (Derive): only its function writes it`
	wantErr(t, app.Append("view.rows.x", 1), insideErr)
	if setErr := app.Set("view.rows.x", 1); setErr == nil || setErr.Error() != insideErr {
		t.Errorf("Set inside a derived path: %v", setErr)
	}
	if err := app.Append("a", "three"); err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "view.rows", []any{1.0, "two", "three"})
}

// SPEC 0.4d test 303: Batch.
func TestD5BatchAppend303(t *testing.T) {
	app := d5App(t)
	err := app.Batch(func(b *tuimark.Batch) error {
		if err := b.Set("log", []any{}); err != nil {
			return err
		}
		return b.Append("log", "x")
	})
	if err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "log", []any{"x"})

	// An append that fails when the batch applies applies nothing.
	before, _ := app.Get("")
	err = app.Batch(func(b *tuimark.Batch) error {
		_ = b.Set("s", "changed")
		_ = b.Append("log", "y")
		_ = b.Append("n", 1) // checked at apply: n holds a number
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), `append "n": "n" holds a number, not a string or an array`) {
		t.Fatalf("batch with a failing append: %v", err)
	}
	if after, _ := app.Get(""); !reflect.DeepEqual(before, after) {
		t.Errorf("a failed batch changed the store:\n%v\n%v", before, after)
	}
	// ... and the store appends correctly after the rolled-back batch.
	if err := app.Append("log", "z"); err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "log", []any{"x", "z"})

	// The argument checks happen when the append is queued, as Set's do,
	// and every failure is a part of the joined error.
	err = app.Batch(func(b *tuimark.Batch) error {
		_ = b.Append("@focus", "x")
		_ = b.Append("", 1)
		_ = b.Append("a..b", 1)
		return nil
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	var parts []string
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		for _, p := range j.Unwrap() {
			parts = append(parts, p.Error())
		}
	}
	want := []string{
		`append "@focus": "@focus" is a reserved path`,
		`append "": the root is not a string or an array`,
		`tuimark: bad path "a..b"`,
	}
	if !reflect.DeepEqual(parts, want) {
		t.Errorf("parts %q, want %q", parts, want)
	}

	// A write earlier in the batch decides what a later append meets, in
	// either direction.
	err = app.Batch(func(b *tuimark.Batch) error {
		_ = b.Set("s", []any{})
		return b.Append("s", "x") // s is an array by then
	})
	if err != nil {
		t.Fatalf("append to a value the batch just set: %v", err)
	}
	mustGet(t, app, "s", []any{"x"})
	err = app.Batch(func(b *tuimark.Batch) error {
		_ = b.Append("s", "y")
		_ = b.Set("s", "str")
		return b.Append("s", "z") // a string by then
	})
	if err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "s", "strz")

	// A derived path is refused when the append is queued.
	if err := app.Derive("count", func(get func(string) (any, bool)) (any, error) { return 0, nil }); err != nil {
		t.Fatal(err)
	}
	err = app.Batch(func(b *tuimark.Batch) error { return b.Append("count", 1) })
	if err == nil || !strings.Contains(err.Error(), `tuimark: "count" is a derived path (Derive): only its function writes it`) {
		t.Errorf("append to a derived path in a batch: %v", err)
	}

	// A Batch kept after its fn returned.
	var kept *tuimark.Batch
	_ = app.Batch(func(b *tuimark.Batch) error { kept = b; return nil })
	if err := kept.Append("log", "late"); err == nil {
		t.Error("Append on a finished Batch must fail")
	}
}

// A batch of appends that fails halfway leaves the arrays it appended to
// whole, however much spare capacity their backing arrays have, and a
// later append gives the right array (the trial never writes into a
// backing array it does not own).
func TestD5BatchAppendRollbackKeepsArrays(t *testing.T) {
	app := parseDoc(t, d5Doc)
	bindJSON(t, app, "", `{"log": [1, 2, 3, 4, 5], "n": 1}`)
	for i := 0; i < 3; i++ {
		err := app.Batch(func(b *tuimark.Batch) error {
			_ = b.Append("log", "p")
			_ = b.Append("log", "q")
			_ = b.Append("n", 1)
			return nil
		})
		if err == nil {
			t.Fatal("expected the batch to fail")
		}
		mustGet(t, app, "log", []any{1.0, 2.0, 3.0, 4.0, 5.0})
	}
	err := app.Batch(func(b *tuimark.Batch) error {
		_ = b.Append("log", "p")
		return b.Append("log", "q")
	})
	if err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "log", []any{1.0, 2.0, 3.0, 4.0, 5.0, "p", "q"})
	if err := app.Append("log", "r"); err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "log", []any{1.0, 2.0, 3.0, 4.0, 5.0, "p", "q", "r"})
}

// Appends from several goroutines are all applied, in some order (-race).
func TestD5AppendConcurrent(t *testing.T) {
	app := parseDoc(t, d5Doc)
	bindJSON(t, app, "", `{"log": [], "s": ""}`)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if err := app.Append("log", float64(i)); err != nil {
					t.Error(err)
				}
				if err := app.Append("s", "x"); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	v, _ := app.Get("log")
	if got := len(v.([]any)); got != 400 {
		t.Errorf("log has %d elements, want 400", got)
	}
	s, _ := app.Get("s")
	if got := len(s.(string)); got != 400 {
		t.Errorf("s has %d bytes, want 400", got)
	}
}

// Append redraws like Set: it wakes a running loop (SPEC §18.3).
func TestD5AppendDump(t *testing.T) {
	app := d5App(t)
	if err := app.Append("s", "c"); err != nil {
		t.Fatal(err)
	}
	d, err := app.Dump(20, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(d.Grid[0], "abc") {
		t.Errorf("grid %q does not show the appended text", d.Grid[0])
	}
}

// SPEC 0.4d test 303: Play takes the step append:PATH=JSON, and a failing
// one stops the call with the error of §18.3 as its reason.
func TestD5PlayAppend303(t *testing.T) {
	app := parseDoc(t, `<tui><screen id="a"><list id="l" each="messages as m" key="m.text"><item><text>{m.text}</text></item></list></screen></tui>`)
	bindJSON(t, app, "", `{"messages": []}`)
	res, err := app.Play(tuimark.PlayOptions{NoHandlers: true}, `append:messages={"text":"hi"}`, `append:messages={"text":"yo"}`)
	if err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "messages", []any{map[string]any{"text": "hi"}, map[string]any{"text": "yo"}})
	if !strings.Contains(strings.Join(res.Dump.Grid, "\n"), "yo") {
		t.Errorf("the dump does not show the appended message: %q", res.Dump.Grid)
	}

	_, err = app.Play(tuimark.PlayOptions{NoHandlers: true}, `append:nope="x"`)
	if err == nil || !strings.Contains(err.Error(), `step 1 (append:nope="x"): append "nope": "nope" is missing`) {
		t.Errorf("a failing append step: %v", err)
	}
	_, err = app.Play(tuimark.PlayOptions{NoHandlers: true}, `append:messages`)
	if err == nil || !strings.Contains(err.Error(), "append: wants PATH=JSON") {
		t.Errorf("append without =: %v", err)
	}
	_, err = app.Play(tuimark.PlayOptions{NoHandlers: true}, `append:messages={bad`)
	if err == nil || !strings.Contains(err.Error(), "bad JSON value") {
		t.Errorf("append with bad JSON: %v", err)
	}
	if errors.Is(err, tuimark.ErrQuit) {
		t.Error("a usage error is not a quit")
	}
}

// SPEC 0.4d test 313, Go host line: Set("@screen", id) before the session
// gives the first frame (here Dump's and Play's step 0, built as Run's
// first frame is) on that screen; an id no screen has is an error.
func TestD5GoHostScreen313(t *testing.T) {
	app := parseDoc(t, `<tui><screen id="a"><text>SCREEN-A</text></screen><screen id="b"><text>SCREEN-B</text></screen></tui>`)
	wantErr(t, app.Set("@screen", "nope"), `tuimark: no screen with id "nope"`)
	if err := app.Set("@screen", "b"); err != nil {
		t.Fatal(err)
	}
	mustGet(t, app, "@screen", "b")
	res, err := app.Play(tuimark.PlayOptions{NoHandlers: true})
	if err != nil {
		t.Fatal(err)
	}
	g := strings.Join(res.Dump.Grid, "\n")
	if !strings.Contains(g, "SCREEN-B") || strings.Contains(g, "SCREEN-A") {
		t.Errorf("step 0 is not screen b:\n%s", g)
	}
}
