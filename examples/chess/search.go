package main

import (
	"sync/atomic"
)

const (
	mateScore = 30000
	infScore  = 32000
	maxPly    = 64
)

// Limits bound a search: Depth is the deepest iteration, Nodes (0 for none)
// the node budget after which the iteration in progress is dropped and the
// previous one answers. Both are counts, not times, so the same position and
// limits always give the same move.
type Limits struct {
	Depth int
	Nodes int64
}

// SearchResult is what a search found. Score is in centipawns for the side
// to move; a score within a hundred of mateScore is a forced mate.
type SearchResult struct {
	Move  Move
	Score int
	Depth int
	Nodes int64
	PV    []Move
	Found bool // false when the position has no legal move or the search was stopped before depth 1 finished
}

// levelLimits are the computer's levels 1 to 6.
var levelLimits = [7]Limits{
	{},
	{Depth: 1, Nodes: 50_000},
	{Depth: 2, Nodes: 150_000},
	{Depth: 3, Nodes: 400_000},
	{Depth: 4, Nodes: 250_000},
	{Depth: 6, Nodes: 1_000_000},
	{Depth: 8, Nodes: 3_000_000},
}

// hintLimits is the budget of a hint.
var hintLimits = Limits{Depth: 4, Nodes: 150_000}

type searcher struct {
	nodes    int64
	limit    int64
	stop     *atomic.Bool
	stopped  bool // the stop flag ended it, as opposed to the node budget
	aborted  bool
	iterDeep int
	path     []uint64
	killers  [maxPly + 2][2]Move
	history  [2][64][64]int32
	pv       [maxPly + 2][maxPly + 2]Move
	pvLen    [maxPly + 2]int
	rootBest Move
}

func newSearcher(limit int64, stop *atomic.Bool) *searcher {
	return &searcher{limit: limit, stop: stop, iterDeep: 1}
}

// Search looks for the best move for the side to move with negamax and
// alpha-beta pruning inside iterative deepening, a quiescence search at the
// leaves, MVV-LVA, killer, and history move ordering, and a check extension.
// keys are the position keys of the game so far, the current position's last:
// a position that came up before counts as a draw. stop, when set, ends the
// search early.
func Search(p Position, keys []uint64, lim Limits, stop *atomic.Bool) SearchResult {
	s := newSearcher(lim.Nodes, stop)
	s.path = append([]uint64(nil), keys...)
	var buf [256]Move
	root := p.LegalMoves(buf[:0])
	if len(root) == 0 {
		return SearchResult{}
	}
	res := SearchResult{Move: root[0], Found: false}
	if lim.Depth < 1 {
		lim.Depth = 1
	}
	if lim.Depth > maxPly-4 {
		lim.Depth = maxPly - 4
	}
	for d := 1; d <= lim.Depth; d++ {
		s.iterDeep = d
		score := s.negamax(p, d, 0, -infScore, infScore)
		if s.aborted {
			if d == 1 && !s.stopped {
				// Even the first iteration ran out of nodes (a crafted
				// position): play the move the ordering likes best.
				var scores [256]int
				s.order(&p, root, scores[:len(root)], 0)
				pickBest(root, scores[:len(root)], 0)
				res.Move, res.Found, res.Depth = root[0], true, 0
				res.PV = []Move{root[0]}
			}
			break
		}
		res.Found = true
		res.Depth = d
		res.Score = score
		if s.pvLen[0] > 0 {
			res.Move = s.pv[0][0]
			res.PV = append(res.PV[:0], s.pv[0][:s.pvLen[0]]...)
		}
		s.rootBest = res.Move
		if score >= mateScore-maxPly || score <= -mateScore+maxPly {
			break
		}
	}
	res.Nodes = s.nodes
	return res
}

// abort decides whether the search must unwind.
func (s *searcher) abort() bool {
	if s.aborted {
		return true
	}
	if s.stop != nil && s.stop.Load() {
		s.aborted, s.stopped = true, true
	}
	if s.limit > 0 && s.nodes > s.limit {
		s.aborted = true
	}
	return s.aborted
}

