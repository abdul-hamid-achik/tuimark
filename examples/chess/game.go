package main

import (
	"errors"
	"fmt"
)

// Zobrist keys for position identity (repetition). The generator is
// splitmix64 from a fixed seed, so a key is the same on every run.
var zob struct {
	piece  [16][64]uint64
	castle [16]uint64
	ep     [8]uint64
	black  uint64
}

func init() {
	s := uint64(0x6368657373202620) // "chess & "
	next := func() uint64 {
		s += 0x9e3779b97f4a7c15
		z := s
		z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
		z = (z ^ (z >> 27)) * 0x94d049bb133111eb
		return z ^ (z >> 31)
	}
	for i := range zob.piece {
		for j := range zob.piece[i] {
			zob.piece[i][j] = next()
		}
	}
	for i := range zob.castle {
		zob.castle[i] = next()
	}
	for i := range zob.ep {
		zob.ep[i] = next()
	}
	zob.black = next()
}

// Key identifies the position as the repetition rule sees it: the pieces,
// the side to move, the castling rights, and an en passant square only when
// a legal en passant capture exists.
func (p *Position) Key() uint64 {
	var h uint64
	for sq, pc := range p.B {
		if pc != 0 {
			h ^= zob.piece[pc][sq]
		}
	}
	h ^= zob.castle[p.Castle&15]
	if p.Turn == Black {
		h ^= zob.black
	}
	if p.EP >= 0 && p.hasLegalEP() {
		h ^= zob.ep[file(int(p.EP))]
	}
	return h
}

// hasLegalEP reports whether the side to move can capture en passant.
func (p *Position) hasLegalEP() bool {
	ep := int(p.EP)
	r := rank(ep) - 1
	if p.Turn == Black {
		r = rank(ep) + 1
	}
	if r < 0 || r > 7 {
		return false
	}
	for _, df := range [2]int{-1, 1} {
		f := file(ep) + df
		if f < 0 || f > 7 {
			continue
		}
		from := r*8 + f
		if p.B[from] != mk(p.Turn, Pawn) {
			continue
		}
		np := p.Make(Move{From: uint8(from), To: uint8(ep)})
		if !np.Attacked(int(np.King[p.Turn]), np.Turn) {
			return true
		}
	}
	return false
}

// InsufficientMaterial reports a position in which no sequence of legal
// moves can end in checkmate by force of the pieces left: bare kings, a king
// and one minor piece against a bare king, or only kings and bishops that
// all stand on squares of one color.
func (p *Position) InsufficientMaterial() bool {
	minors := 0
	bishopColor := -1
	mixed := false
	knights := 0
	for sq, pc := range p.B {
		switch pc.Type() {
		case NoType, King:
		case Knight:
			minors++
			knights++
		case Bishop:
			minors++
			c := (file(sq) + rank(sq)) & 1
			if bishopColor >= 0 && bishopColor != c {
				mixed = true
			}
			bishopColor = c
		default:
			return false
		}
	}
	switch {
	case minors == 0:
		return true
	case minors == 1:
		return true
	case knights == 0 && !mixed:
		return true
	}
	return false
}

// Game is a game of chess: the position it started from, the moves played
// with their notation, the moves taken back (for redo), and the result.
type Game struct {
	Start Position
	Moves []Move
	SAN   []string
	Caps  []Piece    // the piece each move captured, 0 for none
	Pos   []Position // Pos[0] is Start, Pos[i] the position after i moves
	Keys  []uint64
	Stamp [][2]int64 // free per-position numbers (the clocks), same length as Pos
	Redo  []redoMove

	Result string // "*" while the game goes on, else 1-0, 0-1, or 1/2-1/2
	Reason string
	Tags   map[string]string
}

type redoMove struct {
	Move  Move
	Stamp [2]int64
}

// NewGame starts a game from a position.
func NewGame(start Position) *Game {
	g := &Game{Start: start, Tags: map[string]string{}}
	g.Pos = []Position{start}
	g.Keys = []uint64{start.Key()}
	g.Stamp = [][2]int64{{}}
	g.refresh()
	return g
}

// Cur is the position after the last move.
func (g *Game) Cur() *Position { return &g.Pos[len(g.Pos)-1] }

// Ply is the number of moves played.
func (g *Game) Ply() int { return len(g.Moves) }

// Over reports whether the game has a result.
func (g *Game) Over() bool { return g.Result != "*" }

// Push plays a legal move, which clears the redo stack.
func (g *Game) Push(m Move, stamp [2]int64) error {
	p := g.Cur()
	if !p.IsLegal(m) {
		return fmt.Errorf("illegal move %s", m.UCI())
	}
	g.push(m, stamp)
	g.Redo = nil
	return nil
}

func (g *Game) push(m Move, stamp [2]int64) {
	p := g.Cur()
	cap := p.B[m.To]
	if cap == 0 && p.B[m.From].Type() == Pawn && file(int(m.From)) != file(int(m.To)) {
		cap = mk(p.Turn.Other(), Pawn) // en passant
	}
	g.SAN = append(g.SAN, SAN(p, m))
	g.Caps = append(g.Caps, cap)
	np := p.Make(m)
	g.Moves = append(g.Moves, m)
	g.Pos = append(g.Pos, np)
	g.Keys = append(g.Keys, np.Key())
	g.Stamp = append(g.Stamp, stamp)
	g.refresh()
}

