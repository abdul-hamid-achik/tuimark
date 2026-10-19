package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/tuimark"
)

// The store the view reads. The host writes the raw half (raw), the runtime
// keeps the derived half (derive); app.tui holds no expression.
//
//	raw      puzzle vals marks (81 characters) · notes (81 digit-set masks) · cur
//	         pencil · diff clues · time mistakes hints undo_n redo_n · paused won
//	         help_open new_open win_open · mouse · info notice
//	derived  cells (c00..c88: t cur peer same given bad wrong note done)
//	         boxes (b0..b8) · sel · mini · pad · filled · state · diff_label
//	         mode_label

// setter is what both *tuimark.App and *tuimark.Batch offer.
type setter interface {
	Set(path string, v any) error
}

// cellID names a cell by its place on the screen: row then column, from 0.
func cellID(i int) string { return fmt.Sprintf("c%d%d", rowOf[i], colOf[i]) }

// clock formats milliseconds as m:ss, or h:mm:ss from the first hour.
func clock(ms int64) string {
	s := ms / 1000
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// raw is the host's half of the store.
func (a *app) raw() map[string]any {
	g := a.g
	notes := make([]any, size)
	marks := make([]byte, size)
	for i := 0; i < size; i++ {
		notes[i] = int(g.notes[i])
		marks[i] = '0'
		if g.marks[i] {
			marks[i] = '1'
		}
	}
	return map[string]any{
		"puzzle":    g.puz.String(),
		"vals":      g.vals.String(),
		"marks":     string(marks),
		"notes":     notes,
		"cur":       g.cur,
		"pencil":    g.pencil,
		"diff":      levelName(g.level),
		"clues":     g.puz.clues(),
		"time":      clock(a.elapsed.Milliseconds()),
		"mistakes":  g.mistakes,
		"hints":     g.hints,
		"undo_n":    len(g.undo),
		"redo_n":    len(g.redo),
		"paused":    a.paused,
		"won":       g.won,
		"help_open": a.helpOpen,
		"new_open":  a.newOpen,
		"win_open":  a.winOpen,
		"mouse":     a.mouse,
		"info":      a.info,
		"notice":    a.notice,
	}
}

// publish writes the whole raw half in a fixed order: one source of truth
// for the opening store and every handler.
func (a *app) publish(s setter) error {
	raw := a.raw()
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := s.Set(k, raw[k]); err != nil {
			return err
		}
	}
	return nil
}

// pub is publish as a Batch function.
func (a *app) pub(b *tuimark.Batch) error { return a.publish(b) }

// sync publishes the raw store in one batch.
func (a *app) sync() error { return a.ui.Batch(a.pub) }

// The derived values are functions of the store alone: a Derive function
// runs under the app's lock and reads through get.

func asString(v any) string { s, _ := v.(string); return s }
func asBool(v any) bool     { b, _ := v.(bool); return b }

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

func asInts(v any) []int {
	list, _ := v.([]any)
	out := make([]int, len(list))
	for i, e := range list {
		out[i] = asInt(e)
	}
	return out
}

// view is what the derived values are computed from.
type view struct {
	puz, vals, marks string
	notes            []int
	cur              int
	paused, won      bool
}

func readView(get func(string) (any, bool)) view {
	str := func(k string) string { v, _ := get(k); return asString(v) }
	flag := func(k string) bool { v, _ := get(k); return asBool(v) }
	num := func(k string) int { v, _ := get(k); return asInt(v) }
	nv, _ := get("notes")
	v := view{puz: str("puzzle"), vals: str("vals"), marks: str("marks"), notes: asInts(nv),
		cur: num("cur"), paused: flag("paused"), won: flag("won")}
	// A store that lacks a half (a test binding nothing) draws an empty board.
	if len(v.puz) != size {
		v.puz = strings.Repeat("0", size)
	}
	if len(v.vals) != size {
		v.vals = strings.Repeat("0", size)
	}
	if len(v.marks) != size {
		v.marks = strings.Repeat("0", size)
	}
	for len(v.notes) < size {
		v.notes = append(v.notes, 0)
	}
	if v.cur < 0 || v.cur >= size {
		v.cur = 0
	}
	return v
}

// pausedCells is what the middle box says while the clock is paused: its
// middle row and the one under it, three columns for each cell. The word
// stays inside one box, so no border cuts it.
var pausedCells = func() map[int]string {
	m := map[int]string{}
	for r, line := range map[int]string{4: " PAUSED  ", 5: " press p "} {
		runes := []rune(line)
		for c := 0; c < 3; c++ {
			m[r*9+3+c] = string(runes[c*3 : c*3+3])
		}
	}
	return m
}()

