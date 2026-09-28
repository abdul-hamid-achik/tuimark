// Package tuimark is a view language for terminals.
//
// Humans and coding agents author UI as text — structure in .tui (XML),
// style in .tcss (reduced CSS), data as JSON, behavior as named actions —
// and this runtime interprets it: layout on an integer cell grid, Unicode
// paint, and a machine-readable dump.
//
//	ui, err := tuimark.Load("app.tui")
//	if err != nil { return err }
//	_ = ui.Bind("tickets", tickets)
//	ui.On("open", handleOpen)
//	ui.On("quit", func(tuimark.Event) error { return tuimark.ErrQuit })
//	return ui.Run(os.Stdout)
//
// The public surface is deliberately small: Load, LoadFS, Parse, Bind,
// Set, Get, Batch, On, Catalog, Dump, Validate, Play, Run.
package tuimark

import (
	"fmt"
	"io"
	"io/fs"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/play"
)

// Event is the payload a handler receives: the action name, the id of the
// node that fired it, each-alias keys of the selected list item, and the
// input value for change/submit.
type Event = host.Event

// Handler handles a named action. Return ErrQuit to stop Run.
type Handler = host.Handler

// ActionSpec describes an action the document references.
type ActionSpec = host.ActionSpec

// Dump is a rendered frame: grid (no ANSI), node geometry, and diagnostics.
type Dump = dump.Dump

// DumpNode is one laid-out node in a Dump.
type DumpNode = dump.Node

// Diagnostic is a parse, layout, or bind problem.
type Diagnostic = ir.Diagnostic

// ErrQuit stops Run cleanly when returned by a handler.
var ErrQuit = host.ErrQuit

// App is a loaded document plus its data and runtime state.
type App struct {
	h *host.App
}

// Load reads a .tui document from disk. Diagnostics in the document do not
// fail Load; read them with Validate or Dump. The error is for I/O.
func Load(path string) (*App, error) {
	h, err := host.Load(path)
	if err != nil {
		return nil, err
	}
	return &App{h: h}, nil
}

// LoadFS reads a .tui document named name from fsys and is otherwise
// Load: name must satisfy fs.ValidPath, or the error wraps fs.ErrInvalid;
// a read error is returned as an I/O error, like Load's. A relative
// <style src> resolves inside fsys as path.Join(path.Dir(name), src); an
// absolute src, or one whose joined path climbs above fsys's root, is a
// V006 diagnostic ("style src=... is outside the file system") instead
// of an I/O error, so the document still loads and reports it; any other
// src failure (missing, a .tui, included twice) is V006 as for Load.
// Diagnostics name the document and its stylesheets by their base names,
// as Load does, so the same files give the same diagnostics through
// either function. Load itself is unchanged: it still resolves src
// against the directory of the file on disk, .. included.
func LoadFS(fsys fs.FS, name string) (*App, error) {
	h, err := host.LoadFS(fsys, name)
	if err != nil {
		return nil, err
	}
	return &App{h: h}, nil
}

// Parse reads a .tui document from r. Relative <style src> paths resolve
// against the working directory.
func Parse(r io.Reader) (*App, error) {
	h, err := host.Parse(r)
	if err != nil {
		return nil, err
	}
	return &App{h: h}, nil
}

// Bind stores v at a dotted path in the JSON store. v is coerced to JSON
// (null, bool, number, string, array, object). The empty path replaces the
// whole store with an object.
func (a *App) Bind(path string, v any) error { return a.h.Bind(path, v) }

// Set is Bind for a running app: it also schedules a redraw and is safe to
// call from any goroutine. The reserved paths "@focus" ("#id") and
// "@screen" ("id") move focus and switch screens; "@theme" ("dark",
// "light", or "auto") sets the host's theme, which beats the document's
// theme attribute in Dump, Validate, and Run (TUIMARK_THEME still wins in
// Run); any other value is an error and changes nothing.
func (a *App) Set(path string, v any) error { return a.h.Set(path, v) }

// Get returns a deep copy of the value at path, JSON-shaped (nil, bool,
// float64, string, []any, or map[string]any): mutating the returned
// value never changes the store. path has the grammar of Bind and Set
// (dotted, numeric segments index arrays); the empty path returns a copy
// of the whole store. A path that does not resolve, or is malformed,
// returns (nil, false).
//
// The three reserved Set paths return the app's current state instead of
// reading the store:
//
//   - "@focus" is "#id" for the runtime's current focus — moved by every
//     input and focus request applied since the last frame Run or Play
//     built, undone when the next frame rejects it — or nil when nothing
//     is focused or no frame has been built yet (Dump builds no live
//     frame).
//   - "@screen" is the active screen's id, or nil when it has none.
//   - "@theme" is "dark" or "light": the session's theme while Run is
//     active, else the theme Dump uses (the host's @theme, else the
//     document's, with auto as dark).
//
// Any other path, another "@" one included, reads the store, where Set
// writes it. Get is safe to call from any goroutine, including a handler.
func (a *App) Get(path string) (any, bool) { return a.h.Get(path) }

// On registers the handler for a named action.
func (a *App) On(action string, h Handler) { a.h.On(action, h) }

// Batch is a set of Set calls Batch applies together; see App.Batch.
type Batch = host.Batch

