package host

import (
	"io"
	"path/filepath"
	"testing"
	"time"
)

// roundsLoopDoc keeps queuing lifecycle events: a focused button is
// display: none, so each of a frame's three focus passes moves focus and
// queues an on:focus event, and every settle reaches its 8-round cap.
const roundsLoopDoc = `<tui version="1">
<style>button:focus { display: none; }</style>
<keymap><bind keys="k" action="kick"/></keymap>
<screen id="s">
  <button id="a" on:focus="fa">A</button>
  <button id="b" on:focus="fb">B</button>
</screen>
</tui>`

// 115 (under Run). Each settle that ends at the 8th round with events
// still queued writes a TUIMARK_LOG rounds record whose queued is the
// number of events kept (SPEC v0.4a §15.4 point 3, §26.12); the next
// settle dispatches those events first, ahead of the input's own event,
// and none is lost: every on:focus the renders queued is either in an
// action record or still kept when the session ends.
func TestRunLogsRoundsCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.log")
	lg, err := openRunLog(path, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a := doc(t, roundsLoopDoc)
	kicked := make(chan struct{}, 1)
	a.On("kick", func(Event) error { kicked <- struct{}{}; return nil })
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- a.session(pr, &syncBuf{}, func() (int, int) { return 20, 6 }, nil, nil, &termSession{log: lg}, HostHooks{})
	}()
	if _, err := io.WriteString(pw, "k"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-kicked:
	case <-time.After(5 * time.Second):
		t.Fatal("kick never dispatched")
	}
	pw.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the session did not end at EOF")
	}
	lg.end("eof")

	recs := readLog(t, path)
	// The first frame's settle: rounds 2 to 8 dispatch one render's three
	// events each, and the cap keeps the batch taken for a 9th round plus
	// what the render after that take queued.
	const carry = 6
	var seq []string // "rounds:N" and action names, in log order
	for _, rec := range recs {
		switch rec["ev"] {
		case "rounds":
			if rec["queued"] != float64(carry) {
				t.Errorf("rounds record %v, want queued %d", rec, carry)
			}
			seq = append(seq, "rounds")
		case "action":
			seq = append(seq, rec["action"].(string))
		}
	}
	first := -1
	for i, s := range seq {
		if s == "rounds" {
			first = i
			break
		}
	}
	if first != 21 {
		t.Fatalf("first rounds record after %d actions, want 21: %v", first, seq)
	}
	// The key's settle dispatches the kept events first, then kick.
	rest := seq[first+1:]
	if len(rest) <= carry || rest[carry] != "kick" {
		t.Fatalf("after the cap: %v, want %d kept on:focus events, then kick", rest, carry)
	}
	for _, s := range rest[:carry] {
		if s != "fa" && s != "fb" {
			t.Errorf("kept event %q, want an on:focus action", s)
		}
	}
	if got := len(a.carry); got != carry {
		t.Errorf("events kept when the session ended = %d, want %d", got, carry)
	}
}
