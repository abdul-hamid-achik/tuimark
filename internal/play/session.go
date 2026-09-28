package play

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

// Session replays steps against an App through the same primitives Run's
// loop uses (Frame, HandleKeyRun, HandlePaste, Dispatch, TakePending,
// Focus), without a TTY (SPEC §15.4; for a Go host, SPEC v0.3 §18.1).
type Session struct {
	App        *host.App
	Cols, Rows int
	Frame      *host.Frame
	// Events is cumulative, in order, for the top-level "events".
	Events        []dump.Event
	curStepEvents []dump.Event // this step's own events, for --frames
	Quit          bool
	// NoHandlers records host actions without calling their registered
	// handlers (SPEC v0.3 §18.1): tuimark play and tuimark test always set
	// it (a CLI-loaded app never has handlers registered, so a plain
	// Dispatch already behaves this way there); tuimark.Play exposes it as
	// PlayOptions.NoHandlers.
	NoHandlers bool
	CellsFlag  bool
	StylesFlag bool
	// StepFrames and CollectFrames serve both tuimark play --frames and
	// tuimark.Play's PlayOptions.Frames (SPEC v0.3b §18.1 rule 9: --frames
	// and Frames give the same frames, member for member).
	StepFrames    []dump.PlayFrame
	CollectFrames bool

	// UsageErr, UsageStep, and UsageRaw report a step that could not be
	// applied when its turn came (a Set/@focus error, a mouse cell
	// outside the grid): the session stops right there, like a quit, but
	// the caller reports it as a usage error (tuimark play) or wraps it
	// as "step N (RAW): reason" (tuimark.Play), instead of building a
	// dump.
	UsageErr  error
	UsageStep int
	UsageRaw  string
	// Err is a handler's non-ErrQuit error (SPEC v0.3 §18.1 Play rule 5):
	// it ends the call the same way Quit does, and the caller
	// (tuimark.Play) returns it together with the result so far. tuimark
	// play and tuimark test never see it: NoHandlers is always set there.
	Err error
}

// NewSession builds a Session over app at cols×rows. cells/styles/frames
// mirror the --cells/--styles/--frames flags, and frames also mirrors
// PlayOptions.Frames (v0.3b); noHandlers is PlayOptions.NoHandlers,
// always true for tuimark play and tuimark test, which record host
// actions (SPEC §15.4).
func NewSession(app *host.App, cols, rows int, cells, styles, frames, noHandlers bool) *Session {
	return &Session{
		App: app, Cols: cols, Rows: rows,
		Events: []dump.Event{}, CellsFlag: cells, StylesFlag: styles, CollectFrames: frames, NoHandlers: noHandlers,
	}
}

func (s *Session) draw() { s.Frame = s.App.Frame(s.Cols, s.Rows) }

// stopped reports whether the session has ended early: a quit, or a
// handler error (SPEC v0.3 §18.1 Play rules 5 and 9).
func (s *Session) stopped() bool { return s.Quit || s.Err != nil }

// BuildDump builds the dump of the session's current frame with its
// --cells/--styles flags (SPEC §13.2, §15.3).
func (s *Session) BuildDump() *dump.Dump {
	d := s.Frame.Dump(s.CellsFlag)
	if s.StylesFlag {
		d.Theme = s.Frame.Theme
		d.Styles = dump.BuildStyles(s.Frame.Grid)
	}
	return d
}

// snapshot records this step's PlayFrame when CollectFrames is set.
func (s *Session) snapshot(step int, input string) {
	if !s.CollectFrames {
		return
	}
	evs := s.curStepEvents
	if evs == nil {
		evs = []dump.Event{}
	}
	s.StepFrames = append(s.StepFrames, dump.PlayFrame{Step: step, Input: input, Events: evs, Dump: s.BuildDump()})
}

