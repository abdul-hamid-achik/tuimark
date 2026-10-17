package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// register installs the handlers and the derived values.
func (a *app) register() {
	for name, h := range a.handlers() {
		a.ui.On(name, h)
	}
	if err := a.derive(); err != nil {
		panic(err) // only a programming error: the paths are constants
	}
}

var methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// handlers are the named actions of app.tui.
func (a *app) handlers() map[string]tuimark.Handler {
	hs := map[string]tuimark.Handler{}
	on := func(name string, h tuimark.Handler) { hs[name] = h }
	on("quit", func(tuimark.Event) error {
		a.cancelFlight()
		return tuimark.ErrQuit
	})
	on("req_select", func(ev tuimark.Event) error { return a.selectRequest(str(ev.Keys["r"])) })
	on("hist_select", func(ev tuimark.Event) error { return a.selectHistory(str(ev.Keys["e"])) })
	on("edited", func(tuimark.Event) error {
		if truthy(a.get("req.dirty")) {
			return nil
		}
		return a.ui.Set("req.dirty", true)
	})
	on("method_next", func(tuimark.Event) error {
		cur := str(a.get("req.method"))
		next := methods[0]
		for i, m := range methods {
			if m == cur {
				next = methods[(i+1)%len(methods)]
			}
		}
		return a.ui.Batch(func(b *tuimark.Batch) error {
			_ = b.Set("req.method", next)
			return b.Set("req.dirty", true)
		})
	})
	on("header_add", func(ev tuimark.Event) error { return a.addHeader(str(ev.Value)) })
	on("header_del", func(ev tuimark.Event) error { return a.dropHeader(flt(ev.Keys["h"])) })
	on("send", func(tuimark.Event) error { return a.startSend() })
	on("cancel", func(tuimark.Event) error {
		if !a.cancelFlight() {
			return nil
		}
		return a.say("canceling…", false)
	})
	on("req_save", func(tuimark.Event) error { return a.saveRequest() })
	on("req_new", func(tuimark.Event) error {
		return a.ui.Batch(func(b *tuimark.Batch) error {
			_ = b.Set("req", map[string]any{
				"id": "", "name": "new request", "method": "GET", "url": "{{base}}/",
				"body": "", "draft": "", "dirty": false, "headers": []any{},
			})
			_ = b.Set("status", "new request: type a URL, ctrl+s sends, s saves it")
			_ = b.Set("status_err", false)
			return b.Set("@focus", "#url")
		})
	})
	on("req_delete", func(ev tuimark.Event) error { return a.deleteRequest(str(ev.Keys["r"])) })
	on("filter_done", func(tuimark.Event) error { return a.ui.Set("@focus", a.listID()) })
	on("filter_clear", func(tuimark.Event) error {
		return a.ui.Batch(func(b *tuimark.Batch) error {
			_ = b.Set("query", "")
			return b.Set("@focus", a.listID())
		})
	})
	on("help_open", func(tuimark.Event) error { return a.ui.Set("help_open", true) })
	on("help_close", func(tuimark.Event) error { return a.ui.Set("help_open", false) })
	return hs
}

// listID is the focus target of the sidebar's active tab.
func (a *app) listID() string {
	if str(a.get("side_tab")) == "t-hist" {
		return "#hist"
	}
	return "#reqs"
}

// ---- editor <-> collection ----

// headerRows turns "Key: Value" lines into the rows the editor lists.
func headerRows(lines []any) []any {
	rows := []any{}
	for i, l := range lines {
		if k, v, ok := parseHeaderLine(str(l)); ok {
			rows = append(rows, map[string]any{"n": float64(i + 1), "k": k, "v": v})
		}
	}
	return rows
}

// headerLines is the reverse: the rows back to "Key: Value" lines.
func headerLines(rows []any) []any {
	lines := []any{}
	for _, r := range rows {
		m := asMap(r)
		lines = append(lines, str(m["k"])+": "+str(m["v"]))
	}
	return lines
}

// editorFrom builds the store's req object from a saved request or a
// history entry's request.
func editorFrom(id, name string, c map[string]any) map[string]any {
	return map[string]any{
		"id": id, "name": name, "method": str(c["method"]), "url": str(c["url"]),
		"body": str(c["body"]), "draft": "", "dirty": false,
		"headers": headerRows(asList(c["headers"])),
	}
}

func (a *app) selectRequest(id string) error {
	for _, c := range asList(a.get("collection")) {
		m := asMap(c)
		if str(m["id"]) == id {
			return a.ui.Batch(func(b *tuimark.Batch) error {
				_ = b.Set("req", editorFrom(id, str(m["name"]), m))
				_ = b.Set("status", "loaded "+str(m["name"])+": ctrl+s sends it")
				return b.Set("status_err", false)
			})
		}
	}
	return nil
}

