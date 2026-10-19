package main

import (
	"fmt"
	"strings"
)

// The chess engine: board.go holds the position and the rules that move
// pieces, movegen.go the legal moves, san.go the move notation, fen.go and
// pgn.go the text formats, game.go a game with its history, and eval.go and
// search.go the computer player. None of it knows about the view.

// Color is the side of a piece or of the player to move.
type Color uint8

const (
	White Color = iota
	Black
)

func (c Color) Other() Color { return c ^ 1 }

func (c Color) String() string {
	if c == White {
		return "white"
	}
	return "black"
}

// PieceType is what a piece is, without its color.
type PieceType uint8

const (
	NoType PieceType = iota
	Pawn
	Knight
	Bishop
	Rook
	Queen
	King
)

// Piece is a type and a color in one byte: 0 is an empty square, the low
// three bits are the type, bit 3 is the color.
type Piece uint8

func mk(c Color, t PieceType) Piece { return Piece(t) | Piece(c)<<3 }

func (p Piece) Type() PieceType { return PieceType(p & 7) }
func (p Piece) Color() Color    { return Color(p >> 3) }

// pieceLetters are the FEN letters by type: upper case white, lower case
// black.
const pieceLetters = " PNBRQK"

// Letter is the FEN letter of the piece, or "" for an empty square.
func (p Piece) Letter() string {
	if p == 0 {
		return ""
	}
	l := pieceLetters[p.Type()]
	if p.Color() == Black {
		l += 'a' - 'A'
	}
	return string(rune(l))
}

// Glyph is the figurine of the piece. Both sides use the filled set, which
// reads best on a coloured square; the view tells the sides apart by colour.
func (p Piece) Glyph() string {
	if p == 0 {
		return ""
	}
	return string([]rune("♟♞♝♜♛♚")[p.Type()-1])
}

func pieceFromLetter(r byte) (Piece, bool) {
	c := White
	if r >= 'a' && r <= 'z' {
		c = Black
		r -= 'a' - 'A'
	}
	i := strings.IndexByte(pieceLetters, r)
	if i < 1 {
		return 0, false
	}
	return mk(c, PieceType(i)), true
}

// Squares are numbered a1 = 0, b1 = 1 … h8 = 63: file = sq&7, rank = sq>>3.
func file(sq int) int { return sq & 7 }
func rank(sq int) int { return sq >> 3 }

func sqName(sq int) string {
	if sq < 0 || sq > 63 {
		return "-"
	}
	return string([]byte{byte('a' + file(sq)), byte('1' + rank(sq))})
}

func parseSquare(s string) (int, bool) {
	if len(s) != 2 || s[0] < 'a' || s[0] > 'h' || s[1] < '1' || s[1] > '8' {
		return 0, false
	}
	return int(s[1]-'1')*8 + int(s[0]-'a'), true
}

// Castling rights bits.
const (
	castleWK uint8 = 1 << iota
	castleWQ
	castleBK
	castleBQ
)

// castleMask clears the rights a move from or to the square takes away.
var castleMask [64]uint8

func init() {
	for i := range castleMask {
		castleMask[i] = 15
	}
	castleMask[4] &^= castleWK | castleWQ
	castleMask[7] &^= castleWK
	castleMask[0] &^= castleWQ
	castleMask[60] &^= castleBK | castleBQ
	castleMask[63] &^= castleBK
	castleMask[56] &^= castleBQ
}

// Move is a from square, a to square, and the piece a pawn becomes. Castling
// is the king's two-square move and an en passant capture is the pawn's
// diagonal move to the empty en passant square: Make works both out from the
// position.
type Move struct {
	From, To uint8
	Promo    PieceType
}

// UCI is the long algebraic form: e2e4, e7e8q.
func (m Move) UCI() string {
	s := sqName(int(m.From)) + sqName(int(m.To))
	if m.Promo != NoType {
		s += strings.ToLower(string(pieceLetters[m.Promo]))
	}
	return s
}

func (m Move) String() string { return m.UCI() }

// Position is a board with the rest of what a FEN holds.
type Position struct {
	B      [64]Piece
	Turn   Color
	Castle uint8
	EP     int8 // en passant target square, or -1
	Half   int  // halfmove clock
	Full   int  // fullmove number
	King   [2]int8
}

// StartPosition is the standard starting position.
func StartPosition() Position {
	p, err := ParseFEN(startFEN)
	if err != nil {
		panic(err)
	}
	return p
}

const startFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

var (
	knightTargets [64][]uint8
	kingTargets   [64][]uint8
)

var knightDeltas = [8][2]int{{1, 2}, {2, 1}, {2, -1}, {1, -2}, {-1, -2}, {-2, -1}, {-2, 1}, {-1, 2}}
var kingDeltas = [8][2]int{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}
var rookDirs = [4][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}}
var bishopDirs = [4][2]int{{1, 1}, {-1, 1}, {-1, -1}, {1, -1}}

