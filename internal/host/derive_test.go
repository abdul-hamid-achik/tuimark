package host

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// SPEC v0.4a §21 test 114: Derive. The store is {"items": [...], "view":
// {}}, with count counting items and view.label reading count.

const deriveDoc = `<tui version="1"><screen id="s"><text>{count}|{view.label}</text></screen></tui>`

func countItems(get func(string) (any, bool)) (any, error) {
	v, ok := get("items")
	arr, isArr := v.([]any)
	if !ok || !isArr {
		return 0, nil
	}
	return len(arr), nil
}

func labelOf(get func(string) (any, bool)) (any, error) {
	c, _ := get("count")
	return fmt.Sprintf("%v items", c), nil
}

func items(ids ...string) []any {
	out := []any{}
	for _, id := range ids {
		out = append(out, map[string]any{"id": id})
	}
	return out
}

// deriveApp is deriveDoc with count and view.label derived over two items.
func deriveApp(t *testing.T) *App {
	t.Helper()
	a := doc(t, deriveDoc)
	if err := a.Bind("", map[string]any{"items": items("a", "b"), "view": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	if err := a.Derive("count", countItems); err != nil {
		t.Fatal(err)
	}
	if err := a.Derive("view.label", labelOf); err != nil {
		t.Fatal(err)
	}
	return a
}

func mustGet(t *testing.T, a *App, path string) any {
	t.Helper()
	v, ok := a.Get(path)
	if !ok {
		t.Fatalf("Get(%q) is missing", path)
	}
	return v
}

func wantGet(t *testing.T, a *App, path string, want any) {
	t.Helper()
	if got := mustGet(t, a, path); !reflect.DeepEqual(got, want) {
		t.Fatalf("Get(%q) = %#v, want %#v", path, got, want)
	}
}

func dumpLine(a *App) string {
	return strings.TrimRight(a.Dump(30, 1, false).Grid[0], " ")
}

func TestDerive(t *testing.T) {
	t.Run("updates before Set returns", func(t *testing.T) {
		a := deriveApp(t)
		wantGet(t, a, "count", float64(2))
		wantGet(t, a, "view.label", "2 items")
		if err := a.Set("items", items("a", "b", "c")); err != nil {
			t.Fatal(err)
		}
		wantGet(t, a, "count", float64(3))
		wantGet(t, a, "view.label", "3 items")
		if got := dumpLine(a); got != "3|3 items" {
			t.Fatalf("Dump = %q", got)
		}
		// A write inside items (an array element) recomputes too.
		if err := a.Bind("items.0.id", "z"); err != nil {
			t.Fatal(err)
		}
		wantGet(t, a, "count", float64(3))
	})

	t.Run("get inside a Batch sees the batch's earlier writes", func(t *testing.T) {
		a := deriveApp(t)
		var calls atomic.Int32
		if err := a.Derive("pair", func(get func(string) (any, bool)) (any, error) {
			calls.Add(1)
			x, _ := get("x")
			y, _ := get("y")
			return fmt.Sprintf("%v/%v", x, y), nil
		}); err != nil {
			t.Fatal(err)
		}
		calls.Store(0)
		if err := a.Batch(func(b *Batch) error {
			_ = b.Set("x", 1)
			_ = b.Set("y", 2)
			return b.Set("items", items("q"))
		}); err != nil {
			t.Fatal(err)
		}
		wantGet(t, a, "pair", "1/2")
		wantGet(t, a, "count", float64(1))
		wantGet(t, a, "view.label", "1 items")
		if n := calls.Load(); n != 1 {
			t.Fatalf("pair computed %d times for one Batch, want 1", n)
		}
		// A Batch that fails applies nothing: no derived value changes.
		if err := a.Batch(func(b *Batch) error {
			_ = b.Set("x", 9)
			return errors.New("no")
		}); err == nil {
			t.Fatal("Batch: want the fn's error")
		}
		wantGet(t, a, "pair", "1/2")
	})

	t.Run("get of a reserved path is (nil, false)", func(t *testing.T) {
		a := deriveApp(t)
		var seen []string
		if err := a.Derive("probe", func(get func(string) (any, bool)) (any, error) {
			seen = seen[:0]
			for _, p := range []string{"@focus", "@screen", "@theme"} {
				v, ok := get(p)
				seen = append(seen, fmt.Sprintf("%s=%v,%v", p, v, ok))
			}
			return true, nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := a.Set("@theme", "light"); err != nil {
			t.Fatal(err)
		}
		if err := a.Set("items", items()); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(seen, " "); got != "@focus=<nil>,false @screen=<nil>,false @theme=<nil>,false" {
			t.Fatalf("get = %s", got)
		}
	})

	t.Run("writes to a derived path", func(t *testing.T) {
		a := deriveApp(t)
		for _, w := range []struct {
			name string
			fn   func() error
		}{
			{"Set", func() error { return a.Set("count", 1) }},
			{"Bind", func() error { return a.Bind("count", 1) }},
			{"Set inside", func() error { return a.Set("view.label.x", 1) }},
		} {
			err := w.fn()
			if err == nil || !errors.Is(err, errDerived) {
				t.Fatalf("%s: err = %v, want a derived-path error", w.name, err)
			}
		}
		wantGet(t, a, "count", float64(2))
		err := a.Batch(func(b *Batch) error {
			_ = b.Set("items", items())
			_ = b.Set("count", 1)
			return nil
		})
		if err == nil || !errors.Is(err, errDerived) {
			t.Fatalf("Batch: err = %v", err)
		}
		wantGet(t, a, "items", items("a", "b"))
		wantGet(t, a, "count", float64(2))
	})

	t.Run("writing an ancestor", func(t *testing.T) {
		a := deriveApp(t)
		// Set("", …) is followed by the recomputation.
		if err := a.Set("", map[string]any{"items": items("x")}); err != nil {
			t.Fatal(err)
		}
		wantGet(t, a, "count", float64(1))
		wantGet(t, a, "view", map[string]any{"label": "1 items"})
		if err := a.Set("view", map[string]any{"other": true}); err != nil {
			t.Fatal(err)
		}
		wantGet(t, a, "view", map[string]any{"other": true, "label": "1 items"})
		// Set("view", 5) fails, naming view.label, and changes nothing.
		err := a.Set("view", 5)
		if err == nil || !strings.Contains(err.Error(), `"view.label"`) {
			t.Fatalf("Set(view, 5): err = %v", err)
		}
		wantGet(t, a, "view", map[string]any{"other": true, "label": "1 items"})
		// So does a Batch holding it, applying none of its writes.
		err = a.Batch(func(b *Batch) error {
			_ = b.Set("items", items())
			return b.Set("view", 5)
		})
		if err == nil || !strings.Contains(err.Error(), `"view.label"`) {
			t.Fatalf("Batch: err = %v", err)
		}
		wantGet(t, a, "items", items("x"))
		wantGet(t, a, "count", float64(1))
		// A root without an object at view cannot hold view.label.
		if err := a.Set("", map[string]any{"view": "flat"}); err == nil {
			t.Fatal(`Set("", {"view": "flat"}): want an error`)
		}
		wantGet(t, a, "items", items("x"))
	})

	t.Run("registration errors", func(t *testing.T) {
		a := deriveApp(t)
		ok := func(get func(string) (any, bool)) (any, error) { return 1, nil }
		for _, p := range []string{"count.x", "view", "", "@focus", "@screen", "@theme", "a..b", "items.x", "items.5", "count"} {
			if err := a.Derive(p, ok); err == nil {
				t.Errorf("Derive(%q): want an error", p)
			}
		}
		if err := a.Derive("fresh", nil); err == nil {
			t.Error("Derive with a nil fn: want an error")
		}
		// Nothing was registered: the paths stay writable.
		if err := a.Set("count2", 1); err != nil {
			t.Fatal(err)
		}
		if _, ok := a.Get("fresh"); ok {
			t.Fatal("a failed Derive wrote its path")
		}
		// An index inside the array is a writable path.
		if err := a.Derive("items.1.n", ok); err != nil {
			t.Fatal(err)
		}
		wantGet(t, a, "items.1.n", float64(1))
	})

	t.Run("errors from fn write null and are kept once", func(t *testing.T) {
		a := deriveApp(t)
		boom := errors.New("boom")
		if err := a.Derive("bad", func(get func(string) (any, bool)) (any, error) {
			if v, _ := get("items"); len(v.([]any)) == 0 {
				return nil, boom
			}
			return "fine", nil
		}); err != nil {
			t.Fatal(err)
		}
		wantGet(t, a, "bad", "fine")
		if err := a.Set("items", items()); err != nil {
			t.Fatalf("the Set that caused it: %v", err)
		}
		wantGet(t, a, "bad", nil)
		// A value that cannot be coerced writes null too; the first
		// error is the one kept.
		if err := a.Derive("chan", func(func(string) (any, bool)) (any, error) { return make(chan int), nil }); err != nil {
			t.Fatal(err)
		}
		wantGet(t, a, "chan", nil)
		err := a.TakeDeriveErr()
		if !errors.Is(err, boom) || !strings.Contains(err.Error(), `"bad"`) {
			t.Fatalf("TakeDeriveErr = %v", err)
		}
		if err := a.TakeDeriveErr(); err != nil {
			t.Fatalf("reported twice: %v", err)
		}
		// Dump never reports it: chan fails on every recomputation, and
		// its error waits past a Dump.
		_ = a.Set("items", items("z"))
		_ = a.Dump(30, 1, false)
		if err := a.TakeDeriveErr(); err == nil || !strings.Contains(err.Error(), `"chan"`) {
			t.Fatalf("after Dump: %v", err)
		}
	})
}

// deriveWidgetDoc binds an input, a list cursor, a table cursor, a
// checked list, and a tabs to paths a test derives; each widget has a
// keymap row with a built-in targeting it, followed by a host row on the
// same key.
const deriveWidgetDoc = `<tui version="2">
<keymap>
  <bind keys="j" action="move-next" to="#l"/>
  <bind keys="j" action="fell_l"/>
  <bind keys="t" action="move-next" to="#t"/>
  <bind keys="t" action="fell_t"/>
  <bind keys="x" action="check-toggle" to="#c"/>
  <bind keys="x" action="fell_c"/>
  <bind keys="n" action="move-next" to="#tabs"/>
  <bind keys="n" action="fell_tabs"/>
  <bind keys="w" action="switch-to" to="#two"/>
  <bind keys="w" action="fell_w"/>
</keymap>
<screen id="s">
  <input id="q" bind="q" on:change="chg_q"/>
  <list id="l" each="rows as r" key="r.id" bind="cur" on:select="sel_l"><item><text>{r.id}</text></item></list>
  <table id="t" each="rows as r" key="r.id" bind="tcur" on:select="sel_t"><column title="ID">{r.id}</column></table>
  <list id="c" each="rows as r" key="r.id" checked="marked" on:change="chg_c"><item><text>{r.id}</text></item></list>
  <tabs id="tabs" bind="view" focusable="true" on:select="sel_tabs">
    <tab id="one" label="one"><text>1</text></tab>
    <tab id="two" label="two"><text>2</text></tab>
  </tabs>
</screen>
</tui>`

func deriveWidgets(t *testing.T, derived bool) *App {
	t.Helper()
	a := doc(t, deriveWidgetDoc)
	if err := a.Bind("", map[string]any{
		"q": "x", "cur": "a", "tcur": "a", "marked": []any{}, "view": "one",
		"rows": items("a", "b", "c"),
	}); err != nil {
		t.Fatal(err)
	}
	if derived {
		for p, v := range map[string]any{"q": "x", "cur": "a", "tcur": "a", "marked": []any{}, "view": "one"} {
			v := v
			if err := a.Derive(p, func(func(string) (any, bool)) (any, error) { return v, nil }); err != nil {
				t.Fatal(err)
			}
		}
	}
	return a
}

// focusOn focuses id through a frame, as a play focus: step does.
func focusOn(t *testing.T, a *App, id string) {
	t.Helper()
	if err := a.Set("@focus", "#"+id); err != nil {
		t.Fatal(err)
	}
	a.Frame(80, 24)
	if a.Focus() != id {
		t.Fatalf("focus = %q, want %q", a.Focus(), id)
	}
}

func TestDeriveWidgets(t *testing.T) {
	type tc struct {
		focus string
		keys  []Key
		// without Derive, then with it
		plain, blocked string
		path           string
		value          any // the derived path's value afterwards
	}
	cases := map[string]tc{
		"input text":      {"q", typed("ab"), "chg_q chg_q", "", "q", "x"},
		"input backspace": {"q", []Key{named("backspace"), named("ctrl+u")}, "chg_q", "", "q", "x"},
		"list cursor":     {"l", []Key{named("down"), named("end")}, "sel_l sel_l", "", "cur", "a"},
		"list move-next":  {"l", []Key{r('j')}, "sel_l", "fell_l", "cur", "a"},
		"table cursor":    {"t", []Key{named("down"), named("pgdn")}, "sel_t sel_t", "", "tcur", "a"},
		"table move-next": {"t", []Key{r('t')}, "sel_t", "fell_t", "tcur", "a"},
		"check-toggle":    {"c", []Key{r('x')}, "chg_c", "fell_c", "marked", []any{}},
		"tabs keys":       {"tabs", []Key{named("right")}, "sel_tabs", "", "view", "one"},
		"tabs move-next":  {"tabs", []Key{r('n')}, "sel_tabs", "fell_tabs", "view", "one"},
		"switch-to":       {"c", []Key{r('w')}, "sel_tabs", "fell_w", "view", "one"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			// The fixture itself: without Derive, the keys change the
			// widget and fire its event.
			plain := deriveWidgets(t, false)
			focusOn(t, plain, c.focus)
			if got := actions(press(plain, c.keys...)); got != c.plain {
				t.Fatalf("without Derive: events %q, want %q", got, c.plain)
			}
			if reflect.DeepEqual(mustGet(t, plain, c.path), c.value) {
				t.Fatalf("without Derive: %s did not change", c.path)
			}

			a := deriveWidgets(t, true)
			focusOn(t, a, c.focus)
			before := strings.Join(a.Frame(80, 24).Grid.Lines(), "\n")
			if got := actions(press(a, c.keys...)); got != c.blocked {
				t.Fatalf("with Derive: events %q, want %q", got, c.blocked)
			}
			wantGet(t, a, c.path, c.value)
			if after := strings.Join(a.Frame(80, 24).Grid.Lines(), "\n"); after != before {
				t.Fatalf("with Derive the frame changed:\n%s\n---\n%s", before, after)
			}
		})
	}
}

// A derive over an input's bind updates on each keystroke.
func TestDeriveOverInput(t *testing.T) {
	a := doc(t, `<tui version="1"><screen id="s" focus="#q">
<input id="q" bind="query"/>
<text>{hits}</text>
</screen></tui>`)
	if err := a.Bind("", map[string]any{"query": "", "words": []any{"apple", "apricot", "banana"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.Derive("hits", func(get func(string) (any, bool)) (any, error) {
		q, _ := get("query")
		ws, _ := get("words")
		var hits []string
		for _, w := range ws.([]any) {
			if strings.HasPrefix(w.(string), q.(string)) {
				hits = append(hits, w.(string))
			}
		}
		return strings.Join(hits, ","), nil
	}); err != nil {
		t.Fatal(err)
	}
	a.Frame(40, 3)
	for _, step := range []struct{ key, want string }{
		{"a", "apple,apricot"}, {"p", "apple,apricot"}, {"r", "apricot"}, {"x", ""},
	} {
		press(a, r(rune(step.key[0])))
		wantGet(t, a, "hits", step.want)
	}
	press(a, named("backspace"))
	wantGet(t, a, "hits", "apricot")
}

// errOnOne is a Derive function that fails while n is 1.
func errOnOne(get func(string) (any, bool)) (any, error) {
	if v, _ := get("n"); v == float64(1) {
		return nil, errBoom
	}
	return "ok", nil
}

// Under RunWith a Derive error reaches OnError with a nil event, on the
// loop, and the session goes on; without OnError it ends Run. An error
// raised with no session active is reported at the start of the next one.
func TestDeriveErrorsUnderRun(t *testing.T) {
	t.Run("OnError gets a nil event", func(t *testing.T) {
		a := newEveryApp(t)
		if err := a.Derive("d", errOnOne); err != nil {
			t.Fatal(err)
		}
		type call struct {
			ev  *Event
			err error
		}
		calls := make(chan call, 4)
		r := runEvery(t, a, RunOptions{OnError: func(ev *Event, err error) error {
			calls <- call{ev, err}
			return nil
		}})
		waitUntil(t, "the first frame", func() bool { return strings.Contains(screenOf(a), "N=0") })
		if err := a.Set("n", 1); err != nil {
			t.Fatalf("the Set that caused it returns %v", err)
		}
		select {
		case c := <-calls:
			if c.ev != nil || !errors.Is(c.err, errBoom) {
				t.Fatalf("OnError(%v, %v)", c.ev, c.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("OnError was not called")
		}
		if v, ok := a.Get("d"); !ok || v != nil {
			t.Fatalf("d = %v, %v; want null", v, ok)
		}
		// The session goes on.
		if err := a.Set("n", 2); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "N=2", func() bool { return strings.Contains(screenOf(a), "N=2") })
		if err := r.quit(t); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 0 {
			t.Fatalf("OnError called %d more times", len(calls))
		}
	})
	t.Run("without OnError it ends Run", func(t *testing.T) {
		a := newEveryApp(t)
		if err := a.Derive("d", errOnOne); err != nil {
			t.Fatal(err)
		}
		r := runEvery(t, a, RunOptions{})
		waitUntil(t, "the first frame", func() bool { return strings.Contains(screenOf(a), "N=0") })
		if err := a.Set("n", 1); err != nil {
			t.Fatal(err)
		}
		if err := r.wait(t); !errors.Is(err, errBoom) {
			t.Fatalf("Run = %v, want the Derive error", err)
		}
	})
	t.Run("kept until the next Run", func(t *testing.T) {
		a := newEveryApp(t)
		if err := a.Derive("d", errOnOne); err != nil {
			t.Fatal(err)
		}
		if err := a.Set("n", 1); err != nil {
			t.Fatal(err)
		}
		_ = a.Dump(20, 3, false)
		var got []error
		r := runEvery(t, a, RunOptions{OnError: func(ev *Event, err error) error {
			got = append(got, err)
			return ErrQuit
		}})
		if err := r.wait(t); err != nil {
			t.Fatalf("Run = %v; OnError's quit ends it as a quit", err)
		}
		if len(got) != 1 || !errors.Is(got[0], errBoom) {
			t.Fatalf("OnError got %v", got)
		}
	})
}