func (a *app) selectHistory(id string) error {
	for _, e := range asList(a.get("history")) {
		m := asMap(e)
		if str(m["id"]) != id {
			continue
		}
		req := asMap(m["req"])
		name := str(req["method"]) + " " + displayPath(str(req["url"]))
		a.mu.Lock()
		cached := a.cache[id]
		a.mu.Unlock()
		return a.ui.Batch(func(b *tuimark.Batch) error {
			_ = b.Set("req", editorFrom("", name, req))
			msg := "from history: ctrl+s sends it again"
			if cached != nil {
				_ = b.Set("resp", cached)
				msg = "from history, with its response: ctrl+s sends it again"
			}
			_ = b.Set("status", msg)
			return b.Set("status_err", false)
		})
	}
	return nil
}

func (a *app) addHeader(draft string) error {
	k, v, ok := parseHeaderLine(draft)
	if !ok {
		return a.say(`a header looks like "Key: Value"`, true)
	}
	rows := asList(a.get("req.headers"))
	next, replaced := 1.0, false
	out := make([]any, 0, len(rows)+1)
	for _, r := range rows {
		m := asMap(r)
		next = max(next, flt(m["n"])+1)
		if strings.EqualFold(str(m["k"]), k) {
			out = append(out, map[string]any{"n": m["n"], "k": k, "v": v})
			replaced = true
			continue
		}
		out = append(out, m)
	}
	if !replaced {
		out = append(out, map[string]any{"n": next, "k": k, "v": v})
	}
	return a.ui.Batch(func(b *tuimark.Batch) error {
		_ = b.Set("req.headers", out)
		_ = b.Set("req.draft", "")
		_ = b.Set("req.dirty", true)
		_ = b.Set("status", "header "+k+" set")
		return b.Set("status_err", false)
	})
}

func (a *app) dropHeader(n float64) error {
	var out []any
	for _, r := range asList(a.get("req.headers")) {
		if flt(asMap(r)["n"]) != n {
			out = append(out, r)
		}
	}
	if out == nil {
		out = []any{}
	}
	return a.ui.Batch(func(b *tuimark.Batch) error {
		_ = b.Set("req.headers", out)
		_ = b.Set("req.dirty", true)
		_ = b.Set("status", "header dropped")
		return b.Set("status_err", false)
	})
}

// saveRequest writes the editor back to the collection: in place for a
// saved request, as a new entry for a new one or one from the history.
func (a *app) saveRequest() error {
	req := asMap(a.get("req"))
	if strings.TrimSpace(str(req["url"])) == "" {
		return a.say("nothing to save: the URL is empty", true)
	}
	saved := map[string]any{
		"method": str(req["method"]), "url": str(req["url"]),
		"body": str(req["body"]), "headers": headerLines(asList(req["headers"])),
	}
	id, name := str(req["id"]), str(req["name"])
	coll := asList(a.get("collection"))
	found := false
	for i, c := range coll {
		if m := asMap(c); str(m["id"]) == id && id != "" {
			saved["id"], saved["name"] = id, str(m["name"])
			coll[i], found = saved, true
			name = str(m["name"])
		}
	}
	if !found {
		a.mu.Lock()
		a.cseq++
		id = fmt.Sprintf("c%d", a.cseq)
		a.mu.Unlock()
		name = str(req["method"]) + " " + displayPath(str(req["url"]))
		saved["id"], saved["name"] = id, name
		coll = append(coll, saved)
	}
	return a.ui.Batch(func(b *tuimark.Batch) error {
		_ = b.Set("collection", coll)
		_ = b.Set("req.id", id)
		_ = b.Set("req.name", name)
		_ = b.Set("req.dirty", false)
		_ = b.Set("cursor", id)
		_ = b.Set("status", "saved "+name+" to the collection")
		return b.Set("status_err", false)
	})
}

func (a *app) deleteRequest(id string) error {
	var out []any
	name := ""
	for _, c := range asList(a.get("collection")) {
		if m := asMap(c); str(m["id"]) == id {
			name = str(m["name"])
			continue
		}
		out = append(out, c)
	}
	if name == "" {
		return nil
	}
	if out == nil {
		out = []any{}
	}
	return a.ui.Batch(func(b *tuimark.Batch) error {
		_ = b.Set("collection", out)
		if str(a.get("req.id")) == id {
			_ = b.Set("req.id", "")
		}
		_ = b.Set("status", "deleted "+name)
		return b.Set("status_err", false)
	})
}

// ---- sending ----

// snapshot reads the editor into a request, with {{base}} resolved.
func (a *app) snapshot() request {
	req := asMap(a.get("req"))
	base := str(a.get("base"))
	r := request{
		Method: str(req["method"]), URL: resolve(str(req["url"]), base), Body: str(req["body"]),
	}
	for _, l := range headerLines(asList(req["headers"])) {
		r.Headers = append(r.Headers, str(l))
	}
	return r
}

