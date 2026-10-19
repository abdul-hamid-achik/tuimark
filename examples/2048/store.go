package main

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/abdul-hamid-achik/tuimark"
)

// The store the view reads. The host writes the raw half (raw), the runtime
// keeps the derived half (derive): the board as rows of tiles with the class
// guards the stylesheet colours, the size flags, and the labels. app.tui
// holds no expression.
//
//	raw      size · tiles [[value]] · pop [[bool]] · fresh [[bool]]
//	         score · gain · best · moves · largest · hist
//	         auto · help_open · win_open · over · over_open · notice
//	derived  board · dims · gain_label · state · mode_label · size_label

// setter is what both *tuimark.App and *tuimark.Batch offer.
type setter interface {
	Set(path string, v any) error
}

// raw is the host's half of the store.
func (a *app) raw() map[string]any {
	g := a.g
	return map[string]any{
		"size":      g.n,
		"tiles":     intRows(g.cells, g.n),
		"pop":       boolRows(g.pop, g.n),
		"fresh":     boolRows(g.fresh, g.n),
		"score":     g.score,
		"gain":      g.gain,
		"best":      g.best,
		"moves":     g.moves,
		"largest":   g.largest(),
		"hist":      sampleHist(g.hist, histLen),
		"auto":      a.auto,
		"help_open": a.help,
		"win_open":  a.winOpen,
		"over":      g.over,
		"over_open": a.overOpen,
		"notice":    a.notice,
	}
}

func intRows(cells []int, n int) []any {
	rows := make([]any, n)
	for y := 0; y < n; y++ {
		row := make([]any, n)
		for x := 0; x < n; x++ {
			row[x] = cells[y*n+x]
		}
		rows[y] = row
	}
	return rows
}

func boolRows(cells []bool, n int) []any {
	rows := make([]any, n)
	for y := 0; y < n; y++ {
		row := make([]any, n)
		for x := 0; x < n; x++ {
			row[x] = cells[y*n+x]
		}
		rows[y] = row
	}
	return rows
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

// sync publishes the raw store in one batch.
func (a *app) sync() error { return a.ui.Batch(a.pub) }

// The value classes the stylesheet colours; every tile carries exactly one
// of empty, v2 ... v2048, or vbig.
var valueClasses = []int{2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048}

// tileText is what a tile shows: the value, or 2^k when it would not fit in
// six columns (a 7-digit value, which a 6x6 board can reach on paper only).
func tileText(v int) string {
	if v <= 0 {
		return ""
	}
	s := strconv.Itoa(v)
	if len(s) <= 6 {
		return s
	}
	return "2^" + strconv.Itoa(int(exponent(v)))
}

// tileCell is one cell of the board as the view reads it.
func tileCell(v int, pop, fresh bool) map[string]any {
	m := map[string]any{
		"t":     tileText(v),
		"empty": v == 0,
		"vbig":  v > winValue,
		"pop":   pop,
		"fresh": fresh,
	}
	for _, c := range valueClasses {
		m["v"+strconv.Itoa(c)] = v == c
	}
	return m
}

// buildBoard turns the three matrices into rows of tiles.
func buildBoard(tiles [][]int, pop, fresh [][]bool) []any {
	rows := make([]any, len(tiles))
	for y, row := range tiles {
		cells := make([]any, len(row))
		for x, v := range row {
			cells[x] = tileCell(v, pop[y][x], fresh[y][x])
		}
		rows[y] = map[string]any{"cells": cells}
	}
	return rows
}

func readInts(v any) [][]int {
	rows, _ := v.([]any)
	out := make([][]int, len(rows))
	for y, r := range rows {
		cells, _ := r.([]any)
		out[y] = make([]int, len(cells))
		for x, c := range cells {
			f, _ := c.(float64)
			if i, ok := c.(int); ok {
				f = float64(i)
			}
			out[y][x] = int(f)
		}
	}
	return out
}

func readBools(v any) [][]bool {
	rows, _ := v.([]any)
	out := make([][]bool, len(rows))
	for y, r := range rows {
		cells, _ := r.([]any)
		out[y] = make([]bool, len(cells))
		for x, c := range cells {
			out[y][x], _ = c.(bool)
		}
	}
	return out
}

func number(get func(string) (any, bool), path string) int {
	v, _ := get(path)
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	}
	return 0
}

func truth(get func(string) (any, bool), path string) bool {
	v, _ := get(path)
	b, _ := v.(bool)
	return b
}

// sizeFlags are the board-size classes: the stylesheet's breakpoints differ
// per size, and a class: guard is how a document says which size it is.
func sizeFlags(n int) map[string]any {
	m := map[string]any{}
	for s := minSize; s <= maxSize; s++ {
		m["n"+strconv.Itoa(s)] = s == n
	}
	return m
}

// stateOf is the status line's label and the booleans its styles read.
func stateOf(over, won bool) map[string]any {
	switch {
	case over:
		return map[string]any{"label": "✖ GAME OVER", "playing": false, "won": false, "over": true}
	case won:
		return map[string]any{"label": "★ 2048", "playing": false, "won": true, "over": false}
	}
	return map[string]any{"label": "● PLAYING", "playing": true, "won": false, "over": false}
}

func gainLabel(gain int) string {
	if gain <= 0 {
		return ""
	}
	return fmt.Sprintf("+%d", gain)
}

// derive declares the derived values. Each is recomputed by the runtime
// inside every store write, so none is ever stale.
func (a *app) derive() error {
	ui := a.ui
	if err := ui.Derive("board", func(get func(string) (any, bool)) (any, error) {
		tiles, _ := get("tiles")
		pop, _ := get("pop")
		fresh, _ := get("fresh")
		return buildBoard(readInts(tiles), readBools(pop), readBools(fresh)), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("dims", func(get func(string) (any, bool)) (any, error) {
		return sizeFlags(number(get, "size")), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("size_label", func(get func(string) (any, bool)) (any, error) {
		n := number(get, "size")
		return fmt.Sprintf("%d×%d", n, n), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("gain_label", func(get func(string) (any, bool)) (any, error) {
		return gainLabel(number(get, "gain")), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("state", func(get func(string) (any, bool)) (any, error) {
		return stateOf(truth(get, "over"), number(get, "largest") >= winValue), nil
	}); err != nil {
		return err
	}
	return ui.Derive("mode_label", func(get func(string) (any, bool)) (any, error) {
		if truth(get, "auto") {
			return "autopilot", nil
		}
		return "manual", nil
	})
}
