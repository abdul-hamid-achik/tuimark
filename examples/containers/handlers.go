package main

import (
	"errors"
	"fmt"

	"github.com/abdul-hamid-achik/tuimark"
)

func (a *app) register() error {
	if err := a.derive(); err != nil {
		return err
	}
	for name, h := range a.handlers() {
		a.ui.On(name, h)
	}
	return nil
}

// handlers are the named actions of app.tui. The built-ins (quit, focus,
// move-next, switch-to, check-toggle, …) are the runtime's and have none.
func (a *app) handlers() map[string]tuimark.Handler {
	hs := map[string]tuimark.Handler{}
	on := func(name string, h tuimark.Handler) { hs[name] = h }

	// The filter and the sort order.
	on("filter_change", func(tuimark.Event) error { return a.repairCursor(0) })
	on("filter_clear", func(tuimark.Event) error {
		if err := a.ui.Set("filter.q", ""); err != nil {
			return err
		}
		return errors.Join(a.repairCursor(0), a.ui.Set("@focus", "#ctrs"))
	})
	on("container_sort", func(tuimark.Event) error { return a.sortNext() })

	// Containers: stop, restart and remove ask first; start just starts.
	on("container_stop", func(tuimark.Event) error { return a.ask("stop") })
	on("container_restart", func(tuimark.Event) error { return a.ask("restart") })
	on("container_remove", func(tuimark.Event) error { return a.ask("remove") })
	on("container_start", func(tuimark.Event) error { return a.start() })
	on("image_prune", func(tuimark.Event) error { return a.ask("prune-images") })
	on("volume_prune", func(tuimark.Event) error { return a.ask("prune-volumes") })

	// The confirm dialog. The actions are the kit's generic ones; the
	// instance is the prefix of the source before the last "__".
	on("dialog_cancel", func(ev tuimark.Event) error {
		a.pending = nil
		return a.ui.Batch(func(b *tuimark.Batch) error {
			return errors.Join(b.Set(instanceOf(ev)+".open", false), a.say(b, "cancelled"))
		})
	})
	on("dialog_confirm", func(ev tuimark.Event) error {
		p := a.pending
		a.pending = nil
		if err := a.ui.Set(instanceOf(ev)+".open", false); err != nil {
			return err
		}
		if p == nil {
			return nil
		}
		return a.perform(p)
	})

	// The help overlay, the palette, the theme and the toast.
	on("help_open", func(tuimark.Event) error { return a.ui.Set("help.open", true) })
	on("help_close", func(tuimark.Event) error { return a.ui.Set("help.open", false) })
	on("palette_open", func(tuimark.Event) error {
		return a.ui.Batch(func(b *tuimark.Batch) error {
			return errors.Join(b.Set("palette.query", ""), b.Set("palette.cur", "stop"), b.Set("palette.open", true))
		})
	})
	on("palette_close", func(tuimark.Event) error { return a.ui.Set("palette.open", false) })
	on("palette_run", func(tuimark.Event) error { return a.paletteRun() })
	// palette_query is the k-palette input's on:change. `found` is a Derive,
	// which the runtime recomputes on that write: nothing to do.
	on("palette_query", func(tuimark.Event) error { return nil })
	on("theme_toggle", func(tuimark.Event) error { return a.toggleTheme() })
	on("toast_dismiss", func(tuimark.Event) error {
		a.toastLeft = 0
		return a.ui.Set("toast.show", false)
	})
	return hs
}

// sortNext moves to the next sort order of the `o` cycle.
func (a *app) sortNext() error {
	cur := str(a.ui.Get, "sort.key")
	next := sortKeys[0]
	for i, k := range sortKeys {
		if k.key == cur {
			next = sortKeys[(i+1)%len(sortKeys)]
		}
	}
	return a.ui.Batch(func(b *tuimark.Batch) error {
		return errors.Join(b.Set("sort.key", next.key), a.say(b, "sorted by "+next.label))
	})
}

// ---- the cursor and the selection ----

// rowIDs are the ids of the visible rows, in order.
func (a *app) rowIDs() []string {
	var ids []string
	for _, r := range listAt(a.ui.Get, "rows") {
		ids = append(ids, field(r, "id"))
	}
	return ids
}

