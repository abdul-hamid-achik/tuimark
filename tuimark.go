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
// The public surface is deliberately small: Load, Parse, Bind, Set, On,
// Catalog, Dump, Validate, Run.
package tuimark

import (
	"fmt"
	"io"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
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
// "@screen" ("id") move focus and switch screens.
func (a *App) Set(path string, v any) error { return a.h.Set(path, v) }

// On registers the handler for a named action.
func (a *App) On(action string, h Handler) { a.h.On(action, h) }

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
// nothing is left reading stdin when it returns. Printable keys that
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
