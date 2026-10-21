package host

import (
	"io"
	"sync"

	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// The terminal side of an inline session (SPEC v0.4d §26.15): the region
// and what is printed above it, written one at a time, in the order the
// host made them (§26.15.4 "Order").

// enterInline is the enter sequence of an inline session, §26.1 step 3
// without CSI ? 1049 h, CSI H, and CSI 2 J (§26.15.3, decision 243): the
// cursor hidden, autowrap off, and bracketed paste on. The region starts
// at the column 0 of the cursor's row.
const enterInline = "\x1b[?25l\x1b[?7l\x1b[?2004h"

// leaveInline is the leave sequence of §26.1 step 7 without CSI ? 1049 l;
// the region is erased before it (inlineTerm.finish).
const leaveInline = "\x1b[?2004l\x1b[0m\x1b[?25h\x1b[?7h"

// pendingCommit is a commit waiting for the loop to print it.
type pendingCommit struct {
	g     *paint.Grid
	lines int
	done  chan error
}

// inlineTerm is the state of the terminal of an inline session: the
// region, the size commits are rendered at, the commits waiting for the
// loop, and whether the loop is running host code. Every method is safe on
// a nil *inlineTerm (a full-screen session) and from any goroutine.
type inlineTerm struct {
	mu   sync.Mutex
	out  io.Writer
	opts paint.Options
	// sync wraps each write in synchronized output (SPEC v0.4d §26.15.3).
	sync bool
	log  *runLog
	reg  paint.Region
	// cols, rows is the terminal's size, as the loop last saw it.
	cols, rows int
	// host counts the pieces of host code the loop is running (a handler,
	// an Every function, OnError): a Commit from any goroutine prints at
	// once while it is above 0 (SPEC v0.4d §18.3, decision 247).
	host   int
	queue  []*pendingCommit
	closed bool
	// kick asks the loop to settle: a commit is waiting.
	kick chan struct{}
}

func newInlineTerm(out io.Writer, lg *runLog) *inlineTerm {
	return &inlineTerm{out: out, log: lg, kick: make(chan struct{}, 1)}
}

// configure sets what the probe decided: the color profile and whether
// cluster re-positioning and synchronized output are on.
func (t *inlineTerm) configure(opts paint.Options, sync bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.opts, t.sync = opts, sync
	t.mu.Unlock()
}

func (t *inlineTerm) setSize(cols, rows int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.cols, t.rows = cols, rows
	t.mu.Unlock()
}

func (t *inlineTerm) termSize() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cols, t.rows
}

// enterHost and exitHost bracket a piece of host code the loop runs.
func (t *inlineTerm) enterHost() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.host++
	t.mu.Unlock()
}

func (t *inlineTerm) exitHost() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.host--
	t.mu.Unlock()
}

// write writes s in one write, in synchronized output when it is on. The
// caller holds t.mu.
func (t *inlineTerm) write(s string) error {
	if s == "" {
		return nil
	}
	if t.sync {
		s = syncBegin + s + syncEnd
	}
	_, err := io.WriteString(t.out, s)
	return err
}

// print is the sink of App.Commit (SPEC v0.4d §18.3, decision 247): at
// once when the loop runs host code, else queued to the loop, whose next
// frame comes after it; the call returns after the print.
func (t *inlineTerm) print(g *paint.Grid, lines int) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return errCommitNotInline
	}
	if t.host > 0 {
		err := t.printLocked(g, lines)
		t.mu.Unlock()
		return err
	}
	pc := &pendingCommit{g: g, lines: lines, done: make(chan error, 1)}
	t.queue = append(t.queue, pc)
	t.mu.Unlock()
	select {
	case t.kick <- struct{}{}:
	default:
	}
	return <-pc.done
}

// printLocked prints a commit: the region is erased from its first row,
// the commit's rows are written, and the region is drawn again below them
// as it was last drawn (SPEC v0.4d §26.15.4). The caller holds t.mu.
func (t *inlineTerm) printLocked(g *paint.Grid, lines int) error {
	last := t.reg.Last()
	s := t.reg.Erase() + t.opts.CommitRows(g)
	if last != nil {
		s += t.reg.Frame(last, t.opts)
	}
	err := t.write(s)
	t.log.write("commit", member{"line_count", lines}, member{"rows", g.H})
	return err
}

// flush prints the commits waiting for the loop, in order, before the
// frame the loop is about to draw.
func (t *inlineTerm) flush() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	q := t.queue
	t.queue = nil
	var first error
	for _, pc := range q {
		err := t.printLocked(pc.g, pc.lines)
		if first == nil {
			first = err
		}
		pc.done <- err
	}
	t.mu.Unlock()
	return first
}

// frame draws g as the region and returns the bytes written.
func (t *inlineTerm) frame(g *paint.Grid) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.reg.Frame(g, t.opts)
	if t.sync {
		s = syncBegin + s + syncEnd
	}
	_, err := io.WriteString(t.out, s)
	return len(s), err
}

// resized erases the region from its first row, before the frame at the
// new size draws it again (SPEC v0.4d §26.15.5, decision 248). Rows
// already committed are never repainted.
func (t *inlineTerm) resized() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.reg.Drawn() {
		return nil
	}
	return t.write(t.reg.Erase())
}

// finish ends the session's terminal state and returns what its leave
// sequence starts with (SPEC v0.4d §26.15.3): the commits accepted before
// the end, printed, then the move to the region's first row, CR, and
// CSI J. Later commits are the error "not in inline mode". It returns ""
// the second time, so the loop's way out and the signal restore write it
// once.
func (t *inlineTerm) finish() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return ""
	}
	t.closed = true
	s := ""
	for _, pc := range t.queue {
		s += t.reg.Erase() + t.opts.CommitRows(pc.g)
		t.log.write("commit", member{"line_count", pc.lines}, member{"rows", pc.g.H})
		pc.done <- nil
	}
	t.queue = nil
	if t.reg.Drawn() || s != "" {
		s += t.reg.Erase()
	}
	return s
}
