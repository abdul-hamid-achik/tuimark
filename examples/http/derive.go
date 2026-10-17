package main

import (
	"fmt"
	"strings"
)

// getter is the get function Derive hands to its callback.
type getter = func(path string) (any, bool)

func val(get getter, path string) any { v, _ := get(path); return v }

// derive registers the values the markup reads but the host never Sets:
// they are recomputed inside every store change, so the view can show a
// filtered list, a count, or a one-line summary with no expression in it.
//
// Every derived path is also in sample.json (the CLI has no Derive), and a
// test checks that the two agree.
func (a *app) derive() error {
	derived := []struct {
		path string
		fn   func(get getter) (any, error)
	}{
		{"mf", func(get getter) (any, error) { return methodFlags(str(val(get, "req.method"))), nil }},
		{"visible", func(get getter) (any, error) {
			return visibleRequests(asList(val(get, "collection")), str(val(get, "query"))), nil
		}},
		{"history_view", func(get getter) (any, error) {
			return historyView(asList(val(get, "history")), str(val(get, "query"))), nil
		}},
		{"counts", func(get getter) (any, error) {
			q := str(val(get, "query"))
			coll, hist := asList(val(get, "collection")), asList(val(get, "history"))
			return map[string]any{
				"coll": count(len(visibleRequests(coll, q)), len(coll), q),
				"hist": count(len(historyView(hist, q)), len(hist), q),
			}, nil
		}},
		{"rv", func(get getter) (any, error) { return responseView(asMap(val(get, "resp"))), nil }},
		{"summary", func(get getter) (any, error) {
			return summaryLine(asMap(val(get, "req")), str(val(get, "base"))), nil
		}},
		{"edit_title", func(get getter) (any, error) { return editTitle(asMap(val(get, "req"))), nil }},
	}
	for _, d := range derived {
		if err := a.ui.Derive(d.path, d.fn); err != nil {
			return fmt.Errorf("derive %s: %w", d.path, err)
		}
	}
	return nil
}

// methodFlags is one boolean per method: the class:NAME guards of the
// method badge read them.
func methodFlags(method string) map[string]any {
	m := map[string]any{}
	for _, name := range methods {
		m[strings.ToLower(name)] = method == name
	}
	return m
}

func count(shown, total int, query string) string {
	if strings.TrimSpace(query) == "" {
		return fmt.Sprint(total)
	}
	return fmt.Sprintf("%d/%d", shown, total)
}

func matches(query string, fields ...string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(strings.Join(fields, " ")), q)
}

// visibleRequests is the collection filtered by the query, one row per
// request with its method flags.
func visibleRequests(coll []any, query string) []any {
	rows := []any{}
	for _, c := range coll {
		m := asMap(c)
		if !matches(query, str(m["name"]), str(m["method"]), str(m["url"])) {
			continue
		}
		row := methodFlags(str(m["method"]))
		row["id"], row["name"], row["method"] = m["id"], m["name"], m["method"]
		rows = append(rows, row)
	}
	return rows
}

// historyView is the history filtered by the query, with each entry's
// status class flags and a short path.
func historyView(hist []any, query string) []any {
	rows := []any{}
	for _, h := range hist {
		m := asMap(h)
		code := "ERR"
		status := int(flt(m["status"]))
		failed := str(m["error"]) != ""
		if !failed {
			code = fmt.Sprint(status)
		}
		if !matches(query, str(m["method"]), str(m["url"]), code) {
			continue
		}
		row := methodFlags(str(m["method"]))
		for k, v := range classFlags(status, failed) {
			row[k] = v
		}
		row["id"], row["method"], row["code"] = m["id"], m["method"], code
		row["path"], row["took"] = displayPath(str(m["url"])), m["took"]
		rows = append(rows, row)
	}
	return rows
}

// classFlags are the status class guards: 2xx, 3xx, 4xx, 5xx, or a failure.
func classFlags(status int, failed bool) map[string]any {
	return map[string]any{
		"k2":   !failed && status >= 200 && status < 300,
		"k3":   !failed && status >= 300 && status < 400,
		"k4":   !failed && status >= 400 && status < 500,
		"k5":   !failed && status >= 500,
		"kerr": failed,
	}
}

// responseView is the status line of the response pane.
func responseView(resp map[string]any) map[string]any {
	status := int(flt(resp["status"]))
	failed := str(resp["error"]) != ""
	v := classFlags(status, failed)
	switch {
	case failed:
		v["line"] = "ERR"
	case status == 0:
		v["line"] = "no response yet"
	default:
		v["line"] = fmt.Sprintf("%d %s", status, str(resp["reason"]))
	}
	return v
}

// summaryLine reads "POST http://127.0.0.1:8099/users · 2 headers · body 52 B".
func summaryLine(req map[string]any, base string) string {
	url := resolve(str(req["url"]), base)
	if strings.TrimSpace(url) == "" {
		return "no URL yet"
	}
	hs := len(asList(req["headers"]))
	parts := []string{str(req["method"]) + " " + url, plural(hs, "header")}
	if body := str(req["body"]); body != "" {
		parts = append(parts, "body "+humanSize(len(body)))
	} else {
		parts = append(parts, "no body")
	}
	return strings.Join(parts, " · ")
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func editTitle(req map[string]any) string {
	name := str(req["name"])
	if name == "" {
		name = "untitled"
	}
	if truthy(req["dirty"]) {
		return "request · " + name + " ●"
	}
	return "request · " + name
}

// displayPath shortens a URL for a list row: no placeholder, no scheme.
func displayPath(url string) string {
	url = strings.TrimPrefix(url, "{{base}}")
	for _, p := range []string{"https://", "http://"} {
		url = strings.TrimPrefix(url, p)
	}
	if url == "" {
		return "/"
	}
	return url
}
