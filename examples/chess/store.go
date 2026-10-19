package main

import (
	"fmt"
	"sort"

	"github.com/abdul-hamid-achik/tuimark"
)

// The store the view reads. The host writes the raw half (raw): the board
// as 64 FEN letters, the cursor, the selection, the history in SAN, the
// clocks, the flags of the dialogs. The runtime keeps the derived half
// (derive): the 64 cells with their class guards, the move rows, the two
// player strips, the status, the evaluation bar, the labels. app.tui holds
// no expression and chess.go knows no layout.
//
//	raw      mode level human names flip letters mouse review ply total turn pieces
//	         cur curon sel targets caps last checksq hint san start_n start_black
//	         result reason check over claim captured matdiff clock thinking
//	         spin engine evalcp notice help_open new_open resign_open
//	         promo_open pgn_open ng pgn_text cmd save
//	derived  cells labels mrows mcur top bot status evalbar engine_view
//	         mode_label clocks_line think_label ng_view

// getter is the get function Derive hands to its callback.
type getter = func(path string) (any, bool)

func gStr(get getter, path string) string {
	v, _ := get(path)
	s, _ := v.(string)
	return s
}

func gBool(get getter, path string) bool {
	v, _ := get(path)
	b, _ := v.(bool)
	return b
}

func gInt(get getter, path string) int {
	v, _ := get(path)
	f, _ := v.(float64)
	return int(f)
}

func gList(get getter, path string) []any {
	v, _ := get(path)
	l, _ := v.([]any)
	return l
}

func asInts(l []any) []int {
	out := make([]int, len(l))
	for i, v := range l {
		f, _ := v.(float64)
		out[i] = int(f)
	}
	return out
}

func asStr(v any) string { s, _ := v.(string); return s }

// ---- raw ----

func ints(s []int) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

func colorName(c Color) string { return c.String() }

// captured lists the glyphs each side has taken up to the ply on the board,
// biggest piece first, and the material balance (white minus black, in
// pawns) of the position on the board.
func (a *app) captured() (w, b string, diff int) {
	byW := map[PieceType]int{}
	byB := map[PieceType]int{}
	mover := a.g.Start.Turn
	for i := 0; i < a.ply; i++ {
		if c := a.g.Caps[i]; c != 0 {
			if mover == White {
				byW[c.Type()]++
			} else {
				byB[c.Type()]++
			}
		}
		mover = mover.Other()
	}
	list := func(m map[PieceType]int, c Color) string {
		s := ""
		for _, t := range []PieceType{Queen, Rook, Bishop, Knight, Pawn} {
			for i := 0; i < m[t]; i++ {
				s += mk(c, t).Glyph()
			}
		}
		return s
	}
	vals := map[PieceType]int{Pawn: 1, Knight: 3, Bishop: 3, Rook: 5, Queen: 9}
	for _, pc := range a.viewPos().B {
		if pc == 0 {
			continue
		}
		if pc.Color() == White {
			diff += vals[pc.Type()]
		} else {
			diff -= vals[pc.Type()]
		}
	}
	return list(byW, Black), list(byB, White), diff
}

