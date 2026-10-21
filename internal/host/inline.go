package host

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"github.com/abdul-hamid-achik/tuimark/internal/paint"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// Inline mode and Commit (SPEC v0.4d §26.15, §18.3, decisions 241-251 and
// 261). The region is the active screen laid out at the height of its
// content (layout.RegionHeight) and drawn below the terminal's cursor
// (paint.Region); Commit prints rows above it.

// ErrInlineNeedsCurrent is the error of a document that writes version
// asked to run inline (SPEC v0.4d §26.15.1, decision 241).
var ErrInlineNeedsCurrent = errors.New("tuimark: inline mode needs a <tui> without version")

// Commit is what App.Commit prints into the scrollback of an inline
// session, above the live region (SPEC v0.4d §18.3): Lines, or a copy of
// Template. Exactly one of the two is given.
type Commit struct {
	Lines    []any             // each a string, or an array of runs (§6.20.2)
	Template string            // the id of a <template>
	With     string            // with= clauses for the copy, as on a <use>
	Props    map[string]string // the template's props, as on a <use>
}

// The messages of a commit that cannot be made (SPEC v0.4d §18.3).
var (
	errCommitNotInline = errors.New("commit: not in inline mode")
	errCommitDerive    = errors.New("commit: inside a derive")
	errCommitShape     = errors.New("commit: give either lines or template")
	errCommitNoLines   = errors.New("commit: with and props need a template")
)

// inlineSink is where the rows of a commit go: the terminal of a Run
// session (inline_term.go), or the scrollback a Play records (play
// session). It tells the size commits are rendered for.
type inlineSink interface {
	// termSize is the terminal's size, which a commit is rendered at.
	termSize() (cols, rows int)
	// print prints rows, the commit's grid, made of lines lines.
	print(rows *paint.Grid, lines int) error
}

// SetInline makes the frames this app builds an inline region (SPEC v0.4d
// §26.15.2) or full-screen frames again, and returns the previous setting.
// A document that writes version cannot run inline: it is the error
// ErrInlineNeedsCurrent and nothing changes. The tools set it for their
// --inline; Run and Play set it for their sessions.
func (a *App) SetInline(on bool) (prev bool, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if on && !a.doc.Lang.V4() {
		return a.inline, ErrInlineNeedsCurrent
	}
	prev = a.inline
	a.inline = on
	return prev, nil
}

// Inline reports whether the frames are inline regions.
func (a *App) Inline() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.inline
}

// setInlineSink installs the sink Commit prints into, and returns the one
// it replaced. A sink exists exactly while an inline session runs.
func (a *App) setInlineSink(s inlineSink) (prev inlineSink) {
	a.mu.Lock()
	defer a.mu.Unlock()
	prev, a.inl = a.inl, s
	a.inlActive.Store(s != nil)
	return prev
}

// Commit prints c into the scrollback above the live region (SPEC v0.4d
// §18.3, decisions 244-247). It applies while an inline Run runs, or a
// Play with Inline; at any other time it is the error "commit: not in
// inline mode". All or nothing: a commit that returns an error printed
// nothing, and one that succeeds printed all its rows in one write.
//
// Commit is safe from any goroutine. While the loop runs host code or the
// host protocol waits for a reply it prints at once; otherwise it is queued
// to the loop, which prints it before its next frame, and Commit returns
// after the print. From a Derive function it is the error "commit: inside a
// derive" (a Derive function runs under the lock this method takes).
func (a *App) Commit(c Commit) error {
	if a.inlActive.Load() && a.deriveG.Load() == goid() {
		return errCommitDerive
	}
	a.mu.Lock()
	sink := a.inl
	if sink == nil {
		a.mu.Unlock()
		return errCommitNotInline
	}
	cols, rows := sink.termSize()
	g, lines, err := a.buildCommit(c, cols, rows)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	return sink.print(g, lines)
}

