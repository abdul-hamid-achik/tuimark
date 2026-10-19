package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/tuimark"
)

// The store the view reads. The host writes the raw half (raw), the runtime
// keeps the derived half (derive): the board and the two piece previews as
// rows of same-kind runs, the status, and the labels. app.tui holds no
// expression.
//
//	raw      field [22 strings of . and piece letters] · cur {kind, cells}
//	         ghost [{x,y}…] · flash [row…] · queue [5 letters] · hold · hold_used
//	         score best level lines pieces combo last gravity_ms · height_hist
//	         paused over over_modal auto help_open notice
//	derived  board · hold_rows · next_rows · state · mode_label · speed_label

// setter is what both *tuimark.App and *tuimark.Batch offer.
type setter interface {
	Set(path string, v any) error
}

// raw is the host's half of the store.
func (a *app) raw() map[string]any {
	g := a.g
	field := make([]any, boardH)
	for y := 0; y < boardH; y++ {
		var row strings.Builder
		for x := 0; x < boardW; x++ {
			row.WriteString(g.grid[y][x].letter())
		}
		field[y] = row.String()
	}
	curKind, cells := "", []any{}
	if g.canAct() {
		curKind = g.cur.k.letter()
		for _, c := range g.cur.cells() {
			cells = append(cells, pointValue(c))
		}
	}
	ghost := []any{}
	for _, c := range g.ghost() {
		ghost = append(ghost, pointValue(c))
	}
	flash := []any{}
	for _, y := range g.rows {
		flash = append(flash, y)
	}
	queue := []any{}
	for _, k := range g.preview() {
		queue = append(queue, k.letter())
	}
	hold := ""
	if g.hold != kNone {
		hold = g.hold.letter()
	}
	hist := make([]any, histLen)
	for i, h := range g.hist {
		hist[histLen-len(g.hist)+i] = h
	}
	combo := g.combo
	if combo < 0 {
		combo = 0
	}
	return map[string]any{
		"field":       field,
		"cur":         map[string]any{"kind": curKind, "cells": cells},
		"ghost":       ghost,
		"flash":       flash,
		"queue":       queue,
		"hold":        hold,
		"hold_used":   g.holdUsed,
		"score":       g.score,
		"best":        g.best,
		"level":       g.level(),
		"lines":       g.lines,
		"pieces":      g.pieces,
		"combo":       combo,
		"last":        g.last,
		"gravity_ms":  g.gravityMs(),
		"height_hist": hist,
		"paused":      a.paused,
		"over":        g.over,
		"over_modal":  g.over && !a.overHidden,
		"auto":        a.auto,
		"help_open":   a.help,
		"notice":      a.notice,
	}
}

// publish writes the whole raw half, in a fixed order. It is the one source
// of truth for the opening store, a tick, and every handler.
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

func pointValue(p point) map[string]any { return map[string]any{"x": p.x, "y": p.y} }

