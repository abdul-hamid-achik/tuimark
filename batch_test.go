package tuimark

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// SPEC v0.3 §21 tests 78 and 79. package tuimark (internal test) so
// TestBatchWakeCount can reach app.h.WakeCount(), the observation hook
// internal/host adds for exactly this purpose; it is not part of the
// public API.

func newBatchTestApp(t *testing.T) *App {
	t.Helper()
	app, err := Parse(strings.NewReader(`<tui version="1"><screen><text>hi</text></screen></tui>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", map[string]any{"a": 0.0, "b": 0.0}); err != nil {
		t.Fatal(err)
	}
	return app
}

// A goroutine that calls Get and Dump in a loop while the main goroutine
// applies batches of two writes that keep an invariant (both paths
// always equal) never observes them unequal (-race).
func TestBatchAtomicityUnderRace(t *testing.T) {
	app := newBatchTestApp(t)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Both paths are read from one Get("") snapshot: two separate
			// Get calls would each be consistent on their own but could
			// straddle many intervening batches between them, which is
			// not what this test means to observe.
			root, _ := app.Get("")
			m := root.(map[string]any)
			if m["a"] != m["b"] {
				t.Errorf("observed a=%v b=%v unequal mid-batch", m["a"], m["b"])
			}
			if _, err := app.Dump(20, 5); err != nil {
				t.Error(err)
			}
		}
	}()
	for i := 0; i < 500; i++ {
		n := float64(i)
		err := app.Batch(func(b *Batch) error {
			if err := b.Set("a", n); err != nil {
				return err
			}
			return b.Set("b", n)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
}

// A batch whose fn returns an error, and a batch where one b.Set fails
// (a bad @theme value) while fn returns nil, change nothing and return
// the error.
func TestBatchNothingAppliedOnError(t *testing.T) {
	app := newBatchTestApp(t)
	before, _ := app.Get("a")

	fnErr := errors.New("boom")
	err := app.Batch(func(b *Batch) error {
		if err := b.Set("a", 99.0); err != nil {
			t.Fatal(err)
		}
		return fnErr
	})
	if !errors.Is(err, fnErr) {
		t.Errorf("err = %v, want fnErr", err)
	}
	if v, _ := app.Get("a"); v != before {
		t.Errorf("a = %v, want unchanged %v", v, before)
	}

	err = app.Batch(func(b *Batch) error {
		if err := b.Set("a", 42.0); err != nil {
			t.Fatal(err)
		}
		if err := b.Set("@theme", "purple"); err == nil {
			t.Fatal("want an error for a bad @theme value")
		}
		return nil // fn ignores the b.Set error
	})
	if err == nil {
		t.Fatal("want an error")
	}
	if v, _ := app.Get("a"); v != before {
		t.Errorf("a = %v after a bad @theme write, want unchanged %v", v, before)
	}
}

// A batch whose second write goes through a value the first write made
// a string changes nothing.
func TestBatchSecondWriteThroughAString(t *testing.T) {
	app := newBatchTestApp(t)
	before, _ := app.Get("a")
	err := app.Batch(func(b *Batch) error {
		if err := b.Set("a", "now a string"); err != nil {
			return err
		}
		return b.Set("a.nested", 1.0) // a is a string as this batch left it
	})
	if err == nil {
		t.Fatal("want an error")
	}
	if v, _ := app.Get("a"); v != before {
		t.Errorf("a = %v, want unchanged %v (nothing applied)", v, before)
	}
}

// A batch of ten writes calls the app's redraw request (Wake) once; ten
// Sets call it ten times.
func TestBatchWakeCount(t *testing.T) {
	app := newBatchTestApp(t)
	before := app.h.WakeCount()
	err := app.Batch(func(b *Batch) error {
		for i := 0; i < 10; i++ {
			if err := b.Set("k"+strconv.Itoa(i), float64(i)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := app.h.WakeCount() - before; got != 1 {
		t.Errorf("Wake called %d times for one batch of ten writes, want 1", got)
	}

	before = app.h.WakeCount()
	for i := 0; i < 10; i++ {
		if err := app.Set("k"+strconv.Itoa(i), float64(i)); err != nil {
			t.Fatal(err)
		}
	}
	if got := app.h.WakeCount() - before; got != 10 {
		t.Errorf("Wake called %d times for ten Sets, want 10", got)
	}
}

// A b.Set on a *Batch kept after fn returned returns an error; a Batch
// called inside fn applies before the outer one.
func TestBatchSetAfterFnReturnedAndNestedBatch(t *testing.T) {
	app := newBatchTestApp(t)
	var kept *Batch
	if err := app.Batch(func(b *Batch) error {
		kept = b
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := kept.Set("a", 1.0); err == nil {
		t.Fatal("want an error for Set on a Batch kept after fn returned")
	}
	if v, _ := app.Get("a"); v != 0.0 {
		t.Errorf("a = %v, a Set kept after fn returned must change nothing", v)
	}

	var order []string
	err := app.Batch(func(b *Batch) error {
		order = append(order, "outer-queued")
		if err := b.Set("a", 5.0); err != nil {
			return err
		}
		// A Batch called from fn (which runs without the app's lock)
		// applies on its own, before this outer one.
		if err := app.Batch(func(inner *Batch) error {
			order = append(order, "inner-applied")
			return inner.Set("b", 9.0)
		}); err != nil {
			return err
		}
		if v, _ := app.Get("b"); v != 9.0 {
			t.Errorf("inner batch had not applied by the time the outer fn continued: b = %v", v)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "outer-queued" || order[1] != "inner-applied" {
		t.Fatalf("order = %v", order)
	}
	if v, _ := app.Get("b"); v != 9.0 {
		t.Errorf("b = %v, want 9 (the inner batch applied)", v)
	}
	if v, _ := app.Get("a"); v != 5.0 {
		t.Errorf("a = %v, want 5 (the outer batch applied)", v)
	}
}

// SPEC v0.3 §21 test 79 (the Play half; the Run's-loop half lives in
// internal/host, which has the test-71 harness this needs).
//
// A handler that calls Batch, Set, and Get during Play returns without
// deadlock, and its writes are in the next frame.
func TestBatchSetGetFromAHandlerDuringPlay(t *testing.T) {
	doc := `<tui version="1">
  <keymap><bind keys="g" action="go"/></keymap>
  <screen><text>{shown}</text></screen>
</tui>`
	app, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", map[string]any{"shown": "before"}); err != nil {
		t.Fatal(err)
	}
	app.On("go", func(Event) error {
		if err := app.Batch(func(b *Batch) error { return b.Set("shown", "batched") }); err != nil {
			return err
		}
		if err := app.Set("marker", true); err != nil {
			return err
		}
		if v, ok := app.Get("shown"); !ok || v != "batched" {
			t.Errorf("Get(\"shown\") from inside the handler = (%v, %v), want (batched, true)", v, ok)
		}
		return nil
	})
	res, err := app.Play(PlayOptions{}, "g")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Dump.Nodes) == 0 {
		t.Fatal("no nodes in the dump")
	}
	found := false
	for _, n := range res.Dump.Nodes {
		if n.Text == "batched" {
			found = true
		}
	}
	if !found {
		t.Errorf("the handler's Batch write is not in the next frame: nodes=%+v", res.Dump.Nodes)
	}
	if v, _ := app.Get("marker"); v != true {
		t.Errorf("marker = %v, want true", v)
	}
}
