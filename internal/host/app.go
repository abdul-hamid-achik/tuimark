// Package host owns a running document: the JSON store, stylesheets,
// focus, list/input state, events, and the terminal loop. The public
// tuimark package is a thin wrapper over it.
package host

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// Version is the runtime version the start record of TUIMARK_LOG carries
// (SPEC v0.2b §26.12). Release builds set it with -ldflags "-X", like the
// CLI's own version.
var Version = "0.3.1"

// Event is the payload a handler receives (SPEC §8.2).
type Event struct {
	Action string         `json:"action"`
	Source string         `json:"source"`
	Keys   map[string]any `json:"keys"`
	Value  any            `json:"value"`
}

// Handler handles a named action. Returning ErrQuit stops Run.
type Handler func(Event) error

// ErrQuit stops Run cleanly when returned by a handler.
var ErrQuit = errors.New("tuimark: quit")

// ActionSpec describes an action referenced by the document.
type ActionSpec struct {
	Name       string   `json:"name"`
	Builtin    bool     `json:"builtin,omitempty"`
	Registered bool     `json:"registered"`
	Sources    []string `json:"sources"`
}

type listState struct {
	index int
	keys  []any
	alias string
	each  bool
	page  int
}

type inputState struct {
	value  string // used when the input has no bind path
	cursor int
	init   bool
}

// App is a loaded document plus its runtime state.
type App struct {
	mu     sync.Mutex
	doc    *parse.Document
	file   string // display name used in diagnostics
	path   string // path on disk ("" for Parse)
	dir    string
	sheets []*css.Sheet
	// static holds the diagnostics decidable without rendering that do
	// not depend on the theme; tokenDiags caches the token check (V003
	// for an unknown $token) per effective theme (staticFor).
	static     ir.Diags
	tokenDiags map[string]ir.Diags
	store      any
	handlers   map[string]Handler
	strict     bool
	// storeGen counts replacements of the whole store and segGen the
	// writes under each top-level member (wrote), so a table knows when a
	// path its cached rows read may have changed (tableData).
	storeGen uint64
	segGen   map[string]uint64
	tables   map[*ir.Node]*tableCache
	// checkedSets keeps, per list or table node, the key set of its
	// checked array while the store member it reads is unchanged
	// (checkedSet).
	checkedSets map[*ir.Node]*checkedCache

	screen    int
	focus     string
	focusInit bool
	lists     map[string]*listState
	inputs    map[string]*inputState
	scrolls   map[string][2]int
	// stickMax is, per id of a scroll with stick="bottom" (version="3",
	// SPEC v0.3b §11.4), the maximum y offset its last live layout
	// computed (EXTENT - viewport): "that frame's maximum" the stick rule
	// compares the offset a frame brings in against.
	stickMax   map[string]int
	openModals []*ir.Node   // modals open in the last frame, document order (top last)
	modalStack []modalEntry // modals that took the focus trap, bottom first
	pending    []Event
	focusReq   *focusRequest
	// tabMem is the tab the runtime remembers per tabs id, for a tabs
	// without bind (SPEC §6.10.2); tabPrev is the active tab per tabs id in
	// the last frame, for the activation focus rule (§6.10.3).
	tabMem  map[string]string
	tabPrev map[string]string
	// dirty is set, in a version="2" document, by an input that changed
	// what the live frame shows (markStale, TakeDirty; SPEC v0.2b §8.6).
	dirty bool
	wake  chan struct{}
	last  *Frame
	// wakeCount counts Wake calls (SPEC v0.3 §18.1 Batch: "at most one
	// redraw"; not part of the public API, an observation hook for tests).
	wakeCount atomic.Int64
	// mode guards Run, Loop, and Play against each other (SPEC v0.3 §18.1
	// Play rule 8): "" is idle, "run" a live loop (Run or Loop), "play" an
	// active Play call. Guarded by mu.
	mode string
	// press is the hit identity of a pending left press (SPEC v0.2b §8.5,
	// §26.11): the next left release ends it. nil when none is pending.
	press *hitIdentity

	// Theme selection (SPEC §26.4). hostTheme is the reserved Set path
	// @theme (dark, light, or auto; "" until the host sets it); flagTheme
	// is the tools' --theme (SetTheme); runEnvTheme is TUIMARK_THEME while
	// Run runs; runAuto is the theme Run resolved auto to (the probe's
	// background, else COLORFGBG, else dark), "" outside Run.
	hostTheme   string
	flagTheme   string
	runEnvTheme string
	runAuto     string
}

