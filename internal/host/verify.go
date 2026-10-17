package host

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// The checks of the frame caches (framecache.go): FrameDiff compares two
// frames member for member, and coldRender builds the frame a cold app
// (noCache) would build from the same state. The equivalence tests use
// both, and so does the tuimark_verify build tag (verify_on.go) for every
// frame the test suite builds.

// FrameDiff describes the first difference between frames f and g, or
// returns "" when they are the same frame: their JSON dumps with cells,
// their --styles spans, diagnostics, focus, theme, mouse, focus cycle,
// and every exported member of every box (a box's Parent, its tab and
// column lists, and For are compared by the box they name).
func FrameDiff(f, g *Frame) string {
	if f == nil || g == nil {
		if f != g {
			return fmt.Sprintf("frame %v vs %v", f != nil, g != nil)
		}
		return ""
	}
	fj, _ := dump.JSON(f.Dump(true))
	gj, _ := dump.JSON(g.Dump(true))
	if !bytes.Equal(fj, gj) {
		return "dump:\n" + firstLineDiff(string(fj), string(gj))
	}
	fs, _ := json.Marshal(dump.BuildStyles(f.Grid))
	gs, _ := json.Marshal(dump.BuildStyles(g.Grid))
	if !bytes.Equal(fs, gs) {
		return "styles:\n" + firstLineDiff(string(fs), string(gs))
	}
	if a, b := diagsText(f.Diags), diagsText(g.Diags); a != b {
		return "diags:\n" + firstLineDiff(a, b)
	}
	if f.Focus != g.Focus || f.Theme != g.Theme || f.Mouse != g.Mouse || f.Cols != g.Cols || f.Rows != g.Rows {
		return fmt.Sprintf("frame members: focus %q/%q theme %q/%q mouse %v/%v size %dx%d/%dx%d", f.Focus, g.Focus, f.Theme, g.Theme, f.Mouse, g.Mouse, f.Cols, f.Rows, g.Cols, g.Rows)
	}
	if a, b := boxNames(f.Focusables), boxNames(g.Focusables); a != b {
		return fmt.Sprintf("focusables %s vs %s", a, b)
	}
	if a, b := idNames(f.ByID), idNames(g.ByID); a != b {
		return fmt.Sprintf("byID %s vs %s", a, b)
	}
	if d := boxDiff("root", f.Root, g.Root); d != "" {
		return d
	}
	if len(f.Modals) != len(g.Modals) {
		return fmt.Sprintf("modals %d vs %d", len(f.Modals), len(g.Modals))
	}
	for i := range f.Modals {
		if d := boxDiff(fmt.Sprintf("modal[%d]", i), f.Modals[i], g.Modals[i]); d != "" {
			return d
		}
	}
	return ""
}

// frameDigest is everything FrameDiff compares of frame f, as text: its
// JSON dump with cells, its --styles spans, diagnostics, members, and
// every exported member of every box, one box per line (a box's Parent,
// its tab and column lists, and For by the box they name). Two digests
// of the same frame differ exactly when something wrote a member of it
// in between (checkRetained).
func frameDigest(f *Frame) string {
	var sb strings.Builder
	j, _ := dump.JSON(f.Dump(true))
	sb.Write(j)
	s, _ := json.Marshal(dump.BuildStyles(f.Grid))
	sb.WriteString("\n")
	sb.Write(s)
	fmt.Fprintf(&sb, "\n%s\n%q %q %v %dx%d\n%s\n%s\n", diagsText(f.Diags), f.Focus, f.Theme, f.Mouse, f.Cols, f.Rows, boxNames(f.Focusables), idNames(f.ByID))
	var walk func(b *layout.Box)
	walk = func(b *layout.Box) {
		if b == nil {
			sb.WriteString("<nil>\n")
			return
		}
		v := reflect.ValueOf(b).Elem()
		for i := 0; i < boxType.NumField(); i++ {
			fd := boxType.Field(i)
			if !fd.IsExported() {
				continue
			}
			fv := v.Field(i)
			switch fd.Name {
			case "Children":
				fmt.Fprintf(&sb, "Children:%d ", len(b.Children))
			case "Parent", "TabActive", "For":
				fmt.Fprintf(&sb, "%s:%s ", fd.Name, boxName(fv.Interface().(*layout.Box)))
			case "AllColumns", "TabList":
				fmt.Fprintf(&sb, "%s:%s ", fd.Name, boxNames(fv.Interface().([]*layout.Box)))
			case "Src":
				fmt.Fprintf(&sb, "Src:%p ", b.Src)
			default:
				fmt.Fprintf(&sb, "%s:%+v ", fd.Name, fv.Interface())
			}
		}
		sb.WriteString("\n")
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(f.Root)
	for _, m := range f.Modals {
		walk(m)
	}
	return sb.String()
}

func firstLineDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) || i < len(bl); i++ {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			if len(x) > 400 || len(y) > 400 {
				j := 0
				for j < len(x) && j < len(y) && x[j] == y[j] {
					j++
				}
				lo := max(0, j-120)
				x, y = x[lo:min(len(x), j+200)], y[lo:min(len(y), j+200)]
			}
			return fmt.Sprintf("line %d:\n  warm: %s\n  cold: %s", i+1, x, y)
		}
	}
	return "(no line differs)"
}