// raw is the host's half of the store.
func (a *app) raw() map[string]any {
	p := a.viewPos()
	pieces := make([]any, 64)
	for sq := range pieces {
		pieces[sq] = p.B[sq].Letter()
	}
	san := make([]any, len(a.g.SAN))
	for i, s := range a.g.SAN {
		san[i] = s
	}
	last := [2]int{-1, -1}
	if a.ply > 0 {
		m := a.g.Moves[a.ply-1]
		last = [2]int{int(m.From), int(m.To)}
	}
	checksq := -1
	if p.InCheck() {
		checksq = int(p.King[p.Turn])
	}
	over := a.g.Over() && (!a.review || a.ply == len(a.g.Moves))
	capW, capB, diff := a.captured()
	run := ""
	if a.clockRunning() {
		run = colorName(a.g.Cur().Turn)
	}
	claim := ""
	if !a.review {
		claim = a.g.ClaimableDraw()
	}
	eng := map[string]any{"has": a.engine.has, "depth": a.engine.depth, "score": a.engine.score, "nodes": a.engine.nodes, "pv": a.engine.pv}
	ev := a.evalAt(p)
	names := map[string]any{"white": "", "black": ""}
	if a.set.mode == mode2P {
		names["white"], names["black"] = a.g.Tags["White"], a.g.Tags["Black"]
	}
	human := "none"
	if a.set.mode == modeHvC {
		human = colorName(a.set.human)
	}
	return map[string]any{
		"mode":        a.set.mode,
		"level":       a.set.level,
		"human":       human,
		"names":       names,
		"flip":        a.flip,
		"letters":     a.letters,
		"mouse":       a.mouse,
		"review":      a.review,
		"ply":         a.ply,
		"total":       len(a.g.Moves),
		"turn":        colorName(p.Turn),
		"pieces":      pieces,
		"cur":         a.cur,
		"curon":       a.curOn,
		"sel":         a.sel,
		"targets":     ints(a.targets),
		"caps":        ints(a.caps),
		"last":        []any{last[0], last[1]},
		"checksq":     checksq,
		"hint":        []any{a.hint[0], a.hint[1]},
		"san":         san,
		"start_n":     a.g.Start.Full,
		"start_black": a.g.Start.Turn == Black,
		"result":      a.g.Result,
		"reason":      a.g.Reason,
		"check":       p.InCheck(),
		"over":        over,
		"claim":       claim,
		"captured":    map[string]any{"w": capW, "b": capB},
		"matdiff":     diff,
		"clock": map[string]any{
			"on": a.clk.spec.On(), "w": a.clk.ms[White], "b": a.clk.ms[Black], "run": run,
		},
		"thinking":    a.thinking,
		"spin":        spinner[a.spin%len(spinner)],
		"engine":      eng,
		"evalcp":      ev,
		"notice":      a.notice,
		"help_open":   a.help,
		"new_open":    a.newOpen,
		"resign_open": a.resignOpen,
		"promo_open":  a.promoOpen,
		"pgn_open":    a.pgnOpen,
		"ng":          map[string]any{"mode": a.ng.mode, "side": a.ng.side, "level": a.ng.level, "clock": a.ng.clock},
		"pgn_text":    a.pgnText,
		"cmd":         "",
		"save":        a.save,
	}
}

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// setter is what both *tuimark.App and *tuimark.Batch offer.
type setter interface {
	Set(path string, v any) error
}

// publish writes the raw half in a fixed order. cmd belongs to the move box
// (the runtime writes it as you type), so only a load and an explicit clear
// write it.
func publish(s setter, raw map[string]any, withCmd bool) error {
	keys := make([]string, 0, len(raw))
	for k := range raw {
		if k == "cmd" && !withCmd {
			continue
		}
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

// writeLocked writes the whole raw half in one Batch. The caller holds mu.
func (a *app) writeLocked() error {
	if a.pgnOpen {
		a.pgnText = a.dialogPGN() // the game may have moved on behind the dialog
	}
	raw := a.raw()
	af := a.after
	a.after = after{}
	return a.ui.Batch(func(b *tuimark.Batch) error {
		if err := publish(b, raw, af.clearCmd); err != nil {
			return err
		}
		if af.focus != "" {
			return b.Set("@focus", af.focus)
		}
		return nil
	})
}

// ---- derived ----

// screenSq is the square shown at screen row r and column c.
func screenSq(r, c int, flip bool) int {
	if flip {
		return r*8 + (7 - c)
	}
	return (7-r)*8 + c
}

func cellID(r, c int) string { return fmt.Sprintf("v%d%d", r, c) }

// cellsOf builds the 64 cells the board shows.
func cellsOf(get getter) map[string]any {
	pieces := gList(get, "pieces")
	flip, letters, review := gBool(get, "flip"), gBool(get, "letters"), gBool(get, "review")
	cur, sel, checksq := gInt(get, "cur"), gInt(get, "sel"), gInt(get, "checksq")
	curOn := gBool(get, "curon")
	tgt, cap := map[int]bool{}, map[int]bool{}
	for _, s := range asInts(gList(get, "targets")) {
		tgt[s] = true
	}
	for _, s := range asInts(gList(get, "caps")) {
		cap[s] = true
	}
	last, hint := asInts(gList(get, "last")), asInts(gList(get, "hint"))
	out := make(map[string]any, 64)
	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			sq := screenSq(r, c, flip)
			letter := ""
			if sq < len(pieces) {
				letter = asStr(pieces[sq])
			}
			t := ""
			white, black := false, false
			if letter != "" {
				pc, _ := pieceFromLetter(letter[0])
				white, black = pc.Color() == White, pc.Color() == Black
				t = pc.Glyph()
				if letters {
					t = letter
				}
			} else if tgt[sq] {
				t = "•"
			}
			isLast := len(last) == 2 && (sq == last[0] || sq == last[1])
			isHint := len(hint) == 2 && (sq == hint[0] || sq == hint[1]) && hint[0] >= 0
			out[cellID(r, c)] = map[string]any{
				"t": t, "w": white, "b": black,
				"cur":  curOn && !review && sq == cur,
				"sel":  sq == sel,
				"tgt":  tgt[sq] && letter == "",
				"cap":  cap[sq],
				"last": isLast && last[0] >= 0,
				"chk":  sq == checksq,
				"hint": isHint,
			}
		}
	}
	return out
}

