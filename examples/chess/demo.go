package main

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/abdul-hamid-achik/tuimark"
)

// The scripted demo behind -demo COLSxROWS: Morphy's Opera Game (Paris,
// 1858) played move by move through the move box with the real keys, the
// way a person would type it, one frame per move. Each line printed is
// {"ms": N, "dump": D}: the frame and how long it shows. Nothing in it
// depends on a clock, a random number, or the engine, so the output is the
// same on every run and every OS.
const (
	demoLeadMs = 600  // the opening position
	demoPlyMs  = 280  // each move
	demoHoldMs = 1300 // the last frame, held so a loop restarts cleanly
)

// operaGame is Morphy's Opera Game (Paris, 1858), public domain.
const operaGame = "1.e4 e5 2.Nf3 d6 3.d4 Bg4 4.dxe5 Bxf3 5.Qxf3 dxe5 6.Bc4 Nf6 7.Qb3 Qe7 8.Nc3 c6 9.Bg5 b5 10.Nxb5 cxb5 11.Bxb5+ Nbd7 12.O-O-O Rd8 13.Rxd7 Rxd7 14.Rd1 Qe6 15.Bxd7+ Nxd7 16.Qb8+ Nxb8 17.Rd8#"

// operaMoves is the Opera Game (Paris 1858) as bare moves.
func operaMoves() []string {
	var out []string
	for _, f := range strings.Fields(operaGame) {
		if i := strings.IndexByte(f, '.'); i >= 0 {
			f = f[i+1:]
		}
		out = append(out, f)
	}
	return out
}

type frame struct {
	Ms   int `json:"ms"`
	Dump any `json:"dump"`
}

func demo(w io.Writer, dir, theme string, cols, rows int) error {
	o := defaults()
	o.dir = dir
	o.mode = mode2P
	s, err := o.validate()
	if err != nil {
		return err
	}
	a, err := newApp(o, s)
	if err != nil {
		return err
	}
	a.g.Tags["White"], a.g.Tags["Black"] = "Morphy", "Duke & Count"
	if theme != "" {
		if err := a.ui.Set("@theme", theme); err != nil {
			return err
		}
	}
	if err := a.sync(); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	snap := func(ms int) error {
		res, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true, Cells: true})
		if err != nil {
			return err
		}
		return enc.Encode(frame{Ms: ms, Dump: res.Dump})
	}
	if err := snap(demoLeadMs); err != nil {
		return err
	}
	moves := operaMoves()
	for i, san := range moves {
		if _, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, ":", "text:"+san, "enter"); err != nil {
			return err
		}
		ms := demoPlyMs
		if i == len(moves)-1 {
			ms = demoHoldMs
		}
		if err := snap(ms); err != nil {
			return err
		}
	}
	return nil
}

// sync writes the whole raw half once.
func (a *app) sync() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.writeLocked()
}
