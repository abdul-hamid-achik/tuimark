// palette: a command registry behind kit/components/palette.tui.
//
// Copy this file into your project and change the package name. It depends
// on github.com/abdul-hamid-achik/tuimark, and on filter.go (Filter): copy
// that file with this one.
//
// The component reads palette.commands (the registry, [{id, label, hint}]),
// palette.query, found.items and found.any, and reads and writes palette.open
// and palette.cur. This helper writes the registry from your Commands, keeps
// found with Filter (a Derive, so the list follows the query as the user
// types), and implements the actions:
//
//	palette_open   resets the query and the cursor, opens the palette
//	palette_close  closes it
//	palette_run    closes it and runs the command under the cursor (or the
//	               first match), and writes its title to palette.last
//	palette_query  the input's on:change: nothing to do, Derive did it
package kit

import (
	"errors"
	"sync"

	"github.com/abdul-hamid-achik/tuimark"
)

// Command is one entry of the palette.
type Command struct {
	ID    string       // unique; the list's row key
	Title string       // what the row shows and the query matches
	Key   string       // the shortcut shown at the right ("d", "^R"), or ""
	Run   func() error // what the command does
}

// PaletteOptions configures NewPalette.
type PaletteOptions struct {
	Path  string // the palette's store object (default "palette")
	Found string // the filtered list the component reads (default "found")
	Mode  Mode   // Substring (the default) or Fuzzy
}

// Palette is the registry and its handlers.
type Palette struct {
	app  *tuimark.App
	opts PaletteOptions

	mu   sync.Mutex
	cmds []Command
}

// NewPalette registers cmds and the derived found list. It writes
// palette.commands, and palette.open, .query, .cur and .last when the store
// has none of them.
func NewPalette(app *tuimark.App, opts PaletteOptions, cmds ...Command) (*Palette, error) {
	if opts.Path == "" {
		opts.Path = "palette"
	}
	if opts.Found == "" {
		opts.Found = "found"
	}
	p := &Palette{app: app, opts: opts}
	if err := Filter(app, FilterOptions{
		Source: opts.Path + ".commands", Query: opts.Path + ".query", Out: opts.Found,
		Fields: []string{"label", "id"}, Mode: opts.Mode,
	}); err != nil {
		return nil, err
	}
	if err := p.Add(cmds...); err != nil {
		return nil, err
	}
	return p, app.Batch(func(b *tuimark.Batch) error {
		var errs []error
		for name, v := range map[string]any{"open": false, "query": "", "cur": p.first(), "last": ""} {
			if _, ok := app.Get(opts.Path + "." + name); !ok {
				errs = append(errs, b.Set(opts.Path+"."+name, v))
			}
		}
		return errors.Join(errs...)
	})
}

// Add appends commands to the registry and rewrites palette.commands. A
// command whose ID is already there replaces it.
func (p *Palette) Add(cmds ...Command) error {
	p.mu.Lock()
	defer p.mu.Unlock() // held across the Set, so two Adds reach the store in the order they changed the registry
	for _, c := range cmds {
		replaced := false
		for i := range p.cmds {
			if p.cmds[i].ID == c.ID {
				p.cmds[i], replaced = c, true
			}
		}
		if !replaced {
			p.cmds = append(p.cmds, c)
		}
	}
	rows := make([]any, len(p.cmds))
	for i, c := range p.cmds {
		rows[i] = map[string]any{"id": c.ID, "label": c.Title, "hint": c.Key}
	}
	return p.app.Set(p.opts.Path+".commands", rows)
}

func (p *Palette) first() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.cmds) == 0 {
		return ""
	}
	return p.cmds[0].ID
}

// Register installs the four handlers.
func (p *Palette) Register() {
	p.app.On("palette_open", p.Open)
	p.app.On("palette_close", p.Close)
	p.app.On("palette_run", p.Run)
	p.app.On("palette_query", func(tuimark.Event) error { return nil })
}

// Open is the handler of palette_open.
func (p *Palette) Open(tuimark.Event) error {
	return p.app.Batch(func(b *tuimark.Batch) error {
		return errors.Join(
			b.Set(p.opts.Path+".query", ""), b.Set(p.opts.Path+".cur", p.first()),
			b.Set(p.opts.Path+".open", true),
		)
	})
}

// Close is the handler of palette_close.
func (p *Palette) Close(tuimark.Event) error { return p.app.Set(p.opts.Path+".open", false) }

// Run is the handler of palette_run: it runs the command under the
// cursor, or the first match when the cursor's row was filtered away. With no
// match it does nothing and the palette stays open, where its empty line
// says why.
func (p *Palette) Run(tuimark.Event) error {
	cur, _ := p.app.Get(p.opts.Path + ".cur")
	curID, _ := cur.(string)
	items, _ := p.app.Get(p.opts.Found + ".items")
	rows, _ := items.([]any)
	id := ""
	for _, row := range rows {
		m, _ := row.(map[string]any)
		rid, _ := m["id"].(string)
		if id == "" {
			id = rid
		}
		if rid == curID {
			id = rid
			break
		}
	}
	if id == "" {
		return nil
	}
	p.mu.Lock()
	var cmd Command
	for _, c := range p.cmds {
		if c.ID == id {
			cmd = c
		}
	}
	p.mu.Unlock()
	if err := p.app.Batch(func(b *tuimark.Batch) error {
		return errors.Join(b.Set(p.opts.Path+".open", false), b.Set(p.opts.Path+".last", cmd.Title))
	}); err != nil {
		return err
	}
	if cmd.Run != nil {
		return cmd.Run()
	}
	return nil
}