// dispatchAll dispatches evs, then TakePending in rounds, redrawing
// between rounds, for at most 8 rounds (the v0.1 loop; SPEC §15.4 point
// 3). Unless NoHandlers is set, the registered handler for each event's
// action is called, exactly as Run's loop calls it (SPEC v0.3 §18.1 rule
// 5); ErrQuit or the built-in quit with no handler ends the call with
// Quit: true, any other error ends it with Err set, both stopping the
// round and the steps that follow.
func (s *Session) dispatchAll(step int, evs []host.Event) {
	for round := 0; round < 8; round++ {
		for _, ev := range evs {
			de := dump.Event{Step: step, Action: ev.Action, Source: ev.Source, Keys: ev.Keys, Value: ev.Value}
			if de.Keys == nil {
				de.Keys = map[string]any{}
			}
			s.Events = append(s.Events, de)
			s.curStepEvents = append(s.curStepEvents, de)
			var quit bool
			var err error
			if s.NoHandlers {
				quit = ev.Action == "quit"
			} else {
				quit, err = s.App.Dispatch(ev)
			}
			if err != nil {
				s.Err = err
				return
			}
			if quit {
				s.Quit = true
				return
			}
		}
		evs = s.App.TakePending()
		if len(evs) == 0 {
			return
		}
		s.draw()
	}
}

// settle draws and runs the events a render queued, exactly as Run's loop
// settles after applying one step (SPEC §15.4 point 3).
func (s *Session) settle(step int) {
	s.draw()
	s.dispatchAll(step, nil)
}

// handleKeys applies keys through HandleKeyRun's coalescing, dispatching
// each run's events and redrawing when they fired, focus moved, or the
// key changed state the frame shows (TakeDirty: in a version="2"
// document, an input's text or cursor, a list or table cursor, an
// offset, a checked array, a tab), so a later key in the same step sees
// the fresh frame (SPEC v0.2b §8.6; mirrors internal/host/run.go's
// handleKeys).
func (s *Session) handleKeys(step int, keys []host.Key) {
	for len(keys) > 0 && !s.stopped() {
		focus := s.App.Focus()
		evs, n := s.App.HandleKeyRun(keys)
		keys = keys[n:]
		s.dispatchAll(step, evs)
		if s.stopped() {
			return
		}
		if changed := s.App.TakeDirty(); len(evs) > 0 || s.App.Focus() != focus || changed {
			s.draw()
		}
	}
}

// handleText decodes str with the §26.8 decoder and applies the
// resulting keys and pastes in order, exactly as Run's handleInputs
// walks one read (SPEC v0.2 §15.4 `text:STR`).
func (s *Session) handleText(step int, str string) {
	ins := host.DecodeInput([]byte(str))
	for len(ins) > 0 && !s.stopped() {
		if ins[0].IsPaste {
			s.handlePaste(step, ins[0].Paste)
			ins = ins[1:]
			continue
		}
		if ins[0].IsMouse {
			// An SGR report in the text is a mouse event, as in Run: one
			// outside the grid is dropped, not an error (SPEC v0.2b
			// §26.11).
			s.handleMouse(step, ins[0].Mouse)
			ins = ins[1:]
			continue
		}
		// While the mouse is off, a mouse report inside the keys is
		// dropped without ending the run, as in Run (host.App.KeyRun).
		var keys []host.Key
		keys, ins = s.App.KeyRun(ins)
		s.handleKeys(step, keys)
	}
}

// handlePaste applies one bracketed paste and dispatches its events,
// redrawing when they fired, focus moved, or the paste changed the
// input's text (TakeDirty), exactly as Run's loop dispatches a paste, so
// a later input of the same step sees the new text (SPEC v0.2b §8.6).
func (s *Session) handlePaste(step int, payload string) {
	focus := s.App.Focus()
	evs := s.App.HandlePaste(payload)
	s.dispatchAll(step, evs)
	if s.stopped() {
		return
	}
	if changed := s.App.TakeDirty(); len(evs) > 0 || s.App.Focus() != focus || changed {
		s.draw()
	}
}