// buildCells is every cell as the view draws it: the text and the flags
// the stylesheet colours by. While paused the digits are hidden; once won
// every cell is done.
func buildCells(v view) map[string]any {
	cells := make(map[string]any, size)
	cur, curD := v.cur, v.vals[v.cur]-'0'
	peer := map[int]bool{}
	for _, p := range peers[cur] {
		peer[p] = true
	}
	for i := 0; i < size; i++ {
		d := v.vals[i] - '0'
		m := map[string]any{"t": "   ", "cur": false, "peer": false, "same": false,
			"given": false, "bad": false, "wrong": false, "note": false, "done": false}
		switch {
		case v.paused:
			if t, ok := pausedCells[i]; ok {
				m["t"] = t
			}
		default:
			if d != 0 {
				m["t"] = fmt.Sprintf(" %d ", d)
				m["given"] = v.puz[i] != '0'
				for _, p := range peers[i] {
					if v.vals[p] == v.vals[i] {
						m["bad"] = true
						break
					}
				}
				m["wrong"] = v.marks[i] == '1'
			} else if v.notes[i] != 0 {
				m["t"] = " · "
				m["note"] = true
			}
			if v.won {
				m["done"] = true
			} else {
				m["cur"] = i == cur
				m["peer"] = peer[i]
				m["same"] = curD != 0 && d == curD
			}
		}
		cells[cellID(i)] = m
	}
	return cells
}

// buildBoxes marks the box the cursor is in.
func buildBoxes(v view) map[string]any {
	boxes := make(map[string]any, 9)
	for b := 0; b < 9; b++ {
		boxes[fmt.Sprintf("b%d", b)] = !v.paused && !v.won && boxOf[v.cur] == b
	}
	return boxes
}

// buildSel names the cursor cell and what it holds.
func buildSel(v view) map[string]any {
	label := fmt.Sprintf("cell r%d c%d", rowOf[v.cur]+1, colOf[v.cur]+1)
	kind := "empty"
	switch {
	case v.paused:
		label, kind = "paused", "—"
	case v.vals[v.cur] != '0' && v.puz[v.cur] != '0':
		kind = "given"
	case v.vals[v.cur] != '0':
		kind = "entry"
	case v.notes[v.cur] != 0:
		kind = "notes"
	}
	return map[string]any{"label": label, "kind": kind}
}

// buildMini is the cursor cell's pencil marks as a 3x3 grid: a digit where
// the mark is set, a dot where it is not. A cell that holds a digit shows it
// in the middle.
func buildMini(v view) map[string]any {
	m := map[string]any{}
	notes := v.notes[v.cur]
	d := int(v.vals[v.cur] - '0')
	if v.paused {
		notes, d = 0, 0
	}
	for k := 1; k <= 9; k++ {
		t, on := "·", has(uint16(notes), k)
		switch {
		case on:
			t = fmt.Sprint(k)
		case d != 0 && k == 5:
			t, on = fmt.Sprint(d), true
		}
		m[fmt.Sprintf("n%d", k)] = t
		m[fmt.Sprintf("on%d", k)] = on
	}
	return m
}

// buildPad marks the digits that are all placed.
func buildPad(v view) map[string]any {
	pad := map[string]any{}
	for d := 1; d <= 9; d++ {
		pad[fmt.Sprintf("p%d", d)] = strings.Count(v.vals, fmt.Sprint(d)) >= 9
	}
	return pad
}

// stateOf is the status label and the flags its styles read.
func stateOf(paused, won bool) map[string]any {
	switch {
	case won:
		return map[string]any{"label": "★ SOLVED", "playing": false, "paused": false, "won": true}
	case paused:
		return map[string]any{"label": "‖ PAUSED", "playing": false, "paused": true, "won": false}
	}
	return map[string]any{"label": "● PLAYING", "playing": true, "paused": false, "won": false}
}

// derive declares the derived values.
func (a *app) derive() error {
	ui := a.ui
	type fn = func(get func(string) (any, bool)) (any, error)
	decl := []struct {
		path string
		f    fn
	}{
		{"cells", func(get func(string) (any, bool)) (any, error) { return buildCells(readView(get)), nil }},
		{"boxes", func(get func(string) (any, bool)) (any, error) { return buildBoxes(readView(get)), nil }},
		{"sel", func(get func(string) (any, bool)) (any, error) { return buildSel(readView(get)), nil }},
		{"mini", func(get func(string) (any, bool)) (any, error) { return buildMini(readView(get)), nil }},
		{"pad", func(get func(string) (any, bool)) (any, error) { return buildPad(readView(get)), nil }},
		{"filled", func(get func(string) (any, bool)) (any, error) {
			return size - strings.Count(readView(get).vals, "0"), nil
		}},
		{"state", func(get func(string) (any, bool)) (any, error) {
			v := readView(get)
			return stateOf(v.paused, v.won), nil
		}},
		{"diff_label", func(get func(string) (any, bool)) (any, error) {
			d, _ := get("diff")
			c, _ := get("clues")
			return fmt.Sprintf("%s · %d clues", asString(d), asInt(c)), nil
		}},
		{"mode_label", func(get func(string) (any, bool)) (any, error) {
			p, _ := get("pencil")
			if asBool(p) {
				return "notes", nil
			}
			return "digits", nil
		}},
	}
	for _, d := range decl {
		if err := ui.Derive(d.path, d.f); err != nil {
			return err
		}
	}
	return nil
}