// derive declares the derived values. Each is recomputed by the runtime
// inside every store write, so none is ever stale.
func (a *app) derive() error {
	ui := a.ui
	if err := ui.Derive("board", func(get func(string) (any, bool)) (any, error) {
		field, _ := get("field")
		cur, _ := get("cur")
		ghost, _ := get("ghost")
		flash, _ := get("flash")
		paused, _ := get("paused")
		p, _ := paused.(bool)
		return buildBoard(readField(field), readCur(cur), readPoints(ghost), readInts(flash), p), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("hold_rows", func(get func(string) (any, bool)) (any, error) {
		hold, _ := get("hold")
		used, _ := get("hold_used")
		s, _ := hold.(string)
		u, _ := used.(bool)
		return previewRows(kindOf(s), u), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("next_rows", func(get func(string) (any, bool)) (any, error) {
		queue, _ := get("queue")
		list, _ := queue.([]any)
		var ks []kind
		for _, e := range list {
			s, _ := e.(string)
			ks = append(ks, kindOf(s))
		}
		return nextRows(ks), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("state", func(get func(string) (any, bool)) (any, error) {
		paused, over := flag2(get, "paused", "over")
		return stateOf(paused, over), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("mode_label", func(get func(string) (any, bool)) (any, error) {
		v, _ := get("auto")
		if on, _ := v.(bool); on {
			return "autopilot", nil
		}
		return "manual", nil
	}); err != nil {
		return err
	}
	return ui.Derive("speed_label", func(get func(string) (any, bool)) (any, error) {
		v, _ := get("gravity_ms")
		return fmt.Sprintf("%d ms/row", num(v)), nil
	})
}

func flag2(get func(string) (any, bool), a, b string) (bool, bool) {
	f := func(k string) bool {
		v, _ := get(k)
		x, _ := v.(bool)
		return x
	}
	return f(a), f(b)
}

// stateOf is the status line's label and the booleans its styles read.
func stateOf(paused, over bool) map[string]any {
	st := map[string]any{"label": "● PLAYING", "playing": true, "paused": false, "over": false}
	switch {
	case over:
		st = map[string]any{"label": "✖ GAME OVER", "playing": false, "paused": false, "over": true}
	case paused:
		st = map[string]any{"label": "‖ PAUSED", "playing": false, "paused": true, "over": false}
	}
	return st
}

func num(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	}
	return 0
}

func readPoints(v any) []point {
	list, _ := v.([]any)
	out := make([]point, 0, len(list))
	for _, e := range list {
		m, _ := e.(map[string]any)
		out = append(out, point{num(m["x"]), num(m["y"])})
	}
	return out
}

func readInts(v any) []int {
	list, _ := v.([]any)
	out := make([]int, 0, len(list))
	for _, e := range list {
		out = append(out, num(e))
	}
	return out
}

func readField(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, e := range list {
		s, _ := e.(string)
		out = append(out, s)
	}
	return out
}

// activePiece is the falling piece as the store holds it.
type activePiece struct {
	k     kind
	cells []point
}

func readCur(v any) activePiece {
	m, _ := v.(map[string]any)
	s, _ := m["kind"].(string)
	return activePiece{k: kindOf(s), cells: readPoints(m["cells"])}
}

// Cell kinds a run is painted as: nothing, one of the seven pieces (the
// kind's own number), the ghost, and a row that is about to clear.
type ck int

const (
	ckEmpty ck = 0
	ckGhost ck = 8
	ckFlash ck = 9
)

// mkRun is a run of text and the booleans the stylesheet colours it by.
func mkRun(text string, c ck, dim bool) map[string]any {
	return map[string]any{
		"t":     text,
		"empty": c == ckEmpty,
		"ki":    c == ck(kI),
		"ko":    c == ck(kO),
		"kt":    c == ck(kT),
		"ks":    c == ck(kS),
		"kz":    c == ck(kZ),
		"kj":    c == ck(kJ),
		"kl":    c == ck(kL),
		"ghost": c == ckGhost,
		"flash": c == ckFlash,
		"note":  false,
		"dim":   dim && c != ckEmpty,
	}
}

// noteRun is a run of the note style: the word shown over a hidden board.
func noteRun(text string) map[string]any {
	r := mkRun(text, ckEmpty, false)
	r["empty"] = false
	r["note"] = true
	return r
}

// cellText is n cells of kind c: two terminal columns each so a cell looks
// square. A piece and a flash are solid blocks (colour on colour); the ghost
// is shaded; an empty cell is a dot.
func cellText(c ck, n int) string {
	unit := "██"
	switch c {
	case ckEmpty:
		unit = "· "
	case ckGhost:
		unit = "░░"
	case ckFlash:
		unit = "▓▓"
	}
	return strings.Repeat(unit, n)
}

// buildBoard turns the field, the falling piece, its ghost, and the rows
// that are about to clear into the visible rows of runs. Consecutive cells of
// one kind are one run, so a row is a handful of nodes whatever it holds.
// A paused board is empty but for a note: the stack is not shown.
func buildBoard(field []string, cur activePiece, ghost []point, flash []int, paused bool) []any {
	cells := [visibleH][boardW]ck{}
	if !paused {
		for y := hidden; y < boardH && y < len(field); y++ {
			for x := 0; x < boardW && x < len(field[y]); x++ {
				cells[y-hidden][x] = ck(kindOf(field[y][x : x+1]))
			}
		}
		for _, y := range flash {
			if y >= hidden && y < boardH {
				for x := 0; x < boardW; x++ {
					cells[y-hidden][x] = ckFlash
				}
			}
		}
		for _, p := range ghost {
			if p.y >= hidden && p.y < boardH && p.x >= 0 && p.x < boardW {
				cells[p.y-hidden][p.x] = ckGhost
			}
		}
		for _, p := range cur.cells {
			if p.y >= hidden && p.y < boardH && p.x >= 0 && p.x < boardW {
				cells[p.y-hidden][p.x] = ck(cur.k)
			}
		}
	}
	rows := make([]any, visibleH)
	for y := 0; y < visibleH; y++ {
		var runs []any
		for x := 0; x < boardW; {
			c := cells[y][x]
			n := 1
			for x+n < boardW && cells[y][x+n] == c {
				n++
			}
			runs = append(runs, mkRun(cellText(c, n), c, false))
			x += n
		}
		if paused && y == visibleH/2-1 {
			const word = "‖ PAUSED ‖"
			w := len([]rune(word))
			pad := (boardW*2 - w) / 2
			runs = []any{
				mkRun(strings.Repeat(" ", pad), ckEmpty, false),
				noteRun(word),
				mkRun(strings.Repeat(" ", boardW*2-pad-w), ckEmpty, false),
			}
		}
		rows[y] = map[string]any{"runs": runs}
	}
	return rows
}

// previewWidth is the width of a piece preview in terminal columns: four
// cells.
const previewWidth = 8

// previewRows draws piece k in its spawn rotation in two rows of previewWidth
// columns, centred, in solid blocks. Nothing (kNone) is two empty rows. A dim
// piece is the held one that cannot be swapped in again yet.
func previewRows(k kind, dim bool) []any {
	var chars [2][previewWidth]ck
	if k != kNone {
		cells := shapes[k][0]
		minX, maxX, minY := 99, -1, 99
		for _, c := range cells {
			minX, maxX, minY = min(minX, c.x), max(maxX, c.x), min(minY, c.y)
		}
		pad := (previewWidth - 2*(maxX-minX+1)) / 2
		for _, c := range cells {
			col := pad + 2*(c.x-minX)
			chars[c.y-minY][col], chars[c.y-minY][col+1] = ck(k), ck(k)
		}
	}
	rows := make([]any, 2)
	for y := range chars {
		var runs []any
		for x := 0; x < previewWidth; {
			c := chars[y][x]
			n := 1
			for x+n < previewWidth && chars[y][x+n] == c {
				n++
			}
			unit := " "
			if c != ckEmpty {
				unit = "█"
			}
			runs = append(runs, mkRun(strings.Repeat(unit, n), c, dim))
			x += n
		}
		rows[y] = map[string]any{"runs": runs}
	}
	return rows
}

// nextRows stacks the previews of the queue, a blank row between pieces.
func nextRows(queue []kind) []any {
	var rows []any
	for i, k := range queue {
		if i > 0 {
			rows = append(rows, map[string]any{"runs": []any{mkRun(strings.Repeat(" ", previewWidth), ckEmpty, false)}})
		}
		rows = append(rows, previewRows(k, false)...)
	}
	return rows
}