// modalEntry is a modal that took the focus trap when it became the top
// modal, and where focus goes back to when it closes (resolveFocus).
type modalEntry struct {
	node *ir.Node
	ret  string
}

// Load reads and parses a .tui file. Validation problems are diagnostics,
// not errors; the error is for I/O only.
func Load(path string) (*App, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return newApp(src, path, filepath.Dir(path)), nil
}

// Parse reads a document from r. Relative style src paths resolve against
// the working directory.
func Parse(r io.Reader) (*App, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return newApp(src, "", "."), nil
}

// blankApp returns an App with every map/channel field initialized, its
// document and style resolution left to the caller (newApp, newAppFS).
func blankApp() *App {
	return &App{
		store: map[string]any{}, handlers: map[string]Handler{},
		lists: map[string]*listState{}, inputs: map[string]*inputState{},
		scrolls: map[string][2]int{}, stickMax: map[string]int{}, wake: make(chan struct{}, 1),
		tokenDiags: map[string]ir.Diags{},
		segGen:     map[string]uint64{}, tables: map[*ir.Node]*tableCache{},
		checkedSets: map[*ir.Node]*checkedCache{},
		tabMem:      map[string]string{}, tabPrev: map[string]string{},
	}
}

func newApp(src []byte, path, dir string) *App {
	a := blankApp()
	a.path, a.dir, a.file = path, dir, filepath.Base(path)
	if path == "" {
		a.file = ""
	}
	a.doc = parse.Parse(src, path)
	a.static = append(a.static, a.doc.Diags...)
	a.loadStyles(diskResolver(dir, path))
	return a
}

// loadStyles parses every <style>: an inline body, or a src resolved and
// read through res, shared between Load (an OS directory: diskResolver)
// and LoadFS (an fs.FS: fsResolver). SPEC v0.3 §18.1.
func (a *App) loadStyles(res styleResolver) {
	seen := map[string]bool{}
	// A stylesheet is checked against the version of the document that
	// loads it (SPEC §5.1, v0.3b).
	v2, v3 := a.doc.V2, a.doc.V3
	for _, s := range a.doc.Styles {
		if s.Src == "" {
			sh, diags := css.ParseInlineSheetIn(s.Body, a.file, s.BodyLine, s.BodyCol, v2, v3)
			a.static = append(a.static, diags...)
			a.sheets = append(a.sheets, sh)
			continue
		}
		d := ir.Diagnostic{Severity: ir.Error, Code: "V006", File: a.file, Line: s.Line, Col: s.Col, Path: "/style"}
		resolved, outside := res.resolve(s.Src)
		if outside {
			d.Msg = fmt.Sprintf("style src=%q is outside the file system", s.Src)
			a.static = append(a.static, d)
			continue
		}
		canon := res.canon(resolved)
		switch {
		case canon == res.self || strings.HasSuffix(canon, ".tui"):
			d.Msg = fmt.Sprintf("style src=%q includes a .tui document (include cycle)", s.Src)
		case seen[canon]:
			d.Msg = fmt.Sprintf("style src=%q is included twice (include cycle)", s.Src)
		}
		if d.Msg != "" {
			a.static = append(a.static, d)
			continue
		}
		seen[canon] = true
		body, err := res.read(resolved)
		if err != nil {
			d.Msg = fmt.Sprintf("style src=%q cannot be read: %v", s.Src, unwrapOrSelf(err))
			a.static = append(a.static, d)
			continue
		}
		sh, diags := css.ParseSheetIn(string(body), res.base(resolved), v2, v3)
		a.static = append(a.static, diags...)
		a.sheets = append(a.sheets, sh)
	}
}

// unwrapOrSelf is errors.Unwrap(err), falling back to err itself when
// there is nothing to unwrap (a "cannot be read" message strips the
// redundant "open <path>:" os.PathError/fs.PathError prefix when it can).
func unwrapOrSelf(err error) error {
	if u := errors.Unwrap(err); u != nil {
		return u
	}
	return err
}