// Batch calls fn once, with a fresh *Batch that runs without the app's
// lock, so fn may call Get, Set, or an inner Batch (which applies on its
// own, before this one, since neither goes through b). Every b.Set call
// is validated as Set validates it (JSON coercion, path grammar, the
// reserved paths' values); when fn returns nil and every b.Set succeeded,
// the queued writes are applied in order, atomically — no frame, dump,
// Get, handler, or Play step ever observes some of them applied and
// others not — with at most one redraw request. When fn returns an
// error, or any b.Set failed (even if fn ignored it and returned nil),
// nothing is applied and Batch returns the first such error. A b.Set
// called on a *Batch kept after fn has returned also returns an error
// and changes nothing. Batch is safe to call from any goroutine,
// including a handler.
func (a *App) Batch(fn func(b *Batch) error) error { return a.h.Batch(fn) }

// Catalog lists the actions the document references, with where they are
// used and whether a handler is registered.
func (a *App) Catalog() []ActionSpec { return a.h.Catalog() }

// Dump renders the current state at cols×rows. It is a snapshot: it does
// not move focus, fire events, or change what Run starts from, so dumps at
// several sizes do not depend on their order. A negative size is an error;
// 0 gives an empty grid. The dump's grid always has exactly rows lines of
// cols cells.
func (a *App) Dump(cols, rows int) (*Dump, error) {
	if cols < 0 || rows < 0 {
		return nil, fmt.Errorf("tuimark: Dump size %dx%d is negative", cols, rows)
	}
	return a.h.Dump(cols, rows, false), nil
}

// Validate returns every diagnostic: parse and stylesheet problems in the
// whole document (every screen, closed modals, false if= branches, list
// templates), and layout/bind problems at 40, 80, and 120 columns.
func (a *App) Validate() []Diagnostic { return a.h.Validate() }

// Run drives the app in the terminal until quit. Input is read from
// stdin in raw mode; frames are written to w. It returns nil on quit, and
// nothing is left reading stdin when it returns. A version="2" document's
// mouse attribute turns mouse reporting on while it is true (left clicks
// and the wheel act; see the SPEC's mouse section). Printable keys that
// arrive together for the focused input (a paste) are typed as one edit
// with one on:change carrying the final value. SIGTERM, SIGHUP, and
// SIGINT also stop it: the terminal is restored first (cooked mode, main
// screen, visible cursor) and Run returns a non-nil error naming the
// signal, so the program can exit with a failure status, even when a
// handler returned ErrQuit as the signal came in. A handler that is still
// running when the signal arrives does not hold the terminal: it is
// restored right away, and if Run has not returned within a second, or a
// second signal arrives, the process ends with the signal's default action.
func (a *App) Run(w io.Writer) error { return a.h.Run(w) }

// PlayEvent is one action Play dispatched; it marshals exactly as
// tuimark play's events do: {"step","action","source","keys","value"}.
type PlayEvent = dump.Event

// PlayOptions configures Play. Cols and Rows each default on their own
// (0 is 80 for Cols, 24 for Rows, as for tuimark play); after defaulting
// each must be 1-1000. NoHandlers records host actions without calling
// their registered handlers, as tuimark play always does (a CLI-loaded
// app never has any). Styles and Cells add the dump's theme/styles and
// cells fields, as the --styles and --cells flags do.
type PlayOptions struct {
	Cols, Rows int
	NoHandlers bool
	Styles     bool
	Cells      bool
}

// PlayResult is what one Play call did: Events holds every event
// dispatched during the call, in order, step 0 included; Dump is the
// last frame built, at the size in effect then; Quit reports whether the
// call ended with a quit.
type PlayResult struct {
	Events []PlayEvent
	Dump   *Dump
	Quit   bool
}

// Play drives the app itself, headless, exactly as Run would from the
// same input, and returns what happened. The app's state persists across
// calls: a later Play or Run continues from it, and Dump/Validate leave
// it untouched. Every call starts with step 0: the live frame is
// rendered at the size and settled, dispatching any lifecycle events a
// previous call left queued; steps 1..n are then applied and settled the
// same way. Each string in steps is exactly one step of tuimark play's
// --input grammar (a key token, or text:/paste:/set:/focus:/resize:/
// click:/wheel-up:/wheel-down:) — nothing splits it on spaces, so
// "text:hello world" types a space — and every step is parsed before any
// is applied: an invalid one returns an error naming its index and step,
// a nil result, and does nothing, step 0 included.
//
// Unless opts.NoHandlers is set, the handler registered with On for a
// dispatched action is called synchronously, in the order Run's loop
// would call it. A handler that returns ErrQuit, or the built-in quit
// with no handler, ends the call with Quit: true: the remaining events of
// that round and the remaining steps are not applied, and lifecycle
// events already queued stay queued for the next call's step 0. A
// handler that returns any other error ends the call the same way, and
// Play returns that error together with the result so far (both
// non-nil). A step that cannot be applied when its turn comes (a set:
// Set refuses, a mouse cell outside the grid) also ends the call: Play
// returns the error "step N (STEP): reason" together with the result so
// far, and the earlier steps stay applied.
//
// The theme is the one Dump uses (the host's @theme, else the
// document's, with auto as dark); Play never probes a terminal or reads
// an environment variable. Play and Run exclude each other: a call made
// while Run or another Play is active on the app returns an error, a nil
// result, and does nothing; Run called while Play is active returns an
// error before it looks at the terminal.
func (a *App) Play(opts PlayOptions, steps ...string) (*PlayResult, error) {
	res, err := play.Play(a.h, play.Options{
		Cols: opts.Cols, Rows: opts.Rows,
		NoHandlers: opts.NoHandlers, Styles: opts.Styles, Cells: opts.Cells,
	}, steps)
	if res == nil {
		return nil, err
	}
	return &PlayResult{Events: res.Events, Dump: res.Dump, Quit: res.Quit}, err
}
