// Command kit-gallery runs examples/kit-gallery/app.tui, a gallery of every
// component in kit/ (badges, buttons, alerts, cards, stats, progress, forms,
// a data table, a log panel, dialogs, a command palette, toasts, a help
// overlay, a master/detail split) on five tabs.
//
//	go run ./examples/kit-gallery                    # from the repo root
//	go run ./examples/kit-gallery -dump 120x40       # print Dump() as JSON and exit
//	go run ./examples/kit-gallery -theme light       # Set("@theme", "light") before Run
//
// The view lives in app.tui and the kit's two stylesheets. This host holds
// no layout code. It implements the named actions the document uses and
// keeps what the markup reads:
//
//   - Derive: `found` (the palette's filtered commands) and `detail` (the
//     selected incident) are recomputed inside every store write, so typing
//     in the palette or moving the cursor updates them with no handler;
//   - Every: a once-a-second tick clears the toast and the status line;
//   - handlers: dialogs open and close by flipping a path, a form validates
//     into `forms.profile.*.error` paths, the log panel appends lines.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

func main() {
	dir := flag.String("dir", "examples/kit-gallery", "directory with app.tui, app.tcss, sample.json")
	dumpSize := flag.String("dump", "", "print Dump() as JSON at COLSxROWS and exit")
	theme := flag.String("theme", "", "dark or light: Set(\"@theme\", …) before Run")
	flag.Parse()
	if err := run(*dir, *dumpSize, *theme); err != nil {
		fmt.Fprintln(os.Stderr, "kit-gallery:", err)
		os.Exit(1)
	}
}

// Ticks (seconds) a toast and a status message stay on screen.
const (
	toastTTL  = 3
	statusTTL = 5
	idle      = "ready"
)

// app is the host's state. Handlers and Every ticks run on the runtime's
// loop goroutine, one at a time, so none of this needs a lock; a Derive
// function never touches it (it runs under the app's lock and reads the
// store through get).
type app struct {
	ui *tuimark.App

	toastLeft  int // ticks left before the toast clears
	statusLeft int // ticks left before the status line goes back to idle
	lines      int // log lines appended so far
}

func run(dir, dumpSize, theme string) error {
	a, err := load(dir)
	if err != nil {
		return err
	}
	a.register()
	if theme != "" {
		if err := a.setTheme(theme); err != nil {
			return err
		}
	}
	if dumpSize != "" {
		var cols, rows int
		if _, err := fmt.Sscanf(dumpSize, "%dx%d", &cols, &rows); err != nil {
			return fmt.Errorf("-dump wants COLSxROWS, got %q", dumpSize)
		}
		d, err := a.ui.Dump(cols, rows)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	stop := a.ui.Every(time.Second, a.tick)
	defer stop()
	return a.ui.RunWith(context.Background(), os.Stdout, tuimark.RunOptions{OnError: a.onError})
}

// load loads app.tui from dir and binds sample.json at the root.
func load(dir string) (*app, error) {
	ui, err := tuimark.Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "sample.json"))
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	if err := ui.Bind("", data); err != nil {
		return nil, err
	}
	a := &app{ui: ui}
	a.lines = len(listAt(ui.Get, "log.lines"))
	return a, nil
}

// register installs the derived values and every handler.
func (a *app) register() {
	if err := a.derive(); err != nil {
		panic(err) // a derived path that collides with the store: a programming error
	}
	for name, h := range a.handlers() {
		a.ui.On(name, h)
	}
}

// onError is RunOptions.OnError: show the failure as a danger toast and keep
// the session going.
func (a *app) onError(_ *tuimark.Event, err error) error {
	return a.ui.Batch(func(b *tuimark.Batch) error { return a.toast(b, "danger", "error: "+err.Error()) })
}

// tick is the Every function: it clears the toast and the status line when
// their time is up. Tests call it directly.
func (a *app) tick(b *tuimark.Batch) error {
	var errs []error
	if a.toastLeft > 0 {
		a.toastLeft--
		if a.toastLeft == 0 {
			errs = append(errs, b.Set("toast.show", false))
		}
	}
	if a.statusLeft > 0 {
		a.statusLeft--
		if a.statusLeft == 0 {
			errs = append(errs, b.Set("status.text", idle))
		}
	}
	return errors.Join(errs...)
}

// setTheme switches the runtime's theme and the name the app bar shows.
func (a *app) setTheme(name string) error {
	if name != "dark" && name != "light" {
		return fmt.Errorf("theme wants dark or light, got %q", name)
	}
	return errors.Join(a.ui.Set("@theme", name), a.ui.Set("theme.name", name))
}

// getter reads the store: App.Get, or the get a Derive function is given.
type getter func(path string) (any, bool)

// setter writes the store: *tuimark.App or *tuimark.Batch.
type setter interface {
	Set(path string, v any) error
}

func str(get getter, path string) string {
	v, _ := get(path)
	s, _ := v.(string)
	return s
}

func listAt(get getter, path string) []any {
	v, _ := get(path)
	l, _ := v.([]any)
	return l
}

func fieldOf(v any, key string) string {
	m, _ := v.(map[string]any)
	s, _ := m[key].(string)
	return s
}

// say writes the status line and starts its clock.
func (a *app) say(s setter, msg string) error {
	a.statusLeft = statusTTL
	return s.Set("status.text", msg)
}

// instanceOf is the instance a kit event belongs to: the id of its source
// before the last "__" (confirm__yes: confirm). A source with no "__" is its
// own instance.
func instanceOf(ev tuimark.Event) string {
	inst, _ := splitID(ev.Source)
	return inst
}

// partOf is the part of the event's source after the last "__" (confirm__yes:
// yes), or "" when the source has none.
func partOf(ev tuimark.Event) string {
	_, part := splitID(ev.Source)
	return part
}

func splitID(id string) (instance, part string) {
	if i := strings.LastIndex(id, "__"); i >= 0 {
		return id[:i], id[i+2:]
	}
	return id, ""
}

// toast shows a toast of a level (info, ok, warn, danger) and
// starts its clock; tick clears it.
func (a *app) toast(s setter, level, msg string) error {
	a.toastLeft = toastTTL
	return errors.Join(
		s.Set("toast.text", msg),
		s.Set("toast.is_ok", level == "ok"),
		s.Set("toast.is_warn", level == "warn"),
		s.Set("toast.is_danger", level == "danger"),
		s.Set("toast.show", true),
	)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func lower(s string) string { return strings.ToLower(s) }