// repairCursor puts the cursor back on a visible row when its container was
// filtered away or removed: on the row at index hint, clamped. A table whose
// bound key is not among its rows draws no cursor at all, so the host keeps
// the two in step.
func (a *app) repairCursor(hint int) error {
	ids := a.rowIDs()
	cur := str(a.ui.Get, "cur")
	for _, id := range ids {
		if id == cur {
			return nil
		}
	}
	next := ""
	if len(ids) > 0 {
		if hint >= len(ids) {
			hint = len(ids) - 1
		}
		next = ids[hint]
	}
	return a.ui.Set("cur", next)
}

// targets are the containers an action works on: on the containers tab the
// marked rows that the filter shows (marks on hidden rows are kept but never
// acted on), else the one under the cursor; elsewhere the cursor's.
func (a *app) targets() []*container {
	if str(a.ui.Get, "view") == "containers" {
		var out []*container
		visible := map[string]bool{}
		for _, id := range a.rowIDs() {
			visible[id] = true
		}
		for _, id := range listAt(a.ui.Get, "sel") {
			s, _ := id.(string)
			if c := a.sim.byID(s); c != nil && visible[s] {
				out = append(out, c)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if c := a.sim.byID(str(a.ui.Get, "cur")); c != nil {
		return []*container{c}
	}
	return nil
}

func ids(cs []*container) (is, ns []string) {
	for _, c := range cs {
		is = append(is, c.id)
		ns = append(ns, c.name)
	}
	return is, ns
}

// ---- asking and doing ----

// ask opens the confirm dialog for an operation, or says why it cannot.
func (a *app) ask(kind string) error {
	p := &pending{kind: kind}
	var title, body string
	danger := false
	switch kind {
	case "stop", "restart", "remove":
		cs := a.targets()
		if len(cs) == 0 {
			return a.ui.Batch(func(b *tuimark.Batch) error { return a.toast(b, "warn", "No container selected") })
		}
		p.ids, p.names = ids(cs)
		who := names(p.names)
		if len(cs) > 1 {
			who = plural(len(cs), "container", "containers")
		}
		switch kind {
		case "stop":
			title = "Stop " + who + "?"
			body = names(p.names) + " will get SIGTERM and stop. Nothing is removed; start brings it back."
		case "restart":
			title = "Restart " + who + "?"
			body = names(p.names) + " will go down for a moment and come back. Open connections are dropped."
		case "remove":
			title, danger = "Remove "+who+"?", true
			body = names(p.names) + " and its writable layer will be deleted. Volumes are kept. This cannot be undone."
		}
	case "prune-images":
		n := len(a.sim.unusedImages())
		if n == 0 {
			return a.ui.Batch(func(b *tuimark.Batch) error { return a.toast(b, "info", "No unused images") })
		}
		var mb float64
		for _, im := range a.sim.unusedImages() {
			mb += im.mb
		}
		title, danger = "Remove "+plural(n, "unused image", "unused images")+"?", true
		body = "Frees about " + fmtSize(mb) + ". Images that a container uses are kept."
	case "prune-volumes":
		n := len(a.sim.unusedVolumes())
		if n == 0 {
			return a.ui.Batch(func(b *tuimark.Batch) error { return a.toast(b, "info", "No unused volumes") })
		}
		var mb float64
		for _, v := range a.sim.unusedVolumes() {
			mb += v.mb
		}
		title, danger = "Remove "+plural(n, "unused volume", "unused volumes")+"?", true
		body = "Deletes their data and frees about " + fmtSize(mb) + ". This cannot be undone."
	}
	a.pending = p
	return a.ui.Batch(func(b *tuimark.Batch) error {
		return errors.Join(
			b.Set("confirm.title", title), b.Set("confirm.body", body),
			// k-confirm reads its button labels as data, so each open writes
			// them: one dialog serves every operation.
			b.Set("confirm.yes", "Confirm"), b.Set("confirm.no", "Cancel"),
			b.Set("confirm.danger", danger), b.Set("confirm.open", true),
		)
	})
}

// start starts the targets that are not running.
func (a *app) start() error {
	cs := a.targets()
	if len(cs) == 0 {
		return a.ui.Batch(func(b *tuimark.Batch) error { return a.toast(b, "warn", "No container selected") })
	}
	var started []string
	for _, c := range cs {
		if a.sim.start(c) {
			started = append(started, c.name)
		}
	}
	return a.ui.Batch(func(b *tuimark.Batch) error {
		if len(started) == 0 {
			return a.toast(b, "info", "Already running")
		}
		msg := "Started " + names(started)
		return errors.Join(a.write(b), a.toast(b, "ok", msg), a.say(b, msg))
	})
}

// perform does a confirmed operation and reports it in a toast and the
// status bar.
func (a *app) perform(p *pending) error {
	hint := 0
	if ids := a.rowIDs(); len(ids) > 0 {
		for i, id := range ids {
			if id == str(a.ui.Get, "cur") {
				hint = i
			}
		}
	}
	var level, msg string
	switch p.kind {
	case "stop":
		var done []string
		for _, id := range p.ids {
			if c := a.sim.byID(id); c != nil && a.sim.stop(c) {
				done = append(done, c.name)
			}
		}
		level, msg = "ok", "Stopped "+names(done)
		if len(done) == 0 {
			level, msg = "info", "Already stopped"
		}
	case "restart":
		var done []string
		for _, id := range p.ids {
			if c := a.sim.byID(id); c != nil && a.sim.restartC(c) {
				done = append(done, c.name)
			}
		}
		level, msg = "warn", "Restarting "+names(done)
	case "remove":
		var done []string
		for _, id := range p.ids {
			if c := a.sim.byID(id); c != nil {
				done = append(done, c.name)
				a.sim.remove(c)
			}
		}
		level, msg = "ok", "Removed "+names(done)
	case "prune-images":
		n, mb := a.sim.pruneImages()
		level, msg = "ok", fmt.Sprintf("Removed %s, freed %s", plural(n, "image", "images"), fmtSize(mb))
	case "prune-volumes":
		n, mb := a.sim.pruneVolumes()
		level, msg = "ok", fmt.Sprintf("Removed %s, freed %s", plural(n, "volume", "volumes"), fmtSize(mb))
	}
	if err := a.ui.Batch(func(b *tuimark.Batch) error {
		return errors.Join(a.write(b), a.dropMarks(b), a.toast(b, level, msg), a.say(b, msg))
	}); err != nil {
		return err
	}
	return a.repairCursor(hint)
}

// dropMarks forgets marks on containers that no longer exist.
func (a *app) dropMarks(b *tuimark.Batch) error {
	keep := []any{}
	for _, id := range listAt(a.ui.Get, "sel") {
		s, _ := id.(string)
		if a.sim.byID(s) != nil {
			keep = append(keep, s)
		}
	}
	return b.Set("sel", keep)
}

// ---- the palette ----

// paletteRun closes the palette and does the command under the cursor, or
// the first match when the cursor's row was filtered away.
func (a *app) paletteRun() error {
	cur := str(a.ui.Get, "palette.cur")
	id := ""
	for _, it := range listAt(a.ui.Get, "found.items") {
		cid := field(it, "id")
		if id == "" {
			id = cid
		}
		if cid == cur {
			id = cid
			break
		}
	}
	if id == "" {
		return nil // nothing matches: stay open, the input shows why
	}
	if err := a.ui.Set("palette.open", false); err != nil {
		return err
	}
	switch id {
	case "stop", "restart", "remove":
		return a.ask(id)
	case "start":
		return a.start()
	case "prune-images", "prune-volumes":
		return a.ask(id)
	case "detail":
		return a.ui.Set("view", "detail")
	case "filter":
		return errors.Join(a.ui.Set("view", "containers"), a.ui.Set("@focus", "#containers__q"))
	case "sort":
		return a.sortNext()
	case "containers", "images", "volumes", "events":
		return a.ui.Set("view", id)
	case "theme":
		return a.toggleTheme()
	case "help":
		return a.ui.Set("help.open", true)
	}
	return nil
}

func (a *app) toggleTheme() error {
	next := "light"
	if str(a.ui.Get, "theme.name") == "light" {
		next = "dark"
	}
	return a.ui.Batch(func(b *tuimark.Batch) error {
		return errors.Join(b.Set("@theme", next), b.Set("theme.name", next), a.say(b, "theme: "+next))
	})
}
