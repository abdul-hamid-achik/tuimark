package main

import "fmt"

// The store the document reads. The board is one object per screen
// position of the largest grid, "b.cRRCC" (row and column, two digits
// each). A cell of the current grid has every class guard present even
// where it is false, because a missing path is a diagnostic. A cell outside
// the grid is just {"on": false}: the document turns that into an `if`, so
// its guards are never read.
//
//	b.cRRCC.on    the position is part of the current grid (off: the cell is
//	              just {"on": false})
//	b.cRRCC.t     two columns of text: "▒▒", " 3", " ►", ...
//	b.cRRCC.hid   covered (also true for a flag and a question mark)
//	b.cRRCC.n1…n8 an open cell with that number (the cell is open with none)
//	b.cRRCC.flag, q, mine, boom, wrong, cur
//	rows.rRR      the row is part of the current grid
//
// The rest is small and plain: dims, level, state, secs, mines_left,
// flag_mode, paused, help_open, notice, best, settings.mouse. App.Derive
// turns those into the labels and class flags the document shows.

// cellView is what one screen position shows.
type cellView struct {
	on, hid, flag, q, mine, boom, wrong, cur bool
	n                                        int // 0 for none, else 1-8
	t                                        string
}

// The two columns of each kind of cell. All of them are one column per
// character: no emoji.
const (
	glyphHidden   = "▒▒"
	glyphFlag     = " ►"
	glyphQuestion = " ?"
	glyphMine     = " *"
	glyphWrong    = " x"
	glyphEmpty    = "  "
)

func cellID(r, c int) string { return fmt.Sprintf("c%02d%02d", r, c) }
func rowID(r int) string     { return fmt.Sprintf("r%02d", r) }

// parseCellID is the position a cell id names.
func parseCellID(id string) (r, c int, ok bool) {
	if len(id) != 5 || id[0] != 'c' {
		return 0, 0, false
	}
	if _, err := fmt.Sscanf(id[1:], "%02d%02d", &r, &c); err != nil {
		return 0, 0, false
	}
	if r < 0 || r >= maxRows || c < 0 || c >= maxCols || cellID(r, c) != id {
		return 0, 0, false
	}
	return r, c, true
}

// store is the cell as the store holds it: every guard present for a cell
// of the grid; a position outside it is just {"on": false}, because its
// `if` is false and its guards are never read.
func (v cellView) store() map[string]any {
	if !v.on {
		return map[string]any{"on": false}
	}
	m := map[string]any{
		"on": v.on, "t": v.t, "hid": v.hid, "flag": v.flag, "q": v.q,
		"mine": v.mine, "boom": v.boom, "wrong": v.wrong, "cur": v.cur,
	}
	for k := 1; k <= 8; k++ {
		m[fmt.Sprintf("n%d", k)] = v.n == k
	}
	return m
}

// view is what screen position (r, c) shows now.
func (a *app) view(r, c int) cellView {
	g := a.g
	if r >= g.rows || c >= g.cols {
		return cellView{}
	}
	i := g.idx(r, c)
	// The cursor is a tool for the game being played: it goes when the game ends.
	v := cellView{on: true, cur: i == a.cur && !g.over, t: glyphEmpty}
	lost := g.over && !g.won
	switch g.st[i] {
	case stOpen:
		if n := int(g.adj[i]); n > 0 {
			v.n = n
			v.t = fmt.Sprintf(" %d", n)
		}
	case stFlag:
		if lost && !g.mine[i] {
			v.wrong, v.t = true, glyphWrong
		} else {
			v.hid, v.flag, v.t = true, true, glyphFlag
		}
	case stQuestion:
		if lost && g.mine[i] {
			v.mine, v.t = true, glyphMine
		} else {
			v.hid, v.q, v.t = true, true, glyphQuestion
		}
	default: // covered
		if lost && g.mine[i] {
			v.mine, v.t = true, glyphMine
		} else {
			v.hid, v.t = true, glyphHidden
		}
	}
	if lost && i == g.boom {
		v.mine, v.boom, v.hid, v.t = true, true, false, glyphMine
	}
	return v
}

// rowsStore is the rows of the current grid.
func (a *app) rowsStore() map[string]any {
	m := make(map[string]any, maxRows)
	for r := 0; r < maxRows; r++ {
		m[rowID(r)] = r < a.g.rows
	}
	return m
}

// cellsStore is the whole board.
func (a *app) cellsStore() map[string]any {
	m := make(map[string]any, maxRows*maxCols)
	for r := 0; r < maxRows; r++ {
		for c := 0; c < maxCols; c++ {
			m[cellID(r, c)] = a.view(r, c).store()
		}
	}
	return m
}

func (a *app) bestStore() map[string]any {
	m := map[string]any{}
	for _, p := range presets {
		m[p.name] = a.best[p.name]
	}
	return m
}

// state names what the game is doing, for the panel and the classes.
func (a *app) state() string {
	switch {
	case a.g.over && a.g.won:
		return "won"
	case a.g.over:
		return "lost"
	case a.paused:
		return "paused"
	case a.g.placed:
		return "playing"
	}
	return "ready"
}

// secs is the playing time on the clock, in whole seconds, held at 99:59.
func (a *app) secs() int {
	s := int(a.elapsed / 1e9)
	if s > maxSecs {
		s = maxSecs
	}
	return s
}

const maxSecs = 99*60 + 59

// raw is the whole store of the app now, before Derive.
func (a *app) raw() map[string]any {
	return map[string]any{
		"settings":   map[string]any{"mouse": a.mouse},
		"b":          a.cellsStore(),
		"rows":       a.rowsStore(),
		"dims":       map[string]any{"cols": a.g.cols, "rows": a.g.rows, "mines": a.g.mines},
		"level":      a.level,
		"state":      a.state(),
		"secs":       a.secs(),
		"mines_left": a.g.minesLeft(),
		"flag_mode":  a.flagMode,
		"paused":     a.paused,
		"help_open":  a.help,
		"notice":     a.notice,
		"best":       a.bestStore(),
	}
}

// setter is a Batch or the App itself.
type setter interface {
	Set(path string, v any) error
}

// publish writes the scalars and every cell that changed since the last
// publish.
func (a *app) publish(s setter) error {
	for _, w := range []struct {
		path string
		v    any
	}{
		{"dims", map[string]any{"cols": a.g.cols, "rows": a.g.rows, "mines": a.g.mines}},
		{"level", a.level},
		{"state", a.state()},
		{"secs", a.secs()},
		{"mines_left", a.g.minesLeft()},
		{"flag_mode", a.flagMode},
		{"paused", a.paused},
		{"help_open", a.help},
		{"notice", a.notice},
		{"best", a.bestStore()},
		{"settings.mouse", a.mouse},
	} {
		if err := s.Set(w.path, w.v); err != nil {
			return err
		}
	}
	if a.shownRows != a.g.rows {
		if err := s.Set("rows", a.rowsStore()); err != nil {
			return err
		}
		a.shownRows = a.g.rows
	}
	type change struct {
		p int
		v cellView
	}
	var changed []change
	for r := 0; r < maxRows; r++ {
		for c := 0; c < maxCols; c++ {
			v := a.view(r, c)
			if p := r*maxCols + c; a.shown[p] != v {
				if err := s.Set("b."+cellID(r, c), v.store()); err != nil {
					return err
				}
				changed = append(changed, change{p, v})
			}
		}
	}
	for _, ch := range changed { // remember only what was written
		a.shown[ch.p] = ch.v
	}
	return nil
}
