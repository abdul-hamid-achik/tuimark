package kit

import (
	"github.com/abdul-hamid-achik/tuimark"
	"sync"
	"testing"
	"time"
)

func newToasts(t *testing.T, name string, o ToastOptions) (*Toasts, *clock) {
	t.Helper()
	c := newClock()
	o.Now = c.Now
	app := loadComponent(t, name, "")
	ts, err := NewToasts(app, o)
	if err != nil {
		t.Fatal(err)
	}
	return ts, c
}

func TestToastShowsInTheComponent(t *testing.T) {
	ts, _ := newToasts(t, "toast", ToastOptions{})
	notContains(t, screen(t, ts.app), "Settings saved")
	if err := ts.Push("ok", "Settings saved", 0); err != nil {
		t.Fatal(err)
	}
	text := screen(t, ts.app)
	contains(t, text, "Settings saved")
	if !getBool(t, ts.app, "toast.is_ok") || getBool(t, ts.app, "toast.is_danger") {
		t.Errorf("level flags: ok=%v danger=%v", getBool(t, ts.app, "toast.is_ok"), getBool(t, ts.app, "toast.is_danger"))
	}
}

func TestToastsShowOneAtATimeAndExpire(t *testing.T) {
	ts, c := newToasts(t, "toast", ToastOptions{})
	_ = ts.Push("info", "first", 2*time.Second)
	_ = ts.Push("danger", "second", 2*time.Second)
	text := screen(t, ts.app)
	contains(t, text, "first")
	notContains(t, text, "second")

	tick := func() {
		t.Helper()
		if err := ts.app.Batch(ts.Tick); err != nil {
			t.Fatal(err)
		}
	}
	c.advance(time.Second)
	tick()
	contains(t, screen(t, ts.app), "first") // not yet

	c.advance(time.Second)
	tick() // first is up; second takes its place with a fresh clock
	text = screen(t, ts.app)
	contains(t, text, "second")
	notContains(t, text, "first")
	if !getBool(t, ts.app, "toast.is_danger") {
		t.Error("the second toast is a danger toast")
	}

	c.advance(1500 * time.Millisecond)
	tick()
	contains(t, screen(t, ts.app), "second")
	c.advance(time.Second)
	tick()
	notContains(t, screen(t, ts.app), "second")
	if getBool(t, ts.app, "toast.show") || ts.Len() != 0 {
		t.Errorf("show=%v len=%d, want hidden and empty", getBool(t, ts.app, "toast.show"), ts.Len())
	}
}

func TestToastsCapDropsTheOldestWaiting(t *testing.T) {
	ts, c := newToasts(t, "toast", ToastOptions{Max: 3, TTL: time.Second})
	for _, s := range []string{"a", "b", "c", "d"} {
		if err := ts.Push("info", s, 0); err != nil {
			t.Fatal(err)
		}
	}
	if ts.Len() != 3 {
		t.Fatalf("len = %d, want 3", ts.Len())
	}
	var shown []string
	for range 4 {
		shown = append(shown, getStr(t, ts.app, "toast.text"))
		c.advance(time.Second)
		if err := ts.app.Batch(ts.Tick); err != nil {
			t.Fatal(err)
		}
	}
	// a stays on screen; b, the oldest waiting, was dropped for d.
	if want := []string{"a", "c", "d", ""}; !equal(shown, want) {
		t.Errorf("shown %v, want %v", shown, want)
	}
}

func TestToastsMaxOneReplaces(t *testing.T) {
	ts, _ := newToasts(t, "toast", ToastOptions{Max: 1})
	_ = ts.Push("info", "old", 0)
	_ = ts.Push("warn", "new", 0)
	if got := getStr(t, ts.app, "toast.text"); got != "new" || !getBool(t, ts.app, "toast.is_warn") {
		t.Errorf("text %q warn %v", got, getBool(t, ts.app, "toast.is_warn"))
	}
}

func TestEscDismissesAndShowsTheNext(t *testing.T) {
	ts, _ := newToasts(t, "toast", ToastOptions{})
	ts.Register()
	_ = ts.Push("info", "one", 0)
	_ = ts.Push("ok", "two", 0)
	res, text := play(t, ts.app, "esc") // toast.tui binds esc to toast_dismiss
	var dismissed bool
	for _, e := range res.Events {
		dismissed = dismissed || e.Action == "toast_dismiss"
	}
	if !dismissed {
		t.Fatalf("no toast_dismiss event: %+v", res.Events)
	}
	contains(t, text, "two")
	notContains(t, text, "one")
	_, text = play(t, ts.app, "esc")
	notContains(t, text, "two")
}

func TestPushRejectsAnUnknownKindAndUsesABatch(t *testing.T) {
	ts, _ := newToasts(t, "toast", ToastOptions{})
	if err := ts.Push("error", "x", 0); err == nil {
		t.Error("kind error is not info, ok, warn or danger")
	}
	// PushTo joins the batch a handler is already writing.
	err := ts.app.Batch(func(b *tuimark.Batch) error {
		if err := b.Set("other", 1.0); err != nil {
			return err
		}
		return ts.PushTo(b, "warn", "together", 0)
	})
	if err != nil {
		t.Fatal(err)
	}
	contains(t, screen(t, ts.app), "together")
}

func TestToastsAreSafeFromManyGoroutines(t *testing.T) {
	ts, c := newToasts(t, "toast", ToastOptions{Max: 4})
	var wg sync.WaitGroup
	for g := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				_ = ts.Push("info", "from goroutine", time.Second)
				_ = ts.app.Batch(ts.Tick)
				_ = g
			}
		}()
	}
	wg.Wait()
	c.advance(time.Hour)
	for range 5 {
		_ = ts.app.Batch(ts.Tick)
	}
	if ts.Len() > 4 {
		t.Errorf("len = %d, want at most 4", ts.Len())
	}
}

func TestToastsStartAndStop(t *testing.T) {
	ts, _ := newToasts(t, "toast", ToastOptions{})
	stop := ts.Start(time.Hour) // registers the Every; Run is what would call it
	stop()
	stop() // idempotent
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