func labelsOf(get getter) map[string]any {
	flip := gBool(get, "flip")
	out := map[string]any{}
	for i := 0; i < 8; i++ {
		f, r := i, 7-i
		if flip {
			f, r = 7-i, i
		}
		out[fmt.Sprintf("f%d", i)] = string(rune('a' + f))
		out[fmt.Sprintf("r%d", i)] = string(rune('1' + r))
	}
	return out
}

// mrowsOf is the move list as numbered pairs, with the move on the board
// marked.
func mrowsOf(get getter) []any {
	san := gList(get, "san")
	ply := gInt(get, "ply")
	n := gInt(get, "start_n")
	black := gBool(get, "start_black")
	rows := []any{}
	i := 0
	for i < len(san) {
		row := map[string]any{"n": n, "w": "", "b": "", "cw": false, "cb": false}
		if black {
			row["w"] = "…"
		} else {
			row["w"] = asStr(san[i])
			row["cw"] = i+1 == ply
			i++
		}
		black = false
		if i < len(san) {
			row["b"] = asStr(san[i])
			row["cb"] = i+1 == ply
			i++
		}
		rows = append(rows, row)
		n++
	}
	return rows
}

// mcurOf is the number of the row holding the move on the board: the move
// list's cursor, which keeps that row in view.
func mcurOf(get getter) int {
	ply := gInt(get, "ply")
	n := gInt(get, "start_n")
	if ply == 0 {
		return n
	}
	idx := ply - 1
	if gBool(get, "start_black") {
		idx++
	}
	return n + idx/2
}

func playerName(get getter, c Color) string {
	switch gStr(get, "mode") {
	case modeHvC:
		if gStr(get, "human") == c.String() {
			return "you"
		}
		return levelName(gInt(get, "level"))
	case modeCvC:
		return levelName(gInt(get, "level"))
	}
	n, _ := get("names")
	nm, _ := n.(map[string]any)
	if name := asStr(nm[c.String()]); name != "" {
		return name
	}
	return capital(c.String())
}

