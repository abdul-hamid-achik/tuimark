package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/tuimark"
)

// derive keeps the three computed values the markup reads: cols (the lanes,
// filtered, as rows), counts (the lane titles and totals) and summary (the
// status line's right side). Each reads the store only through get, and a
// later one reads the earlier ones' new values.
func (a *app) derive() error {
	err := a.ui.Derive("cols", func(get func(string) (any, bool)) (any, error) {
		lanes := visible(decodeCards(get("cards")), text(get, "filter"))
		today := dayOf(get, "today")
		out := map[string]any{}
		for _, col := range columns {
			out[col] = rows(lanes[col], today)
		}
		return out, nil
	})
	if err != nil {
		return err
	}
	err = a.ui.Derive("counts", func(get func(string) (any, bool)) (any, error) {
		cards := decodeCards(get("cards"))
		filtering := strings.TrimSpace(text(get, "filter")) != ""
		total := map[string]int{}
		for _, c := range cards {
			total[c.Col]++
		}
		out := map[string]any{}
		shown := 0
		for _, col := range columns {
			n := len(laneIDs(get, col))
			shown += n
			out[col] = fmt.Sprint(n)
			if filtering {
				out[col] = fmt.Sprintf("%d/%d", n, total[col])
			}
		}
		out["shown"], out["total"], out["marked"] = shown, len(cards), len(marked(get))
		return out, nil
	})
	if err != nil {
		return err
	}
	return a.ui.Derive("summary", func(get func(string) (any, bool)) (any, error) {
		shown, _ := get("counts.shown")
		total, _ := get("counts.total")
		marks, _ := get("counts.marked")
		s := plural(int(num(total)), "card", "cards")
		if q := strings.TrimSpace(text(get, "filter")); q != "" {
			s = fmt.Sprintf("%d of %s match “%s”", int(num(shown)), s, q)
		}
		if n := int(num(marks)); n > 0 {
			s += fmt.Sprintf(" · %d marked", n)
		}
		return s, nil
	})
}

func num(v any) float64 {
	f, _ := v.(float64)
	if f == 0 {
		if i, ok := v.(int); ok {
			return float64(i)
		}
	}
	return f
}

// handlers are the named actions of app.tui. The built-ins (focus,
// check-toggle, move-next, …) are the runtime's and have none.
func (a *app) handlers() map[string]tuimark.Handler {
	hs := map[string]tuimark.Handler{}
	on := func(name string, h tuimark.Handler) { hs[name] = h }
	on("quit", func(tuimark.Event) error { return tuimark.ErrQuit })

	on("col_focus", func(ev tuimark.Event) error {
		if colIndex(ev.Source) >= 0 {
			a.lastCol = ev.Source
		}
		return nil
	})
	on("card_left", func(ev tuimark.Event) error { return a.shift(ev, -1) })
	on("card_right", func(ev tuimark.Event) error { return a.shift(ev, 1) })
	on("card_up", func(ev tuimark.Event) error { return a.reorder(ev, -1) })
	on("card_down", func(ev tuimark.Event) error { return a.reorder(ev, 1) })

	on("card_new", a.cardNew)
	on("card_edit", a.cardEdit)
	on("form_prio", func(tuimark.Event) error { return a.setPrio(a.ui, a.formPrio%3+1) })
	on("card_save", func(tuimark.Event) error { return a.cardSave() })
	on("card_cancel", func(tuimark.Event) error { return a.ui.Set("form.open", false) })

	on("card_delete", a.cardDelete)
	on("delete_confirm", func(tuimark.Event) error { return a.deleteConfirm() })
	on("delete_cancel", func(tuimark.Event) error {
		a.pending = map[string]bool{}
		return a.ui.Set("del.open", false)
	})

	on("filter_apply", func(tuimark.Event) error { return a.ui.Set("@focus", "#"+a.lastCol) })
	on("filter_clear", func(tuimark.Event) error {
		return errors.Join(a.ui.Set("filter", ""), a.ui.Set("@focus", "#"+a.lastCol))
	})
	return hs
}

