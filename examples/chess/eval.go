package main

// The evaluation is material plus piece-square tables (the "simplified
// evaluation function" tables), in centipawns, with a bonus for the bishop
// pair and a king table that changes in the endgame. All integer, so every
// platform agrees to the digit.

var pieceValue = [7]int{0, 100, 320, 330, 500, 900, 0}

// Tables are written as the board looks to white: the first row is rank 8.
var pst = [7][64]int{
	Pawn: {
		0, 0, 0, 0, 0, 0, 0, 0,
		50, 50, 50, 50, 50, 50, 50, 50,
		10, 10, 20, 30, 30, 20, 10, 10,
		5, 5, 10, 25, 25, 10, 5, 5,
		0, 0, 0, 20, 20, 0, 0, 0,
		5, -5, -10, 0, 0, -10, -5, 5,
		5, 10, 10, -20, -20, 10, 10, 5,
		0, 0, 0, 0, 0, 0, 0, 0,
	},
	Knight: {
		-50, -40, -30, -30, -30, -30, -40, -50,
		-40, -20, 0, 0, 0, 0, -20, -40,
		-30, 0, 10, 15, 15, 10, 0, -30,
		-30, 5, 15, 20, 20, 15, 5, -30,
		-30, 0, 15, 20, 20, 15, 0, -30,
		-30, 5, 10, 15, 15, 10, 5, -30,
		-40, -20, 0, 5, 5, 0, -20, -40,
		-50, -40, -30, -30, -30, -30, -40, -50,
	},
	Bishop: {
		-20, -10, -10, -10, -10, -10, -10, -20,
		-10, 0, 0, 0, 0, 0, 0, -10,
		-10, 0, 5, 10, 10, 5, 0, -10,
		-10, 5, 5, 10, 10, 5, 5, -10,
		-10, 0, 10, 10, 10, 10, 0, -10,
		-10, 10, 10, 10, 10, 10, 10, -10,
		-10, 5, 0, 0, 0, 0, 5, -10,
		-20, -10, -10, -10, -10, -10, -10, -20,
	},
	Rook: {
		0, 0, 0, 0, 0, 0, 0, 0,
		5, 10, 10, 10, 10, 10, 10, 5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		0, 0, 0, 5, 5, 0, 0, 0,
	},
	Queen: {
		-20, -10, -10, -5, -5, -10, -10, -20,
		-10, 0, 0, 0, 0, 0, 0, -10,
		-10, 0, 5, 5, 5, 5, 0, -10,
		-5, 0, 5, 5, 5, 5, 0, -5,
		0, 0, 5, 5, 5, 5, 0, -5,
		-10, 5, 5, 5, 5, 5, 0, -10,
		-10, 0, 5, 0, 0, 0, 0, -10,
		-20, -10, -10, -5, -5, -10, -10, -20,
	},
	King: {
		-30, -40, -40, -50, -50, -40, -40, -30,
		-30, -40, -40, -50, -50, -40, -40, -30,
		-30, -40, -40, -50, -50, -40, -40, -30,
		-30, -40, -40, -50, -50, -40, -40, -30,
		-20, -30, -30, -40, -40, -30, -30, -20,
		-10, -20, -20, -20, -20, -20, -20, -10,
		20, 20, 0, 0, 0, 0, 20, 20,
		20, 30, 10, 0, 0, 10, 30, 20,
	},
}

var kingEnd = [64]int{
	-50, -40, -30, -20, -20, -30, -40, -50,
	-30, -20, -10, 0, 0, -10, -20, -30,
	-30, -10, 20, 30, 30, 20, -10, -30,
	-30, -10, 30, 40, 40, 30, -10, -30,
	-30, -10, 30, 40, 40, 30, -10, -30,
	-30, -10, 20, 30, 30, 20, -10, -30,
	-30, -30, 0, 0, 0, 0, -30, -30,
	-50, -30, -30, -30, -30, -30, -30, -50,
}

// tableIndex maps a square to the table cell for the color: white reads the
// table as drawn, black reads it upside down.
func tableIndex(sq int, c Color) int {
	if c == White {
		return (7-rank(sq))*8 + file(sq)
	}
	return rank(sq)*8 + file(sq)
}

// Evaluate scores the position in centipawns for the side to move.
func Evaluate(p *Position) int {
	var score [2]int
	var bishops [2]int
	material := 0
	for _, pc := range p.B {
		if pc != 0 && pc.Type() != Pawn && pc.Type() != King {
			material += pieceValue[pc.Type()]
		}
	}
	endgame := material <= 2400
	for sq, pc := range p.B {
		if pc == 0 {
			continue
		}
		c, t := pc.Color(), pc.Type()
		v := pieceValue[t]
		i := tableIndex(sq, c)
		if t == King && endgame {
			v += kingEnd[i]
		} else {
			v += pst[t][i]
		}
		score[c] += v
		if t == Bishop {
			bishops[c]++
		}
	}
	for c := 0; c < 2; c++ {
		if bishops[c] >= 2 {
			score[c] += 30
		}
	}
	s := score[White] - score[Black]
	if p.Turn == Black {
		s = -s
	}
	return s
}

// evalNodes caps the quiescence search behind the evaluation bar.
const evalNodes = 4000

// EvalWhite is the static score for white, after settling the captures that
// are on the board (so a hanging piece does not skew the evaluation bar),
// clamped to ±2000. A position with no legal move scores as it ends.
func EvalWhite(p *Position) int {
	if !p.HasLegalMove() {
		if p.InCheck() {
			if p.Turn == White {
				return -2000
			}
			return 2000
		}
		return 0
	}
	// The evaluation runs on the loop under the lock on every store write, so
	// its quiescence search gets a node cap; a position that blows it is
	// scored statically.
	s := newSearcher(evalNodes, nil)
	v := s.quiesce(*p, -mateScore, mateScore, 0)
	if s.aborted {
		v = Evaluate(p)
	}
	if p.Turn == Black {
		v = -v
	}
	if v > 2000 {
		v = 2000
	}
	if v < -2000 {
		v = -2000
	}
	return v
}
