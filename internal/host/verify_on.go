//go:build tuimark_verify

package host

import (
	"fmt"
	"math"
	"reflect"

	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// verifyFrames checks every frame Frame and Dump give against a cold build
// from the same state (coldRender) and panics on the first difference:
// `go test -tags tuimark_verify ./...` runs the whole suite that way, and
// a tuimark binary built with the tag checks every golden and play step.
const verifyFrames = true

// verifiedFrame is Frame's build (liveFrame) checked against a cold
// render from the state it started from: the same frame, the same events
// queued, and the same state left behind. A skipped build is checked the
// same way, which is the frame skip's own premise.
func (a *App) verifiedFrame(cols, rows int, theme string) *Frame {
	if a.noCache {
		return a.liveFrame(cols, rows, theme)
	}
	in := a.snapAll()
	nPending := len(a.pending)
	f := a.liveFrame(cols, rows, theme)
	warmPending := append([]Event(nil), a.pending[nPending:]...)
	warmAfter := a.frameInputs(cols, rows, theme)
	g, coldPending, coldAfter := a.coldRender(in, cols, rows, theme)
	a.mustMatch("Frame", f, g, warmPending, coldPending[nPending:], warmAfter, coldAfter)
	a.checkRetained("Frame")
	if f != a.verifyPrev {
		a.verifyPrev, a.verifyDigest = f, frameDigest(f)
	}
	return f
}

// verifiedRender is Dump's render checked the same way.
func (a *App) verifiedRender(cols, rows int, theme string) *Frame {
	if a.noCache {
		return a.render(cols, rows, theme)
	}
	in := a.snapAll()
	nPending := len(a.pending)
	f := a.render(cols, rows, theme)
	warmPending := append([]Event(nil), a.pending[nPending:]...)
	warmAfter := a.frameInputs(cols, rows, theme)
	g, coldPending, coldAfter := a.coldRender(in, cols, rows, theme)
	a.mustMatch("Dump", f, g, warmPending, coldPending[nPending:], warmAfter, coldAfter)
	a.checkRetained("Dump")
	return f
}

// checkRetained is the guard of the live builds' slab reuse (listSlab):
// the frame Frame gave before (verifyPrev), which its caller may still
// hold, is unchanged by every render since, the next live build included,
// member for member (frameDigest, taken when it was built).
func (a *App) checkRetained(what string) {
	if a.verifyPrev == nil {
		return
	}
	if d := frameDigest(a.verifyPrev); d != a.verifyDigest {
		panic(fmt.Sprintf("tuimark_verify: %s: a render changed the frame Frame gave before it: %s", what, firstLineDiff(a.verifyDigest, d)))
	}
}

func (a *App) mustMatch(what string, f, g *Frame, wp, cp []Event, wa, ca string) {
	if d := FrameDiff(f, g); d != "" {
		panic(fmt.Sprintf("tuimark_verify: %s: the cached frame differs from a cold build: %s", what, d))
	}
	if d := eventsDiff(wp, cp); d != "" {
		panic(fmt.Sprintf("tuimark_verify: %s: %s", what, d))
	}
	if wa != ca {
		panic(fmt.Sprintf("tuimark_verify: %s: the state left behind differs:\n  warm: %q\n  cold: %q", what, wa, ca))
	}
}

// checkReuse is the guard of the reuse of a slab's list rows
// (listRows.reuse): the items it made this frame's again, and every box
// under them, are member for member what materializing r into fresh
// memory gives, so the frame is what writing every box whole would have
// built. Style is not compared (the cascade writes it next), Parent and
// Children are compared by place, and an item's Key, Index, Chan, and
// Mark, which the row loop writes next (inflateListCached), are skipped.
// What the engine memoizes on a box is valid only for the engine run it
// names (memoGen), and what it measured of its text only for the same
// text, so neither is compared.
func checkReuse(r *listRows, parent *layout.Box, items []*layout.Box) {
	fresh := r.materialize(parent, nil)
	if len(fresh) != len(items) {
		panic(fmt.Sprintf("tuimark_verify: reused list rows: %d items, fresh %d", len(items), len(fresh)))
	}
	for i := range items {
		if d := reusedBoxDiff(items[i], fresh[i], parent, parent, true); d != "" {
			panic(fmt.Sprintf("tuimark_verify: reused list row %d differs from a fresh one: %s", i, d))
		}
	}
}

func reusedBoxDiff(x, y, xp, yp *layout.Box, item bool) string {
	if (x == nil) != (y == nil) {
		return fmt.Sprintf("box %v, fresh %v", x != nil, y != nil)
	}
	if x == nil {
		return ""
	}
	if x.Parent != xp || y.Parent != yp {
		return x.Kind + ": parent"
	}
	if len(x.Children) != len(y.Children) {
		return fmt.Sprintf("%s: %d children, fresh %d", x.Kind, len(x.Children), len(y.Children))
	}
	vx, vy := reflect.ValueOf(x).Elem(), reflect.ValueOf(y).Elem()
	t := vx.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		switch f.Name {
		case "Parent", "Children", "Style":
			continue
		case "Key", "Index", "Chan", "Mark":
			if item {
				continue
			}
		}
		a, b := vx.Field(i), vy.Field(i)
		if a.Kind() == reflect.Float64 {
			if fa, fb := a.Float(), b.Float(); fa != fb && !(math.IsNaN(fa) && math.IsNaN(fb)) {
				return fmt.Sprintf("%s.%s: %v, fresh %v", x.Kind, f.Name, fa, fb)
			}
			continue
		}
		if !reflect.DeepEqual(a.Interface(), b.Interface()) {
			return fmt.Sprintf("%s.%s: %#v, fresh %#v", x.Kind, f.Name, a.Interface(), b.Interface())
		}
	}
	for j := range x.Children {
		if d := reusedBoxDiff(x.Children[j], y.Children[j], x, y, false); d != "" {
			return d
		}
	}
	return ""
}
