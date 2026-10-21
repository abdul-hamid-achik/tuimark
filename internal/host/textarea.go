package host

import (
	"strings"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// The <textarea> of SPEC v0.4d §6.19 at run time (phase D3): its value and
// caret, its keys, its paste, and the hints' second pass. Its geometry (the
// visual lines, where the caret stands, the offset) is internal/layout's
// (layout/textarea.go), which the engine, the painter, and the dump read
// too.
//
// A textarea keeps its state in the same table as an input does
// (App.inputs, by id): the caret in inputState.cursor, a rune index into
// the text as the textarea reads it; the value, when it has no bind, in
// inputState.value; and, for the textarea alone, the goal column, whether
// the offset must follow the caret, and the raw value it last saw in the
// store, which tells an outside change from its own edits.

// textareaRaw is the value the store (or, without bind, the runtime) holds
// for textarea b, formatted as text (SPEC v0.4d §6.19.1: a value that is
// not a string is shown formatted, and the first edit writes a string over
// it), as the host wrote it. The caller holds a.mu.
func (a *App) textareaRaw(b *layout.Box) string {
	if b.Src != nil && b.Src.Bind != "" {
		v, _ := lookup(a.store, b.Src.Bind)
		return Format(v)
	}
	return a.input(b.ID).value
}

// textareaText is the text of textarea b as everything reads it (SPEC
// v0.4d §6.19.2, decision 262): the value read as run text. The caller
// holds a.mu.
func (a *App) textareaText(b *layout.Box) string {
	return NormalizeRunText(a.textareaRaw(b))
}

// syncTextarea reads raw as the text of the textarea whose state is st and
// applies the caret rules of SPEC v0.4d §6.19.2: the caret starts at the end
// of the value, and goes to the end of a value changed from outside (raw is
// not what the textarea saw last: decision 226), where the goal column is
// set again (resolved when the geometry is known) and the offset follows.
// It returns the text and the caret, aligned to a cluster boundary.
func syncTextarea(st *inputState, raw string) (string, int) {
	text := NormalizeRunText(raw)
	n := utf8.RuneCountInString(text)
	if !st.init || (st.seenOK && raw != st.seen) {
		st.init = true
		st.cursor = n
		st.goalSet, st.follow = false, true
	}
	st.seen, st.seenOK = raw, true
	st.cursor = boundAtOrAfter(textareaBounds(text), min(max(st.cursor, 0), n))
	return text, st.cursor
}

// inflateTextarea resolves a textarea for this frame (SPEC v0.4d §6.19.1,
// §6.19.2): its placeholder, its text and its caret. The offset follows the
// caret when it has none yet (a first frame, dump, a fresh app) or when a
// key or an outside change placed the caret since the last frame
// (decision 233).
func (fb *builder) inflateTextarea(n *ir.Node, b *layout.Box, sc *scope) {
	a := fb.a
	b.Placeholder = fb.interp(n, "placeholder", n.Attrs["placeholder"], sc)
	st := a.input(n.ID)
	raw := st.value
	if n.Bind != "" {
		v, found := sc.resolve(a.store, n.Bind)
		if !found {
			fb.missingBind(n, "bind", 0, n.Bind, sc)
		}
		raw = Format(v)
	}
	b.Text, b.Cursor = syncTextarea(st, raw)
	if _, had := a.scrolls[n.ID]; !had || st.follow {
		b.Follow = 0
	}
}

// settleTextarea is what a laid-out textarea leaves behind (rememberScroll):
// the goal column of a caret placed without a layout to measure it, and the
// end of the follow, which this frame's layout carried out.
func (a *App) settleTextarea(b *layout.Box) {
	st := a.inputs[b.ID]
	if st == nil || !b.Laid {
		return
	}
	// The box leaves the frame as a settled one: whether layout followed
	// the caret is not a member of the frame (a skipped frame and a cold
	// build agree).
	b.Follow = -1
	if !st.goalSet {
		st.goal, st.goalSet = layout.CaretOf(b).VCol, true
	}
	st.follow = false
}

// textareaBounds returns the cluster boundaries of text, in code points: the
// clusters of each hard line (a line break is a cluster of its own, and no
// cluster crosses one).
func textareaBounds(text string) []int {
	bounds := []int{0}
	base := 0
	parts := strings.Split(text, "\n")
	for i, p := range parts {
		cb := clusterBounds(p)
		for _, c := range cb[1:] {
			bounds = append(bounds, base+c)
		}
		base += cb[len(cb)-1]
		if i < len(parts)-1 {
			base++
			bounds = append(bounds, base)
		}
	}
	return bounds
}

// taModel is a textarea's text with what the key rules measure on it.
type taModel struct {
	text   string
	runes  []rune
	bounds []int
	lines  []layout.TextLine
	offs   []int // the byte offset of each code point, then len(text)
}

func newTAModel(text string, w int) *taModel {
	m := &taModel{text: text, runes: []rune(text), bounds: textareaBounds(text), lines: layout.TextareaLines(text, w)}
	m.offs = make([]int, 0, len(m.runes)+1)
	for i := range text {
		m.offs = append(m.offs, i)
	}
	m.offs = append(m.offs, len(text))
	return m
}

// locate is where the caret c stands: its visual line and its column in it.
func (m *taModel) locate(c int) (line, col int) {
	return layout.LocateCaret(m.text, m.lines, m.offs[c])
}

func (m *taModel) hardStart(c int) int {
	for c > 0 && m.runes[c-1] != '\n' {
		c--
	}
	return c
}

func (m *taModel) hardEnd(c int) int {
	for c < len(m.runes) && m.runes[c] != '\n' {
		c++
	}
	return c
}

// atColumn is the caret on visual line ti at the cluster boundary whose
// column is the largest not past goal (SPEC v0.4d §6.19.3). The end of a
// visual line that wraps without a line break is the start of the next one,
// so it is not a place of this line.
func (m *taModel) atColumn(ti, goal int) int {
	ln := m.lines[ti]
	lo, hi := layout.RuneIndex(m.text, ln.Start), layout.RuneIndex(m.text, ln.End)
	if !ln.LF && ti < len(m.lines)-1 {
		hi = boundBefore(m.bounds, hi)
	}
	best := lo
	for _, b := range m.bounds {
		if b < lo {
			continue
		}
		if b > hi {
			break
		}
		if layout.Width(m.text[ln.Start:m.offs[b]]) > goal {
			break
		}
		best = b
	}
	return best
}

// vertical is the caret c moved d visual lines (clamped) to the goal column.
func (m *taModel) vertical(c, d, goal int) int {
	li, _ := m.locate(c)
	return m.atColumn(min(max(li+d, 0), len(m.lines)-1), goal)
}

// deleteWordBack is where ctrl+w and alt+backspace delete back to from the
// caret c (SPEC v0.4d §6.19.3): the start of the previous word within the
// hard line, past the space characters right before the caret.
func (m *taModel) deleteWordBack(c int) int {
	hs := m.hardStart(c)
	i := clusterAt(m.bounds, c)
	for i > 0 && m.bounds[i] > hs && spaceCluster(m.runes, m.bounds, i-1) {
		i--
	}
	for i > 0 && m.bounds[i] > hs && !spaceCluster(m.runes, m.bounds, i-1) {
		i--
	}
	return max(m.bounds[i], hs)
}

// taGeometry returns the live width and rows of textarea b's content box.
func taGeometry(b *layout.Box) (w, h int) { return b.Content.W, b.Content.H }

// textareaConsumes reports whether the focused, enabled textarea b consumes
// k (SPEC v0.4d §6.19.3, §8.6 step 2): every printable key, the editing and
// moving keys, enter (it submits or breaks the line), shift+enter, and
// alt+enter. up and down are consumed except on the first and last visual
// line, and pgup and pgdn only while it scrolls. Before layout the box has
// no lines: the four are consumed, and step 6a of SPEC §18 judges them
// again once it has (decision 259).
func textareaConsumes(b *layout.Box, k Key) bool {
	if isPrintable(k) {
		return true
	}
	switch k.Name {
	case "backspace", "ctrl+h", "delete", "ctrl+u", "ctrl+k", "ctrl+w", "alt+backspace",
		"left", "right", "home", "end", "ctrl+a", "ctrl+e", "alt+b", "alt+f",
		"enter", "shift+enter", "alt+enter":
		return true
	case "up", "down", "pgup", "pgdn":
		if !b.Laid {
			return true
		}
		switch k.Name {
		case "up":
			return layout.CaretOf(b).VLine > 0
		case "down":
			return layout.CaretOf(b).VLine < layout.TextareaLineCount(b.Text, b.Content.W)-1
		}
		return b.ContentH > b.Content.H
	}
	return false
}

// textareaKey applies a key the focused textarea b consumes and returns its
// events (SPEC v0.4d §6.19.3). The caller holds a.mu.
func (a *App) textareaKey(b *layout.Box, k Key) []Event {
	if isPrintable(k) {
		return a.insertText(b, []rune{k.Rune})
	}
	st := a.input(b.ID)
	text, c := syncTextarea(st, a.textareaRaw(b))
	w, h := taGeometry(b)
	m := newTAModel(text, w)
	name := inputKeyName(k)
	switch name {
	case "enter":
		if !k.LF {
			if act, ok := b.Src.On["submit"]; ok {
				return []Event{a.eventFor(act, b)}
			}
		}
		return a.taEdit(b, st, m, c, c, "\n")
	case "shift+enter", "alt+enter":
		return a.taEdit(b, st, m, c, c, "\n")
	case "backspace", "ctrl+h":
		return a.taEdit(b, st, m, boundBefore(m.bounds, c), c, "")
	case "delete":
		return a.taEdit(b, st, m, c, boundAfter(m.bounds, c), "")
	case "ctrl+u":
		return a.taEdit(b, st, m, m.hardStart(c), c, "")
	case "ctrl+k":
		if he := m.hardEnd(c); he > c {
			return a.taEdit(b, st, m, c, he, "")
		} else if c < len(m.runes) {
			return a.taEdit(b, st, m, c, c+1, "") // the line break
		}
		return nil
	case "ctrl+w", "alt+backspace":
		if c > 0 && c == m.hardStart(c) {
			return a.taEdit(b, st, m, c-1, c, "") // the line break
		}
		return a.taEdit(b, st, m, m.deleteWordBack(c), c, "")
	case "left":
		a.taPlace(b, st, m, boundBefore(m.bounds, c))
	case "right":
		a.taPlace(b, st, m, boundAfter(m.bounds, c))
	case "home", "ctrl+a":
		a.taPlace(b, st, m, m.hardStart(c))
	case "end", "ctrl+e":
		a.taPlace(b, st, m, m.hardEnd(c))
	case "alt+b":
		a.taPlace(b, st, m, wordStart(m.runes, m.bounds, c))
	case "alt+f":
		a.taPlace(b, st, m, wordEnd(m.runes, m.bounds, c))
	case "up", "down", "pgup", "pgdn":
		d := 1
		switch name {
		case "up":
			d = -1
		case "pgup":
			d = -max(1, h)
		case "pgdn":
			d = max(1, h)
		}
		if !st.goalSet {
			_, col := m.locate(c)
			st.goal, st.goalSet = col, true
		}
		goal := st.goal
		a.taPlaceKeep(b, st, m.vertical(c, d, goal))
	}
	return nil
}

// taPlace puts the caret of textarea b at c, a placement other than a
// vertical move: it sets the goal column (SPEC v0.4d §6.19.2, decision
// 228) and the offset follows. The caller holds a.mu.
func (a *App) taPlace(b *layout.Box, st *inputState, m *taModel, c int) {
	_, col := m.locate(c)
	st.goal, st.goalSet = col, true
	a.taPlaceKeep(b, st, c)
}

// taPlaceKeep puts the caret at c and keeps the goal column (up, down, pgup,
// and pgdn).
func (a *App) taPlaceKeep(b *layout.Box, st *inputState, c int) {
	if c != st.cursor {
		st.cursor = c
		st.follow = true
		a.markStale()
	}
}

// taEdit replaces the code points [from, to) of textarea b's text, which m
// is, by repl, with the caret after it: one edit, written to the store (or
// the runtime's own value) only when it changes the text (SPEC v0.4d
// §6.19.2), with one on:change. An edit of a textarea bound to a derived
// path changes nothing (SPEC v0.4a §18.2). The caller holds a.mu.
func (a *App) taEdit(b *layout.Box, st *inputState, m *taModel, from, to int, repl string) []Event {
	if from == to && repl == "" {
		return nil
	}
	if a.inputBlocked(b) {
		return nil
	}
	out := string(m.runes[:from]) + repl + string(m.runes[to:])
	if out == m.text {
		return nil
	}
	w, _ := taGeometry(b)
	nm := newTAModel(out, w)
	// The caret stands between clusters of the new text, after what merged
	// with the cluster that follows (SPEC v0.2 §12.1).
	caret := boundAtOrAfter(nm.bounds, from+utf8.RuneCountInString(repl))
	st.cursor, st.follow = caret, true
	_, col := nm.locate(caret)
	st.goal, st.goalSet = col, true
	a.setTextareaValue(b, st, out)
	if act, ok := b.Src.On["change"]; ok {
		return []Event{a.eventFor(act, b)}
	}
	return nil
}

// setTextareaValue writes the new text of textarea b to its bind path, or
// to its runtime state without bind, and marks the live frame stale. The
// textarea saw what it wrote, so the next frame does not take it for a
// change from outside.
func (a *App) setTextareaValue(b *layout.Box, st *inputState, v string) {
	a.setInputValue(b, v)
	st.seen, st.seenOK = v, true
}

// insertTextarea types text into textarea b at its caret as one edit
// (coalesced printable keys, a paste, a line break). The caller holds a.mu.
func (a *App) insertTextarea(b *layout.Box, text []rune) []Event {
	st := a.input(b.ID)
	t, c := syncTextarea(st, a.textareaRaw(b))
	w, _ := taGeometry(b)
	m := newTAModel(t, w)
	return a.taEdit(b, st, m, c, c, string(text))
}

// NormalizeTextareaPaste returns the text a bracketed paste inserts into a
// textarea (SPEC v0.4d §6.19.6, decisions 234 and 265): the first 1 MiB of
// the payload, invalid UTF-8 dropped, CR LF and a lone CR made LF, then run
// text's normalizer (escape sequences whole, each TAB four spaces, every
// other control but LF removed).
func NormalizeTextareaPaste(payload string) string {
	if len(payload) > maxPaste {
		payload = payload[:maxPaste]
	}
	s := strings.ToValidUTF8(payload, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return NormalizeRunText(s)
}

// hintsSecondPass is step 6a of SPEC v0.4d §18 (decision 259). Step 5 chose
// the items of each hints treating a focused textarea's up, down, pgup, and
// pgdn as consumed, since the layout that decides whether the caret is on
// its first or last visual line, and whether it scrolls, did not exist
// yet. With the textarea laid out, the dispatch is run again on the tree
// (textareaConsumes judges a laid-out box); when the items of the hints
// with scope="active" differ, they are cascaded and the layout runs once
// more, from the frame being built alone. That set is final, even when the
// second layout would change it again. It returns the engine of the second
// layout, or nil when nothing changed.
func (fb *builder) hintsSecondPass(casc *css.Cascade, root *layout.Box, cols, rows int) *layout.Engine {
	a := fb.a
	f := fb.byID[a.focus]
	if a.focus == "" || f == nil || f.Kind != "textarea" || f.Disabled || !f.Laid {
		return nil
	}
	var hs []*layout.Box
	collect := func(r *layout.Box) {
		if r == nil {
			return
		}
		r.Walk(func(b *layout.Box) {
			if b.Kind == "hints" && b.Laid && (b.Src == nil || b.Src.Attrs["scope"] != "all") {
				hs = append(hs, b)
			}
		})
	}
	collect(root)
	for _, m := range fb.modals {
		collect(m)
	}
	if len(hs) == 0 {
		return nil
	}
	v := keyView{focused: f, root: root, modals: fb.modals, byID: fb.byID}
	its := a.hintItems(v, false)
	if sameHintItems(its, fb.hintsBuilt[false]) {
		return nil
	}
	for _, h := range hs {
		setHintItems(casc, h, its)
	}
	// The first layout's outputs are not an input of the second, except the
	// offsets it clamped, which a viewport keeps.
	reset := func(r *layout.Box) {
		if r == nil {
			return
		}
		r.Walk(func(b *layout.Box) {
			sx, sy := b.ScrollX, b.ScrollY
			b.ResetOutputs()
			b.ScrollX, b.ScrollY = sx, sy
		})
	}
	reset(root)
	for _, m := range fb.modals {
		reset(m)
	}
	eng := &layout.Engine{File: a.file, V3: a.doc.V3, Lang: a.doc.Lang, Scratch: &a.scratch}
	eng.Layout(root, fb.modals, cols, rows) // step 6 again
	return eng
}

func sameHintItems(x, y []hintItem) bool {
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// pasteNormalized is the text a bracketed paste inserts where the focus is
// (SPEC v0.4d §6.19.6): a textarea's normalization when a textarea has
// focus (it keeps the line breaks), else the input's (§26.7). It is what
// TUIMARK_LOG measures.
func (a *App) pasteNormalized(payload string) string {
	a.mu.Lock()
	b := a.focusedBox()
	ta := b != nil && !b.Disabled && b.Kind == "textarea"
	a.mu.Unlock()
	if ta {
		return NormalizeTextareaPaste(payload)
	}
	return NormalizePaste(payload)
}