func diagsText(ds ir.Diags) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.String())
	}
	return strings.Join(out, "\n")
}

func boxName(b *layout.Box) string {
	if b == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s#%s[%s/%d]", b.Kind, b.ID, b.Key, b.Index)
}

func boxNames(bs []*layout.Box) string {
	var out []string
	for _, b := range bs {
		out = append(out, boxName(b))
	}
	return strings.Join(out, " ")
}

func idNames(m map[string]*layout.Box) string {
	var out []string
	for _, k := range sortedKeys(m) {
		out = append(out, k+"="+boxName(m[k]))
	}
	return strings.Join(out, " ")
}

var boxType = reflect.TypeOf(layout.Box{})

// boxDiff compares two box trees member for member.
func boxDiff(path string, x, y *layout.Box) string {
	if x == nil || y == nil {
		if x != y {
			return fmt.Sprintf("%s: box %s vs %s", path, boxName(x), boxName(y))
		}
		return ""
	}
	path += "/" + boxName(x)
	xv, yv := reflect.ValueOf(x).Elem(), reflect.ValueOf(y).Elem()
	for i := 0; i < boxType.NumField(); i++ {
		fd := boxType.Field(i)
		if !fd.IsExported() {
			continue
		}
		xf, yf := xv.Field(i), yv.Field(i)
		switch fd.Name {
		case "Children":
			continue
		case "Parent", "TabActive", "For":
			if a, b := boxName(xf.Interface().(*layout.Box)), boxName(yf.Interface().(*layout.Box)); a != b {
				return fmt.Sprintf("%s: %s %s vs %s", path, fd.Name, a, b)
			}
		case "AllColumns", "TabList":
			if a, b := boxNames(xf.Interface().([]*layout.Box)), boxNames(yf.Interface().([]*layout.Box)); a != b {
				return fmt.Sprintf("%s: %s %s vs %s", path, fd.Name, a, b)
			}
		case "Series":
			a, b := xf.Interface().([]float64), yf.Interface().([]float64)
			if len(a) != len(b) {
				return fmt.Sprintf("%s: Series %v vs %v", path, a, b)
			}
			for j := range a {
				if math.Float64bits(a[j]) != math.Float64bits(b[j]) {
					return fmt.Sprintf("%s: Series %v vs %v", path, a, b)
				}
			}
		default:
			if !reflect.DeepEqual(xf.Interface(), yf.Interface()) {
				return fmt.Sprintf("%s: %s %+v vs %+v", path, fd.Name, xf.Interface(), yf.Interface())
			}
		}
	}
	if len(x.Children) != len(y.Children) {
		return fmt.Sprintf("%s: %d children vs %d", path, len(x.Children), len(y.Children))
	}
	for i := range x.Children {
		if x.Children[i].Parent != x || y.Children[i].Parent != y {
			return fmt.Sprintf("%s: child %d has another parent", path, i)
		}
		if d := boxDiff(path, x.Children[i], y.Children[i]); d != "" {
			return d
		}
	}
	return ""
}

// stateSnap is every member of the App a render reads or writes, deep
// enough that a render on the restored state starts where the snapped one
// did (saveState plus the store and its counters).
type stateSnap struct {
	saved    savedState
	store    any
	storeGen uint64
	segGen   map[string]uint64
	writes   uint64
	dirty    bool
}

// snapAll snaps the state for coldRender, with a deep copy of the store
// (a render may write it in place: a tab's bind). The caller holds a.mu.
func (a *App) snapAll() stateSnap {
	s := a.snapKeep()
	s.store = deepCopyJSON(a.store)
	return s
}

// snapKeep is snapAll keeping the store itself, for state that only needs
// to be put back after a render on another store. The caller holds a.mu.
func (a *App) snapKeep() stateSnap {
	seg := make(map[string]uint64, len(a.segGen))
	for k, v := range a.segGen {
		seg[k] = v
	}
	return stateSnap{saved: a.saveState(), store: a.store, storeGen: a.storeGen, segGen: seg, writes: a.writes, dirty: a.dirty}
}

// restoreAll puts back a snapAll. The store is the snap's own copy, so a
// snap is restored at most once. The caller holds a.mu.
func (a *App) restoreAll(s stateSnap) {
	a.restoreState(s.saved)
	a.store, a.storeGen, a.segGen, a.writes, a.dirty = s.store, s.storeGen, s.segGen, s.writes, s.dirty
}

// coldRender builds, from state in, the frame a cold app (noCache) builds,
// with the events it queues and the state it leaves behind (frameInputs),
// and then puts the state back as it is now. The caller holds a.mu.
func (a *App) coldRender(in stateSnap, cols, rows int, theme string) (g *Frame, pending []Event, after string) {
	now := a.snapKeep()
	a.restoreAll(in)
	was := a.noCache
	a.noCache = true
	g = a.render(cols, rows, theme)
	pending, after = append([]Event(nil), a.pending...), a.frameInputs(cols, rows, theme)
	a.noCache = was
	a.restoreAll(now)
	return g, pending, after
}

// eventsDiff compares two event queues.
func eventsDiff(a, b []Event) string {
	if len(a) == 0 && len(b) == 0 {
		return ""
	}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	if !bytes.Equal(x, y) {
		return fmt.Sprintf("pending events:\n  warm: %s\n  cold: %s", x, y)
	}
	return ""
}