func (s *searcher) negamax(p Position, depth, ply, alpha, beta int) int {
	s.pvLen[ply] = 0
	s.nodes++
	if s.abort() {
		return 0
	}
	if ply > 0 {
		if p.Half >= 100 || p.InsufficientMaterial() {
			return 0
		}
	}
	inCheck := p.InCheck()
	if inCheck && ply < maxPly-2 {
		depth++
	}
	if depth <= 0 || ply >= maxPly-1 {
		return s.quiesce(p, alpha, beta, ply)
	}
	var buf [256]Move
	moves := p.LegalMoves(buf[:0])
	if len(moves) == 0 {
		if inCheck {
			return -mateScore + ply
		}
		return 0
	}
	var scores [256]int
	s.order(&p, moves, scores[:len(moves)], ply)
	us := p.Turn
	best := -infScore
	for i := range moves {
		pickBest(moves, scores[:len(moves)], i)
		m := moves[i]
		child := p.Make(m)
		key := child.Key()
		var score int
		if s.repeated(key) {
			score = 0
			s.pvLen[ply+1] = 0
		} else {
			s.path = append(s.path, key)
			score = -s.negamax(child, depth-1, ply+1, -beta, -alpha)
			s.path = s.path[:len(s.path)-1]
		}
		if s.aborted {
			return 0
		}
		if score > best {
			best = score
		}
		if score > alpha {
			alpha = score
			s.pv[ply][ply] = m
			n := s.pvLen[ply+1]
			copy(s.pv[ply][ply+1:], s.pv[ply+1][ply+1:ply+1+n])
			s.pvLen[ply] = n + 1
			if alpha >= beta {
				if p.B[m.To] == 0 && m.Promo == NoType {
					k := &s.killers[ply]
					if k[0] != m {
						k[1], k[0] = k[0], m
					}
					s.history[us][m.From][m.To] += int32(depth * depth)
				}
				break
			}
		}
	}
	return best
}

func (s *searcher) repeated(key uint64) bool {
	for _, k := range s.path {
		if k == key {
			return true
		}
	}
	return false
}

// quiesce searches the captures and promotions until the position is quiet.
func (s *searcher) quiesce(p Position, alpha, beta, ply int) int {
	s.nodes++
	if s.abort() {
		return 0
	}
	stand := Evaluate(&p)
	if ply >= maxPly-1 || stand >= beta {
		return stand
	}
	if stand > alpha {
		alpha = stand
	}
	var buf [256]Move
	all := p.LegalMoves(buf[:0])
	var caps [256]Move
	moves := caps[:0]
	for _, m := range all {
		if p.B[m.To] != 0 || m.Promo != NoType || isEP(&p, m) {
			moves = append(moves, m)
		}
	}
	var scores [256]int
	for i, m := range moves {
		scores[i] = s.captureScore(&p, m)
	}
	for i := range moves {
		pickBest(moves, scores[:len(moves)], i)
		child := p.Make(moves[i])
		score := -s.quiesce(child, -beta, -alpha, ply+1)
		if s.aborted {
			return 0
		}
		if score >= beta {
			return score
		}
		if score > alpha {
			alpha = score
		}
	}
	return alpha
}

func isEP(p *Position, m Move) bool {
	return p.B[m.From].Type() == Pawn && file(int(m.From)) != file(int(m.To)) && p.B[m.To] == 0
}

// captureScore is MVV-LVA: take the biggest victim with the smallest
// attacker first. Promotions come with their gain.
func (s *searcher) captureScore(p *Position, m Move) int {
	victim := p.B[m.To].Type()
	if victim == NoType && isEP(p, m) {
		victim = Pawn
	}
	score := 0
	if victim != NoType {
		score = 10000 + int(victim)*10 - int(p.B[m.From].Type())
	}
	if m.Promo != NoType {
		score += 9000 + pieceValue[m.Promo]/10
	}
	return score
}

func (s *searcher) order(p *Position, moves []Move, scores []int, ply int) {
	for i, m := range moves {
		switch {
		case ply == 0 && m == s.rootBest && s.rootBest != (Move{}):
			scores[i] = 1_000_000
		case p.B[m.To] != 0 || m.Promo != NoType || isEP(p, m):
			scores[i] = s.captureScore(p, m)
		case ply < len(s.killers) && s.killers[ply][0] == m:
			scores[i] = 8000
		case ply < len(s.killers) && s.killers[ply][1] == m:
			scores[i] = 7000
		default:
			h := s.history[p.Turn][m.From][m.To]
			if h > 6000 {
				h = 6000
			}
			scores[i] = int(h)
		}
	}
}

// pickBest swaps the best-scored move from i on into position i. Ties keep
// the earliest, so the order depends on nothing but the position.
func pickBest(moves []Move, scores []int, i int) {
	b := i
	for j := i + 1; j < len(moves); j++ {
		if scores[j] > scores[b] {
			b = j
		}
	}
	if b != i {
		moves[i], moves[b] = moves[b], moves[i]
		scores[i], scores[b] = scores[b], scores[i]
	}
}
