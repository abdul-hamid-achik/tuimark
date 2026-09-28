---
title: Go API
description: The Tuimark Go package - load a document (from disk or an embedded file system), bind and read data, handle actions, dump frames, test with Play and run in the terminal - with Event, ErrQuit and the reserved Set paths.
---

# Go API

The runtime is one Go package, `github.com/abdul-hamid-achik/tuimark`, with no CGO. A host program uses it to load a document, fill the data store, handle the actions the document names, and run it in the terminal. Layout, painting, focus, and terminal handling are internal and not importable.

```sh
go get github.com/abdul-hamid-achik/tuimark@latest
```

## The functions

```go
func Load(path string) (*App, error)
func LoadFS(fsys fs.FS, name string) (*App, error)            // 0.3
func Parse(r io.Reader) (*App, error)

func (a *App) Bind(path string, v any) error
func (a *App) Set(path string, v any) error
func (a *App) Get(path string) (any, bool)                     // 0.3
func (a *App) Batch(fn func(b *Batch) error) error              // 0.3
func (a *App) On(action string, h Handler)
func (a *App) Catalog() []ActionSpec
func (a *App) Dump(cols, rows int) (*Dump, error)
func (a *App) Validate() []Diagnostic
func (a *App) Play(opts PlayOptions, steps ...string) (*PlayResult, error) // 0.3
func (a *App) Run(w io.Writer) error
```

That is the whole surface, plus the types they use: `Event`, `Handler`, `ActionSpec`, `Dump`, `DumpNode`, `Diagnostic`, `ErrQuit`, and, since 0.3, `Batch`, `PlayOptions`, `PlayEvent`, and `PlayResult`.

## Loading

`Load` reads a `.tui` file; `<style src>` paths resolve relative to it. `Parse` reads a document from an `io.Reader`, with `src` paths relative to the working directory.

`LoadFS` (0.3) reads a document from an `fs.FS`. Its main use is shipping the view inside the binary with `go:embed`:

```go
//go:embed ui/app.tui ui/theme.tcss
var views embed.FS

ui, err := tuimark.LoadFS(views, "ui/app.tui") // <style src="theme.tcss"/> is ui/theme.tcss
```

A relative `<style src>` resolves inside the file system, next to the document. A `src` that climbs out of it is a `V006` diagnostic.

None of them fails because of what is in the document: a document with errors still loads. The error is only for I/O. Read the problems with `Validate()`, or from a dump's `Errors`.

## Data

The store is JSON-shaped: `null`, booleans, numbers, strings, arrays, and objects. Both `Bind` and `Set` convert what you pass (structs with `json` tags, maps, slices, numbers) to that shape, and reject what has no JSON form, such as channels, functions, `NaN`, and infinities.

- `Bind(path, v)` stores `v` at a dotted path; the empty path replaces the whole store.
- `Set(path, v)` does the same for a running app, schedules a redraw, and is safe to call from any goroutine, such as a ticker that refreshes data every second.

```go
_ = ui.Bind("", data)                         // the whole store
_ = ui.Set("status", "deploying")             // one member
_ = ui.Set("selected_ticket", tickets[0])     // a struct, as JSON
_ = ui.Set("steps.2.status", "done")          // host paths may index arrays
```

A host path may use a number to reach into an array, as above; markup paths cannot, and reach arrays through `each`.

`Get(path)` (0.3) reads a value back as a copy: changing what it returns never changes the store. It uses the same paths, and `@focus`, `@screen`, and `@theme` return the app's current focus (`"#id"`), screen, and theme.

`Batch` (0.3) applies several writes as one change. Use it to publish one sample of live data, so no frame shows half of it:

```go
err := ui.Batch(func(b *tuimark.Batch) error {
	_ = b.Set("cpu", cpu)
	_ = b.Set("memory", mem)
	return b.Set("processes", procs)
})
```

The writes are applied together and in order, and they cause a single redraw. If any of them fails, or `fn` returns an error, none is applied. `fn` runs without holding the app's lock, so it can call `Get` to compute what to write.

### Reserved paths

Three paths starting with `@` are not in the store. They let the host drive the runtime:

| Path | Value | Does |
|---|---|---|
| `@focus` | `"#id"` | moves focus to a node; into an inactive tab, it activates the tab |
| `@screen` | `"id"` | switches the active screen |
| `@theme` | `"dark"`, `"light"`, `"auto"` | sets the theme, over the document's `theme` (`TUIMARK_THEME` still wins in `Run`) |

An invalid value returns an error and changes nothing.

## Actions

`On` registers the handler for an action name:

```go
type Handler func(Event) error

type Event struct {
	Action string         // the action name
	Source string         // the id of the node that fired it
	Keys   map[string]any // the each-aliases of the row involved: {"t": "t-12"}
	Value  any            // an input's text, a new checked array, a tab id, or nil
}
```

