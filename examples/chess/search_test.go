package main

import (
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestSearchFindsMateInOne(t *testing.T) {
	for _, c := range []struct{ fen, san string }{
		{"6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1", "Ra8#"},
		{"r5k1/8/8/8/8/8/5PPP/6K1 b - - 0 1", "Ra1#"},
		{"k7/8/1K6/8/8/8/8/7R w - - 0 1", "Rh8#"},
	} {
		p := mustFEN(t, c.fen)
		for level := 1; level <= 3; level++ {
			res := Search(p, []uint64{p.Key()}, levelLimits[level], nil)
			if got := SAN(&p, res.Move); got != c.san {
				t.Errorf("%s level %d: %s, want %s", c.fen, level, got, c.san)
			}
			if res.Score < mateScore-10 {
				t.Errorf("%s level %d: score %d, want a mate score", c.fen, level, res.Score)
			}
		}
	}
}

func TestSearchFindsMateInTwoAndWinsMaterial(t *testing.T) {
	// Two rooks against a bare king on the back rank: 1.Ra7 and 2.Rb8#.
	p := mustFEN(t, "6k1/8/8/8/8/8/R7/1R4K1 w - - 0 1")
	res := Search(p, []uint64{p.Key()}, Limits{Depth: 4}, nil)
	if res.Score != mateScore-3 {
		t.Errorf("mate in two: %s with score %d, want %d", SAN(&p, res.Move), res.Score, mateScore-3)
	}
	// A queen hangs: take it.
	q := mustFEN(t, "4k3/8/8/3q4/8/8/8/3RK3 w - - 0 1")
	res = Search(q, []uint64{q.Key()}, levelLimits[2], nil)
	if SAN(&q, res.Move) != "Rxd5" {
		t.Errorf("hanging queen: %s", SAN(&q, res.Move))
	}
	// A knight attacked by a pawn walks away (or takes) rather than stay.
	n := mustFEN(t, "4k3/8/3p4/4N3/8/8/8/4K3 w - - 0 1")
	res = Search(n, []uint64{n.Key()}, levelLimits[3], nil)
	if res.Move.From != 36 { // e5: the pawn on d6 attacks it, so the knight moves
		t.Errorf("attacked knight: %s", SAN(&n, res.Move))
	}
}

func TestSearchIsDeterministic(t *testing.T) {
	p := StartPosition()
	lim := levelLimits[4]
	a := Search(p, []uint64{p.Key()}, lim, nil)
	b := Search(p, []uint64{p.Key()}, lim, nil)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("two searches differ:\n%+v\n%+v", a, b)
	}
	if !a.Found || a.Depth != 4 || a.Nodes == 0 || len(a.PV) == 0 || a.PV[0] != a.Move {
		t.Errorf("result = %+v", a)
	}
}

func TestSearchNodeBudgetKeepsTheLastDepth(t *testing.T) {
	p := mustFEN(t, "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1")
	res := Search(p, []uint64{p.Key()}, Limits{Depth: 8, Nodes: 20_000}, nil)
	if !res.Found || res.Depth < 1 || res.Depth >= 8 {
		t.Errorf("depth = %d with a 20k node budget", res.Depth)
	}
	if res.Nodes > 40_000 {
		t.Errorf("%d nodes with a 20k budget", res.Nodes)
	}
	again := Search(p, []uint64{p.Key()}, Limits{Depth: 8, Nodes: 20_000}, nil)
	if !reflect.DeepEqual(res, again) {
		t.Error("a budgeted search is not repeatable")
	}
	if !p.IsLegal(res.Move) {
		t.Errorf("illegal move %v", res.Move)
	}
}

func TestSearchStops(t *testing.T) {
	var stop atomic.Bool
	stop.Store(true)
	p := StartPosition()
	res := Search(p, []uint64{p.Key()}, Limits{Depth: 12}, &stop)
	if res.Found {
		t.Errorf("a stopped search found %+v", res)
	}
	if !p.IsLegal(res.Move) {
		t.Error("a stopped search still names a legal move")
	}
	// No legal move: nothing found.
	m := mustFEN(t, "R5k1/5ppp/8/8/8/8/8/6K1 b - - 0 1")
	if r := Search(m, nil, Limits{Depth: 3}, nil); r.Found {
		t.Error("search found a move in a checkmate")
	}
}

// A position that came up before counts as a draw: with the other moves all
// leading into positions already seen, the one new move is the only one with
// a winning score, even though it is not the move the search would pick.
func TestSearchSeesRepetitions(t *testing.T) {
	p := mustFEN(t, "4k3/8/8/8/8/8/8/R3K3 w - - 0 1")
	var buf [256]Move
	free := Search(p, []uint64{p.Key()}, Limits{Depth: 3}, nil)
	if free.Move.UCI() == "e1d1" {
		t.Fatal("the test needs a position where Kd1 is not the best move")
	}
	keys := []uint64{p.Key()}
	var fresh Move
	for _, m := range p.LegalMoves(buf[:0]) {
		if m.UCI() == "e1d1" {
			fresh = m
			continue
		}
		child := p.Make(m)
		keys = append(keys, child.Key())
	}
	res := Search(p, keys, Limits{Depth: 3}, nil)
	if res.Move != fresh {
		t.Errorf("chose %s with %d, want the one new move e1d1 (%d)", res.Move, res.Score, free.Score)
	}
}

func TestLevelsGetStronger(t *testing.T) {
	// Levels 5 and 6 take seconds under the race detector: TUIMARK_PERFT_DEEP=1
	// adds them, as it adds the deep perft counts.
	top := 4
	if os.Getenv("TUIMARK_PERFT_DEEP") != "" {
		top = 6
	}
	p := mustFEN(t, "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1")
	prev := 0
	for level := 1; level <= top; level++ {
		start := time.Now()
		res := Search(p, []uint64{p.Key()}, levelLimits[level], nil)
		t.Logf("level %d: depth %d, %d nodes, %v, %s %d cp", level, res.Depth, res.Nodes, time.Since(start).Round(time.Millisecond), SAN(&p, res.Move), res.Score)
		if res.Depth < prev {
			t.Errorf("level %d searched depth %d, less than the level before", level, res.Depth)
		}
		prev = res.Depth
	}
}
