package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/tuimark"
)

// The store the view reads. The host writes the raw half (raw), the runtime
// keeps the derived half (derive): the board as rows of same-kind runs, the
// status, and the labels. app.tui holds no expression.
//
//	raw      snake [{x,y}…] head first · food {x,y} · score · best · level
//	         interval_ms · elapsed · speed_hist · paused · over · won · auto
//	         help_open · notice
//	derived  board · length · state · mode_label · speed_label · level_label
//	         over_title

// setter is what both *tuimark.App and *tuimark.Batch offer.
type setter interface {
	Set(path string, v any) error
}

// raw is the host's half of the store.
func (a *app) raw() map[string]any {
	g := a.g
	snake := make([]any, len(g.snake))
	for i, p := range g.snake {
		snake[i] = pointValue(p)
	}
	return map[string]any{
		"snake":       snake,
		"food":        pointValue(g.food),
		"score":       g.score,
		"best":        g.best,
		"level":       g.level(),
		"interval_ms": g.intervalMs(),
		"elapsed":     clock(a.elapsed.Milliseconds()),
		"speed_hist":  append([]any(nil), a.hist...),
		"paused":      a.paused,
		"over":        g.over,
		"won":         g.won,
		"auto":        a.auto,
		"help_open":   a.help,
		"notice":      a.notice,
	}
}

// publish writes the whole raw half, in a fixed order. It is one source of
// truth for the opening store, a tick, and every handler.
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

func pointValue(p point) map[string]any { return map[string]any{"x": p.x, "y": p.y} }

// clock formats milliseconds as m:ss.
func clock(ms int64) string {
	s := ms / 1000
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// derive declares the derived values. Each is recomputed by the runtime
// inside every store write, so none is ever stale.
func (a *app) derive() error {
	ui := a.ui
	if err := ui.Derive("board", func(get func(string) (any, bool)) (any, error) {
		snake, _ := get("snake")
		food, _ := get("food")
		over, _ := get("over")
		o, _ := over.(bool)
		return buildBoard(boardW, boardH, readPoints(snake), readPoint(food), o), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("length", func(get func(string) (any, bool)) (any, error) {
		snake, _ := get("snake")
		s, _ := snake.([]any)
		return len(s), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("state", func(get func(string) (any, bool)) (any, error) {
		paused, over, won := flag3(get, "paused", "over", "won")
		return stateOf(paused, over, won), nil
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
	if err := ui.Derive("speed_label", func(get func(string) (any, bool)) (any, error) {
		ms, _ := get("interval_ms")
		f, _ := ms.(float64)
		return fmt.Sprintf("%d ms", int(f)), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("level_label", func(get func(string) (any, bool)) (any, error) {
		v, _ := get("level")
		f, _ := v.(float64)
		return fmt.Sprintf("%d / %d", int(f), maxLevel), nil
	}); err != nil {
		return err
	}
	return ui.Derive("over_title", func(get func(string) (any, bool)) (any, error) {
		v, _ := get("won")
		if won, _ := v.(bool); won {
			return "BOARD CLEARED", nil
		}
		return "GAME OVER", nil
	})
}

func flag3(get func(string) (any, bool), a, b, c string) (bool, bool, bool) {
	f := func(k string) bool {
		v, _ := get(k)
		x, _ := v.(bool)
		return x
	}
	return f(a), f(b), f(c)
}

// stateOf is the status line's label and the booleans its styles read.
func stateOf(paused, over, won bool) map[string]any {
	st := map[string]any{"label": "● PLAYING", "playing": true, "paused": false, "over": false}
	switch {
	case won:
		st = map[string]any{"label": "★ CLEARED", "playing": false, "paused": false, "over": true}
	case over:
		st = map[string]any{"label": "✖ GAME OVER", "playing": false, "paused": false, "over": true}
	case paused:
		st = map[string]any{"label": "‖ PAUSED", "playing": false, "paused": true, "over": false}
	}
	return st
}

func readPoint(v any) point {
	m, _ := v.(map[string]any)
	x, _ := m["x"].(float64)
	y, _ := m["y"].(float64)
	return point{int(x), int(y)}
}

func readPoints(v any) []point {
	list, _ := v.([]any)
	out := make([]point, len(list))
	for i, e := range list {
		out[i] = readPoint(e)
	}
	return out
}

// Cell kinds on the board.
const (
	cellEmpty = iota
	cellBody
	cellHead
	cellFood
)

// buildBoard turns the snake and the food into rows of runs: consecutive
// cells of one kind are one run, so a row is a handful of nodes whatever the
// board's width. A run's text is two columns per cell, which makes a cell
// look square (spaces in a bound value keep their width; only the source
// text of a <text> body is trimmed); its booleans are the class: guards the
// stylesheet colours. After the game is over every snake cell also carries
// dead.
func buildBoard(w, h int, snake []point, food point, dead bool) []any {
	kinds := make([]int, w*h)
	for i, p := range snake {
		if p.x < 0 || p.x >= w || p.y < 0 || p.y >= h {
			continue
		}
		if i == 0 {
			kinds[p.y*w+p.x] = cellHead
		} else {
			kinds[p.y*w+p.x] = cellBody
		}
	}
	if food.x >= 0 && food.x < w && food.y >= 0 && food.y < h {
		kinds[food.y*w+food.x] = cellFood
	}
	rows := make([]any, h)
	for y := 0; y < h; y++ {
		var runs []any
		for x := 0; x < w; {
			k := kinds[y*w+x]
			n := 1
			for x+n < w && kinds[y*w+x+n] == k {
				n++
			}
			runs = append(runs, cellRun(k, n, dead))
			x += n
		}
		rows[y] = map[string]any{"runs": runs}
	}
	return rows
}

// pub is publish as a Batch function.
func (a *app) pub(b *tuimark.Batch) error { return a.publish(b) }

// cellRun is n cells of kind k (a head or a food is always one cell).
func cellRun(k, n int, dead bool) map[string]any {
	var t string
	switch k {
	case cellHead:
		t = "••"
		if dead {
			t = "××"
		}
	case cellFood:
		t = "▐▌"
	default:
		t = strings.Repeat("  ", n)
	}
	return map[string]any{
		"t":     t,
		"empty": k == cellEmpty,
		"snake": k == cellBody,
		"head":  k == cellHead,
		"food":  k == cellFood,
		"dead":  dead && (k == cellBody || k == cellHead),
	}
}