// playerOf is the strip of one side: the name, the clock, what it has taken.
func playerOf(get getter, c Color) map[string]any {
	clock, _ := get("clock")
	cm, _ := clock.(map[string]any)
	ms, _ := cm[map[Color]string{White: "w", Black: "b"}[c]].(float64)
	on, _ := cm["on"].(bool)
	run, _ := cm["run"].(string)
	capd, _ := get("captured")
	capm, _ := capd.(map[string]any)
	taken := asStr(capm[map[Color]string{White: "w", Black: "b"}[c]])
	diff := gInt(get, "matdiff")
	if c == Black {
		diff = -diff
	}
	lead := ""
	if diff > 0 {
		lead = fmt.Sprintf("+%d", diff)
	}
	clockText := "–"
	if on {
		clockText = fmtClock(int64(ms))
	}
	over := gBool(get, "over")
	turn := gStr(get, "turn") == c.String() && !over
	glyph := mk(c, King).Glyph()
	return map[string]any{
		"name":  playerName(get, c),
		"glyph": glyph,
		"clock": clockText,
		"cap":   taken,
		"lead":  lead,
		"white": c == White,
		"black": c == Black,
		// What a side took is of the other colour.
		"capwhite": c == Black,
		"capblack": c == White,
		"turn":     turn,
		"run":      run == c.String(),
		"low":      on && ms < 20000,
		"think":    turn && gBool(get, "thinking"),
		"mark":     map[bool]string{true: "▶", false: " "}[turn],
	}
}

// resultText writes a result with an en dash: 1–0, 0–1, ½–½.
func resultText(r string) string {
	switch r {
	case "1-0":
		return "1–0"
	case "0-1":
		return "0–1"
	case "1/2-1/2":
		return "½–½"
	}
	return r
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}

// statusOf is the status box: what is happening in the game, and a second
// line with the reason or the next thing to do.
func statusOf(get getter) map[string]any {
	turn := gStr(get, "turn")
	st := map[string]any{"text": "", "sub": "", "playing": false, "check": false, "over": false, "review": false, "draw": false}
	result := gStr(get, "result")
	reason := gStr(get, "reason")
	review := gBool(get, "review")
	ply, total := gInt(get, "ply"), gInt(get, "total")
	atEnd := !review || ply == total
	side := capital(turn)
	switch {
	case result != "*" && atEnd:
		st["over"] = true
		st["draw"] = result == "1/2-1/2"
		head := capital(reason)
		if reason == "" {
			head = "Game over"
		}
		st["text"] = head + " · " + resultText(result)
		st["sub"] = outcomeLine(result, reason)
	case review:
		st["review"] = true
		st["text"] = fmt.Sprintf("Review · move %d of %d", ply, total)
		st["sub"] = side + " to move"
		if gBool(get, "check") {
			st["sub"] = "Check · " + side + " to move"
		}
	case gBool(get, "thinking"):
		st["playing"] = true
		st["text"] = side + " is thinking…"
		if gBool(get, "check") {
			st["text"] = "Check · " + side + " is thinking…"
			st["check"] = true
		}
	case gBool(get, "check"):
		st["check"] = true
		st["text"] = "Check · " + side + " to move"
	default:
		st["playing"] = true
		st["text"] = side + " to move"
	}
	if claim := gStr(get, "claim"); claim != "" && result == "*" && !review {
		st["sub"] = "d claims a draw: " + claim
	}
	return st
}

func outcomeLine(result, reason string) string {
	switch result {
	case "1-0":
		switch reason {
		case "resignation":
			return "Black resigned"
		case "time forfeit":
			return "Black ran out of time"
		}
		return "White wins"
	case "0-1":
		switch reason {
		case "resignation":
			return "White resigned"
		case "time forfeit":
			return "White ran out of time"
		}
		return "Black wins"
	}
	return "Draw"
}

