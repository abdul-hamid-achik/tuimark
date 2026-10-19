package main

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseFEN reads a position in Forsyth-Edwards notation. The last two
// fields (the clocks) may be left out. A FEN that cannot be a position of
// a game, such as one without a king or with the side that just moved in
// check, is an error. Castling rights the pieces cannot use are dropped.
func ParseFEN(s string) (Position, error) {
	var p Position
	p.EP = -1
	p.King = [2]int8{-1, -1}
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return p, fmt.Errorf("FEN needs at least a piece placement and a side to move")
	}
	if len(fields) > 6 {
		return p, fmt.Errorf("FEN has %d fields, want at most 6", len(fields))
	}
	rows := strings.Split(fields[0], "/")
	if len(rows) != 8 {
		return p, fmt.Errorf("FEN placement has %d ranks, want 8", len(rows))
	}
	var kings [2]int
	for i, row := range rows {
		r := 7 - i
		f := 0
		for j := 0; j < len(row); j++ {
			c := row[j]
			if c >= '1' && c <= '8' {
				f += int(c - '0')
				continue
			}
			pc, ok := pieceFromLetter(c)
			if !ok {
				return p, fmt.Errorf("FEN has the unknown piece %q", string(c))
			}
			if f > 7 {
				return p, fmt.Errorf("FEN rank %d has more than 8 files", r+1)
			}
			if pc.Type() == Pawn && (r == 0 || r == 7) {
				return p, fmt.Errorf("FEN has a pawn on rank %d", r+1)
			}
			p.B[r*8+f] = pc
			if pc.Type() == King {
				kings[pc.Color()]++
				p.King[pc.Color()] = int8(r*8 + f)
			}
			f++
		}
		if f != 8 {
			return p, fmt.Errorf("FEN rank %d has %d files, want 8", r+1, f)
		}
	}
	if kings[White] != 1 || kings[Black] != 1 {
		return p, fmt.Errorf("FEN needs exactly one king per side")
	}
	switch fields[1] {
	case "w":
		p.Turn = White
	case "b":
		p.Turn = Black
	default:
		return p, fmt.Errorf("FEN side to move is %q, want w or b", fields[1])
	}
	if len(fields) > 2 && fields[2] != "-" {
		for _, c := range fields[2] {
			switch c {
			case 'K':
				p.Castle |= castleWK
			case 'Q':
				p.Castle |= castleWQ
			case 'k':
				p.Castle |= castleBK
			case 'q':
				p.Castle |= castleBQ
			default:
				return p, fmt.Errorf("FEN castling field has %q", string(c))
			}
		}
	}
	// A right needs its king and rook at home.
	if p.B[4] != mk(White, King) {
		p.Castle &^= castleWK | castleWQ
	}
	if p.B[60] != mk(Black, King) {
		p.Castle &^= castleBK | castleBQ
	}
	if p.B[7] != mk(White, Rook) {
		p.Castle &^= castleWK
	}
	if p.B[0] != mk(White, Rook) {
		p.Castle &^= castleWQ
	}
	if p.B[63] != mk(Black, Rook) {
		p.Castle &^= castleBK
	}
	if p.B[56] != mk(Black, Rook) {
		p.Castle &^= castleBQ
	}
	if len(fields) > 3 && fields[3] != "-" {
		sq, ok := parseSquare(fields[3])
		if !ok {
			return p, fmt.Errorf("FEN en passant square is %q", fields[3])
		}
		wantRank := 5
		if p.Turn == Black {
			wantRank = 2
		}
		if rank(sq) != wantRank {
			return p, fmt.Errorf("FEN en passant square %s is on the wrong rank", fields[3])
		}
		p.EP = int8(sq)
	}
	p.Full = 1
	if len(fields) > 4 {
		n, err := strconv.Atoi(fields[4])
		if err != nil || n < 0 || n > 1000 {
			return p, fmt.Errorf("FEN halfmove clock is %q", fields[4])
		}
		p.Half = n
	}
	if len(fields) > 5 {
		n, err := strconv.Atoi(fields[5])
		if err != nil || n < 1 || n > 10000 {
			return p, fmt.Errorf("FEN fullmove number is %q", fields[5])
		}
		p.Full = n
	}
	// The side that just moved cannot be in check.
	other := p.Turn.Other()
	if p.Attacked(int(p.King[other]), p.Turn) {
		return p, fmt.Errorf("FEN has the %s king in check with %s to move", other, p.Turn)
	}
	return p, nil
}

// FEN writes the position in Forsyth-Edwards notation.
func (p *Position) FEN() string {
	var b strings.Builder
	for r := 7; r >= 0; r-- {
		empty := 0
		for f := 0; f < 8; f++ {
			pc := p.B[r*8+f]
			if pc == 0 {
				empty++
				continue
			}
			if empty > 0 {
				b.WriteByte(byte('0' + empty))
				empty = 0
			}
			b.WriteString(pc.Letter())
		}
		if empty > 0 {
			b.WriteByte(byte('0' + empty))
		}
		if r > 0 {
			b.WriteByte('/')
		}
	}
	b.WriteByte(' ')
	if p.Turn == White {
		b.WriteByte('w')
	} else {
		b.WriteByte('b')
	}
	b.WriteByte(' ')
	if p.Castle == 0 {
		b.WriteByte('-')
	} else {
		for _, c := range []struct {
			bit uint8
			ch  byte
		}{{castleWK, 'K'}, {castleWQ, 'Q'}, {castleBK, 'k'}, {castleBQ, 'q'}} {
			if p.Castle&c.bit != 0 {
				b.WriteByte(c.ch)
			}
		}
	}
	b.WriteByte(' ')
	if p.EP >= 0 {
		b.WriteString(sqName(int(p.EP)))
	} else {
		b.WriteByte('-')
	}
	fmt.Fprintf(&b, " %d %d", p.Half, p.Full)
	return b.String()
}