// staticFor returns the static diagnostics of a render under an effective
// theme: the theme-independent ones plus the token check for that theme.
// Token references in every rule, including rules no render matches
// (other sizes, closed modals, other screens), are checked once per theme
// (SPEC §10.4: a token is checked against the theme's set and the :root
// rules that match under it). The caller holds a.mu.
func (a *App) staticFor(theme string) ir.Diags {
	return append(append(ir.Diags(nil), a.static...), a.tokenCheck(theme)...)
}

// tokenCheck is css.CheckTokens for one effective theme, cached. The
// caller holds a.mu.
func (a *App) tokenCheck(theme string) ir.Diags {
	theme = css.EffectiveTheme(theme)
	ds, ok := a.tokenDiags[theme]
	if !ok {
		ds = css.CheckTokens(a.sheets, theme, a.doc.InlineDecls()...)
		a.tokenDiags[theme] = ds
	}
	return ds
}

// Files returns the document path and every stylesheet path (for --watch).
func (a *App) Files() []string {
	var out []string
	if a.path != "" {
		out = append(out, a.path)
	}
	for _, s := range a.doc.Styles {
		if s.Src != "" {
			p := s.Src
			if !filepath.IsAbs(p) {
				p = filepath.Join(a.dir, p)
			}
			out = append(out, p)
		}
	}
	return out
}

// Document returns the parsed document (read-only use).
func (a *App) Document() *parse.Document { return a.doc }

// Sheets returns the loaded stylesheets.
func (a *App) Sheets() []*css.Sheet { return a.sheets }

// SetStrict upgrades B003 (missing bind path) to an error.
func (a *App) SetStrict(on bool) {
	a.mu.Lock()
	a.strict = on
	a.mu.Unlock()
}

// Bind stores v (coerced to JSON) at path; "" replaces the whole store,
// which must then be an object.
func (a *App) Bind(path string, v any) error {
	return a.set(path, v, false)
}

// Set is Bind plus a redraw request for a running app. The reserved paths
// "@focus" ("#id") and "@screen" ("id") move focus and switch screens;
// "@theme" ("dark", "light", or "auto") sets the theme the host chose
// (SPEC §18, §26.4).
func (a *App) Set(path string, v any) error {
	return a.set(path, v, true)
}

// ThemePath is the reserved Set path of the host's theme (SPEC §18).
const ThemePath = "@theme"

func (a *App) set(path string, v any, redraw bool) error {
	jv, err := ToJSON(v)
	if err != nil {
		return err
	}
	a.mu.Lock()
	err = a.applyLocked(path, jv)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	if redraw {
		a.Wake()
	}
	return nil
}

// applyLocked applies path=v (v already JSON-coerced) as Set does: the
// reserved paths' meanings, or a store write (SPEC §18, v0.3 §18.1). It is
// the primitive Set and Batch's atomic apply share; the caller holds a.mu.
func (a *App) applyLocked(path string, v any) error {
	switch path {
	case "@focus":
		s, _ := v.(string)
		a.requestFocus(strings.TrimPrefix(s, "#"), false)
	case "@screen":
		s, _ := v.(string)
		if !a.switchScreen(s) {
			return fmt.Errorf("tuimark: no screen with id %q", s)
		}
	case ThemePath:
		// SPEC §18: reserved in both versions; it does not live in the
		// store. Any other value is an error and changes nothing.
		s, ok := validThemeValue(v)
		if !ok {
			return fmt.Errorf("tuimark: %s must be \"dark\", \"light\", or \"auto\" (got %s)", ThemePath, jsonText(v))
		}
		a.hostTheme = s
	default:
		if path != "" && !validStorePath(path) {
			return fmt.Errorf("tuimark: bad path %q", path)
		}
		if path == "" {
			if _, ok := v.(map[string]any); !ok {
				return fmt.Errorf("tuimark: the root value must be a JSON object, got %T", v)
			}
		}
		root, err := assign(a.store, path, v)
		if err != nil {
			return err
		}
		a.store = root
		a.wrote(path)
	}
	return nil
}

// validThemeValue reports whether v is a legal @theme value (SPEC §18):
// "dark", "light", or "auto". Shared by Set's applyLocked and Batch.Set's
// queue-time check, so both give the same error.
func validThemeValue(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok && (s == "dark" || s == "light" || s == "auto")
}

// screenExists reports whether the document has a screen with this id, so
// Batch.Set can validate "@screen" without mutating anything (switchScreen
// applies the change as a side effect of checking it). No lock is needed:
// a.doc is immutable once Load/Parse/LoadFS returns (Files does the same).
func (a *App) screenExists(id string) bool {
	for _, s := range a.doc.Screens {
		if s.ID == id {
			return true
		}
	}
	return false
}