func resolve(url, base string) string { return strings.ReplaceAll(url, "{{base}}", base) }

// startSend marks the request in flight (the first frame already shows it,
// before any Every tick) and hands the work to a.dispatch.
func (a *app) startSend() error {
	r := a.snapshot()
	if strings.TrimSpace(r.URL) == "" {
		return a.say("nothing to send: the URL is empty", true)
	}
	a.mu.Lock()
	if a.flight != nil {
		a.mu.Unlock()
		return a.say("a request is already in flight: esc cancels it", true)
	}
	ctx, cancel := context.WithCancel(a.root)
	typed := asMap(a.get("req"))
	label := r.Method + " " + displayPath(str(typed["url"]))
	f := &flight{ctx: ctx, cancel: cancel, start: time.Now(), label: label}
	a.flight = f
	a.mu.Unlock()

	sent := historyReq(typed)
	err := a.ui.Batch(func(b *tuimark.Batch) error {
		_ = b.Set("flight", flightView(label, 0, 0, a.timeout))
		_ = b.Set("status", "sending "+label+"…")
		return b.Set("status_err", false)
	})
	a.dispatch(func() { a.perform(ctx, f, r, sent) })
	return err
}

// historyReq is the request as typed (placeholder unresolved), for the
// history entry.
func historyReq(req map[string]any) map[string]any {
	return map[string]any{
		"method": str(req["method"]), "url": str(req["url"]), "body": str(req["body"]),
		"headers": headerLines(asList(req["headers"])),
	}
}

// perform sends r, then applies the outcome. Tests call it directly.
func (a *app) perform(ctx context.Context, f *flight, r request, sent map[string]any) {
	res := send(ctx, a.hc, r, a.timeout)
	_ = a.finish(f, sent, res)
}

// cancelFlight cancels the request in flight, if any.
func (a *app) cancelFlight() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.flight == nil {
		return false
	}
	a.flight.cancel()
	return true
}

// respObject is the store's resp value.
func respObject(res response) map[string]any {
	rows := make([]any, 0, len(res.Headers))
	for i, h := range res.Headers {
		rows = append(rows, map[string]any{"n": float64(i + 1), "name": h.Name, "value": h.Value})
	}
	size, ctype := humanSize(res.Size), res.CType
	if res.Err != "" {
		size, ctype = "-", res.Err
	}
	return map[string]any{
		"status": float64(res.Status), "reason": res.Reason, "error": res.Err,
		"time": humanDuration(res.Dur), "size": size, "ctype": ctype,
		"body": res.Body, "headers": rows, "hcount": float64(len(rows)),
	}
}

// finish applies a response: the pane, the history, and the status line,
// as one Batch. A canceled request shows its response but is not recorded.
func (a *app) finish(f *flight, sent map[string]any, res response) error {
	a.mu.Lock()
	if a.flight != f {
		a.mu.Unlock()
		return nil
	}
	a.flight = nil
	f.cancel()
	resp := respObject(res)
	canceled := res.Err == "canceled"
	id := ""
	if !canceled {
		a.hseq++
		id = fmt.Sprintf("h%d", a.hseq)
		a.cache[id] = resp
	}
	a.mu.Unlock()

	msg, isErr := f.label+" → "+statusLine(res), res.Err != "" || res.Status >= 400
	return a.ui.Batch(func(b *tuimark.Batch) error {
		_ = b.Set("resp", resp)
		_ = b.Set("flight", idleFlight())
		_ = b.Set("status", msg)
		_ = b.Set("status_err", isErr)
		if canceled {
			return nil
		}
		entry := map[string]any{
			"id": id, "method": str(sent["method"]), "url": str(sent["url"]),
			"status": float64(res.Status), "error": res.Err, "took": humanDuration(res.Dur),
			"at": time.Now().Format("15:04:05"), "req": sent,
		}
		hist := append([]any{entry}, asList(a.get("history"))...)
		if len(hist) > 50 {
			hist = hist[:50]
		}
		a.pruneCache(hist)
		return b.Set("history", hist)
	})
}

// pruneCache drops the cached responses of entries that left the history.
func (a *app) pruneCache(hist []any) {
	keep := map[string]bool{}
	for _, h := range hist {
		keep[str(asMap(h)["id"])] = true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for id := range a.cache {
		if !keep[id] {
			delete(a.cache, id)
		}
	}
}

func statusLine(res response) string {
	if res.Err != "" {
		return res.Err
	}
	return fmt.Sprintf("%d %s in %s", res.Status, res.Reason, humanDuration(res.Dur))
}
