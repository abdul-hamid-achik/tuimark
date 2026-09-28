package tuimark_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.3b §21 test 101: Batch errors. Batch always returns an
// errors.Join value on failure, in the draft's exact order rule (every
// failed b.Set's own error, in call order, then fn's own error unless it
// is == to one of those), and an apply-time failure is reported as the
// one joined part.

func newBatchErrorsApp(t *testing.T) *tuimark.App {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(`<tui version="1"><screen><text>hi</text></screen></tui>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", map[string]any{"n": 1.0, "m": 1.0}); err != nil {
		t.Fatal(err)
	}
	return app
}

// unwrapParts returns err's joined parts via Unwrap() []error, the shape
// errors.Join's return value has; it fails the test if err is not one.
func unwrapParts(t *testing.T, err error) []error {
	t.Helper()
	u, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("err = %v (%T) does not implement Unwrap() []error (want an errors.Join value)", err, err)
	}
	return u.Unwrap()
}

// Two b.Set calls fail when called (a bad @theme value, then a
// malformed path); fn returns nil. The parts are [e1, e2], errors.Is
// reaches both, and the store is unchanged.
func TestBatchErrorsTwoFailedSets(t *testing.T) {
	app := newBatchErrorsApp(t)
	var e1, e2 error
	err := app.Batch(func(b *tuimark.Batch) error {
		e1 = b.Set("@theme", "purple")
		e2 = b.Set("a..b", 1.0)
		return nil
	})
	if e1 == nil || e2 == nil {
		t.Fatal("want both b.Set calls to fail when called")
	}
	if err == nil {
		t.Fatal("want an error")
	}
	if parts := unwrapParts(t, err); len(parts) != 2 || parts[0] != e1 || parts[1] != e2 {
		t.Errorf("parts = %v, want [e1, e2] in call order", parts)
	}
	if !errors.Is(err, e1) || !errors.Is(err, e2) {
		t.Errorf("errors.Is does not reach both parts of %v", err)
	}
	root, _ := app.Get("")
	m := root.(map[string]any)
	if m["n"] != 1.0 || m["m"] != 1.0 {
		t.Errorf("store changed: %v, want unchanged", m)
	}
}

// One b.Set fails, then fn returns a distinct sentinel error: the parts
// are [e1, errAbort].
func TestBatchErrorsFailedSetThenDistinctFnError(t *testing.T) {
	app := newBatchErrorsApp(t)
	errAbort := errors.New("abort")
	var e1 error
	err := app.Batch(func(b *tuimark.Batch) error {
		e1 = b.Set("a..b", 1.0)
		return errAbort
	})
	if e1 == nil {
		t.Fatal("want b.Set to fail when called")
	}
	parts := unwrapParts(t, err)
	if len(parts) != 2 || parts[0] != e1 || parts[1] != errAbort {
		t.Errorf("parts = %v, want [e1, errAbort]", parts)
	}
}

// fn returns the same error a failed b.Set already returned: it reports
// once, not twice, and the joined value is still distinct from it.
func TestBatchErrorsFnReturnsTheFailedSetsOwnError(t *testing.T) {
	app := newBatchErrorsApp(t)
	var e1 error
	err := app.Batch(func(b *tuimark.Batch) error {
		e1 = b.Set("a..b", 1.0)
		return e1
	})
	if e1 == nil {
		t.Fatal("want b.Set to fail when called")
	}
	parts := unwrapParts(t, err)
	if len(parts) != 1 || parts[0] != e1 {
		t.Errorf("parts = %v, want exactly [e1]", parts)
	}
	if got := strings.Count(err.Error(), e1.Error()); got != 1 {
		t.Errorf("err.Error() = %q holds e1's message %d times, want once", err.Error(), got)
	}
	if err == e1 {
		t.Errorf("err == e1, want a joined value distinct from it (err == sentinel no longer matches)")
	}
}

// fn fails with no failed b.Set at all: the parts are [errAbort], and
// errors.Is still reaches it although err != errAbort.
func TestBatchErrorsFnErrorAlone(t *testing.T) {
	app := newBatchErrorsApp(t)
	errAbort := errors.New("abort")
	err := app.Batch(func(b *tuimark.Batch) error { return errAbort })
	parts := unwrapParts(t, err)
	if len(parts) != 1 || parts[0] != errAbort {
		t.Errorf("parts = %v, want exactly [errAbort]", parts)
	}
	if !errors.Is(err, errAbort) {
		t.Errorf("errors.Is(err, errAbort) = false")
	}
	if err == errAbort {
		t.Errorf("err == errAbort, want a joined value distinct from it")
	}
}

// fn wraps the failed b.Set's error with %w: since the wrapping error is
// not == to e1, both are separate parts.
func TestBatchErrorsFnWrapsTheFailedSetsError(t *testing.T) {
	app := newBatchErrorsApp(t)
	var e1 error
	err := app.Batch(func(b *tuimark.Batch) error {
		e1 = b.Set("a..b", 1.0)
		return fmt.Errorf("ctx: %w", e1)
	})
	if e1 == nil {
		t.Fatal("want b.Set to fail when called")
	}
	parts := unwrapParts(t, err)
	if len(parts) != 2 || parts[0] != e1 {
		t.Errorf("parts = %v, want [e1, the wrapping error]", parts)
	}
	if !errors.Is(err, e1) {
		t.Errorf("errors.Is(err, e1) = false for the wrapped part")
	}
}

// Both b.Set calls are accepted when queued (the store's "n" and "m" are
// both numbers, so neither path is malformed), but applying them fails
// at the first, "n.a" (n is not an object): there is exactly one part,
// naming n.a and not m.a, and the store stays unchanged.
func TestBatchErrorsApplyTimeFailureIsOnePart(t *testing.T) {
	app := newBatchErrorsApp(t)
	err := app.Batch(func(b *tuimark.Batch) error {
		if err := b.Set("n.a", 1.0); err != nil {
			t.Fatalf("b.Set(n.a, ...) should be accepted when called: %v", err)
		}
		if err := b.Set("m.a", 1.0); err != nil {
			t.Fatalf("b.Set(m.a, ...) should be accepted when called: %v", err)
		}
		return nil
	})
	if err == nil {
		t.Fatal("want an error: n is a number, not an object")
	}
	parts := unwrapParts(t, err)
	if len(parts) != 1 {
		t.Fatalf("parts = %v, want exactly one part (application stops at the first failing write)", parts)
	}
	if !strings.Contains(parts[0].Error(), "n.a") {
		t.Errorf("part = %q, want it to name n.a", parts[0].Error())
	}
	if strings.Contains(parts[0].Error(), "m.a") {
		t.Errorf("part = %q, want it not to name m.a (never tried)", parts[0].Error())
	}
	root, _ := app.Get("")
	m := root.(map[string]any)
	if m["n"] != 1.0 || m["m"] != 1.0 {
		t.Errorf("store changed: %v, want unchanged", m)
	}
}
