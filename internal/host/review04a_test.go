package host

import (
	"context"
	"io"
	"testing"
	"time"
)

// Regression tests for the adversarial review of the 0.4a runtime.

// returnsWithin fails t when f does not return within a second: a call
// that blocks on a.mu left held.
func returnsWithin(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("%s blocks: the app's lock was left held", what)
	}
}

// recovered calls f and returns what it panicked with, or nil.
func recovered(f func()) (r any) {
	defer func() { r = recover() }()
	f()
	return nil
}

// panicAt is a Derive fn that panics when n is 2 and is n*10 otherwise.
func panicAt(get func(string) (any, bool)) (any, error) {
	v, _ := get("n")
	n, _ := v.(float64)
	if n == 2 {
		panic("boom")
	}
	return n * 10, nil
}

const panicDoc = `<tui version="3"><keymap><bind keys="g" action="go"/></keymap><screen id="s"><text>{n} {d}</text></screen></tui>`

func panicApp(t *testing.T) *App {
	t.Helper()
	a := doc(t, panicDoc)
	if err := a.Bind("", map[string]any{"n": 1.0}); err != nil {
		t.Fatal(err)
	}
	if err := a.Derive("d", panicAt); err != nil {
		t.Fatal(err)
	}
	return a
}

// A panic in a Derive fn is not recovered (SPEC v0.4a §18.2), but it must
// leave the app's lock released: every later call used to block forever.
func TestDerivePanicReleasesLock(t *testing.T) {
	t.Run("Set", func(t *testing.T) {
		a := panicApp(t)
		if r := recovered(func() { _ = a.Set("n", 2.0) }); r != "boom" {
			t.Fatalf("Set: recovered %v, want the fn's panic", r)
		}
		returnsWithin(t, "Get after the panic", func() { a.Get("n") })
		// The write itself stays applied (Derive's doc comment).
		if v, _ := a.Get("n"); v != 2.0 {
			t.Errorf("n = %v, want 2", v)
		}
		if err := a.Set("n", 3.0); err != nil {
			t.Fatal(err)
		}
		if v, _ := a.Get("d"); v != 30.0 {
			t.Errorf("d = %v after a later Set, want 30", v)
		}
	})
	t.Run("Batch", func(t *testing.T) {
		a := panicApp(t)
		r := recovered(func() {
			_ = a.Batch(func(b *Batch) error { return b.Set("n", 2.0) })
		})
		if r != "boom" {
			t.Fatalf("Batch: recovered %v, want the fn's panic", r)
		}
		returnsWithin(t, "Get after the panic", func() { a.Get("n") })
		// The batch is undone: the store is not left on its trial copy.
		if v, _ := a.Get("n"); v != 1.0 {
			t.Errorf("n = %v, want 1 (the batch undone)", v)
		}
		// The recomputation is not left held (deriveHold restored).
		if err := a.Set("n", 3.0); err != nil {
			t.Fatal(err)
		}
		if v, _ := a.Get("d"); v != 30.0 {
			t.Errorf("d = %v after a later Set, want 30", v)
		}
	})
	t.Run("Derive", func(t *testing.T) {
		a := doc(t, panicDoc)
		if err := a.Bind("", map[string]any{"n": 2.0}); err != nil {
			t.Fatal(err)
		}
		if r := recovered(func() { _ = a.Derive("d", panicAt) }); r != "boom" {
			t.Fatalf("Derive: recovered %v, want the fn's panic", r)
		}
		returnsWithin(t, "Get after the panic", func() { a.Get("n") })
		// The registration is dropped, as for an error.
		if err := a.Derive("d", func(func(string) (any, bool)) (any, error) { return "ok", nil }); err != nil {
			t.Fatalf("Derive again after the panic: %v", err)
		}
	})
}

