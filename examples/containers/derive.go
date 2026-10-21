package main

import (
	"fmt"
	"sort"
	"strings"
)

// getter reads the store: App.Get, or the get a Derive function is given.
type getter func(path string) (any, bool)

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

func field(v any, key string) string {
	m, _ := v.(map[string]any)
	s, _ := m[key].(string)
	return s
}

func numField(v any, key string) float64 {
	m, _ := v.(map[string]any)
	n, _ := m[key].(float64)
	return n
}

func boolField(v any, key string) bool {
	m, _ := v.(map[string]any)
	b, _ := m[key].(bool)
	return b
}

// sortKeys is the cycle of `o`: key, label shown in the table title, and
// whether the biggest value comes first.
var sortKeys = []struct {
	key, label string
	desc       bool
}{
	{"name", "name ↑", false},
	{"cpu", "cpu ↓", true},
	{"mem", "mem ↓", true},
	{"uptime", "uptime ↓", true},
	{"status", "status", false},
}

// derive installs the four computed values the markup reads. Each reads the
// store only through get (a Derive function runs under the app's lock).
//
//   - rows: the containers that match filter.q, in the order of sort.key;
//   - counts: the status line and the table title;
//   - detail: the container under the cursor, with its log;
//   - found: the palette's commands that match palette.query.
func (a *app) derive() error {
	if err := a.ui.Derive("rows", func(get func(string) (any, bool)) (any, error) {
		return visibleRows(get), nil
	}); err != nil {
		return err
	}
	if err := a.ui.Derive("counts", func(get func(string) (any, bool)) (any, error) {
		return countsOf(get), nil
	}); err != nil {
		return err
	}
	if err := a.ui.Derive("detail", func(get func(string) (any, bool)) (any, error) {
		return detailOf(get), nil
	}); err != nil {
		return err
	}
	return a.ui.Derive("found", func(get func(string) (any, bool)) (any, error) {
		words := strings.Fields(strings.ToLower(str(get, "palette.query")))
		items := []any{}
		for _, c := range listAt(get, "palette.commands") {
			hay := strings.ToLower(field(c, "label") + " " + field(c, "id"))
			if matchesAll(hay, words) {
				items = append(items, map[string]any{"id": field(c, "id"), "label": field(c, "label"), "hint": field(c, "hint")})
			}
		}
		return map[string]any{"any": len(items) > 0, "items": items}, nil
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

func visibleRows(get getter) []any {
	words := strings.Fields(strings.ToLower(str(get, "filter.q")))
	var rows []any
	for _, c := range listAt(get, "all") {
		hay := strings.ToLower(field(c, "name") + " " + field(c, "image") + " " + field(c, "state"))
		if matchesAll(hay, words) {
			rows = append(rows, c)
		}
	}
	key := str(get, "sort.key")
	less := func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch key {
		case "cpu":
			if x, y := numField(a, "cpu_n"), numField(b, "cpu_n"); x != y {
				return x > y
			}
		case "mem":
			if x, y := numField(a, "mem_n"), numField(b, "mem_n"); x != y {
				return x > y
			}
		case "uptime":
			if x, y := numField(a, "up_n"), numField(b, "up_n"); x != y {
				return x > y
			}
		case "status":
			if x, y := numField(a, "state_n"), numField(b, "state_n"); x != y {
				return x < y
			}
		}
		return field(a, "name") < field(b, "name")
	}
	sort.SliceStable(rows, less)
	// The table needs only its cells, not the samples and the variables of
	// the container, which would be copied again on every write.
	out := make([]any, len(rows))
	for i, r := range rows {
		m, _ := r.(map[string]any)
		row := make(map[string]any, len(rowFields))
		for _, f := range rowFields {
			row[f] = m[f]
		}
		out[i] = row
	}
	return out
}

// rowFields are the members of a container that a table row reads.
var rowFields = []string{"id", "name", "image", "state", "ok", "warn", "bad", "cpu", "mem", "up", "ports"}

func countsOf(get getter) map[string]any {
	all := listAt(get, "all")
	var up, restarting, exited int
	for _, c := range all {
		switch {
		case boolField(c, "ok"):
			up++
		case boolField(c, "warn"):
			restarting++
		default:
			exited++
		}
	}
	shown := len(listAt(get, "rows")) // rows is derived first: one filter and sort per write
	line := fmt.Sprintf("%d up", up)
	if restarting > 0 {
		line += fmt.Sprintf(" · %d restarting", restarting)
	}
	line += fmt.Sprintf(" · %d exited", exited)
	sel := ""
	if n := len(listAt(get, "sel")); n > 0 {
		sel = fmt.Sprintf(" · %d selected", n)
	}
	sortLabel := ""
	for _, k := range sortKeys {
		if k.key == str(get, "sort.key") {
			sortLabel = k.label
		}
	}
	return map[string]any{
		"total": float64(len(all)), "shown": float64(shown), "up": float64(up),
		"restarting": float64(restarting), "exited": float64(exited),
		"any": len(all) > 0, "line": line,
		"images": float64(len(listAt(get, "images"))), "volumes": float64(len(listAt(get, "volumes"))), "sel": sel, "sort": sortLabel,
	}
}

// detailOf is the container under the cursor, with its log lines; an empty
// shell with the same keys when there is none.
func detailOf(get getter) map[string]any {
	cur := str(get, "cur")
	var pick map[string]any
	for _, c := range listAt(get, "all") {
		m, _ := c.(map[string]any)
		if m != nil && m["id"] == cur {
			pick = m
		}
	}
	if pick == nil {
		return map[string]any{
			"any": false, "id": "", "name": "no container", "image": "", "state": "", "ok": false, "warn": false, "bad": false,
			"up": "", "command": "", "network": "", "restart": "", "created": "", "mounts": "",
			"stats": map[string]any{
				"cpu": map[string]any{"value": "", "delta": "", "hist": []any{}},
				"mem": map[string]any{"value": "", "delta": "", "hist": []any{}},
			},
			"env": []any{}, "port_rows": []any{},
			"logs": []any{}, "nlogs": float64(0), "nports": float64(0), "nenv": float64(0),
		}
	}
	out := make(map[string]any, len(pick)+6)
	for k, v := range pick {
		out[k] = v
	}
	// Only this container's lines are read: get copies what it returns.
	v, _ := get("logs." + cur)
	lines, _ := v.([]any)
	if lines == nil {
		lines = []any{}
	}
	out["any"] = true
	out["logs"] = lines
	out["nlogs"] = float64(len(lines))
	ports, _ := pick["port_rows"].([]any)
	env, _ := pick["env"].([]any)
	out["nports"] = float64(len(ports))
	out["nenv"] = float64(len(env))
	return out
}
