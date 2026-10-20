package host

import (
	"strings"
	"testing"
	"time"
)

const spinnerDoc = `<tui><screen id="main"><row height="1" gap="1"><progress id="sp" indeterminate="true"/><text>loading</text></row></screen></tui>`

func glyphOf(f *Frame) string { return strings.Fields(strings.Join(f.Grid.Lines(), "\n"))[0] }

// SPEC v0.4b §12.6: the phase is floor(t / 100 ms).
func TestPhaseOf(t *testing.T) {
	for d, want := range map[time.Duration]int{
		-time.Second: 0, 0: 0, 99 * time.Millisecond: 0, 100 * time.Millisecond: 1,
		time.Second: 10, 1250 * time.Millisecond: 12,
	} {
		if got := PhaseOf(d); got != want {
			t.Errorf("PhaseOf(%v) = %d, want %d", d, got, want)
		}
	}
}

// SPEC v0.4b §26.13 ("the phase is an input of the frame"): a frame built
// again with nothing changed but the phase is not the cached one. The
// cache is on (the frame skip), and a cold app agrees at every phase.
func TestFramePhaseIsAnInputOfTheFrame(t *testing.T) {
	warm, cold := doc(t, spinnerDoc), doc(t, spinnerDoc)
	cold.noCache = true
	for _, tc := range []struct {
		phase int
		want  string
	}{{0, "⠋"}, {0, "⠋"}, {1, "⠙"}, {1, "⠙"}, {0, "⠋"}, {9, "⠏"}, {10, "⠋"}, {12, "⠹"}} {
		w, c := warm.FrameAt(20, 2, tc.phase), cold.FrameAt(20, 2, tc.phase)
		if glyphOf(w) != tc.want || glyphOf(c) != tc.want {
			t.Errorf("phase %d: warm %q, cold %q, want %q", tc.phase, glyphOf(w), glyphOf(c), tc.want)
		}
		if strings.Join(w.Grid.Lines(), "\n") != strings.Join(c.Grid.Lines(), "\n") {
			t.Errorf("phase %d: warm and cold grids differ", tc.phase)
		}
	}
	if glyphOf(warm.Frame(20, 2)) != "⠋" {
		t.Error("Frame is FrameAt at phase 0")
	}
}

// Dump paints phase 0 whatever the last live frame was, and leaves the
// live phase to the next FrameAt (SPEC v0.4b §12.6: MUST 13).
func TestDumpPaintsPhase0(t *testing.T) {
	a := doc(t, spinnerDoc)
	live := a.FrameAt(20, 2, 3)
	if glyphOf(live) != "⠸" {
		t.Fatalf("live glyph %q", glyphOf(live))
	}
	d := a.Dump(20, 2, false)
	if !strings.HasPrefix(d.Grid[0], "⠋") {
		t.Errorf("Dump = %q, want phase 0", d.Grid[0])
	}
	if got := glyphOf(a.FrameAt(20, 2, 3)); got != "⠸" {
		t.Errorf("FrameAt after Dump = %q, want phase 3 again", got)
	}
}

// The side table of a live frame names the visible spinner's cell.
func TestFrameSpinnerTable(t *testing.T) {
	a := doc(t, spinnerDoc)
	f := a.FrameAt(20, 2, 4)
	if len(f.Grid.Spinners) != 1 || f.Grid.Spinners[0].X != 0 || f.Grid.Spinners[0].Y != 0 {
		t.Errorf("Spinners = %v", f.Grid.Spinners)
	}
	b := doc(t, `<tui><screen id="main"><progress indeterminate="false"/></screen></tui>`)
	if f := b.FrameAt(20, 2, 4); len(f.Grid.Spinners) != 0 {
		t.Errorf("a bar is no spinner: %v", f.Grid.Spinners)
	}
}

// The virtual clock is app state, guarded: Every reads it from any
// goroutine while a tick runs.
func TestVirtualClockAndDeadlines(t *testing.T) {
	a := doc(t, spinnerDoc)
	a.Every(100*time.Millisecond, func(*Batch) error { return nil })
	if a.VClock() != 0 || a.VPhase() != 0 {
		t.Fatal("a new app's clock is not 0")
	}
	settles := 0
	fnErr, applyErr := a.VirtualTick(350*time.Millisecond, false, func() bool { settles++; return false })
	if fnErr != nil || applyErr != nil || a.VClock() != 350*time.Millisecond || a.VPhase() != 3 {
		t.Fatalf("fnErr %v applyErr %v V %v", fnErr, applyErr, a.VClock())
	}
	if settles != 4 { // 100, 200, 300, and the arrival at 350
		t.Errorf("%d settles, want one per instant (3) and one on arrival", settles)
	}
	settles = 0
	_, _ = a.VirtualTick(40*time.Millisecond, true, func() bool { settles++; return false })
	if a.VClock() != 390*time.Millisecond || settles != 1 {
		t.Errorf("NoHandlers: V %v, %d settles", a.VClock(), settles)
	}
}
