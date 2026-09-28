package host

import (
	"testing"
	"time"
)

// SPEC v0.3 §21 tests 79 and 83 (the halves that need test 71's harness,
// startLoop, an internal test file of this package). The Play-driven
// halves live in the root package (batch_test.go), against the public
// API; internal/play's Play calls exactly EnterPlay/ExitPlay, so testing
// them here directly is equivalent without an import cycle (internal/play
// imports host, so an internal host test file cannot import it back).

// A handler that calls Batch, Set, and Get during Run's loop (driven over
// pipes in process, as in test 71) returns without deadlock, and its
// writes are in the next frame.
func TestBatchSetGetDuringRunLoop(t *testing.T) {
	rig := startLoop(t, func(a *App) {
		a.On("go", func(Event) error {
			if err := a.Batch(func(b *Batch) error { return b.Set("batched", true) }); err != nil {
				return err
			}
			if err := a.Set("marker", "done"); err != nil {
				return err
			}
			if v, ok := a.Get("marker"); !ok || v != "done" {
				t.Errorf("Get from inside a handler during Run's loop = (%v, %v), want (done, true)", v, ok)
			}
			return nil
		})
	})
	rig.next(t) // hello (initial on:focus)
	rig.send(t, "\r")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if v, _ := rig.app.Get("batched"); v == true {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if v, _ := rig.app.Get("batched"); v != true {
		t.Fatalf("batched = %v, want true (the handler's Batch write reached the store)", v)
	}
	rig.send(t, "\t")
	rig.send(t, "q")
	if err := rig.wait(t); err != nil {
		t.Fatalf("quit: %v", err)
	}
}

// SPEC v0.3 §21 test 83: Play while Run's loop is active (test 71's
// harness) returns an error and does nothing. internal/play.Play's first
// call is app.EnterPlay(); this tests that primitive directly, which the
// public tuimark.Play (tested against the public API in the root
// package) relies on for exactly this exclusion.
func TestEnterPlayWhileRunLoopIsActive(t *testing.T) {
	rig := startLoop(t, nil)
	rig.next(t) // hello

	if err := rig.app.EnterPlay(); err == nil {
		t.Fatal("EnterPlay succeeded while Run's loop (Loop) is active, want an error")
	}

	rig.send(t, "\t")
	rig.send(t, "q")
	if err := rig.wait(t); err != nil {
		t.Fatalf("quit: %v", err)
	}

	// Once the loop has returned, Play may run again.
	if err := rig.app.EnterPlay(); err != nil {
		t.Fatalf("EnterPlay after the loop ended: %v", err)
	}
	rig.app.ExitPlay()
}
