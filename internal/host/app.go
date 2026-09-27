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

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// Version is the runtime version: `tuimark version` prints it and the
// start record of TUIMARK_LOG carries it (SPEC v0.2b §26.12).
const Version = "0.2.0-a"

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

	screen     int
	focus      string
	focusInit  bool
	lists      map[string]*listState
	inputs     map[string]*inputState
	scrolls    map[string][2]int
	openModals []*ir.Node   // modals open in the last frame, document order (top last)
	modalStack []modalEntry // modals that took the focus trap, bottom first
	pending    []Event
	focusReq   *focusRequest
	// dirty is set by a built-in action of SPEC §8.4 that changed what the
	// live frame shows (TakeDirty).
	dirty bool
	wake  chan struct{}
	last  *Frame

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

func newApp(src []byte, path, dir string) *App {
	a := &App{
		path: path, dir: dir, file: filepath.Base(path),
		store: map[string]any{}, handlers: map[string]Handler{},
		lists: map[string]*listState{}, inputs: map[string]*inputState{},
		scrolls: map[string][2]int{}, wake: make(chan struct{}, 1),
		tokenDiags: map[string]ir.Diags{},
		segGen:     map[string]uint64{}, tables: map[*ir.Node]*tableCache{},
	}
	if path == "" {
		a.file = ""
	}
	a.doc = parse.Parse(src, path)
	a.static = append(a.static, a.doc.Diags...)
	a.loadStyles()
	return a
}

func (a *App) loadStyles() {
	seen := map[string]bool{}
	self := ""
	if a.path != "" {
		self, _ = filepath.Abs(a.path)
	}
	// A stylesheet is checked against the version of the document that
	// loads it (SPEC §5.1).
	v2 := a.doc.V2
	for _, s := range a.doc.Styles {
		if s.Src == "" {
			sh, diags := css.ParseInlineSheetIn(s.Body, a.file, s.BodyLine, s.BodyCol, v2)
			a.static = append(a.static, diags...)
			a.sheets = append(a.sheets, sh)
			continue
		}
		p := s.Src
		if !filepath.IsAbs(p) {
			p = filepath.Join(a.dir, p)
		}
		abs, _ := filepath.Abs(p)
		d := ir.Diagnostic{Severity: ir.Error, Code: "V006", File: a.file, Line: s.Line, Col: s.Col, Path: "/style"}
		switch {
		case abs == self || strings.HasSuffix(abs, ".tui"):
			d.Msg = fmt.Sprintf("style src=%q includes a .tui document (include cycle)", s.Src)
		case seen[abs]:
			d.Msg = fmt.Sprintf("style src=%q is included twice (include cycle)", s.Src)
		}
		if d.Msg != "" {
			a.static = append(a.static, d)
			continue
		}
		seen[abs] = true
		body, err := os.ReadFile(p)
		if err != nil {
			d.Msg = fmt.Sprintf("style src=%q cannot be read: %v", s.Src, errors.Unwrap(err))
			if errors.Unwrap(err) == nil {
				d.Msg = fmt.Sprintf("style src=%q cannot be read: %v", s.Src, err)
			}
			a.static = append(a.static, d)
			continue
		}
		sh, diags := css.ParseSheetIn(string(body), filepath.Base(p), v2)
		a.static = append(a.static, diags...)
		a.sheets = append(a.sheets, sh)
	}
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
	switch path {
	case "@focus":
		s, _ := jv.(string)
		a.requestFocus(strings.TrimPrefix(s, "#"), false)
	case "@screen":
		s, _ := jv.(string)
		if !a.switchScreen(s) {
			a.mu.Unlock()
			return fmt.Errorf("tuimark: no screen with id %q", s)
		}
	case ThemePath:
		// SPEC §18: reserved in both versions; it does not live in the
		// store. Any other value is an error and changes nothing.
		s, ok := jv.(string)
		if !ok || (s != "dark" && s != "light" && s != "auto") {
			a.mu.Unlock()
			return fmt.Errorf("tuimark: %s must be \"dark\", \"light\", or \"auto\" (got %s)", ThemePath, jsonText(jv))
		}
		a.hostTheme = s
	default:
		if path != "" && !validStorePath(path) {
			a.mu.Unlock()
			return fmt.Errorf("tuimark: bad path %q", path)
		}
		if path == "" {
			if _, ok := jv.(map[string]any); !ok {
				a.mu.Unlock()
				return fmt.Errorf("tuimark: the root value must be a JSON object, got %T", jv)
			}
		}
		root, err := assign(a.store, path, jv)
		if err != nil {
			a.mu.Unlock()
			return err
		}
		a.store = root
		a.wrote(path)
	}
	a.mu.Unlock()
	if redraw {
		a.Wake()
	}
	return nil
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

// Get returns a copy-free view of the value at path (hosts only).
func (a *App) Get(path string) (any, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return lookup(a.store, path)
}

// Wake asks a running loop to redraw.
func (a *App) Wake() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

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
	lists      map[string]*listState
	inputs     map[string]*inputState
}

func (a *App) saveState() savedState {
	// Copies: a render edits the modal slices in place.
	om := append([]*ir.Node(nil), a.openModals...)
	ms := append([]modalEntry(nil), a.modalStack...)
	sc := map[string][2]int{}
	for k, v := range a.scrolls {
		sc[k] = v
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
	return savedState{a.screen, a.focus, a.focusInit, req, om, ms, append([]Event(nil), a.pending...), a.last, sc, ls, in}
}

func (a *App) restoreState(s savedState) {
	a.screen, a.focus, a.focusInit, a.focusReq = s.screen, s.focus, s.focusInit, s.focusReq
	a.openModals, a.modalStack, a.pending, a.last = s.openModals, s.modalStack, s.pending, s.last
	a.scrolls, a.lists, a.inputs = s.scrolls, s.lists, s.inputs
}