// say writes the status line; bad paints it as an error.
func (a *app) say(s setter, msg string, bad bool) error {
	a.ttl = statusTTL
	return errors.Join(s.Set("status", msg), s.Set("status_err", bad))
}

// commit publishes a new board in one batch: the cards, the checked marks
// re-homed to the lanes their cards now sit in (a card that was deleted
// loses its mark), and whatever extra writes the caller adds. It saves the
// file last, so a failed save is reported while the board on screen has
// already changed.
func (a *app) commit(cards []card, extra func(b *tuimark.Batch) error) error {
	err := a.ui.Batch(func(b *tuimark.Batch) error {
		marks := rawMarks(a.ui.Get)
		sel := map[string]any{}
		for _, col := range columns {
			ids := []string{}
			for _, c := range cards {
				if c.Col == col && marks[c.ID] {
					ids = append(ids, c.ID)
				}
			}
			sel[col] = ids
		}
		if err := errors.Join(b.Set("cards", cards), b.Set("sel", sel)); err != nil {
			return err
		}
		if extra != nil {
			return extra(b)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if a.file == "" {
		return nil
	}
	if err := writeBoard(a.file, cards); err != nil {
		return fmt.Errorf("save %s: %w", a.file, err)
	}
	return nil
}

// targets are the cards an action applies to: the checked cards the board
// shows, else the card under the cursor of the focused lane. The keymap
// row's event names that card in Keys["c"] and the lane in Source.
func (a *app) targets(ev tuimark.Event) (ids map[string]bool, cursor string) {
	cursor, _ = ev.Keys["c"].(string)
	ids = map[string]bool{}
	for _, id := range marked(a.ui.Get) {
		ids[id] = true
	}
	if len(ids) == 0 && cursor != "" {
		ids[cursor] = true
	}
	return ids, cursor
}

// shift moves the target cards one lane (dir -1 left, 1 right). The cursor
// and the focus follow the card under the cursor to its new lane.
func (a *app) shift(ev tuimark.Event, dir int) error {
	ids, cursor := a.targets(ev)
	if len(ids) == 0 {
		return a.say(a.ui, "no card selected", false)
	}
	cards := a.cards()
	next, n := moveCards(cards, ids, dir)
	if n == 0 {
		edge := "last"
		if dir < 0 {
			edge = "first"
		}
		return a.say(a.ui, "already in the "+edge+" lane", false)
	}
	dest := ""
	if c, ok := find(next, cursor); ok && ids[cursor] {
		if old, _ := find(cards, cursor); old.Col != c.Col {
			dest = c.Col
		}
	}
	var title string
	if dest != "" {
		title = colTitles[dest]
	} else {
		title = colTitles[columns[colIndex(firstMoved(cards, next))]]
	}
	err := a.commit(next, func(b *tuimark.Batch) error {
		errs := []error{a.say(b, fmt.Sprintf("moved %s to %s", plural(n, "card", "cards"), title), false)}
		if dest != "" {
			errs = append(errs, b.Set("cur."+dest, cursor))
		}
		return errors.Join(errs...)
	})
	if err != nil {
		return err
	}
	if dest != "" {
		return a.ui.Set("@focus", "#"+dest)
	}
	return nil
}

// firstMoved is the lane of the first card that changed lanes.
func firstMoved(before, after []card) string {
	was := map[string]string{}
	for _, c := range before {
		was[c.ID] = c.Col
	}
	for _, c := range after {
		if was[c.ID] != c.Col {
			return c.Col
		}
	}
	return columns[0]
}

// reorder swaps the cursor card with its neighbour in the lane. The cursor
// is a key, so it stays on the card; the write just makes that explicit.
func (a *app) reorder(ev tuimark.Event, dir int) error {
	id, _ := ev.Keys["c"].(string)
	if id == "" {
		return nil
	}
	next, ok := reorder(a.cards(), id, dir, text(a.ui.Get, "filter"))
	if !ok {
		return nil
	}
	c, _ := find(next, id)
	return a.commit(next, func(b *tuimark.Batch) error { return b.Set("cur."+c.Col, id) })
}

// setPrio writes the form's priority and the three things the view shows
// of it.
func (a *app) setPrio(s setter, p int) error {
	a.formPrio = p
	return errors.Join(
		s.Set("form.prio_mark", prioMark(p)),
		s.Set("form.prio_label", prioLabel(p)),
		s.Set("form.p_high", p >= 3), s.Set("form.p_med", p == 2), s.Set("form.p_low", p < 2),
	)
}

// openForm fills and opens the card form.
func (a *app) openForm(heading string, c card) error {
	return a.ui.Batch(func(b *tuimark.Batch) error {
		return errors.Join(
			b.Set("form.heading", heading), b.Set("form.title", c.Title), b.Set("form.tag", c.Tag),
			b.Set("form.due", c.Due), b.Set("form.error", ""), a.setPrio(b, c.Prio), b.Set("form.open", true),
		)
	})
}

func (a *app) cardNew(ev tuimark.Event) error {
	a.addCol, a.editID = a.lastCol, ""
	if colIndex(ev.Source) >= 0 {
		a.addCol = ev.Source
	}
	return a.openForm("New card in "+colTitles[a.addCol], card{Prio: 2})
}

func (a *app) cardEdit(ev tuimark.Event) error {
	id, _ := ev.Keys["c"].(string)
	c, ok := find(a.cards(), id)
	if !ok {
		return a.say(a.ui, "no card selected", false)
	}
	a.editID = id
	return a.openForm("Edit card", c)
}

// cardSave validates the form, then adds or updates the card. A problem
// stays on the form and keeps it open.
func (a *app) cardSave() error {
	get := a.ui.Get
	title := strings.TrimSpace(text(get, "form.title"))
	tag := strings.ToLower(strings.TrimSpace(text(get, "form.tag")))
	due := strings.TrimSpace(text(get, "form.due"))
	fail := func(msg string) error { return a.ui.Set("form.error", msg) }
	if title == "" {
		return fail("a card needs a title")
	}
	if due != "" {
		if _, ok := parseDay(due); !ok {
			return fail("due wants YYYY-MM-DD")
		}
	}
	if tag == "" {
		tag = "task"
	}
	cards := a.cards()
	msg := ""
	var id, col string
	if a.editID != "" {
		for i := range cards {
			if cards[i].ID == a.editID {
				cards[i].Title, cards[i].Tag, cards[i].Due, cards[i].Prio = title, tag, due, a.formPrio
				id, col = cards[i].ID, cards[i].Col
			}
		}
		if id == "" {
			return fail("that card is gone")
		}
		msg = fmt.Sprintf("updated “%s”", title)
	} else {
		id, col = fmt.Sprintf("c%d", a.next), a.addCol
		a.next++
		cards = append(cards, card{ID: id, Title: title, Tag: tag, Prio: a.formPrio, Due: due, Col: col})
		msg = fmt.Sprintf("added “%s” to %s", title, colTitles[col])
	}
	if !matches(card{Title: title, Tag: tag, Prio: a.formPrio}, text(get, "filter")) {
		msg += " (hidden by the filter)"
	}
	return a.commit(cards, func(b *tuimark.Batch) error {
		return errors.Join(b.Set("cur."+col, id), b.Set("form.open", false), a.say(b, msg, false))
	})
}

// cardDelete asks before deleting the target cards.
func (a *app) cardDelete(ev tuimark.Event) error {
	ids, _ := a.targets(ev)
	if len(ids) == 0 {
		return a.say(a.ui, "no card selected", false)
	}
	a.pending = ids
	q := plural(len(ids), "card", "cards")
	if len(ids) == 1 {
		c, _ := find(a.cards(), sortedIDs(ids)[0])
		q = "“" + c.Title + "”"
	}
	return a.ui.Batch(func(b *tuimark.Batch) error {
		return errors.Join(b.Set("del.text", "Delete "+q+"?"), b.Set("del.open", true))
	})
}

func (a *app) deleteConfirm() error {
	ids := a.pending
	a.pending = map[string]bool{}
	next := deleteCards(a.cards(), ids)
	return a.commit(next, func(b *tuimark.Batch) error {
		return errors.Join(b.Set("del.open", false), a.say(b, "deleted "+plural(len(ids), "card", "cards"), false))
	})
}
