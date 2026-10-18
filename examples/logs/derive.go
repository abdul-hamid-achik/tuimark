package main

import (
	"fmt"
	"strings"
)

// derive declares the values the markup reads but the host never writes:
// the filtered line list, the per-level counts, and two status labels.
// Each is recomputed by the runtime inside every store write (a tick's
// batch, a toggle, a keystroke in the filter box), so none is ever stale.
// Derived values are computed in registration order: stats reads visible.
func (a *app) derive() error {
	ui := a.ui
	if err := ui.Derive("visible", func(get func(string) (any, bool)) (any, error) {
		lines, _ := get("lines")
		levels, _ := get("levels")
		query, _ := get("filter")
		ls, _ := lines.([]any)
		lv, _ := levels.(map[string]any)
		q, _ := query.(string)
		return visibleLines(ls, lv, q), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("stats", func(get func(string) (any, bool)) (any, error) {
		lines, _ := get("lines")
		visible, _ := get("visible")
		ls, _ := lines.([]any)
		vs, _ := visible.([]any)
		return computeStats(ls, vs), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("state_label", func(get func(string) (any, bool)) (any, error) {
		paused, _ := get("paused")
		if p, _ := paused.(bool); p {
			return "‖ PAUSED", nil
		}
		return "● LIVE", nil
	}); err != nil {
		return err
	}
	return ui.Derive("rate_label", func(get func(string) (any, bool)) (any, error) {
		hist, _ := get("rate_hist")
		hs, _ := hist.([]any)
		last := 0.0
		if len(hs) > 0 {
			last, _ = hs[len(hs)-1].(float64)
		}
		return rateLabel(last), nil
	})
}

// rateLabel is the lines-per-second label for the last full second.
func rateLabel(perSec float64) string { return fmt.Sprintf("%d lines/s", int(perSec)) }

// parseQuery splits the filter text into the words a line must contain
// and the words it must not (a leading - or !). Matching is by lowercase
// substring, so "pg 5" finds lines holding both.
func parseQuery(q string) (include, exclude []string) {
	for _, w := range strings.Fields(strings.ToLower(q)) {
		switch {
		case (w[0] == '-' || w[0] == '!') && len(w) > 1:
			exclude = append(exclude, w[1:])
		case w == "-" || w == "!":
		default:
			include = append(include, w)
		}
	}
	return include, exclude
}

// visibleLines keeps the lines whose level is switched on and whose search
// text (service, level, message; a stack frame carries its parent's) holds
// every wanted word and none of the excluded ones. The result drops the
// search text, which the markup never reads.
func visibleLines(lines []any, levels map[string]any, query string) []any {
	include, exclude := parseQuery(query)
	out := make([]any, 0, len(lines))
	for _, it := range lines {
		l, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if lvl, _ := l["level"].(string); levels != nil {
			if on, present := levels[lvl].(bool); present && !on {
				continue
			}
		}
		hay, _ := l["hay"].(string)
		if !matches(hay, include, exclude) {
			continue
		}
		v := make(map[string]any, len(l))
		for k, x := range l {
			if k != "hay" {
				v[k] = x
			}
		}
		out = append(out, v)
	}
	return out
}

func matches(hay string, include, exclude []string) bool {
	for _, w := range include {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	for _, w := range exclude {
		if strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

// computeStats counts the buffered events per level (a stack frame is part
// of its error, not another one) and the rows shown out of the rows kept.
// none is true when the filter leaves nothing to show.
func computeStats(lines, visible []any) map[string]any {
	n := map[string]int{}
	for _, it := range lines {
		l, _ := it.(map[string]any)
		if cont, _ := l["cont"].(bool); cont {
			continue
		}
		lvl, _ := l["level"].(string)
		n[lvl]++
	}
	return map[string]any{
		"error": n[lvlError], "warn": n[lvlWarn], "info": n[lvlInfo], "debug": n[lvlDebug],
		"total": len(lines), "shown": len(visible), "none": len(visible) == 0,
	}
}
