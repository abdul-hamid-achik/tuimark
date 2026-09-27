package host

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/uniwidth"
)

// Key is one decoded key press. Name is the key token (SPEC §8.1); Rune is
// the printable character, if any.
type Key struct {
	Name string
	Rune rune
}

// switchScreen activates the screen with the given id. Caller holds a.mu.
func (a *App) switchScreen(id string) bool {
	for i, s := range a.doc.Screens {
		if s.ID == id {
			if a.screen != i {
				a.screen = i
				a.focusInit = false
				a.focus = ""
				// A focus move still waiting for its frame belongs to the
				// screen being left.
				a.focusReq = nil
			}
			return true
		}
	}
	return false
}

// focusRequest is a focus move waiting for the next frame to check that
// its target can take focus (resolveFocus). The prev fields are the state
// before the first pending request, so a request that cannot land is undone
// completely, including the screen switch it made (render).
type focusRequest struct {
	target     string
	prev       string // focus before the first pending request
	prevScreen int    // active screen before the first pending request
	prevInit   bool   // focusInit before the first pending request
	fire       bool   // queue the target's on:focus once it lands
	landed     bool   // set by resolveFocus when the target took focus
	// acts are the inactive tabs the request activated, outermost first
	// (SPEC §6.10.3): their on:select events come before the target's
	// on:focus when it lands (action="focus" only), and they are undone
	// when it does not.
	acts []tabAct
}

// focusTarget returns the node a focus move to id may target: nil for an
// unknown id and for ids under a list <item> (rows are navigated by their
// list; their widgets are never focus targets, and the runtime treats
// their ids as missing).
func (a *App) focusTarget(id string) *ir.Node {
	n := a.doc.IDs[id]
	for p := n; p != nil; p = p.Parent {
		if p.Kind == "item" {
			return nil
		}
		// Nothing inside a container each template is a focus target
		// either (SPEC §6.8 item 8; its ids are V004).
		if p != n && a.doc.V2 && p.Each != "" && (p.Kind == "col" || p.Kind == "row" || p.Kind == "box") {
			return nil
		}
	}
	return n
}

// requestFocus moves focus to id like setFocus, but the next frame only
// keeps it when the target is focusable there; otherwise the move is undone
// (focus and screen go back to where they were, and no on:focus fires).
// fire queues the target's on:focus when the move lands. Caller holds a.mu.
func (a *App) requestFocus(id string, fire bool) {
	if a.focusTarget(id) == nil {
		return
	}
	req := &focusRequest{target: id, prev: a.focus, prevScreen: a.screen, prevInit: a.focusInit, fire: fire}
	if p := a.focusReq; p != nil {
		req.prev, req.prevScreen, req.prevInit = p.prev, p.prevScreen, p.prevInit
		req.acts = p.acts
	}
	// A target inside inactive tabs activates them as part of the request
	// (SPEC §6.10.3).
	if a.doc.V2 {
		req.acts = append(req.acts, a.openTabsFor(id)...)
	}
	a.setFocus(id)
	a.focusReq = req
}

// setFocus moves focus to id, switching screens when the target lives on
// another one. Caller holds a.mu.
func (a *App) setFocus(id string) {
	n, ok := a.doc.IDs[id]
	if !ok {
		return
	}
	for p := n; p != nil; p = p.Parent {
		if p.Kind == "screen" && p.ID != "" {
			a.switchScreen(p.ID)
			a.focusInit = true
			break
		}
	}
	a.focus = id
}

func (a *App) focusedBox() *layout.Box {
	if a.last == nil || a.focus == "" {
		return nil
	}
	return a.last.ByID[a.focus]
}

func (a *App) topModal() *layout.Box {
	if a.last == nil || len(a.last.Modals) == 0 {
		return nil
	}
	return a.last.Modals[len(a.last.Modals)-1]
}