// handleMouse applies one mouse event to the live frame (SPEC v0.2b §8.5)
// and dispatches its events, redrawing when they fired, focus moved, or
// a cursor, an offset, or a tab changed, exactly as Run's loop dispatches
// a mouse event. While the frame's mouse is off it changes nothing and
// fires nothing.
func (s *Session) handleMouse(step int, m host.Mouse) {
	if s.stopped() {
		return
	}
	focus := s.App.Focus()
	evs, _ := s.App.HandleMouse(m)
	s.dispatchAll(step, evs)
	if s.stopped() {
		return
	}
	if changed := s.App.TakeDirty(); len(evs) > 0 || s.App.Focus() != focus || changed {
		s.draw()
	}
}

// Run replays step 0 (the first live frame) and then every step in
// order, stopping early on quit, a handler error, or a step that could
// not be applied (SPEC §15.4; SPEC v0.3 §18.1 Play rules 3-5).
func (s *Session) Run(steps []Step) {
	s.settle(0)
	s.snapshot(0, "")
	for i, st := range steps {
		if s.stopped() || s.UsageErr != nil {
			break
		}
		s.curStepEvents = nil
		stepNum := i + 1
		switch st.Kind {
		case KindKey:
			s.handleKeys(stepNum, []host.Key{st.Key})
		case KindText:
			s.handleText(stepNum, st.Text)
		case KindPaste:
			s.handlePaste(stepNum, st.Text)
		case KindSet:
			if err := s.App.Set(st.Path, st.Value); err != nil {
				s.UsageErr, s.UsageStep, s.UsageRaw = err, stepNum, st.Raw
				return
			}
		case KindFocus:
			if err := s.App.Set("@focus", st.Focus); err != nil {
				s.UsageErr, s.UsageStep, s.UsageRaw = err, stepNum, st.Raw
				return
			}
		case KindResize:
			s.Cols, s.Rows = st.Cols, st.Rows
			// A resize drops a pending left press, as in Run (SPEC
			// v0.2b §26.11).
			s.App.DropPress()
		case KindClick, KindWheel:
			// SPEC v0.2b §15.4: handled by §8.5 on the current settled
			// frame; a cell outside the current grid is a usage error.
			if st.X < 0 || st.Y < 0 || st.X >= s.Cols || st.Y >= s.Rows {
				s.UsageErr = fmt.Errorf("cell %d,%d is outside the %dx%d grid", st.X, st.Y, s.Cols, s.Rows)
				s.UsageStep, s.UsageRaw = stepNum, st.Raw
				return
			}
			if st.Kind == KindClick {
				s.handleMouse(stepNum, host.Mouse{Kind: host.MousePress, X: st.X, Y: st.Y})
				s.handleMouse(stepNum, host.Mouse{Kind: host.MouseRelease, X: st.X, Y: st.Y})
				break
			}
			kind := host.MouseWheelDown
			if st.Dir < 0 {
				kind = host.MouseWheelUp
			}
			s.handleMouse(stepNum, host.Mouse{Kind: kind, X: st.X, Y: st.Y})
		}
		if !s.stopped() {
			s.settle(stepNum)
		}
		s.snapshot(stepNum, st.Raw)
	}
}

// FormatEventLine renders e as tuimark play's text format's one line per
// event: "STEP ACTION SOURCE KEYS VALUE", SOURCE "-" when empty, KEYS and
// VALUE compact JSON (SPEC §15.4).
func FormatEventLine(e dump.Event) string {
	src := e.Source
	if src == "" {
		src = "-"
	}
	keysJSON, _ := json.Marshal(e.Keys)
	valJSON, _ := json.Marshal(e.Value)
	return fmt.Sprintf("%d %s %s %s %s", e.Step, e.Action, src, keysJSON, valJSON)
}

// WriteEventsSection appends the "=== events ===" section of tuimark
// play's text format (SPEC §15.4) to b.
func WriteEventsSection(b *strings.Builder, evs []dump.Event) {
	b.WriteString("=== events ===\n")
	if len(evs) == 0 {
		b.WriteString("none\n")
		return
	}
	for _, e := range evs {
		b.WriteString(FormatEventLine(e))
		b.WriteByte('\n')
	}
}