// evalbarOf is the evaluation bar: white's share of the bar from 0 to 100,
// and its label.
func evalbarOf(get getter) map[string]any {
	cp := gInt(get, "evalcp")
	pct := 50 + clamp(cp, -600, 600)/12
	label := cpText(cp)
	if cp >= 2000 || cp <= -2000 {
		label = "mate"
	}
	if gBool(get, "over") {
		label = resultText(gStr(get, "result"))
		switch gStr(get, "result") {
		case "1-0":
			pct = 100
		case "0-1":
			pct = 0
		default:
			pct = 50
		}
	}
	return map[string]any{"pct": pct, "label": label}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func engineViewOf(get getter) map[string]any {
	e, _ := get("engine")
	em, _ := e.(map[string]any)
	if gBool(get, "over") {
		return map[string]any{"has": false, "line": "game over · " + resultText(gStr(get, "result")), "pv": ""}
	}
	if has, _ := em["has"].(bool); !has {
		line := "no search yet"
		if gStr(get, "mode") == mode2P {
			line = "idle: H asks for a hint"
		}
		return map[string]any{"has": false, "line": line, "pv": ""}
	}
	depth, _ := em["depth"].(float64)
	return map[string]any{
		"has":  true,
		"line": fmt.Sprintf("depth %d · %s · %s nodes", int(depth), asStr(em["score"]), asStr(em["nodes"])),
		"pv":   "pv " + asStr(em["pv"]),
	}
}

func modeLabel(mode string, level int) string {
	switch mode {
	case modeHvC:
		return fmt.Sprintf("vs computer · level %d", level)
	case modeCvC:
		return fmt.Sprintf("computer vs computer · level %d", level)
	}
	return "two players"
}

var modeNames = map[string]string{modeHvC: "human vs computer", mode2P: "two players", modeCvC: "computer vs computer"}

func ngViewOf(get getter) map[string]any {
	n, _ := get("ng")
	m, _ := n.(map[string]any)
	level := 0
	if f, ok := m["level"].(float64); ok {
		level = int(f)
	}
	depth := "—"
	if level >= 1 && level <= 6 {
		depth = fmt.Sprintf("%d (up to depth %d)", level, levelLimits[level].Depth)
	}
	clock := asStr(m["clock"])
	if clock == "none" {
		clock = "no clock"
	}
	side := capital(asStr(m["side"]))
	return map[string]any{
		"mode":   modeNames[asStr(m["mode"])],
		"side":   side,
		"level":  depth,
		"clock":  clock,
		"hvc":    asStr(m["mode"]) == modeHvC,
		"engine": asStr(m["mode"]) != mode2P,
	}
}

// derive declares the derived values. Each is recomputed by the runtime
// inside every store write, so none is ever stale.
func (a *app) derive() error {
	type fn = func(get getter) (any, error)
	derived := []struct {
		path string
		fn   fn
	}{
		{"cells", func(get getter) (any, error) { return cellsOf(get), nil }},
		{"labels", func(get getter) (any, error) { return labelsOf(get), nil }},
		{"mrows", func(get getter) (any, error) { return mrowsOf(get), nil }},
		{"mcur", func(get getter) (any, error) { return mcurOf(get), nil }},
		{"top", func(get getter) (any, error) {
			if gBool(get, "flip") {
				return playerOf(get, White), nil
			}
			return playerOf(get, Black), nil
		}},
		{"bot", func(get getter) (any, error) {
			if gBool(get, "flip") {
				return playerOf(get, Black), nil
			}
			return playerOf(get, White), nil
		}},
		{"status", func(get getter) (any, error) { return statusOf(get), nil }},
		{"evalbar", func(get getter) (any, error) { return evalbarOf(get), nil }},
		{"engine_view", func(get getter) (any, error) { return engineViewOf(get), nil }},
		{"mode_label", func(get getter) (any, error) {
			return modeLabel(gStr(get, "mode"), gInt(get, "level")), nil
		}},
		{"clocks_line", func(get getter) (any, error) {
			c, _ := get("clock")
			cm, _ := c.(map[string]any)
			if on, _ := cm["on"].(bool); !on {
				return "", nil
			}
			w, _ := cm["w"].(float64)
			b, _ := cm["b"].(float64)
			return "W " + fmtClock(int64(w)) + " B " + fmtClock(int64(b)), nil
		}},
		{"think_label", func(get getter) (any, error) {
			if gBool(get, "thinking") {
				return gStr(get, "spin") + " thinking…", nil
			}
			return "", nil
		}},
		{"ng_view", func(get getter) (any, error) { return ngViewOf(get), nil }},
	}
	for _, d := range derived {
		if err := a.ui.Derive(d.path, d.fn); err != nil {
			return fmt.Errorf("derive %s: %w", d.path, err)
		}
	}
	return nil
}