func init() {
	for sq := 0; sq < 64; sq++ {
		f, r := file(sq), rank(sq)
		for _, d := range knightDeltas {
			nf, nr := f+d[0], r+d[1]
			if nf >= 0 && nf < 8 && nr >= 0 && nr < 8 {
				knightTargets[sq] = append(knightTargets[sq], uint8(nr*8+nf))
			}
		}
		for _, d := range kingDeltas {
			nf, nr := f+d[0], r+d[1]
			if nf >= 0 && nf < 8 && nr >= 0 && nr < 8 {
				kingTargets[sq] = append(kingTargets[sq], uint8(nr*8+nf))
			}
		}
	}
}

// Attacked reports whether any piece of color by attacks the square.
func (p *Position) Attacked(sq int, by Color) bool {
	f, r := file(sq), rank(sq)
	// Pawns attack diagonally forward, so the attacker stands one rank
	// behind the square, from the attacker's point of view.
	pr := r - 1
	if by == Black {
		pr = r + 1
	}
	if pr >= 0 && pr < 8 {
		pawn := mk(by, Pawn)
		if f > 0 && p.B[pr*8+f-1] == pawn {
			return true
		}
		if f < 7 && p.B[pr*8+f+1] == pawn {
			return true
		}
	}
	knight := mk(by, Knight)
	for _, t := range knightTargets[sq] {
		if p.B[t] == knight {
			return true
		}
	}
	king := mk(by, King)
	for _, t := range kingTargets[sq] {
		if p.B[t] == king {
			return true
		}
	}
	rook, bishop, queen := mk(by, Rook), mk(by, Bishop), mk(by, Queen)
	for _, d := range rookDirs {
		for nf, nr := f+d[0], r+d[1]; nf >= 0 && nf < 8 && nr >= 0 && nr < 8; nf, nr = nf+d[0], nr+d[1] {
			q := p.B[nr*8+nf]
			if q == 0 {
				continue
			}
			if q == rook || q == queen {
				return true
			}
			break
		}
	}
	for _, d := range bishopDirs {
		for nf, nr := f+d[0], r+d[1]; nf >= 0 && nf < 8 && nr >= 0 && nr < 8; nf, nr = nf+d[0], nr+d[1] {
			q := p.B[nr*8+nf]
			if q == 0 {
				continue
			}
			if q == bishop || q == queen {
				return true
			}
			break
		}
	}
	return false
}

// InCheck reports whether the side to move has its king attacked.
func (p *Position) InCheck() bool {
	return p.Attacked(int(p.King[p.Turn]), p.Turn.Other())
}

// Make plays m, which must be pseudo-legal, and returns the new position.
func (p Position) Make(m Move) Position {
	from, to := int(m.From), int(m.To)
	piece := p.B[from]
	captured := p.B[to]
	us := p.Turn
	ep := int(p.EP)
	p.EP = -1

	p.B[from] = 0
	switch {
	case piece.Type() == Pawn && to == ep && file(from) != file(to) && captured == 0:
		// En passant: the captured pawn stands behind the target square.
		cap := to - 8
		if us == Black {
			cap = to + 8
		}
		p.B[cap] = 0
		captured = 1 // not empty: resets the clock
	case piece.Type() == King && abs(file(to)-file(from)) == 2:
		// Castling: the rook jumps over the king.
		rank0 := rank(from) * 8
		if file(to) == 6 {
			p.B[rank0+5], p.B[rank0+7] = p.B[rank0+7], 0
		} else {
			p.B[rank0+3], p.B[rank0] = p.B[rank0], 0
		}
	case piece.Type() == Pawn && abs(rank(to)-rank(from)) == 2:
		p.EP = int8((from + to) / 2)
	}
	if m.Promo != NoType {
		piece = mk(us, m.Promo)
	}
	p.B[to] = piece
	if piece.Type() == King {
		p.King[us] = int8(to)
	}
	p.Castle &= castleMask[from] & castleMask[to]
	if piece.Type() == Pawn || m.Promo != NoType || captured != 0 {
		p.Half = 0
	} else {
		p.Half++
	}
	if us == Black {
		p.Full++
	}
	p.Turn = us.Other()
	return p
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// String draws the board for a test failure message.
func (p *Position) String() string {
	var b strings.Builder
	for r := 7; r >= 0; r-- {
		for f := 0; f < 8; f++ {
			l := p.B[r*8+f].Letter()
			if l == "" {
				l = "."
			}
			b.WriteString(l)
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "%s %s", p.Turn, p.FEN())
	return b.String()
}
