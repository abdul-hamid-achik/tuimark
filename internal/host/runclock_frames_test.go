package host

import (
	"reflect"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// A spinner redraw is the previous frame's grid with the glyph cells of
// its visible spinners changed (SPEC v0.4b §26.13: "the frame it produces
// equals the frame a fresh build of the same state would produce at the
// new phase"). Checked against a cold build at each phase, over spinners
// of every kind: a placed one, a styled one, a disabled one, and one in
// each row of a list.
func TestRunClockRedrawEqualsFreshBuild(t *testing.T) {
	src := `<tui><screen id="main">
<progress id="a" indeterminate="true" width="5" style="content-align: center; color: red; bold: true"/>
<progress id="b" indeterminate="true" width="5" style="content-align: end; background: blue"/>
<progress id="c" indeterminate="true" disabled="true"/>
<list id="l" each="rows as r" key="r"><item><progress indeterminate="true"/><text>{r}</text></item></list>
</screen></tui>`
	data := map[string]any{"rows": []any{"x", "y", "z"}}
	warm := doc(t, src)
	_ = warm.Bind("", data)
	f0 := warm.FrameAt(20, 8, 0)
	if len(f0.Grid.Spinners) < 6 {
		t.Fatalf("%d visible spinners, want at least 6", len(f0.Grid.Spinners))
	}
	snapshot := append([]paint.Cell(nil), f0.Grid.Cells...)
	for p := 0; p < 25; p++ {
		cold := doc(t, src)
		_ = cold.Bind("", data)
		want := cold.FrameAt(20, 8, p).Grid
		got := f0.Grid.SpinnerGlyphs(p, false)
		if !reflect.DeepEqual(got.Cells, want.Cells) || !reflect.DeepEqual(got.Spinners, want.Spinners) {
			t.Fatalf("phase %d: the redraw differs from a fresh build", p)
		}
		// The ASCII set changes nothing but the glyph of those cells.
		a := f0.Grid.SpinnerGlyphs(p, true)
		spin := map[int]bool{}
		for _, s := range a.Spinners {
			spin[s.Y*a.W+s.X] = true
		}
		for i, c := range a.Cells {
			w := want.Cells[i]
			if spin[i] {
				if c.Ch != paint.Glyph(p, true) {
					t.Fatalf("phase %d: ASCII spinner cell %d holds %q", p, i, c.Ch)
				}
				w.Ch = c.Ch
			}
			if !reflect.DeepEqual(c, w) {
				t.Fatalf("phase %d: cell %d changed beyond a spinner glyph", p, i)
			}
		}
	}
	if !reflect.DeepEqual(f0.Grid.Cells, snapshot) {
		t.Error("SpinnerGlyphs changed the frame it was given")
	}
}

// lastFrame is the live frame, by identity.
func (a *App) lastFrame() *Frame {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.last
}

// The phase is an input of the frame only for a document with a spinner:
// a document without one keeps the frame skip as the session's clock
// moves (no new frame is built for a tick that changes nothing), and one
// with a spinner builds at the new phase.
func TestRunClockFrameSkipKeptWithoutSpinner(t *testing.T) {
	for _, c := range []struct {
		name, src string
		rebuilt   bool
	}{
		{"version 1, no spinner", v1(`<text>x</text>`), false},
		{"current language, no spinner", `<tui><screen id="main"><text>x</text></screen></tui>`, false},
		{"spinner", spinDoc("", ""), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := startClockRig(t, c.src, rigOpts{setup: func(a *App) {
				a.Every(100*time.Millisecond, func(*Batch) error { return nil }) // a tick that writes nothing
			}})
			first := r.app.lastFrame()
			r.advance100(3)
			if rebuilt := r.app.lastFrame() != first; rebuilt != c.rebuilt {
				t.Errorf("frame rebuilt = %v, want %v", rebuilt, c.rebuilt)
			}
			if err := r.quit(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// The hook: Run, RunHost, and Loop take their clock from testRunClock
// when a test sets it, from the real clock otherwise (the clock a
// session's own termSession.clk overrides).
func TestRunClockHook(t *testing.T) {
	if _, ok := defaultRunClock().(realClock); !ok {
		t.Fatalf("the default clock is %T, want the real one", defaultRunClock())
	}
	fake := newFakeClock()
	testRunClock = fake
	defer func() { testRunClock = nil }()
	if defaultRunClock() != runClock(fake) || (&termSession{}).clock() != runClock(fake) {
		t.Error("the hook is not the session's clock")
	}
	own := newFakeClock()
	if (&termSession{clk: own}).clock() != runClock(own) {
		t.Error("a session's own clock does not win over the hook")
	}
}

// A spinner redraw builds no frame: the live frame is the same object
// after ten redraws.
func TestRunClockRedrawBuildsNoFrame(t *testing.T) {
	r := startClockRig(t, spinDoc("", ""), rigOpts{})
	first := r.app.lastFrame()
	if runs, _ := r.advance100(10); len(runs[9]) != 1 {
		t.Fatalf("the spinner did not turn: %v", runs[9])
	}
	if r.app.lastFrame() != first {
		t.Error("a spinner redraw built a frame")
	}
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
}
