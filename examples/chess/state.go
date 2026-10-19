package main

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// Game modes.
const (
	modeHvC = "hvc" // a human against the computer
	mode2P  = "2p"  // two humans at one keyboard
	modeCvC = "cvc" // the computer against itself, to watch
)

const (
	tickStep    = 100 * time.Millisecond
	noticeTicks = 30                     // a notice stays three seconds
	cvcDelay    = 450 * time.Millisecond // between two computer moves when watching
	minThink    = 350 * time.Millisecond // a reply is never faster than this in live play
)

// settings are what a new game is made of.
type settings struct {
	mode  string
	human Color
	level int
	clock clockSpec
}

// app is the host. Every field below mu is guarded by it: handlers and Every
// run on the runtime's loop, but a search finishes on its own goroutine and
// applies its result with Batch, so both take mu. Nothing holds mu while it
// calls into a.ui except the code that writes the store (writeLocked), and no
// Derive function ever takes mu.
type app struct {
	ui  *tuimark.App
	mu  sync.Mutex
	now func() time.Time
	// dispatch runs a search. The default starts a goroutine; tests run it
	// inline.
	dispatch func(func())
	rng      *rand.Rand
	// thinkDelay is the least a live search takes, so a quick level does not
	// answer in no time.
	thinkDelay time.Duration

	set     settings
	g       *Game
	flip    bool
	letters bool
	mouse   bool
	save    string // the -save path, "" to show the PGN in a dialog

	review bool
	ply    int // the position on the board: the last one while live

	cur     int  // the keyboard cursor, a square
	curOn   bool // whether it is drawn: the first arrow key shows it, a click or a typed move hides it
	sel     int  // the selected piece's square, or -1
	targets []int
	caps    []int
	hint    [2]int

	clk      clockState
	lastTick time.Time

	gen      int // bumped whenever a running search stops mattering
	thinking bool
	stop     *atomic.Bool
	engineAt time.Time // watching: the earliest start of the next search
	spin     int
	engine   engineInfo

	notice      string
	noticeTicks int

	after after // asked of the next store write
	extra *job  // a search a handler asked for (a hint)

	help, newOpen, resignOpen, promoOpen, pgnOpen bool
	promo                                         struct{ from, to int }
	ng                                            ngDraft
	pgnText                                       string

	evals map[uint64]int
}

// ngDraft is the new game dialog's choices.
type ngDraft struct {
	mode  string
	side  string // white, black, or random
	level int
	clock string
}

type engineInfo struct {
	has   bool
	depth int
	score string
	nodes string
	pv    string
}

// clockState is the two clocks in milliseconds.
type clockState struct {
	spec clockSpec
	ms   [2]int64
}

func (a *app) modalOpen() bool {
	return a.help || a.newOpen || a.resignOpen || a.promoOpen || a.pgnOpen
}

func (a *app) computer(c Color) bool {
	switch a.set.mode {
	case modeCvC:
		return true
	case modeHvC:
		return c != a.set.human
	}
	return false
}

// humanTurn reports whether a person moves next: the game is live, not over,
// and the side to move is not the computer.
func (a *app) humanTurn() bool {
	return !a.review && !a.g.Over() && !a.computer(a.g.Cur().Turn)
}

func (a *app) viewPos() *Position { return &a.g.Pos[a.ply] }

// say shows a notice for a few seconds.
func (a *app) say(msg string) {
	a.notice, a.noticeTicks = msg, noticeTicks
}

// invalidate drops the search in flight and every result it may still send.
func (a *app) invalidate() {
	a.gen++
	if a.stop != nil {
		a.stop.Store(true)
		a.stop = nil
	}
	a.thinking = false
}

func levelName(level int) string { return fmt.Sprintf("computer L%d", level) }