// goid is the id of the calling goroutine, which Commit compares with the
// one a Derive function runs on: a Commit from another goroutine at the
// same time is no commit from inside the derive.
func goid() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := strings.TrimPrefix(string(buf[:n]), "goroutine ")
	if i := strings.IndexByte(s, ' '); i > 0 {
		if id, err := strconv.ParseInt(s[:i], 10, 64); err == nil {
			return id
		}
	}
	return -1
}

// deriveGoroutine is the field type of App.deriveG: the goroutine running
// Derive functions, 0 for none.
type deriveGoroutine = atomic.Int64

// buildCommit checks c and renders its rows at the terminal's size. The
// caller holds a.mu.
func (a *App) buildCommit(c Commit, cols, rows int) (*paint.Grid, int, error) {
	hasLines, hasTmpl := len(c.Lines) > 0, c.Template != ""
	switch {
	case hasLines == hasTmpl:
		return nil, 0, errCommitShape
	case hasLines && (c.With != "" || len(c.Props) > 0):
		return nil, 0, errCommitNoLines
	}
	if hasLines {
		return a.commitLines(c.Lines, cols, rows)
	}
	return a.commitTemplate(c, cols, rows)
}

// commitLines renders the lines of a commit (SPEC v0.4d §18.3, §26.15.4):
// each is a generated text, a child of the active screen, laid out as a
// root of its own at the terminal's width.
func (a *App) commitLines(lines []any, cols, rows int) (*paint.Grid, int, error) {
	runs := make([]any, len(lines))
	for i, l := range lines {
		r, err := commitRuns(i, l)
		if err != nil {
			return nil, 0, err
		}
		runs[i] = r
	}
	shell := a.commitShell()
	store := map[string]any{}
	for i, r := range runs {
		name := "l" + strconv.Itoa(i)
		store[name] = r
		t := &ir.Node{Tag: "text", Kind: "text", Runs: name, Parent: shell, Attrs: map[string]string{}, On: map[string]string{}, Path: "/screen/text"}
		shell.Children = append(shell.Children, t)
	}
	g := a.renderCommit(shell, store, cols, rows, true)
	return g, len(lines), nil
}

// commitShell is a copy of the active screen's node with no children, the
// parent a committed line or copy is styled under (SPEC v0.4d §26.15.4).
func (a *App) commitShell() *ir.Node {
	var sc ir.Node
	if a.doc.Root != nil && !a.doc.Spike && a.screen >= 0 && a.screen < len(a.doc.Screens) {
		sc = *a.doc.Screens[a.screen]
	} else {
		sc = ir.Node{Tag: "screen", Kind: "screen", Attrs: map[string]string{}, On: map[string]string{}, Path: "/screen"}
	}
	sc.Children, sc.Source = nil, nil
	return &sc
}

// commitRuns checks line i of a commit and returns it as the runs
// inflateRuns reads: a string is one run (SPEC v0.4d §18.3). The errors
// are the first found, in the order of the lines and the runs.
func commitRuns(i int, line any) (any, error) {
	if s, ok := line.(string); ok {
		return []any{s}, nil
	}
	v, err := ToJSON(line)
	if err != nil {
		return nil, fmt.Errorf("commit: line %d is %s, not a string or an array of runs", i, goTypeName(line))
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("commit: line %d is %s, not a string or an array of runs", i, runTypeName(v))
	}
	for m, el := range arr {
		switch r := el.(type) {
		case string:
		case map[string]any:
			if _, ok := r["text"].(string); !ok {
				return nil, fmt.Errorf("commit: line %d: run %d has no string \"text\"", i, m)
			}
			if cv, has := r["class"]; has && cv != nil {
				cs, isStr := cv.(string)
				if isStr {
					_, isStr = runClasses(cs)
				}
				if !isStr {
					return nil, fmt.Errorf("commit: line %d: run %d has a \"class\" that is not a list of class names", i, m)
				}
			}
		default:
			return nil, fmt.Errorf("commit: line %d: run %d is %s, not a string or an object", i, m, runTypeName(el))
		}
	}
	return arr, nil
}

