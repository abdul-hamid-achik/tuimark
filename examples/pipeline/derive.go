package main

import (
	"fmt"
	"strings"
)

// The Derive functions below turn the raw store (jobs, clock, cur, follow,
// freeze) into what app.tui reads. They are pure: they see the store only
// through get, so the same code builds sample.json for the tests.

// spinner is the braille spinner a running job shows.
var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// icons by status; a running job shows a spinner frame instead.
var icons = map[string]string{
	stQueued:   "○",
	stPassed:   "✓",
	stFailed:   "✗",
	stCanceled: "⊘",
	stSkipped:  "-",
}

// derive declares the values the markup reads but the host never writes.
// They are recomputed inside every store write, in registration order.
func (a *app) derive() error {
	ui := a.ui
	if err := ui.Derive("rows", func(get func(string) (any, bool)) (any, error) {
		jobs, _ := get("jobs")
		clock, _ := get("clock")
		return jobRows(asList(jobs), asMap(clock)), nil
	}); err != nil {
		return err
	}
	if err := ui.Derive("run", func(get func(string) (any, bool)) (any, error) {
		jobs, _ := get("jobs")
		clock, _ := get("clock")
		return runSummary(asList(jobs), asMap(clock)), nil
	}); err != nil {
		return err
	}
	return ui.Derive("log", func(get func(string) (any, bool)) (any, error) {
		jobs, _ := get("jobs")
		cur, _ := get("cur")
		follow, _ := get("follow")
		freeze, _ := get("freeze")
		f, _ := follow.(bool)
		n, _ := freeze.(float64)
		id, _ := cur.(string)
		return logView(asList(jobs), id, f, int(n)), nil
	})
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func num(m map[string]any, k string) float64 {
	f, _ := m[k].(float64)
	return f
}

// jobRows is the list the jobs pane shows: one row per job, with the
// status icon (a spinner frame while it runs), the stage name on the
// first job of each stage, the progress, and the elapsed time.
func jobRows(jobs []any, clock map[string]any) []any {
	spin := int(num(clock, "spin"))
	out := make([]any, 0, len(jobs))
	prev := ""
	for _, it := range jobs {
		j := asMap(it)
		status := str(j, "status")
		icon := icons[status]
		if status == stRunning {
			icon = spinner[spin%len(spinner)]
		}
		stage := str(j, "stage")
		label := ""
		if stage != prev {
			label = stage
		}
		prev = stage
		t := ""
		if status != stQueued && status != stSkipped {
			t = fmtElapsed(num(j, "elapsed"))
		}
		out = append(out, map[string]any{
			"id": str(j, "id"), "name": str(j, "name"), "stage": stage, "stage_label": label,
			"status": status, "icon": icon, "pct": num(j, "pct"), "time": t,
			"queued": status == stQueued, "running": status == stRunning,
			"passed": status == stPassed, "failed": status == stFailed,
			"canceled": status == stCanceled, "skipped": status == stSkipped,
			"idle": status == stQueued || status == stSkipped,
		})
	}
	return out
}

// runSummary is the overall state of the run: its status and badge, the
// mean progress of the jobs, and the labels beside the progress bar.
func runSummary(jobs []any, clock map[string]any) map[string]any {
	counts := map[string]int{}
	sum := 0.0
	for _, it := range jobs {
		j := asMap(it)
		counts[str(j, "status")]++
		sum += num(j, "pct")
	}
	total := len(jobs)
	pct := 0.0
	if total > 0 {
		pct = sum / float64(total)
	}
	status := runStatus(counts, total)
	badge := map[string]string{
		"queued": "○ QUEUED", "running": "● RUNNING", "passed": "✓ PASSED",
		"failed": "✗ FAILED", "canceled": "- CANCELED",
	}[status]
	summary := fmt.Sprintf("%d/%d jobs", counts[stPassed], total)
	if n := counts[stFailed]; n > 0 {
		summary += fmt.Sprintf(" · %d failed", n)
	}
	summary += " · " + fmtElapsed(num(clock, "t"))
	live := ""
	if n := counts[stRunning]; n > 0 {
		live = fmt.Sprintf("%d running", n)
	}
	stopping := counts[stRunning] + counts[stQueued]
	return map[string]any{
		"status": status, "badge": badge, "pct": round1(pct), "pct_label": fmt.Sprintf("%3d%%", int(pct)),
		"summary": summary, "live": live,
		"is_queued": status == "queued", "is_running": status == "running",
		"is_passed": status == "passed", "is_failed": status == "failed", "is_canceled": status == "canceled",
		"cancel_note": fmt.Sprintf("%d running and %d waiting jobs will be stopped.", counts[stRunning], counts[stQueued]),
		"active":      stopping > 0,
	}
}

func runStatus(counts map[string]int, total int) string {
	switch {
	case counts[stCanceled] > 0:
		return "canceled"
	case counts[stRunning] > 0:
		return "running"
	case counts[stQueued] == total:
		return "queued"
	case counts[stQueued] > 0:
		return "running"
	case counts[stFailed] > 0:
		return "failed"
	}
	return "passed"
}

// logView is the selected job's log: the lines to show (held back while
// follow is off), a title that says whether the pane is live, and how many
// lines arrived since it was frozen.
func logView(jobs []any, cur string, follow bool, freeze int) map[string]any {
	var sel map[string]any
	for _, it := range jobs {
		if j := asMap(it); str(j, "id") == cur {
			sel = j
		}
	}
	if sel == nil && len(jobs) > 0 {
		sel = asMap(jobs[0])
	}
	if sel == nil {
		return map[string]any{"title": "log", "lines": []any{}, "none": true, "empty": "no job selected", "new": 0}
	}
	all := asList(sel["log"])
	shown, held := all, 0
	if !follow && freeze < len(all) {
		shown, held = all[:freeze], len(all)-freeze
	}
	lines := make([]any, len(shown))
	for i, it := range shown {
		l := asMap(it)
		kind := str(l, "kind")
		lines[i] = map[string]any{
			"n": num(l, "n"), "ts": str(l, "ts"), "text": str(l, "text"),
			"cmd": kind == kCmd, "info": kind == kInfo, "ok": kind == kOK,
			"warn": kind == kWarn, "err": kind == kErr, "dim": kind == kDim,
		}
	}
	status := str(sel, "status")
	var tag string
	switch {
	case !follow:
		tag = "paused"
		if held > 0 {
			tag = fmt.Sprintf("paused, %d new", held)
		}
	case status == stRunning:
		tag = "following"
	default:
		tag = status
	}
	empty := "waiting for runner"
	switch status {
	case stSkipped:
		empty = "skipped: an earlier stage failed"
	case stCanceled:
		empty = "canceled before it started"
	case stQueued:
		empty = "waiting for the earlier stages"
	}
	return map[string]any{
		"title": strings.TrimSpace(fmt.Sprintf("%s / %s · %s", str(sel, "stage"), str(sel, "name"), tag)),
		"lines": lines, "none": len(lines) == 0, "empty": empty, "new": held,
	}
}
