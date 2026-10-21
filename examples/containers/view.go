package main

import (
	"fmt"
	"strings"
	"time"
)

// This file turns the simulation into plain store values: strings already
// formatted, flags for the class: guards, numbers for sorting. The markup
// has no expressions, so everything a cell shows is decided here.

func fmtMem(mib float64) string {
	if mib >= 1024 {
		return fmt.Sprintf("%.1fG", mib/1024)
	}
	return fmt.Sprintf("%.0fM", mib)
}

func fmtSize(mb float64) string {
	if mb >= 1000 {
		return fmt.Sprintf("%.1fGB", mb/1000)
	}
	return fmt.Sprintf("%.0fMB", mb)
}

// fmtUp is a compact uptime: 12d4h, 3h12m, 45m10s, 8s.
func fmtUp(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	switch {
	case s >= 86400:
		return fmt.Sprintf("%dd%dh", s/86400, s%86400/3600)
	case s >= 3600:
		return fmt.Sprintf("%dh%02dm", s/3600, s%3600/60)
	case s >= 60:
		return fmt.Sprintf("%dm%02ds", s/60, s%60)
	}
	return fmt.Sprintf("%ds", s)
}

func ago(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	case d >= time.Hour:
		return plural(int(d.Hours()), "hour", "hours") + " ago"
	case d >= time.Minute:
		return plural(int(d.Minutes()), "minute", "minutes") + " ago"
	}
	return "just now"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func statusText(c *container) string {
	if c.state == stExited {
		return fmt.Sprintf("exit %d", c.exit)
	}
	return c.state
}

func stateRank(c *container) int {
	switch c.state {
	case stRunning:
		return 0
	case stRestarting:
		return 1
	}
	return 2
}

func portsText(c *container) string {
	if len(c.ports) == 0 {
		return "-"
	}
	p := c.ports[0]
	s := fmt.Sprintf("%d→%d", p.host, p.cont)
	if len(c.ports) > 1 {
		s += fmt.Sprintf(" +%d", len(c.ports)-1)
	}
	return s
}

func avg(h []float64) float64 {
	if len(h) == 0 {
		return 0
	}
	t := 0.0
	for _, v := range h {
		t += v
	}
	return t / float64(len(h))
}

func floats(h []float64) []any {
	out := make([]any, len(h))
	for i, v := range h {
		out[i] = float64(int(v*10+0.5)) / 10
	}
	return out
}

// containerView is one container as the store holds it.
func (s *sim) containerView(c *container) map[string]any {
	up := "-"
	upN := 0.0
	if c.state == stRunning {
		d := s.now.Sub(c.started)
		up, upN = fmtUp(d), d.Seconds()
	}
	cpu, mem := "-", "-"
	if c.state == stRunning {
		cpu, mem = fmt.Sprintf("%.1f", c.cpu), fmtMem(c.mem)
	}
	env := make([]any, 0, len(c.env))
	for _, e := range c.env {
		env = append(env, map[string]any{"k": e.k, "v": e.v})
	}
	ports := make([]any, 0, len(c.ports))
	for _, p := range c.ports {
		ports = append(ports, map[string]any{
			"key": fmt.Sprintf("%d/%s", p.host, p.proto), "host": float64(p.host), "cont": float64(p.cont),
			"proto": p.proto, "addr": "0.0.0.0",
		})
	}
	memTxt := "-"
	if c.state == stRunning {
		memTxt = fmtMem(c.mem) + " / " + fmtMem(c.memLimit)
	}
	cpuTxt := "-"
	if c.state == stRunning {
		cpuTxt = cpu + "%"
	}
	mounts := "-"
	if len(c.mounts) > 0 {
		mounts = strings.Join(c.mounts, ", ")
	}
	return map[string]any{
		"id": c.id, "name": c.name, "image": c.image,
		"state": statusText(c), "ok": c.state == stRunning, "warn": c.state == stRestarting,
		"bad": c.state == stExited && c.exit != 0,
		"cpu": cpu, "mem": mem, "up": up, "ports": portsText(c),
		"cpu_n": float64(int(c.cpu*10)) / 10, "mem_n": float64(int(c.mem)), "up_n": float64(int(upN)), "state_n": float64(stateRank(c)),
		"command": c.command, "network": c.network + " · " + c.ip, "restart": c.restart,
		"created": ago(s.now, c.created), "mounts": mounts,
		// The two k-stat cards read value, delta and hist under their alias.
		"stats": map[string]any{
			"cpu": map[string]any{"value": cpuTxt, "delta": fmt.Sprintf("avg %.1f%%", avg(c.cpuHist)), "hist": floats(c.cpuHist)},
			"mem": map[string]any{"value": memTxt, "delta": fmt.Sprintf("avg %.0f%% of limit", avg(c.memHist)), "hist": floats(c.memHist)},
		},
		"env": env, "port_rows": ports,
	}
}

func lineViews(ls []logLine) []any {
	out := make([]any, len(ls))
	for i, l := range ls {
		out[i] = map[string]any{
			"n": float64(l.n), "text": l.text,
			"debug": l.lvl == "debug", "info": l.lvl == "info", "warn": l.lvl == "warn", "err": l.lvl == "err",
		}
	}
	return out
}

func (s *sim) allView() []any {
	out := make([]any, len(s.cs))
	for i, c := range s.cs {
		out[i] = s.containerView(c)
	}
	return out
}

func (s *sim) logsView() map[string]any {
	out := make(map[string]any, len(s.cs))
	for _, c := range s.cs {
		out[c.id] = lineViews(c.logs)
	}
	return out
}

func (s *sim) eventsView() map[string]any {
	return map[string]any{"lines": lineViews(s.events), "count": float64(len(s.events))}
}

func (s *sim) imagesView() []any {
	used := map[string][]string{}
	for _, c := range s.cs {
		used[c.image] = append(used[c.image], c.name)
	}
	out := make([]any, len(s.images))
	for i, im := range s.images {
		ref := im.repo + ":" + im.tag
		by := "-"
		if n := len(used[ref]); n > 0 {
			by = fmt.Sprint(n)
		}
		out[i] = map[string]any{
			"id": im.id, "repo": im.repo, "tag": im.tag, "short": im.id, "size": fmtSize(im.mb),
			"created": ago(s.now, im.created), "used": by, "idle": by == "-",
		}
	}
	return out
}

func (s *sim) volumesView() []any {
	used := map[string][]string{}
	for _, c := range s.cs {
		for _, m := range c.mounts {
			used[m] = append(used[m], c.name)
		}
	}
	out := make([]any, len(s.volumes))
	for i, v := range s.volumes {
		by := "-"
		if len(used[v.name]) > 0 {
			by = strings.Join(used[v.name], ", ")
		}
		out[i] = map[string]any{
			"name": v.name, "driver": v.driver, "size": fmtSize(v.mb), "used": by, "idle": by == "-",
			"created": ago(s.now, v.created),
		}
	}
	return out
}

// engineView is the status bar's chip: ok, or degraded while a container is
// restarting or has exited with a failure.
func (s *sim) engineView() map[string]any {
	warn := false
	for _, c := range s.cs {
		if c.state == stRestarting || (c.state == stExited && c.exit != 0) {
			warn = true
		}
	}
	label := "ENGINE UP"
	if warn {
		label = "DEGRADED"
	}
	return map[string]any{"label": label, "ok": !warn, "warn": warn, "server": "docker 27.1.1 · simulated"}
}