// goTypeName is the TYPE of a line a Go host passed that is no JSON value
// (a channel, a function): the best name the error can give it.
func goTypeName(v any) string { return runTypeName(v) }

// commitTemplate renders the copy of a template (SPEC v0.4d §26.15.4,
// decision 245): a <use> without id, with the commit's with and props,
// placed at the root of the active screen. The copy is built by expanding
// that use in a document of the original's templates and includes
// (parse.CommitDocument), so it is exactly what a <use> in the document
// would build; its ids are dropped, its slot is empty, and its paths are
// read once, here.
func (a *App) commitTemplate(c Commit, cols, rows int) (*paint.Grid, int, error) {
	var with []ir.Each
	if c.With != "" {
		seen := map[string]bool{}
		for _, part := range strings.Split(c.With, ", ") {
			e, err := ir.ParseEach(part)
			if err != nil || seen[e.Alias] {
				with = nil
				seen = nil
				break
			}
			seen[e.Alias] = true
			with = append(with, e)
		}
		if seen == nil {
			return nil, 0, fmt.Errorf("commit: with=%q is malformed (want \"path as alias\", clauses separated by \", \")", c.With)
		}
	}
	props := parse.SortProps(c.Props)
	// A name a <use> keeps for itself, or that no template can declare, is
	// not a prop of any template: it is never passed on as an attribute.
	var bad []int
	passed := props[:0:0]
	for i, p := range props {
		if !commitPropName(p.Name) {
			bad = append(bad, i)
			continue
		}
		passed = append(passed, p)
	}
	doc := parse.CommitDocument(a.doc, a.ldr, c.Template, c.With, passed)
	var undeclared, pair, other *ir.Diagnostic
	var noTemplate bool
	// Index of the prop each diagnostic is about: the position in props.
	idx := func(col int) int {
		if col < 1 || col > len(passed) {
			return -1
		}
		name := passed[col-1].Name
		for i, p := range props {
			if p.Name == name {
				return i
			}
		}
		return -1
	}
	best := func(cur **ir.Diagnostic, d *ir.Diagnostic) {
		if *cur == nil || idx(d.Col) < idx((*cur).Col) {
			*cur = d
		}
	}
	for i := range doc.Diags {
		d := &doc.Diags[i]
		if d.Line != parse.CommitLine || d.Severity != ir.Error {
			continue
		}
		switch {
		case d.Code == "V019" && strings.Contains(d.Msg, "names no <template>"), d.Code == "V019" && strings.Contains(d.Msg, "needs template"):
			noTemplate = true
		case d.Code == "V002" && d.Col >= 1:
			best(&undeclared, d)
		case d.Code == "V003" && strings.Contains(d.Msg, "makes the pair"):
			best(&pair, d)
		case d.Code == "V011":
			// the with, checked above
		default:
			if other == nil {
				other = d
			}
		}
	}
	if noTemplate {
		return nil, 0, fmt.Errorf("commit: no <template id=%q>", c.Template)
	}
	for _, e := range with {
		if _, found := lookup(a.store, e.Path); !found {
			return nil, 0, fmt.Errorf("commit: with path %q (alias %s) is missing", e.Path, e.Alias)
		}
	}
	// The first of the prop errors, by the name of the prop.
	type propErr struct {
		at  int
		err error
	}
	var errs []propErr
	for _, i := range bad {
		errs = append(errs, propErr{i, fmt.Errorf("commit: prop %q is not declared by <template id=%q>", props[i].Name, c.Template)})
	}
	if undeclared != nil {
		i := idx(undeclared.Col)
		errs = append(errs, propErr{i, fmt.Errorf("commit: prop %q is not declared by <template id=%q>", props[i].Name, c.Template)})
	}
	if pair != nil {
		errs = append(errs, propErr{idx(pair.Col), errors.New("commit: " + pair.Msg)})
	}
	if len(errs) > 0 {
		sort.SliceStable(errs, func(x, y int) bool { return errs[x].at < errs[y].at })
		return nil, 0, errs[0].err
	}
	if other != nil {
		return nil, 0, errors.New("commit: " + other.Msg)
	}
	if len(doc.Screens) == 0 {
		return nil, 0, fmt.Errorf("commit: no <template id=%q>", c.Template)
	}
	copyScreen := doc.Screens[len(doc.Screens)-1]
	shell := a.commitShell()
	shell.Children = copyScreen.Children
	for _, n := range shell.Children {
		n.Parent = shell
		dropIDs(n)
	}
	g := a.renderCommit(shell, a.store, cols, rows, false)
	return g, 1, nil
}

