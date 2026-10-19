package main

// pseudoMoves appends every move of the side to move that obeys how the
// pieces move, castling's through-check rule included, but may leave the
// own king attacked.
func (p *Position) pseudoMoves(out []Move) []Move {
	us, them := p.Turn, p.Turn.Other()
	for sq := 0; sq < 64; sq++ {
		pc := p.B[sq]
		if pc == 0 || pc.Color() != us {
			continue
		}
		f, r := file(sq), rank(sq)
		switch pc.Type() {
		case Pawn:
			out = p.pawnMoves(out, sq, us)
		case Knight:
			for _, t := range knightTargets[sq] {
				if q := p.B[t]; q == 0 || q.Color() == them {
					out = append(out, Move{From: uint8(sq), To: t})
				}
			}
		case King:
			for _, t := range kingTargets[sq] {
				if q := p.B[t]; q == 0 || q.Color() == them {
					out = append(out, Move{From: uint8(sq), To: t})
				}
			}
			out = p.castlingMoves(out, sq, us)
		case Bishop:
			out = p.slide(out, sq, f, r, us, bishopDirs[:])
		case Rook:
			out = p.slide(out, sq, f, r, us, rookDirs[:])
		case Queen:
			out = p.slide(out, sq, f, r, us, bishopDirs[:])
			out = p.slide(out, sq, f, r, us, rookDirs[:])
		}
	}
	return out
}

func (p *Position) slide(out []Move, sq, f, r int, us Color, dirs [][2]int) []Move {
	for _, d := range dirs {
		for nf, nr := f+d[0], r+d[1]; nf >= 0 && nf < 8 && nr >= 0 && nr < 8; nf, nr = nf+d[0], nr+d[1] {
			t := nr*8 + nf
			q := p.B[t]
			if q == 0 {
				out = append(out, Move{From: uint8(sq), To: uint8(t)})
				continue
			}
			if q.Color() != us {
				out = append(out, Move{From: uint8(sq), To: uint8(t)})
			}
			break
		}
	}
	return out
}

var promoTypes = [4]PieceType{Queen, Rook, Bishop, Knight}

func (p *Position) pawnMoves(out []Move, sq int, us Color) []Move {
	f, r := file(sq), rank(sq)
	dir, startRank, lastRank := 1, 1, 7
	if us == Black {
		dir, startRank, lastRank = -1, 6, 0
	}
	add := func(to int) {
		if rank(to) == lastRank {
			for _, t := range promoTypes {
				out = append(out, Move{From: uint8(sq), To: uint8(to), Promo: t})
			}
			return
		}
		out = append(out, Move{From: uint8(sq), To: uint8(to)})
	}
	nr := r + dir
	if nr < 0 || nr > 7 {
		return out
	}
	one := nr*8 + f
	if p.B[one] == 0 {
		add(one)
		if r == startRank {
			if two := (r+2*dir)*8 + f; p.B[two] == 0 {
				out = append(out, Move{From: uint8(sq), To: uint8(two)})
			}
		}
	}
	for _, df := range [2]int{-1, 1} {
		nf := f + df
		if nf < 0 || nf > 7 {
			continue
		}
		to := nr*8 + nf
		if q := p.B[to]; q != 0 && q.Color() != us {
			add(to)
		} else if q == 0 && to == int(p.EP) {
			// The captured pawn stands beside the capturing one.
			if v := p.B[r*8+nf]; v == mk(us.Other(), Pawn) {
				out = append(out, Move{From: uint8(sq), To: uint8(to)})
			}
		}
	}
	return out
}

// castlingMoves adds the king's two-square moves: the right is still held,
// the squares between king and rook are empty, and the king is not in check
// and does not cross an attacked square (it must not land on one either,
// which the legality filter checks like any other move).
func (p *Position) castlingMoves(out []Move, sq int, us Color) []Move {
	home := 4
	k, q := castleWK, castleWQ
	if us == Black {
		home, k, q = 60, castleBK, castleBQ
	}
	if sq != home || p.Castle&(k|q) == 0 {
		return out
	}
	them := us.Other()
	rook := mk(us, Rook)
	if p.Castle&k != 0 && p.B[home+3] == rook && p.B[home+1] == 0 && p.B[home+2] == 0 {
		if !p.Attacked(home, them) && !p.Attacked(home+1, them) {
			out = append(out, Move{From: uint8(home), To: uint8(home + 2)})
		}
	}
	if p.Castle&q != 0 && p.B[home-4] == rook && p.B[home-1] == 0 && p.B[home-2] == 0 && p.B[home-3] == 0 {
		if !p.Attacked(home, them) && !p.Attacked(home-1, them) {
			out = append(out, Move{From: uint8(home), To: uint8(home - 2)})
		}
	}
	return out
}

// LegalMoves appends the legal moves of the side to move to out.
func (p *Position) LegalMoves(out []Move) []Move {
	var buf [256]Move
	pseudo := p.pseudoMoves(buf[:0])
	us := p.Turn
	them := us.Other()
	for _, m := range pseudo {
		np := p.Make(m)
		if !np.Attacked(int(np.King[us]), them) {
			out = append(out, m)
		}
	}
	return out
}

// HasLegalMove reports whether the side to move has any legal move.
func (p *Position) HasLegalMove() bool {
	var buf [256]Move
	pseudo := p.pseudoMoves(buf[:0])
	us := p.Turn
	for _, m := range pseudo {
		np := p.Make(m)
		if !np.Attacked(int(np.King[us]), us.Other()) {
			return true
		}
	}
	return false
}

// IsLegal reports whether m is a legal move here.
func (p *Position) IsLegal(m Move) bool {
	var buf [256]Move
	for _, l := range p.LegalMoves(buf[:0]) {
		if l == m {
			return true
		}
	}
	return false
}

// Perft counts the leaf nodes of the legal move tree to the given depth.
func (p *Position) Perft(depth int) uint64 {
	if depth == 0 {
		return 1
	}
	var buf [256]Move
	moves := p.LegalMoves(buf[:0])
	if depth == 1 {
		return uint64(len(moves))
	}
	var n uint64
	for _, m := range moves {
		np := p.Make(m)
		n += np.Perft(depth - 1)
	}
	return n
}

// Divide is Perft split by the first move: the node count under each, for
// finding where a generator and a reference part ways.
func (p *Position) Divide(depth int) map[string]uint64 {
	out := map[string]uint64{}
	var buf [256]Move
	for _, m := range p.LegalMoves(buf[:0]) {
		np := p.Make(m)
		out[m.UCI()] = np.Perft(depth - 1)
	}
	return out
}
