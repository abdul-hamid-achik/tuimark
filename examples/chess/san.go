package main

import (
	"errors"
	"fmt"
	"strings"
)

// SAN writes m, a legal move in p, in standard algebraic notation: the
// piece letter, the least disambiguation the legal moves need, x for a
// capture, =Q for a promotion, O-O and O-O-O for castling, and + or # when
// the move gives check or mate.
func SAN(p *Position, m Move) string {
	var legal [256]Move
	moves := p.LegalMoves(legal[:0])
	return sanWith(p, m, moves)
}

func sanWith(p *Position, m Move, legal []Move) string {
	from, to := int(m.From), int(m.To)
	pc := p.B[from]
	var b strings.Builder
	switch {
	case pc.Type() == King && abs(file(to)-file(from)) == 2:
		if file(to) == 6 {
			b.WriteString("O-O")
		} else {
			b.WriteString("O-O-O")
		}
	case pc.Type() == Pawn:
		capture := file(from) != file(to)
		if capture {
			b.WriteByte(byte('a' + file(from)))
			b.WriteByte('x')
		}
		b.WriteString(sqName(to))
		if m.Promo != NoType {
			b.WriteByte('=')
			b.WriteByte(pieceLetters[m.Promo])
		}
	default:
		b.WriteByte(pieceLetters[pc.Type()])
		// Which other pieces of the kind can reach the square?
		sameFile, sameRank, other := false, false, false
		for _, o := range legal {
			if o == m || int(o.To) != to || o.From == m.From || p.B[o.From] != pc {
				continue
			}
			other = true
			if file(int(o.From)) == file(from) {
				sameFile = true
			}
			if rank(int(o.From)) == rank(from) {
				sameRank = true
			}
		}
		if other {
			switch {
			case !sameFile:
				b.WriteByte(byte('a' + file(from)))
			case !sameRank:
				b.WriteByte(byte('1' + rank(from)))
			default:
				b.WriteString(sqName(from))
			}
		}
		if p.B[to] != 0 {
			b.WriteByte('x')
		}
		b.WriteString(sqName(to))
	}
	np := p.Make(m)
	if np.InCheck() {
		if np.HasLegalMove() {
			b.WriteByte('+')
		} else {
			b.WriteByte('#')
		}
	}
	return b.String()
}

// ErrNeedPromotion is returned by ParseMove for a pawn move to the last
// rank that does not say what the pawn becomes. The move it returns has the
// from and to squares set.
var ErrNeedPromotion = errors.New("promotion piece needed")

// ParseMove reads a move typed in SAN or UCI, leniently: check and mate
// marks, x, =, and annotation marks are optional, castling may be written
// with zeros, piece letters may be lower case, a promotion may be e8=Q or
// e8Q, and e2e4 or e7e8q work as UCI. The move must be legal in p.
func ParseMove(p *Position, s string) (Move, error) {
	var buf [256]Move
	legal := p.LegalMoves(buf[:0])
	s = strings.TrimSpace(s)
	if s == "" {
		return Move{}, errors.New("no move")
	}
	trim := strings.TrimRight(s, "+#!?")
	// An exact SAN wins over any lenient reading.
	for _, m := range legal {
		if san := strings.TrimRight(sanWith(p, m, legal), "+#"); san == trim {
			return m, nil
		}
	}
	norm := normalizeMove(trim)
	var found []Move
	needPromo := map[[2]uint8]bool{}
	for _, m := range legal {
		forms := moveForms(p, m)
		for _, f := range forms.exact {
			if f == norm {
				found = append(found, m)
				break
			}
		}
		if m.Promo != NoType {
			for _, f := range forms.bare {
				if f == norm {
					needPromo[[2]uint8{m.From, m.To}] = true
				}
			}
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}
	if len(found) > 1 {
		return Move{}, fmt.Errorf("%q is ambiguous: %s", s, joinSAN(p, found, legal))
	}
	if len(needPromo) == 1 {
		for k := range needPromo {
			return Move{From: k[0], To: k[1]}, ErrNeedPromotion
		}
	}
	if len(needPromo) > 1 {
		return Move{}, fmt.Errorf("%q is ambiguous: say which pawn", s)
	}
	return Move{}, fmt.Errorf("illegal move: %s", s)
}

func joinSAN(p *Position, ms []Move, legal []Move) string {
	var out []string
	for _, m := range ms {
		out = append(out, sanWith(p, m, legal))
	}
	return strings.Join(out, ", ")
}

// normalizeMove lower-cases the text and drops x, =, -, and : (castling
// keeps its dashes, written with the letter o).
func normalizeMove(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, "0", "o"))
	if s == "o-o" || s == "o-o-o" {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case 'x', '=', '-', ':', ' ':
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

type moveFormSet struct {
	exact []string // forms that name the move completely
	bare  []string // promotion forms without the piece letter
}

// moveForms are the normalized spellings that select m.
func moveForms(p *Position, m Move) moveFormSet {
	from, to := int(m.From), int(m.To)
	pc := p.B[from]
	fs, ts := sqName(from), sqName(to)
	var out moveFormSet
	add := func(base string) {
		if m.Promo != NoType {
			out.bare = append(out.bare, base)
			out.exact = append(out.exact, base+strings.ToLower(string(pieceLetters[m.Promo])))
			return
		}
		out.exact = append(out.exact, base)
	}
	// UCI works for every move.
	add(fs + ts)
	switch {
	case pc.Type() == King && abs(file(to)-file(from)) == 2:
		if file(to) == 6 {
			out.exact = append(out.exact, "o-o")
		} else {
			out.exact = append(out.exact, "o-o-o")
		}
	case pc.Type() == Pawn:
		if file(from) == file(to) {
			add(ts)
		} else {
			add(fs[:1] + ts)
		}
	default:
		l := strings.ToLower(string(pieceLetters[pc.Type()]))
		add(l + ts)
		add(l + fs[:1] + ts)
		add(l + fs[1:] + ts)
		add(l + fs + ts)
	}
	return out
}