// startGame replaces the game. g is nil for a fresh game from start.
func (a *app) startGame(s settings, start Position, g *Game) {
	a.invalidate()
	if g == nil {
		g = NewGame(start)
	}
	a.set = s
	a.g = g
	a.review, a.ply = false, len(g.Moves)
	a.sel, a.targets, a.caps = -1, nil, nil
	a.hint = [2]int{-1, -1}
	a.engine = engineInfo{}
	a.flip = s.mode == modeHvC && s.human == Black
	a.clk = clockState{spec: s.clock}
	a.clk.ms = [2]int64{s.clock.Base, s.clock.Base}
	g.Stamp[0] = [2]int64{a.clk.ms[0], a.clk.ms[1]}
	for i := range g.Stamp {
		g.Stamp[i] = g.Stamp[0]
	}
	a.lastTick = a.now()
	a.engineAt = time.Time{}
	a.help, a.newOpen, a.resignOpen, a.promoOpen, a.pgnOpen = false, false, false, false, false
	// The cursor starts on the side that moves first for a person.
	a.curOn = false
	a.cur = 12
	if a.flip {
		a.cur = 52
	}
	names := map[string]string{}
	switch s.mode {
	case modeHvC:
		if s.human == White {
			names["White"], names["Black"] = "Player", levelName(s.level)
		} else {
			names["White"], names["Black"] = levelName(s.level), "Player"
		}
	case modeCvC:
		names["White"], names["Black"] = levelName(s.level), levelName(s.level)
	default:
		names["White"], names["Black"] = "White", "Black"
	}
	for k, v := range names {
		if _, ok := g.Tags[k]; !ok {
			g.Tags[k] = v
		}
	}
	if s.clock.On() {
		g.Tags["TimeControl"] = fmt.Sprintf("%d+%d", s.clock.Base/1000, s.clock.Inc/1000)
	}
	a.recalc()
}

// recalc rebuilds what depends on the selection and the position shown.
func (a *app) recalc() {
	a.targets, a.caps = nil, nil
	if a.sel < 0 || a.review || a.g.Over() {
		return
	}
	p := a.g.Cur()
	var buf [256]Move
	seen := map[int]bool{}
	for _, m := range p.LegalMoves(buf[:0]) {
		if int(m.From) != a.sel || seen[int(m.To)] {
			continue
		}
		seen[int(m.To)] = true
		a.targets = append(a.targets, int(m.To))
		if p.B[m.To] != 0 || isEP(p, m) {
			a.caps = append(a.caps, int(m.To))
		}
	}
}

// ---- moves ----

// applyMove plays a legal move for the side to move: the increment goes to
// the mover's clock, the position and the history move on, and whatever was
// being searched stops mattering.
func (a *app) applyMove(m Move) error {
	// Settle the clock at the move: the mover pays for the time since the
	// last tick, and a flag that fell in that time ends the game instead.
	a.tickClock(a.now())
	if a.g.Over() {
		a.say("the game is over: " + a.g.Reason)
		return nil
	}
	p := a.g.Cur()
	mover := p.Turn
	// A side's first move is free (its clock starts after it), so it earns
	// no increment either.
	if a.clk.spec.On() && len(a.g.Moves) >= 2 {
		a.clk.ms[mover] += a.clk.spec.Inc
	}
	if err := a.g.Push(m, a.clk.ms); err != nil {
		return err
	}
	a.invalidate()
	a.ply = len(a.g.Moves)
	a.sel, a.targets, a.caps = -1, nil, nil
	a.hint = [2]int{-1, -1}
	a.engineAt = a.now().Add(cvcDelay)
	return nil
}

// playMove plays a move for a person: from a click, the keyboard, or the
// move box.
func (a *app) playMove(m Move) error { return a.applyMove(m) }

// selectSquare is a click on a square, or enter on the cursor's.
func (a *app) selectSquare(sq int) error {
	a.cur = sq
	switch {
	case a.review:
		a.say("reviewing: v goes back to the game, enter plays on from here")
		return nil
	case a.g.Over():
		a.say("the game is over: n starts another")
		return nil
	case !a.humanTurn():
		a.say("it is the computer's turn")
		return nil
	}
	p := a.g.Cur()
	pc := p.B[sq]
	switch {
	case a.sel == sq:
		a.sel = -1
	case pc != 0 && pc.Color() == p.Turn:
		a.sel = sq
	case a.sel >= 0 && contains(a.targets, sq):
		return a.moveFromTo(a.sel, sq)
	default:
		a.sel = -1
	}
	a.recalc()
	return nil
}

// moveFromTo plays the move between two squares; a pawn reaching the last
// rank opens the promotion dialog instead.
func (a *app) moveFromTo(from, to int) error {
	p := a.g.Cur()
	var buf [256]Move
	var found []Move
	for _, m := range p.LegalMoves(buf[:0]) {
		if int(m.From) == from && int(m.To) == to {
			found = append(found, m)
		}
	}
	switch {
	case len(found) == 0:
		return fmt.Errorf("illegal move %s%s", sqName(from), sqName(to))
	case len(found) > 1 || found[0].Promo != NoType:
		a.promo.from, a.promo.to = from, to
		a.promoOpen = true
		return nil
	}
	return a.playMove(found[0])
}

