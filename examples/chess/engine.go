package main

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// job is one search handed to a goroutine: it works on a copy of the
// position and names the generation it was started in, so a result that
// arrives after the game moved on is dropped.
type job struct {
	gen  int
	hint bool
	pos  Position
	keys []uint64
	lim  Limits
	stop *atomic.Bool
}

// takeJob decides, with mu held, whether the computer must start thinking
// and returns the search to run. The caller runs it after releasing mu.
func (a *app) takeJob() *job {
	if a.thinking || a.review || a.g.Over() || a.modalOpen() {
		return nil
	}
	p := a.g.Cur()
	if !a.computer(p.Turn) {
		return nil
	}
	if a.set.mode == modeCvC && a.now().Before(a.engineAt) {
		return nil
	}
	lim := levelLimits[a.set.level]
	if a.clk.spec.On() && a.clk.ms[p.Turn] < 10_000 {
		lim = Limits{Depth: min(lim.Depth, 3), Nodes: 20_000} // short of time: think fast
	}
	a.thinking = true
	a.stop = &atomic.Bool{}
	return &job{gen: a.gen, pos: *p, keys: append([]uint64(nil), a.g.Keys...), lim: lim, stop: a.stop}
}

// hintJob asks for the engine's best move for the side to move.
func (a *app) hintJob() *job {
	if a.review || a.g.Over() {
		return nil
	}
	return &job{gen: a.gen, hint: true, pos: *a.g.Cur(), keys: append([]uint64(nil), a.g.Keys...), lim: hintLimits, stop: &atomic.Bool{}}
}

// run searches. It touches no app state; finish applies the result.
func (a *app) runJob(j *job) {
	start := time.Now()
	res := Search(j.pos, j.keys, j.lim, j.stop)
	var slept time.Duration
	if d := a.thinkDelay - time.Since(start); d > 0 && !j.hint {
		time.Sleep(d)
		slept = d
	}
	a.finish(j, res, slept)
}

// finish applies a search result, if the game it was for is still the game.
// slept is the pause added after the search so a quick level does not answer
// at once: it was not thinking time, and the clock gets it back.
func (a *app) finish(j *job, res SearchResult, slept time.Duration) {
	a.mu.Lock()
	if j.gen != a.gen || !res.Found || a.g.Over() || a.review {
		a.mu.Unlock()
		return
	}
	var next *job
	if j.hint {
		a.hint = [2]int{int(res.Move.From), int(res.Move.To)}
		a.say("hint: " + SAN(&j.pos, res.Move))
	} else {
		a.thinking = false
		a.stop = nil
		if a.clk.spec.On() && len(a.g.Moves) >= 2 {
			side := a.g.Cur().Turn
			a.clk.ms[side] += slept.Milliseconds()
		}
		a.engine = infoOf(&j.pos, res)
		switch {
		case a.g.ClaimableDraw() != "" && res.Score <= 0:
			// The computer, no better than equal, takes the draw.
			_ = a.g.ClaimDraw()
			a.say("the computer claims a draw: " + a.g.Reason)
		default:
			if err := a.applyMove(res.Move); err != nil {
				a.say(err.Error())
			}
		}
		next = a.takeJob()
	}
	err := a.writeLocked()
	a.mu.Unlock()
	if err != nil {
		a.onError(nil, err)
	}
	if next != nil {
		a.dispatch(func() { a.runJob(next) })
	}
}

// whiteScore converts a side-to-move score to white's point of view.
func whiteScore(p *Position, s int) int {
	if p.Turn == Black {
		return -s
	}
	return s
}

// infoOf is the engine panel's content for a result.
func infoOf(p *Position, res SearchResult) engineInfo {
	return engineInfo{
		has:   true,
		depth: res.Depth,
		score: scoreText(p, res.Score),
		nodes: commas(res.Nodes),
		pv:    pvText(p, res.PV),
	}
}

// scoreText writes a side-to-move score from white's point of view: +0.32,
// -1.50, #3 for white mating in three, -#2 for black mating in two.
func scoreText(p *Position, s int) string {
	w := whiteScore(p, s)
	if s >= mateScore-maxPly || s <= -mateScore+maxPly {
		n := (mateScore - abs(s) + 1) / 2
		if w < 0 {
			return fmt.Sprintf("-#%d", n)
		}
		return fmt.Sprintf("#%d", n)
	}
	return cpText(w)
}

// cpText writes centipawns as pawns with two decimals and a sign.
func cpText(cp int) string {
	sign := "+"
	if cp < 0 {
		sign, cp = "-", -cp
	}
	return fmt.Sprintf("%s%d.%02d", sign, cp/100, cp%100)
}

// commas writes 1234567 as 1,234,567.
func commas(n int64) string {
	s := fmt.Sprint(n)
	if n < 1000 {
		return s
	}
	var out []byte
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// pvText plays the principal variation from p in SAN with move numbers:
// 12.Nf3 Nc6 13.d4.
func pvText(p *Position, pv []Move) string {
	var parts []string
	cur := *p
	for i, m := range pv {
		if i >= 8 || !cur.IsLegal(m) {
			break
		}
		san := SAN(&cur, m)
		switch {
		case cur.Turn == White:
			parts = append(parts, fmt.Sprintf("%d.%s", cur.Full, san))
		case i == 0:
			parts = append(parts, fmt.Sprintf("%d...%s", cur.Full, san))
		default:
			parts = append(parts, san)
		}
		cur = cur.Make(m)
	}
	return strings.Join(parts, " ")
}

// evalAt is the evaluation bar's score for a position, white's point of
// view, cached by position.
func (a *app) evalAt(p *Position) int {
	k := p.Key()
	if v, ok := a.evals[k]; ok {
		return v
	}
	v := EvalWhite(p)
	if len(a.evals) > 4096 {
		a.evals = map[uint64]int{}
	}
	a.evals[k] = v
	return v
}