// commitPropName reports whether name can be a prop of a template (SPEC
// v0.4c §6.18.9): [a-z_][a-z0-9_-]*, and none of the names a <use> keeps.
func commitPropName(name string) bool {
	switch name {
	case "id", "template", "with", "if", "hidden", "class":
		return false
	}
	return ir.IsGuardName(name)
}

// dropIDs takes the ids out of a committed copy, so nothing in it can take
// focus, be named by to= or #id, or be a duplicate (SPEC v0.4d §26.15.4).
func dropIDs(n *ir.Node) {
	n.ID = ""
	for _, c := range n.Children {
		dropIDs(c)
	}
}

// renderCommit renders the children of shell as roots of their own at the
// terminal's width, one under the other, and returns their cells as one
// grid. The shell is styled as the active screen, and its children as its
// children, by the document's cascade at the terminal's size; each child
// is then laid out from column 0 with the height of its content, and
// painted alone: the screen's padding, border, and background are not
// part of it (SPEC v0.4d §26.15.4). line is set for a committed line,
// whose wrap is always pre-wrap and which is at least one row high. The
// caller holds a.mu.
func (a *App) renderCommit(shell *ir.Node, store any, cols, termRows int, line bool) *paint.Grid {
	sub := blankApp()
	d := *a.doc
	d.Screens = []*ir.Node{shell}
	sub.doc, sub.sheets, sub.store, sub.file, sub.noCache = &d, a.sheets, store, a.file, true
	sub.keyboard = a.keyboard
	theme := css.EffectiveTheme(a.frameTheme())
	casc := css.NewCascadeLang(sub.sheets, css.Env{Cols: cols, Rows: termRows, Theme: theme}, d.Lang)
	fb := &builder{a: sub, seen: map[diagID]bool{}, byID: map[string]*layout.Box{}, rc: &renderCtx{tabDone: map[string]bool{}}}
	root := fb.inflate(shell, nil, nil, false)
	root = fb.cascade(casc, root)
	if root == nil {
		return paint.NewGrid(cols, 0)
	}
	fb.activateTabs(casc, root)
	eng := &layout.Engine{File: a.file, V3: true, Lang: d.Lang, Scratch: &sub.scratch}
	heights := make([]int, len(root.Children))
	for i, c := range root.Children {
		st := *c.Style
		st.Dock = ""
		if line {
			st.Wrap = layout.ModePreWrap
		}
		c.Style = &st
		h := eng.LayoutRoot(c, cols)
		if line && h < 1 {
			// An empty line is one empty row; its box is that high.
			h = 1
			c.H = 1
		}
		heights[i] = h
		fb.placeTableRows(casc, c)
	}
	fb.styleRuns(casc)
	total := 0
	for _, h := range heights {
		total += h
	}
	out := paint.NewGrid(cols, total)
	y := 0
	for i, c := range root.Children {
		if heights[i] == 0 {
			continue
		}
		g := paint.PaintFrame(c, nil, nil, cols, heights[i], 0)
		copy(out.Cells[y*cols:], g.Cells)
		y += heights[i]
	}
	return out
}