func contains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// undo takes back the last move; against the computer, your move and its
// reply.
func (a *app) undo() {
	switch {
	case a.review:
		a.say("leave the review first: v")
		return
	case a.g.Ply() == 0:
		a.say("nothing to undo")
		return
	}
	if a.set.mode == modeHvC {
		// Take back to your own turn; if the computer opened, there is
		// nothing of yours to take back (it would only play the move again).
		target := a.g.Ply() - 1
		for target > 0 && a.g.Pos[target].Turn != a.set.human {
			target--
		}
		if a.g.Pos[target].Turn != a.set.human {
			a.say("nothing to take back: the computer opened")
			return
		}
	}
	a.invalidate()
	a.g.Undo()
	if a.set.mode == modeHvC {
		for a.g.Ply() > 0 && a.g.Cur().Turn != a.set.human {
			a.g.Undo()
		}
	}
	a.afterRewind()
}

func (a *app) redo() {
	switch {
	case a.review:
		a.say("leave the review first: v")
		return
	case !a.g.Redoable():
		a.say("nothing to redo")
		return
	}
	a.invalidate()
	a.g.Replay()
	if a.set.mode == modeHvC {
		for a.g.Redoable() && a.g.Cur().Turn != a.set.human {
			a.g.Replay()
		}
	}
	a.afterRewind()
}

// afterRewind puts the clocks back to where the position had them.
func (a *app) afterRewind() {
	a.ply = len(a.g.Moves)
	a.sel, a.targets, a.caps = -1, nil, nil
	a.hint = [2]int{-1, -1}
	if a.clk.spec.On() {
		st := a.g.Stamp[len(a.g.Stamp)-1]
		a.clk.ms = st
	}
	a.lastTick = a.now()
	a.engineAt = a.now().Add(cvcDelay)
	a.recalc()
}

// ---- review ----

func (a *app) enterReview(ply int) {
	a.invalidate()
	a.review = true
	a.setPly(ply)
	a.sel, a.targets, a.caps = -1, nil, nil
	a.hint = [2]int{-1, -1}
}

// moveLabel names the move that made ply: 12.Nf3 or 12...Nf6.
func (a *app) moveLabel(ply int) string {
	if ply <= 0 || ply > len(a.g.SAN) {
		return "the start"
	}
	idx := ply - 1
	if a.g.Start.Turn == Black {
		idx++
	}
	num := a.g.Start.Full + idx/2
	if idx%2 == 0 {
		return fmt.Sprintf("%d.%s", num, a.g.SAN[ply-1])
	}
	return fmt.Sprintf("%d...%s", num, a.g.SAN[ply-1])
}

func (a *app) leaveReview() {
	a.review = false
	a.ply = len(a.g.Moves)
	a.lastTick = a.now()
	a.engineAt = a.now().Add(cvcDelay)
	a.recalc()
}

func (a *app) setPly(ply int) {
	if ply < 0 {
		ply = 0
	}
	if ply > len(a.g.Moves) {
		ply = len(a.g.Moves)
	}
	a.ply = ply
}

// ---- clocks ----

// clockRunning reports whether the side to move is losing time.
// A side's clock starts after its own first move, so nothing runs until each
// side has moved once; and while the viewer's pause between two computer
// moves lasts, the game is waiting, not the player.
func (a *app) clockRunning() bool {
	return a.clk.spec.On() && !a.review && !a.g.Over() && !a.modalOpen() && len(a.g.Moves) >= 2
}

// tickClock charges dt to the side to move and ends the game when its time
// is gone. It reports whether the display changed.
func (a *app) tickClock(now time.Time) bool {
	dt := now.Sub(a.lastTick)
	a.lastTick = now
	if !a.clockRunning() || dt <= 0 {
		return false
	}
	if a.set.mode == modeCvC && !a.thinking && now.Before(a.engineAt) {
		return false
	}
	if dt > time.Second {
		dt = time.Second // a suspended terminal is not thinking time
	}
	side := a.g.Cur().Turn
	a.clk.ms[side] -= dt.Milliseconds()
	if a.clk.ms[side] <= 0 {
		a.clk.ms[side] = 0
		a.g.Flag(side)
		a.invalidate()
		a.sel, a.targets, a.caps = -1, nil, nil
	}
	return true
}