// eventFor builds the payload for an action fired from the focused node.
func (a *App) eventFor(action string, b *layout.Box) Event {
	ev := Event{Action: action, Keys: map[string]any{}}
	if b == nil {
		return ev
	}
	ev.Source = b.ID
	switch b.Kind {
	case "list", "table":
		// A table always has each: keys = {alias: key of the cursor row}
		// (SPEC §8.2).
		ls := a.lists[b.ID]
		if ls != nil && ls.index < len(ls.keys) {
			if ls.each {
				ev.Keys[ls.alias] = ls.keys[ls.index]
			} else {
				ev.Value = ls.keys[ls.index]
			}
		}
	case "input":
		ev.Value = a.inputValue(b)
	}
	return ev
}

func (a *App) inputValue(b *layout.Box) string {
	if b.Src != nil && b.Src.Bind != "" {
		v, _ := lookup(a.store, b.Src.Bind)
		return Format(v)
	}
	return a.input(b.ID).value
}

// setInputValue writes an input's new text to its bind path, or to its
// runtime state without bind, and marks the live frame stale (markStale):
// when selectors, class guards, and if may read that text. The caller
// holds a.mu.
func (a *App) setInputValue(b *layout.Box, v string) {
	a.markStale()
	if b.Src != nil && b.Src.Bind != "" {
		if root, err := assign(a.store, b.Src.Bind, v); err == nil {
			a.store = root
			a.wrote(b.Src.Bind)
		}
		return
	}
	a.input(b.ID).value = v
}

// HandleKey applies a key to the runtime state and returns the events to
// dispatch, in order. quit reports the built-in quit with no handler.
// Caller must not hold a.mu.
func (a *App) HandleKey(k Key) []Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.handleKey(k)
}