```go
ui.On("open", func(ev tuimark.Event) error {
	id, _ := ev.Keys["t"].(string)
	return ui.Set("current", lookup(id))
})
ui.On("search", func(ev tuimark.Event) error {
	q, _ := ev.Value.(string)
	return ui.Set("tickets", filter(all, q))
})
```

A handler that returns `ErrQuit` stops `Run` cleanly; any other error stops it and is returned. The built-in `quit` action (a keymap row, or `ctrl+c` when nothing claims it) stops `Run` by itself when no `quit` handler is registered; register one to confirm first, and return `ErrQuit` when it should really quit.

Handlers run on the runtime's goroutine, one at a time. Long work belongs in a goroutine of its own that reports back with `Set`.

## Catalog

`Catalog()` lists every action the document names, with where it is used (`Sources`), whether a handler is `Registered`, and whether it is a built-in (`Builtin`). Use it in a test to check that every action the view fires has a handler.

## Dump and Validate

`Dump(cols, rows)` renders the current state as a frame (the same data as `tuimark dump --format json`), and `Validate()` returns every diagnostic, laying the document out at 40, 80, and 120 columns. Both are pure: they never move focus, fire events, or change what `Run` starts from, so you can call them at any size, in any order, as often as you like. `Dump` uses the document's theme (or `@theme`) and ignores the environment.

That makes them the natural base for Go tests of a view:

```go
func TestTasksFit(t *testing.T) {
	ui, err := tuimark.Load("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	if err := ui.Bind("", map[string]any{
		"project":  "launch checklist",
		"selected": "t1",
		"tasks":    []map[string]any{{"id": "t1", "title": "write the release notes"}},
	}); err != nil {
		t.Fatal(err)
	}
	for _, d := range ui.Validate() {
		if d.Severity == "error" {
			t.Errorf("validate: %s", d)
		}
	}
	frame, err := ui.Dump(48, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !frame.OK {
		t.Fatalf("dump errors: %v", frame.Errors)
	}
	for _, n := range frame.Nodes {
		if n.ID == "tasks" && n.H != 3 {
			t.Errorf("tasks is %d rows tall, want 3 (one row and its border)", n.H)
		}
	}
}
```

## Play

`Play` (0.3) is `Run` without a terminal, for Go tests. Each step is one step of [`tuimark play`](/guide/tools): a key, `text:…`, `paste:…`, `set:…`, `focus:…`, `resize:…`, `click:X,Y`, or `wheel-up:X,Y`/`wheel-down:X,Y`. A step may contain spaces (`"text:hello world"`).

```go
res, err := ui.Play(tuimark.PlayOptions{Cols: 120, Rows: 30}, "7", "/", "text:fire", "enter", "K")
if err != nil {
	t.Fatal(err)
}
// res.Events: the actions that fired, in order, with their step, keys, and value
// res.Dump:   the final frame, as Dump returns it
// res.Quit:   whether a quit ended the call
```

- `Play` drives the app itself and calls the handlers you registered with `On`, exactly as `Run` would. The state carries over to the next `Play` or `Run`, so a test can play a few keys, check the store with `Get`, and play more.
- Every step is checked before any runs: a bad step returns an error and changes nothing. A handler that returns `ErrQuit` ends the call with `Quit: true`. Any other handler error ends the call and is returned together with the events so far.
- Every call starts by rendering and settling what changed since the last one, so `ui.Play(opts)` with no steps picks up writes a handler made from another goroutine.
- With `NoHandlers: true` on a freshly loaded app, `Play` gives exactly what `tuimark play` prints. The CLI and `Play` share one engine.
- `Play` and `Run` exclude each other: calling one while the other is active returns an error.

## Run

`Run(w)` puts the terminal in raw mode, reads keys (and, when the document turns it on, the mouse) from stdin, and draws frames to `w`, usually `os.Stdout`, until the app quits. On the way it:

- asks the terminal what it supports (colors, synchronized output, its background for `theme="auto"`) and paints only the cells that changed;
- delivers pastes as one edit, and never lets a paste or a terminal reply trigger a key binding;
- restores the terminal (cooked mode, main screen, visible cursor) however it ends.

It returns `nil` on quit. On `SIGINT`, `SIGTERM`, or `SIGHUP` it restores the terminal first and returns an error naming the signal, so the program can exit with a failure status. It reads the `TUIMARK_*` variables described in [environment variables](/guide/tools#environment-variables).

## A complete host

The [getting started](/guide/getting-started#run-it) page builds one step by step. The repository's examples show larger ones: `examples/inbox` (search and selection), `examples/dashboard` (screens and a confirm modal), `examples/monitor` (a live collector, sorting, filtering, settings that switch the mouse and the theme), and `examples/agent` (a coding-agent chat with streaming replies and tool approvals). None of them has any layout, width, or color code: see [Examples](/examples).
