package main

import (
	"errors"

	"github.com/abdul-hamid-achik/tuimark"
)

// handlers are the named actions of app.tui. Each changes the player and
// publishes it. A keymap row on a table (the rows with when="#lib", "#q",
// "#albums") fires its action with the row under the cursor in the event's
// Keys, under the table's alias "r"; the handler reads the row from there.
// It never reads the key bound in cur.*: that path is only written when the
// cursor moves, so after a filter or a removal narrows the rows it can name
// a row that is gone while the table highlights the clamped one.
func (a *app) handlers() map[string]tuimark.Handler {
	ui, p := a.ui, a.p
	hs := map[string]tuimark.Handler{}
	// on registers a handler. A handler that succeeds also clears the
	// notice of an earlier failure.
	on := func(name string, fn func(ev tuimark.Event) error) {
		hs[name] = func(ev tuimark.Event) error {
			if err := fn(ev); err != nil {
				return err
			}
			return a.clearNotice()
		}
	}
	plain := func(fn func() error) func(tuimark.Event) error {
		return func(tuimark.Event) error { return fn() }
	}
	// do runs a player change and publishes the result.
	do := func(change func()) func(tuimark.Event) error {
		return func(tuimark.Event) error {
			change()
			return a.publish()
		}
	}

	on("quit", plain(func() error { return tuimark.ErrQuit }))
	on("help_open", plain(func() error { return ui.Set("help_open", true) }))
	on("help_close", plain(func() error { return ui.Set("help_open", false) }))

	on("toggle_play", do(p.toggle))
	on("next", do(p.next))
	on("prev", do(p.prev))
	on("toggle_shuffle", do(func() { p.shuffle = !p.shuffle }))
	on("cycle_repeat", do(p.cycleRepeat))
	on("vol_up", do(func() { p.setVolume(p.volume + volStep) }))
	on("vol_down", do(func() { p.setVolume(p.volume - volStep) }))
	on("seek_fwd", do(func() { p.seek(seekMs) }))
	on("seek_back", do(func() { p.seek(-seekMs) }))

	// The library tab.
	on("lib_play", func(ev tuimark.Event) error {
		if id := rowKey(ev); trackIndex(id) >= 0 {
			p.playID(id)
		}
		return a.publish()
	})
	on("lib_add", func(ev tuimark.Event) error {
		if id := rowKey(ev); trackIndex(id) >= 0 {
			p.enqueue(id)
		}
		return a.publish()
	})
	on("filter_apply", plain(func() error { return ui.Set("@focus", "#lib") }))
	on("filter_clear", plain(func() error {
		return errors.Join(ui.Set("filter", ""), ui.Set("@focus", "#lib"))
	}))

	// The queue tab.
	on("q_play", func(ev tuimark.Event) error {
		if q, ok := p.queued(rowKey(ev)); ok {
			p.dequeue(q.QID)
			p.playID(q.ID)
		}
		return a.publish()
	})
	on("q_remove", func(ev tuimark.Event) error {
		if q, ok := p.queued(rowKey(ev)); ok {
			p.dequeue(q.QID)
		}
		return a.publish()
	})
	on("q_clear", do(func() { p.queue = nil }))

	// The albums tab: play replaces the queue with the rest of the album.
	on("album_play", func(ev tuimark.Event) error {
		if al, ok := albumNamed(rowKey(ev)); ok {
			p.queue = nil
			p.playID(al.IDs[0])
			for _, id := range al.IDs[1:] {
				p.enqueue(id)
			}
		}
		return a.publish()
	})
	on("album_add", func(ev tuimark.Event) error {
		if al, ok := albumNamed(rowKey(ev)); ok {
			for _, id := range al.IDs {
				p.enqueue(id)
			}
		}
		return a.publish()
	})
	return hs
}

// rowKey is the key of the table row under the cursor, as the event of a
// keymap row on that table carries it: the one entry of Keys (the
// tables' alias is "r"). It is "" when the table has no rows.
func rowKey(ev tuimark.Event) string {
	s, _ := ev.Keys["r"].(string)
	return s
}

// queued is the queue entry with this QID.
func (p *player) queued(qid string) (qitem, bool) {
	for _, it := range p.queue {
		if it.QID == qid {
			return it, true
		}
	}
	return qitem{}, false
}

// albumNamed is the album with this name.
func albumNamed(name string) (album, bool) {
	for _, al := range albumList() {
		if al.Name == name {
			return al, true
		}
	}
	return album{}, false
}

// clearNotice drops the notice of an earlier failure, once.
func (a *app) clearNotice() error {
	if a.notice == "" {
		return nil
	}
	a.notice = ""
	return a.ui.Set("notice", "")
}