// Under Run, a handler whose Set makes a Derive fn panic used to hang
// the loop in its deferred terminal restore (endRun) instead of
// propagating the panic.
func TestDerivePanicUnderRunPropagates(t *testing.T) {
	a := panicApp(t)
	a.On("go", func(Event) error { return a.Set("n", 2.0) })
	pr, pw := io.Pipe()
	t.Cleanup(func() { pw.Close() })
	got := make(chan any, 1)
	go func() {
		got <- recovered(func() {
			_ = a.loopWith(context.Background(), pr, &syncBuf{}, func() (int, int) { return 30, 5 }, nil, RunOptions{}, nil)
		})
	}()
	waitUntil(t, "the first frame", func() bool { return screenOf(a) != "" })
	if _, err := pw.Write([]byte("g")); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-got:
		if r != "boom" {
			t.Fatalf("the loop ended with %v, want the fn's panic", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the loop hangs after a panic in a Derive fn")
	}
	returnsWithin(t, "Get after the loop", func() { a.Get("n") })
}

// keyObjDoc is a list whose row key is an object.
const keyObjDoc = `<tui version="2">
<keymap><bind keys="space" action="check-toggle" to="#l"/></keymap>
<screen id="s" focus="#l"><list id="l" each="rows as r" key="r.k" bind="cur" checked="sel"><item><text>{r.k.id}:{r.name}</text></item></list></screen></tui>`

func keyObjData() map[string]any {
	return map[string]any{"rows": []any{
		map[string]any{"name": "a", "k": map[string]any{"id": 0.0}},
		map[string]any{"name": "b", "k": map[string]any{"id": 1.0}},
		map[string]any{"name": "c", "k": map[string]any{"id": 2.0}},
	}, "sel": []any{}}
}

// A list's key that is an object was written to bind and to the checked
// array by reference: the store then shared memory between the two paths,
// so a Set through one changed the other, unseen by the row cache.
func TestListObjectKeyIsCopied(t *testing.T) {
	t.Run("bind", func(t *testing.T) {
		a := doc(t, keyObjDoc)
		if err := a.Bind("", keyObjData()); err != nil {
			t.Fatal(err)
		}
		a.Frame(30, 6)
		a.HandleKey(Key{Name: "down"})
		if err := a.Set("cur.id", 99.0); err != nil {
			t.Fatal(err)
		}
		if v, _ := a.Get("rows.1.k.id"); v != 1.0 {
			t.Errorf("rows.1.k.id = %v after Set(cur.id), want 1", v)
		}
	})
	t.Run("checked", func(t *testing.T) {
		a := doc(t, keyObjDoc)
		if err := a.Bind("", keyObjData()); err != nil {
			t.Fatal(err)
		}
		a.Frame(30, 6)
		a.HandleKey(Key{Name: "space"})
		if err := a.Set("sel.0.id", 42.0); err != nil {
			t.Fatal(err)
		}
		if v, _ := a.Get("rows.0.k.id"); v != 0.0 {
			t.Errorf("rows.0.k.id = %v after Set(sel.0.id), want 0", v)
		}
	})
	t.Run("event", func(t *testing.T) {
		src := `<tui version="2"><screen id="s" focus="#l"><list id="l" each="rows as r" key="r.k" on:select="moved"><item><text>{r.k.id}</text></item></list></screen></tui>`
		a := doc(t, src)
		if err := a.Bind("", keyObjData()); err != nil {
			t.Fatal(err)
		}
		a.Frame(30, 6)
		evs := a.HandleKey(Key{Name: "down"})
		if len(evs) != 1 {
			t.Fatalf("events = %v, want one on:select", evs)
		}
		k, _ := evs[0].Keys["r"].(map[string]any)
		if k == nil {
			t.Fatalf("keys = %v, want an object key", evs[0].Keys)
		}
		k["id"] = 77.0 // a handler changing its event
		if v, _ := a.Get("rows.1.k.id"); v != 1.0 {
			t.Errorf("rows.1.k.id = %v after changing the event's key, want 1", v)
		}
	})
	t.Run("warm and cold", func(t *testing.T) {
		runEquivalence(t, keyObjDoc, keyObjData(), 30, 6, []eqStep{
			eqKeys("down"), eqSet("cur.id", 99.0), eqKeys("space"), eqSet("sel.0.id", 42.0),
		})
	})
}
