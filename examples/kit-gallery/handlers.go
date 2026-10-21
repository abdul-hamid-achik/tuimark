package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/tuimark"
)

// derive keeps the two computed values the markup reads. Each reads the
// store only through get.
//
//   - found: the palette's commands that match palette.query (every word of
//     the query must appear in the label or the id), with `any` for the
//     "no match" line;
//   - detail: the selected incident of the split pane, copied field by
//     field so the markup binds `detail.title` and friends.
func (a *app) derive() error {
	err := a.ui.Derive("found", func(get func(string) (any, bool)) (any, error) {
		words := strings.Fields(lower(str(get, "palette.query")))
		items := []any{}
		for _, c := range listAt(get, "palette.commands") {
			hay := lower(fieldOf(c, "label") + " " + fieldOf(c, "id"))
			if matchesAll(hay, words) {
				items = append(items, map[string]any{
					"id": fieldOf(c, "id"), "label": fieldOf(c, "label"), "hint": fieldOf(c, "hint"),
				})
			}
		}
		return map[string]any{"any": len(items) > 0, "items": items}, nil
	})
	if err != nil {
		return err
	}
	return a.ui.Derive("detail", func(get func(string) (any, bool)) (any, error) {
		cur := str(get, "split.cur")
		items := listAt(get, "split.items")
		var pick map[string]any
		for _, it := range items {
			m, _ := it.(map[string]any)
			if m == nil {
				continue
			}
			if pick == nil {
				pick = m
			}
			if m["id"] == cur {
				pick = m
				break
			}
		}
		if pick == nil {
			return map[string]any{"title": "", "age": "", "status": "", "open": false, "summary": "", "owner": ""}, nil
		}
		open, _ := pick["open"].(bool)
		return map[string]any{
			"title": pick["title"], "age": pick["age"], "status": pick["status"],
			"open": open, "summary": pick["summary"], "owner": pick["owner"],
		}, nil
	})
}