// wrote records a write of path into the store ("" is the whole store):
// every cached table whose rows read a path under the same top-level
// member, or any path after a whole-store write, computes them again.
// The caller holds a.mu.
func (a *App) wrote(path string) {
	if path == "" {
		a.storeGen++
		return
	}
	head, _, _ := strings.Cut(path, ".")
	a.segGen[head]++
}

func validStorePath(p string) bool {
	for _, seg := range strings.Split(p, ".") {
		if seg == "" {
			return false
		}
	}
	return true
}

// Get returns a deep copy of the value at path, JSON-shaped, or the app's
// current state for the three reserved paths (SPEC v0.3 §18.1):
//
//   - "@focus" is "#id" for the runtime's current focus (moved by every
//     input and focus request applied since the last frame Run or Play
//     built; a request the next frame rejects is undone then), nil when
//     nothing is focused or no frame has been built yet (last is nil:
//     Dump builds no live frame).
//   - "@screen" is the active screen's id, nil when it has none (the
//     spike root, or a screen without id).
//   - "@theme" is "dark" or "light": frameTheme reduces to toolTheme
//     (what Dump uses) outside Run, since runEnvTheme/runAuto are then
//     both "", so this is correct in both cases without a separate
//     "is Run active" flag.
//
// Any other path, "@" ones included, reads the store; a path that does
// not resolve, or is malformed, returns (nil, false).
func (a *App) Get(path string) (any, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch path {
	case "@focus":
		if a.last == nil || a.focus == "" {
			return nil, true
		}
		return "#" + a.focus, true
	case "@screen":
		if a.doc == nil || a.doc.Spike || a.screen < 0 || a.screen >= len(a.doc.Screens) {
			return nil, true
		}
		if id := a.doc.Screens[a.screen].ID; id != "" {
			return id, true
		}
		return nil, true
	case ThemePath:
		return a.frameTheme(), true
	}
	v, ok := lookup(a.store, path)
	if !ok {
		return nil, false
	}
	return deepCopyJSON(v), true
}

