package host

import (
	"sync"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// Recorder is the scrollback of a headless inline session (SPEC v0.4d
// §15.4, §18.3): play --inline and Play with Inline lay the frames out as
// a region and record every commit the steps and the handlers make, in
// order, in place of printing it.
type Recorder struct {
	mu         sync.Mutex
	app        *App
	cols, rows int
	grids      []*paint.Grid
	nrows      int
	prevInline bool
	prevSink   inlineSink
}

// StartRecording makes a's frames an inline region and its commits go to
// the returned Recorder, for a headless session of the terminal size cols ×
// rows. A document that writes version is ErrInlineNeedsCurrent. Stop ends
// it.
func (a *App) StartRecording(cols, rows int) (*Recorder, error) {
	prev, err := a.SetInline(true)
	if err != nil {
		return nil, err
	}
	r := &Recorder{app: a, cols: cols, rows: rows, prevInline: prev}
	r.prevSink = a.setInlineSink(r)
	return r, nil
}

// Stop puts a back as StartRecording found it.
func (r *Recorder) Stop() {
	r.app.setInlineSink(r.prevSink)
	r.app.mu.Lock()
	r.app.inline = r.prevInline
	r.app.mu.Unlock()
}

// Resize tells the recorder the terminal is now cols × rows: the size the
// next commits are rendered at.
func (r *Recorder) Resize(cols, rows int) {
	r.mu.Lock()
	r.cols, r.rows = cols, rows
	r.mu.Unlock()
}

// Rows is the number of rows committed so far.
func (r *Recorder) Rows() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nrows
}

func (r *Recorder) termSize() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cols, r.rows
}

func (r *Recorder) print(g *paint.Grid, lines int) error {
	r.mu.Lock()
	r.grids = append(r.grids, g)
	r.nrows += g.H
	r.mu.Unlock()
	return nil
}

// Scrollback is the committed rows as play --inline prints them: one
// string of the terminal's width per row, and, with styles, the style spans
// of each (SPEC v0.4d §15.4).
func (r *Recorder) Scrollback(styles bool) *dump.Scrollback {
	r.mu.Lock()
	defer r.mu.Unlock()
	sb := &dump.Scrollback{Grid: []string{}}
	if styles {
		sb.Styles = [][]dump.Span{}
	}
	for _, g := range r.grids {
		sb.Grid = append(sb.Grid, g.Lines()...)
		if styles {
			sb.Styles = append(sb.Styles, dump.BuildStyles(g)...)
		}
	}
	return sb
}