// HandleKeyRun is HandleKey for keys[0] plus the printable keys right
// after it, when they all go to the focused input: such a run (a paste, or
// keys typed faster than the loop reads them) is typed as one edit, with
// one on:change carrying the final value, instead of one edit, change
// event, and frame per character. It returns the events and how many keys
// it used (at least one for a non-empty keys). Caller must not hold a.mu.
func (a *App) HandleKeyRun(keys []Key) ([]Event, int) {
	if len(keys) == 0 {
		return nil, 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// A printable key never reaches esc handling or the keymap while an
	// enabled input has focus: the input takes it (handleKey step 2).
	if b := a.focusedBox(); b != nil && !b.Disabled && b.Kind == "input" && isPrintable(keys[0]) {
		n := 1
		for n < len(keys) && isPrintable(keys[n]) {
			n++
		}
		text := make([]rune, n)
		for i, k := range keys[:n] {
			text[i] = k.Rune
		}
		return a.insertText(b, text), n
	}
	return a.handleKey(keys[0]), 1
}

// clusterBounds returns the grapheme cluster boundaries of v, counted in
// code points: 0, then the end of each cluster, the last being the code
// point count of v (SPEC v0.2 §12.1: an input's cursor stands between
// clusters).
func clusterBounds(v string) []int {
	bounds := []int{0}
	n := 0
	uniwidth.Each(v, func(c string, _ int) bool {
		n += utf8.RuneCountInString(c)
		bounds = append(bounds, n)
		return true
	})
	return bounds
}

// boundAtOrAfter returns the first boundary at or after c: a cursor
// inside a cluster stands after that cluster.
func boundAtOrAfter(bounds []int, c int) int {
	for _, b := range bounds {
		if b >= c {
			return b
		}
	}
	return bounds[len(bounds)-1]
}

// boundBefore returns the last boundary before c, or 0.
func boundBefore(bounds []int, c int) int {
	p := 0
	for _, b := range bounds {
		if b >= c {
			break
		}
		p = b
	}
	return p
}

// boundAfter returns the first boundary after c, or the last one.
func boundAfter(bounds []int, c int) int {
	for _, b := range bounds {
		if b > c {
			return b
		}
	}
	return bounds[len(bounds)-1]
}

// inputCursor returns the state of input b, its value as code points, and
// the value's cluster boundaries, after clamping the cursor to the value
// and moving it to a cluster boundary (a cursor inside a cluster stands
// after it, as painting shows it). Caller holds a.mu.
func (a *App) inputCursor(b *layout.Box) (*inputState, []rune, []int) {
	st := a.input(b.ID)
	v := a.inputValue(b)
	runes := []rune(v)
	bounds := clusterBounds(v)
	st.cursor = boundAtOrAfter(bounds, min(max(st.cursor, 0), len(runes)))
	return st, runes, bounds
}

// insertText types text into input b at its cursor: one edit, and one
// on:change when the input has one. The cursor ends after the inserted
// text, moved forward to the next cluster boundary when the text merged
// with the cluster that follows (SPEC v0.2 §12.1). Caller holds a.mu.
func (a *App) insertText(b *layout.Box, text []rune) []Event {
	st, runes, _ := a.inputCursor(b)
	out := make([]rune, 0, len(runes)+len(text))
	out = append(append(append(out, runes[:st.cursor]...), text...), runes[st.cursor:]...)
	v := string(out)
	st.cursor = boundAtOrAfter(clusterBounds(v), st.cursor+len(text))
	a.setInputValue(b, v)
	if act, ok := b.Src.On["change"]; ok {
		return []Event{a.eventFor(act, b)}
	}
	return nil
}

// focusTo handles action="focus" and tab cycling. A target the last frame
// shows as focusable takes focus now and its on:focus is returned. Any
// other target (disabled, hidden, not focusable, outside an open modal, on
// another screen, not rendered yet) is checked by the next frame: it only
// takes focus, and fires on:focus, if it can; otherwise nothing changes.
func (a *App) focusTo(id string) []Event {
	if id == "" || id == a.focus {
		return nil
	}
	n := a.focusTarget(id)
	if n == nil {
		return nil
	}
	if a.last != nil {
		for _, b := range a.last.Focusables {
			if b.ID == id {
				a.requestFocus(id, false)
				if act, ok := n.On["focus"]; ok {
					return []Event{{Action: act, Source: id, Keys: map[string]any{}}}
				}
				return nil
			}
		}
	}
	a.requestFocus(id, true)
	return nil
}

func (a *App) cycleFocus(dir int) []Event {
	if a.last == nil || len(a.last.Focusables) == 0 {
		return nil
	}
	list := a.last.Focusables
	idx := -1
	for i, b := range list {
		if b.ID == a.focus {
			idx = i
		}
	}
	next := 0
	if idx >= 0 {
		next = (idx + dir + len(list)) % len(list)
	} else if dir < 0 {
		next = len(list) - 1
	}
	return a.focusTo(list[next].ID)
}

func isPrintable(k Key) bool {
	return k.Rune != 0 && !strings.HasPrefix(k.Name, "ctrl+")
}

// Dispatch runs the handler for ev. It returns quit=true when the app
// should stop: the built-in quit without a handler, or a handler that
// returned ErrQuit.
func (a *App) Dispatch(ev Event) (quit bool, err error) {
	a.mu.Lock()
	h := a.handlers[ev.Action]
	a.mu.Unlock()
	if h == nil {
		return ev.Action == "quit", nil
	}
	if err := h(ev); err != nil {
		if errors.Is(err, ErrQuit) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// Focus returns the focused id.
func (a *App) Focus() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.focus
}

// Static returns every diagnostic decidable without rendering: parse,
// stylesheet and token problems plus the static V010 dock check, which
// SPEC §14 files under the parse pass. Layout and bind diagnostics need a
// render; use Validate for those.
func (a *App) Static() ir.Diags {
	a.mu.Lock()
	defer a.mu.Unlock()
	themes := a.validateThemes()
	out := append(ir.Diags(nil), a.static...)
	seen := map[string]bool{}
	for _, d := range out {
		seen[d.String()] = true
	}
	add := func(ds ir.Diags) {
		for _, d := range ds {
			if !seen[d.String()] {
				seen[d.String()] = true
				out = append(out, d)
			}
		}
	}
	for _, th := range themes {
		add(a.tokenCheck(th))
	}
	add(a.checkDocks(themes))
	return out.Sorted()
}