func matchesAll(hay string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

// handlers are the named actions of app.tui. The built-ins (quit, focus,
// move-next, switch-to, check-toggle, …) are the runtime's and have none.
func (a *app) handlers() map[string]tuimark.Handler {
	hs := map[string]tuimark.Handler{}
	on := func(name string, h tuimark.Handler) { hs[name] = h }

	// Buttons and rows that only report themselves.
	on("button_press", func(ev tuimark.Event) error { return a.say(a.ui, "pressed "+ev.Source) })
	on("table_select", func(ev tuimark.Event) error {
		return a.say(a.ui, "service: "+keyOf(ev, "s"))
	})
	on("table_open", func(tuimark.Event) error {
		cur := str(a.ui.Get, "svc.cur")
		return a.ui.Batch(func(b *tuimark.Batch) error { return a.toast(b, "info", "Opening "+cur) })
	})
	on("menu_change", func(ev tuimark.Event) error {
		on, _ := ev.Value.([]any)
		return a.say(a.ui, plural(len(on), "channel", "channels")+" on")
	})
	on("menu_select", func(ev tuimark.Event) error {
		return a.say(a.ui, "incident: "+keyOf(ev, "it"))
	})
	on("empty_create", func(tuimark.Event) error {
		return a.ui.Batch(func(b *tuimark.Batch) error { return a.toast(b, "ok", "Deployment created") })
	})

	// Toasts.
	on("toast_show", func(ev tuimark.Event) error {
		// The buttons are toast__info, toast__ok, toast__warn, toast__danger:
		// the part of the id after "__" is the level.
		level := partOf(ev)
		msgs := map[string]string{
			"info": "Heads up: deploys pause at 09:00", "ok": "Settings saved",
			"warn": "92% of build minutes used", "danger": "Build failed at step 3",
		}
		msg, ok := msgs[level]
		if !ok {
			level, msg = "info", "Hello from a toast"
		}
		return a.ui.Batch(func(b *tuimark.Batch) error { return a.toast(b, level, msg) })
	})
	on("toast_dismiss", func(tuimark.Event) error {
		a.toastLeft = 0
		return a.ui.Set("toast.show", false)
	})

	// Dialogs. The actions are generic (dialog_*); the instance is the prefix
	// of the event's source before the last "__" (confirm__yes, confirm__open,
	// confirm__dialog), and the instance's state lives under its own name
	// (confirm.open), so a second dialog would need no new handler.
	on("dialog_open", func(ev tuimark.Event) error { return a.ui.Set(instanceOf(ev)+".open", true) })
	on("dialog_cancel", func(ev tuimark.Event) error {
		inst := instanceOf(ev)
		return a.ui.Batch(func(b *tuimark.Batch) error {
			return errors.Join(b.Set(inst+".open", false), a.toast(b, "info", "Kept the project"))
		})
	})
	on("dialog_confirm", func(ev tuimark.Event) error {
		inst := instanceOf(ev)
		return a.ui.Batch(func(b *tuimark.Batch) error {
			return errors.Join(b.Set(inst+".open", false), a.toast(b, "ok", "Project deleted"))
		})
	})

	// The help overlay.
	on("help_open", func(tuimark.Event) error { return a.ui.Set("help.open", true) })
	on("help_close", func(tuimark.Event) error { return a.ui.Set("help.open", false) })

	// The command palette: open resets the query and the cursor; run closes
	// it and does the command under the cursor.
	on("palette_open", func(tuimark.Event) error {
		return a.ui.Batch(func(b *tuimark.Batch) error {
			return errors.Join(b.Set("palette.query", ""), b.Set("palette.cur", firstCommand(a.ui.Get)), b.Set("palette.open", true))
		})
	})
	on("palette_close", func(tuimark.Event) error { return a.ui.Set("palette.open", false) })
	on("palette_run", func(tuimark.Event) error { return a.paletteRun() })
	// palette_query fires as the query changes. `found` is a Derive, which
	// the runtime recomputes on that write, so there is nothing to do.
	on("palette_query", func(tuimark.Event) error { return nil })

	// Look and navigation.
	on("theme_toggle", func(tuimark.Event) error { return a.toggleTheme() })
	on("breadcrumb_up", func(tuimark.Event) error { return a.crumbUp() })

	// The log panel.
	on("log_append", func(tuimark.Event) error { return a.logAppend() })

	// The composer of the chat tab: a message is appended to the transcript
	// as one block (Append adds to the array in place), and the draft is
	// cleared in the same batch, since enter changes nothing in a textarea.
	on("composer_change", func(tuimark.Event) error { return nil })
	on("composer_submit", func(ev tuimark.Event) error {
		text, _ := ev.Value.(string)
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		id := float64(len(listAt(a.ui.Get, "chat.blocks")))
		block := map[string]any{"id": id, "runs": []any{map[string]any{"text": "You: ", "class": "k-strong"}, text}}
		return a.ui.Batch(func(b *tuimark.Batch) error {
			return errors.Join(b.Append("chat.blocks", block), b.Set("chat.composer.draft", ""), a.say(b, "message sent"))
		})
	})

	// The form.
	on("form_validate", func(ev tuimark.Event) error {
		v, _ := ev.Value.(string)
		name, email := str(a.ui.Get, "forms.profile.name.value"), str(a.ui.Get, "forms.profile.email.value")
		var field, msg string
		switch formFieldOf(ev) { // profile__name__input, profile__email__input
		case "name":
			field, msg, name = "name", checkName(v), v
		case "email":
			field, msg, email = "email", checkEmail(v), v
		default:
			return nil
		}
		return a.ui.Batch(func(b *tuimark.Batch) error {
			return errors.Join(b.Set("forms.profile."+field+".error", msg),
				b.Set("forms.profile.valid", checkName(name) == "" && checkEmail(email) == ""))
		})
	})
	on("form_submit", func(tuimark.Event) error { return a.formSubmit() })
	return hs
}

// keyOf is the row key an event carries under an each alias.
func keyOf(ev tuimark.Event, alias string) string {
	if ev.Keys == nil {
		return ""
	}
	return fmt.Sprint(ev.Keys[alias])
}

func firstCommand(get getter) string {
	for _, c := range listAt(get, "palette.commands") {
		return fieldOf(c, "id")
	}
	return ""
}

// paletteRun closes the palette and does the command under the cursor, or
// the first match when the cursor's row was filtered away.
func (a *app) paletteRun() error {
	cur := str(a.ui.Get, "palette.cur")
	id := ""
	for _, it := range listAt(a.ui.Get, "found.items") {
		cid := fieldOf(it, "id")
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
	return a.ui.Batch(func(b *tuimark.Batch) error {
		errs := []error{b.Set("palette.open", false)}
		switch id {
		case "deploy":
			errs = append(errs, a.toast(b, "ok", "Deploying to production…"))
		case "rollback":
			errs = append(errs, a.toast(b, "warn", "Rolling back the last deploy"))
		case "logs":
			errs = append(errs, b.Set("view", "data"), a.say(b, "opened the logs"))
		case "theme":
			errs = append(errs, a.themeBatch(b))
		case "help":
			errs = append(errs, b.Set("help.open", true))
		}
		return errors.Join(errs...)
	})
}

func (a *app) toggleTheme() error {
	return a.ui.Batch(func(b *tuimark.Batch) error { return a.themeBatch(b) })
}

func (a *app) themeBatch(b *tuimark.Batch) error {
	next := "light"
	if str(a.ui.Get, "theme.name") == "light" {
		next = "dark"
	}
	return errors.Join(b.Set("@theme", next), b.Set("theme.name", next), a.say(b, "theme: "+next))
}

// crumbUp drops the last breadcrumb and marks the new last one current.
func (a *app) crumbUp() error {
	crumbs := listAt(a.ui.Get, "crumbs")
	if len(crumbs) <= 1 {
		return nil
	}
	crumbs = crumbs[:len(crumbs)-1]
	out := make([]any, len(crumbs))
	for i, c := range crumbs {
		m, _ := c.(map[string]any)
		out[i] = map[string]any{"id": m["id"], "label": m["label"], "last": i == len(crumbs)-1}
	}
	return a.ui.Batch(func(b *tuimark.Batch) error {
		return errors.Join(b.Set("crumbs", out), a.say(b, "up to "+fieldOf(out[len(out)-1], "label")))
	})
}

// logMessages are the lines `a` appends to the log panel, in a cycle of
// five levels: debug, info, plain, warning, error.
var logMessages = []string{
	"cache hit for pkg/router",
	"compiling %d packages",
	"ok   pkg/router   0.%02ds",
	"warning: slow test pkg/search",
	"FAIL pkg/billing  timeout after %ds",
}

const maxLogLines = 200

// logAppend adds one line to the log panel; the panel follows the end
// while the view is at the bottom (stick="bottom").
func (a *app) logAppend() error {
	a.lines++
	n := a.lines
	level := n % len(logMessages)
	text := logMessages[level]
	switch level {
	case 1, 4:
		text = fmt.Sprintf(text, n)
	case 2:
		text = fmt.Sprintf(text, n%100)
	}
	stamp := fmt.Sprintf("12:%02d:%02d ", (n/60)%60, n%60)
	line := map[string]any{
		"n": float64(n), "text": stamp + text,
		"debug": level == 0, "info": level == 1, "warn": level == 3, "err": level == 4,
	}
	lines := append(listAt(a.ui.Get, "log.lines"), line)
	if len(lines) > maxLogLines {
		lines = lines[len(lines)-maxLogLines:]
	}
	return a.ui.Batch(func(b *tuimark.Batch) error {
		return errors.Join(b.Set("log.lines", lines), b.Set("log.count", float64(len(lines))),
			b.Set("log.title", logTitle(len(lines))))
	})
}

// logTitle is the log panel's title, which the k-log template reads from
// log.title.
func logTitle(n int) string { return "build log · " + plural(n, "line", "lines") }

func checkName(s string) string {
	if strings.TrimSpace(s) == "" {
		return "Name is required."
	}
	return ""
}

func checkEmail(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "Email is required."
	}
	at := strings.Index(s, "@")
	if at < 1 || !strings.Contains(s[at+1:], ".") || strings.HasSuffix(s, ".") {
		return "Enter a full address like name@host.example."
	}
	return ""
}

// formSubmit validates both fields, shows what is wrong, and moves focus to
// the first invalid one; with nothing wrong it toasts success.
func (a *app) formSubmit() error {
	nameErr := checkName(str(a.ui.Get, "forms.profile.name.value"))
	emailErr := checkEmail(str(a.ui.Get, "forms.profile.email.value"))
	err := a.ui.Batch(func(b *tuimark.Batch) error {
		errs := []error{
			b.Set("forms.profile.name.error", nameErr), b.Set("forms.profile.email.error", emailErr),
			b.Set("forms.profile.valid", nameErr == "" && emailErr == ""),
		}
		switch {
		case nameErr != "" || emailErr != "":
			n := 0
			for _, e := range []string{nameErr, emailErr} {
				if e != "" {
					n++
				}
			}
			errs = append(errs, a.toast(b, "danger", plural(n, "field needs", "fields need")+" attention"))
		default:
			errs = append(errs, a.toast(b, "ok", "Profile saved"))
		}
		return errors.Join(errs...)
	})
	if err != nil {
		return err
	}
	switch {
	case nameErr != "":
		return a.ui.Set("@focus", "#profile__name__input")
	case emailErr != "":
		return a.ui.Set("@focus", "#profile__email__input")
	}
	return nil
}