// Undo takes the last move back, onto the redo stack.
func (g *Game) Undo() bool {
	n := len(g.Moves)
	if n == 0 {
		return false
	}
	g.Redo = append(g.Redo, redoMove{Move: g.Moves[n-1], Stamp: g.Stamp[n]})
	g.Moves = g.Moves[:n-1]
	g.SAN = g.SAN[:n-1]
	g.Caps = g.Caps[:n-1]
	g.Pos = g.Pos[:n]
	g.Keys = g.Keys[:n]
	g.Stamp = g.Stamp[:n]
	g.refresh()
	return true
}

// Redoable reports whether a taken-back move waits on the redo stack.
func (g *Game) Redoable() bool { return len(g.Redo) > 0 }

// Replay plays the last taken-back move again.
func (g *Game) Replay() bool {
	n := len(g.Redo)
	if n == 0 {
		return false
	}
	r := g.Redo[n-1]
	g.Redo = g.Redo[:n-1]
	g.push(r.Move, r.Stamp)
	return true
}

// Repetitions counts how often the current position has occurred.
func (g *Game) Repetitions() int {
	cur := g.Keys[len(g.Keys)-1]
	n := 0
	for _, k := range g.Keys {
		if k == cur {
			n++
		}
	}
	return n
}

// refresh sets the result from the position: the outcomes no one can
// decline. It drops a result set by resignation or the clock, which an undo
// takes back with the move.
func (g *Game) refresh() {
	p := g.Cur()
	g.Result, g.Reason = "*", ""
	switch {
	case !p.HasLegalMove():
		if p.InCheck() {
			g.Reason = "checkmate"
			if p.Turn == White {
				g.Result = "0-1"
			} else {
				g.Result = "1-0"
			}
		} else {
			g.Result, g.Reason = "1/2-1/2", "stalemate"
		}
	case p.InsufficientMaterial():
		g.Result, g.Reason = "1/2-1/2", "insufficient material"
	case p.Half >= 150:
		g.Result, g.Reason = "1/2-1/2", "seventy-five-move rule"
	case g.Repetitions() >= 5:
		g.Result, g.Reason = "1/2-1/2", "fivefold repetition"
	}
}

// ClaimableDraw names the draw a player may claim now, or "".
func (g *Game) ClaimableDraw() string {
	if g.Over() {
		return ""
	}
	p := g.Cur()
	switch {
	case g.Repetitions() >= 3:
		return "threefold repetition"
	case p.Half >= 100:
		return "fifty-move rule"
	}
	return ""
}

var errNoClaim = errors.New("no draw to claim: neither the fifty-move rule nor a threefold repetition applies")

// ClaimDraw ends the game in a draw when one is claimable.
func (g *Game) ClaimDraw() error {
	why := g.ClaimableDraw()
	if why == "" {
		return errNoClaim
	}
	g.Result, g.Reason = "1/2-1/2", why
	return nil
}

// Resign ends the game with a win for the other side.
func (g *Game) Resign(loser Color) {
	if loser == White {
		g.Result = "0-1"
	} else {
		g.Result = "1-0"
	}
	g.Reason = "resignation"
}

// Flag ends the game on time: loser's clock ran out. The other side wins
// unless it cannot possibly checkmate (FIDE 6.9), which is decided
// conservatively: it has only a king, or a lone knight or bishop against a
// bare king.
func (g *Game) Flag(loser Color) {
	if g.Cur().cannotWinOnTime(loser.Other()) {
		g.Result, g.Reason = "1/2-1/2", "timeout vs insufficient material"
		return
	}
	if loser == White {
		g.Result = "0-1"
	} else {
		g.Result = "1-0"
	}
	g.Reason = "time forfeit"
}

// cannotWinOnTime reports whether side has too little to be given the win
// when the other side's flag falls: a bare king, or a single knight or
// bishop (and nothing else) while the other side has only a king.
func (p *Position) cannotWinOnTime(side Color) bool {
	minors, others, foeOthers := 0, 0, 0
	for _, pc := range p.B {
		if pc == 0 || pc.Type() == King {
			continue
		}
		if pc.Color() != side {
			foeOthers++
			continue
		}
		switch pc.Type() {
		case Knight, Bishop:
			minors++
		default:
			others++
		}
	}
	if minors+others == 0 {
		return true
	}
	return others == 0 && minors == 1 && foeOthers == 0
}

// Truncate drops every move after ply and the redo stack.
func (g *Game) Truncate(ply int) {
	if ply < 0 || ply >= len(g.Moves) {
		return
	}
	g.Moves = g.Moves[:ply]
	g.SAN = g.SAN[:ply]
	g.Caps = g.Caps[:ply]
	g.Pos = g.Pos[:ply+1]
	g.Keys = g.Keys[:ply+1]
	g.Stamp = g.Stamp[:ply+1]
	g.Redo = nil
	g.refresh()
}