// Wake asks a running loop to redraw.
func (a *App) Wake() {
	a.wakeCount.Add(1)
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// WakeCount is how many times Wake has been called so far: an
// observation hook for tests (SPEC v0.3 §18.1 Batch: "at most one
// redraw"), not part of the public API.
func (a *App) WakeCount() int64 { return a.wakeCount.Load() }

// On registers a handler for a named action.
func (a *App) On(action string, h Handler) {
	a.mu.Lock()
	a.handlers[action] = h
	a.mu.Unlock()
}

// Builtins are the actions every runtime implements.
var Builtins = map[string]bool{"quit": true, "focus": true}

// isBuiltin reports whether a keymap action is built in: quit and focus,
// or, in a version="2" document, a hyphenated built-in of SPEC §8.4 (a
// version="1" document cannot name one: V003). Built-ins never produce
// B004 and are listed with Builtin: true.
func (a *App) isBuiltin(name string) bool {
	return Builtins[name] || (a.doc.V2 && ir.IsBuiltinActionV2(name))
}

// Catalog lists the actions the document references.
func (a *App) Catalog() []ActionSpec {
	a.mu.Lock()
	defer a.mu.Unlock()
	specs := map[string]*ActionSpec{}
	get := func(name string) *ActionSpec {
		s, ok := specs[name]
		if !ok {
			s = &ActionSpec{Name: name, Builtin: a.isBuiltin(name), Sources: []string{}}
			_, s.Registered = a.handlers[name]
			specs[name] = s
		}
		return s
	}
	for _, k := range a.doc.Keymap {
		if k.Action != "" {
			get(k.Action).Sources = append(get(k.Action).Sources, "keys "+k.KeysRaw)
		}
	}
	if a.doc.Root != nil {
		walkIR(a.doc.Root, func(n *ir.Node) {
			events := make([]string, 0, len(n.On))
			for ev := range n.On {
				events = append(events, ev)
			}
			sort.Strings(events)
			for _, ev := range events {
				who := n.Tag
				if n.ID != "" {
					who = "#" + n.ID
				}
				get(n.On[ev]).Sources = append(get(n.On[ev]).Sources, who+" on:"+ev)
			}
		})
	}
	out := make([]ActionSpec, 0, len(specs))
	for _, s := range specs {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func walkIR(n *ir.Node, fn func(*ir.Node)) {
	fn(n)
	for _, c := range n.Children {
		walkIR(c, fn)
	}
}

// CheckCatalog reports B004 for referenced actions missing from known.
func (a *App) CheckCatalog(known map[string]bool) ir.Diags {
	var out ir.Diags
	if a.doc.Root == nil {
		return out
	}
	for _, k := range a.doc.Keymap {
		if k.Action != "" && !a.isBuiltin(k.Action) && !known[k.Action] {
			out = append(out, ir.Diagnostic{Severity: ir.Warning, Code: "B004", Msg: fmt.Sprintf("action %q (keys %s) is not in the catalog", k.Action, k.KeysRaw), File: a.file, Line: k.Line, Col: k.Col, Path: "/keymap/bind"})
		}
	}
	walkIR(a.doc.Root, func(n *ir.Node) {
		for ev, act := range n.On {
			if !Builtins[act] && !known[act] {
				out = append(out, ir.At(n, a.file, ir.Warning, "B004", "action %q (on:%s) is not in the catalog", act, ev))
			}
		}
	})
	return out.Sorted()
}

// validateCols are the documented validate breakpoints (× 24 rows).
var validateCols = []int{40, 80, 120}

// Validate returns static diagnostics, V010 for every node of the document
// (checkDocks), plus layout/bind diagnostics at the documented breakpoints
// (40, 80, 120 columns × 24 rows), under the effective theme, or, for a
// theme="auto" document that nothing overrides, under both themes, dark
// first, each diagnostic reported once (SPEC §15.1). It leaves the
// runtime state untouched.
func (a *App) Validate() ir.Diags {
	a.mu.Lock()
	defer a.mu.Unlock()
	saved := a.saveState()
	defer a.restoreState(saved)
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
	if a.doc.Root != nil {
		add(a.checkDocks(themes))
		for _, th := range themes {
			for _, cols := range validateCols {
				add(a.render(cols, 24, th).Diags)
			}
		}
	}
	return out.Sorted()
}

// savedState is the runtime state a render advances. Validate and Dump
// render on a copy and put it back (Frame is the only call that keeps it).
type savedState struct {
	screen     int
	focus      string
	focusInit  bool
	focusReq   *focusRequest
	openModals []*ir.Node
	modalStack []modalEntry
	pending    []Event
	last       *Frame
	scrolls    map[string][2]int
	stickMax   map[string]int
	lists      map[string]*listState
	inputs     map[string]*inputState
	tabMem     map[string]string
	tabPrev    map[string]string
}

func (a *App) saveState() savedState {
	// Copies: a render edits the modal slices in place.
	om := append([]*ir.Node(nil), a.openModals...)
	ms := append([]modalEntry(nil), a.modalStack...)
	sc := map[string][2]int{}
	for k, v := range a.scrolls {
		sc[k] = v
	}
	sm := map[string]int{}
	for k, v := range a.stickMax {
		sm[k] = v
	}
	// Rendering creates list/input state, clamps selections, fills keys,
	// initializes cursors, and sets list pages for its size: keep copies.
	ls := map[string]*listState{}
	for k, v := range a.lists {
		c := *v
		c.keys = append([]any(nil), v.keys...)
		ls[k] = &c
	}
	in := map[string]*inputState{}
	for k, v := range a.inputs {
		c := *v
		in[k] = &c
	}
	var req *focusRequest
	if a.focusReq != nil {
		r := *a.focusReq
		req = &r
	}
	return savedState{a.screen, a.focus, a.focusInit, req, om, ms, append([]Event(nil), a.pending...), a.last, sc, sm, ls, in, copyStrings(a.tabMem), copyStrings(a.tabPrev)}
}

func (a *App) restoreState(s savedState) {
	a.screen, a.focus, a.focusInit, a.focusReq = s.screen, s.focus, s.focusInit, s.focusReq
	a.openModals, a.modalStack, a.pending, a.last = s.openModals, s.modalStack, s.pending, s.last
	a.scrolls, a.stickMax, a.lists, a.inputs = s.scrolls, s.stickMax, s.lists, s.inputs
	a.tabMem, a.tabPrev = s.tabMem, s.tabPrev
}

func copyStrings(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
