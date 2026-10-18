package main

import (
	"fmt"
	"time"
)

// The Derive functions below are pure functions of the raw store (pos,
// left_ms, total_ms, running, flash_ms, now_ms, zone, plan, sessions): they
// read numbers as float64, as the store holds them, and call no App method
// (Derive functions run under the app's lock).

// getter is the get function Derive hands its callback.
type getter func(path string) (any, bool)

func num(get getter, path string) float64 {
	v, _ := get(path)
	f, _ := v.(float64)
	return f
}

func str(get getter, path string) string {
	v, _ := get(path)
	s, _ := v.(string)
	return s
}

func boolAt(get getter, path string) bool {
	v, _ := get(path)
	b, _ := v.(bool)
	return b
}

func ms(get getter, path string) time.Duration {
	return time.Duration(num(get, path)) * time.Millisecond
}

// register installs the derived values. The order matters: a later one may
// read an earlier one.
func (a *app) derive() error {
	for _, d := range []struct {
		path string
		fn   func(getter) (any, error)
	}{
		{"timer", deriveTimer},
		{"is", deriveIs},
		{"clock", deriveClock},
		{"world", deriveWorld},
		{"stats", deriveStats},
	} {
		if err := a.ui.Derive(d.path, func(get func(string) (any, bool)) (any, error) { return d.fn(get) }); err != nil {
			return err
		}
	}
	return nil
}

// colonOn is the blink: the colon is drawn in the first half of each
// second.
func colonOn(nowMs int64) bool { return ((nowMs%1000)+1000)%1000 < 500 }

// phaseView is the raw state the derived values read, decoded once.
type phaseView struct {
	pos           int
	kind          string
	left, total   time.Duration
	running       bool
	flash         time.Duration
	nowMs         int64
	flashing      bool
	paused, ready bool
	last10, pulse bool
	flashOn       bool
	secs          int // seconds shown, rounded up
	progress      float64
}

func readPhase(get getter) phaseView {
	v := phaseView{
		pos:     int(num(get, "pos")),
		left:    ms(get, "left_ms"),
		total:   ms(get, "total_ms"),
		running: boolAt(get, "running"),
		flash:   ms(get, "flash_ms"),
		nowMs:   int64(num(get, "now_ms")),
	}
	v.kind = kindAt(v.pos)
	v.flashing = v.flash > 0
	v.paused = !v.running && !v.flashing && v.left < v.total
	v.ready = !v.running && !v.flashing && v.left >= v.total
	v.last10 = v.running && !v.flashing && v.left <= 10*time.Second
	v.pulse = v.last10 && (v.nowMs/500)%2 == 0
	v.flashOn = v.flashing && (v.flash/(500*time.Millisecond))%2 == 1
	v.secs = int((v.left + time.Second - 1) / time.Second)
	switch {
	case v.flashing:
		v.progress = 100
	case v.total > 0:
		v.progress = 100 * float64(v.total-v.left) / float64(v.total)
	}
	return v
}

var kindNames = map[string]string{kindWork: "work", kindShort: "short break", kindLong: "long break"}

func mmss(secs int) string { return fmt.Sprintf("%02d:%02d", secs/60, secs%60) }

func shortDur(d time.Duration) string {
	s := int(d / time.Second)
	if s%60 == 0 {
		return fmt.Sprintf("%dm", s/60)
	}
	return fmt.Sprintf("%dm%02ds", s/60, s%60)
}

// deriveTimer is everything the Pomodoro tab shows as text.
func deriveTimer(get getter) (any, error) {
	v := readPhase(get)
	label := mmss(v.secs)
	colon := !v.running || colonOn(v.nowMs)
	title, state := "", ""
	switch {
	case v.flashing && v.kind == kindWork:
		title, state = "DONE — time for a break", "✓ DONE"
	case v.flashing:
		title, state = "BREAK OVER — back to work", "✓ DONE"
	case v.kind == kindWork:
		title = "FOCUS"
	case v.kind == kindShort:
		title = "SHORT BREAK"
	default:
		title = "LONG BREAK"
	}
	switch {
	case v.flashing:
	case v.running:
		state = "▶ RUNNING"
	case v.paused:
		state = "‖ PAUSED"
	default:
		state = "○ READY"
	}
	done := (v.pos + 1) / 2
	if v.flashing && v.kind == kindWork {
		done++
	}
	dots := ""
	for i := 0; i < 4; i++ {
		d := "○"
		switch {
		case i < done:
			d = "●"
		case i == done && v.kind == kindWork && !v.flashing:
			d = "◉"
		}
		if i > 0 {
			dots += " "
		}
		dots += d
	}
	nextKind := kindAt(v.pos + 1)
	pl := plan{
		work:  time.Duration(num(get, "plan.work_ms")) * time.Millisecond,
		short: time.Duration(num(get, "plan.short_ms")) * time.Millisecond,
		long:  time.Duration(num(get, "plan.long_ms")) * time.Millisecond,
	}
	next := fmt.Sprintf("next: %s · %s", kindNames[nextKind], shortDur(pl.of(nextKind)))
	return map[string]any{
		"label":    label,
		"big":      bigText(label, colon),
		"title":    title,
		"state":    state,
		"dots":     dots,
		"next":     next,
		"progress": v.progress,
		"pct":      fmt.Sprintf("%d%%", int(v.progress)),
	}, nil
}

// deriveIs holds the booleans the class: guards read.
func deriveIs(get getter) (any, error) {
	v := readPhase(get)
	return map[string]any{
		"work":     v.kind == kindWork,
		"short":    v.kind == kindShort,
		"long":     v.kind == kindLong,
		"running":  v.running,
		"paused":   v.paused,
		"ready":    v.ready,
		"last10":   v.last10,
		"pulse":    v.pulse,
		"flash":    v.flashing,
		"flash_on": v.flashOn,
	}, nil
}

// wall is the local wall clock as a UTC time: the instant shifted by the
// zone's offset, so formatting it needs no time zone database.
func wall(nowMs int64, offSec float64) time.Time {
	return time.UnixMilli(nowMs).UTC().Add(time.Duration(offSec) * time.Second)
}

func offsetText(offSec int) string {
	sign := '+'
	if offSec < 0 {
		sign, offSec = '-', -offSec
	}
	h, m := offSec/3600, offSec%3600/60
	if m == 0 {
		return fmt.Sprintf("UTC%c%d", sign, h)
	}
	return fmt.Sprintf("UTC%c%d:%02d", sign, h, m)
}

func deriveClock(get getter) (any, error) {
	nowMs := int64(num(get, "now_ms"))
	off := num(get, "zone.off")
	t := wall(nowMs, off)
	zone := str(get, "zone.name")
	if zone != offsetText(int(off)) {
		zone += " · " + offsetText(int(off))
	}
	return map[string]any{
		"big":  bigText(t.Format("15:04:05"), colonOn(nowMs)),
		"time": t.Format("15:04:05"),
		"date": t.Format("Monday, 2 January 2006"),
		"zone": zone,
	}, nil
}

// worldCity is a world clock row: a fixed UTC offset, no DST, no tz
// database, so the rows are the same on every machine.
type worldCity struct {
	name string
	off  int // seconds east of UTC
}

var worldCities = []worldCity{
	{"Honolulu", -10 * 3600},
	{"New York", -5 * 3600},
	{"London", 0},
	{"Berlin", 1 * 3600},
	{"Mumbai", 5*3600 + 30*60},
	{"Tokyo", 9 * 3600},
	{"Sydney", 10 * 3600},
}

func deriveWorld(get getter) (any, error) {
	nowMs := int64(num(get, "now_ms"))
	localOff := num(get, "zone.off")
	local := wall(nowMs, localOff)
	rows := []any{map[string]any{
		"city": "Local", "offset": offsetText(int(localOff)), "time": local.Format("15:04"), "day": "",
	}}
	for _, c := range worldCities {
		t := wall(nowMs, float64(c.off))
		day := ""
		switch {
		case t.YearDay() == local.YearDay() && t.Year() == local.Year():
		case t.After(local):
			day = "+1d"
		default:
			day = "-1d"
		}
		rows = append(rows, map[string]any{
			"city": c.name, "offset": offsetText(c.off), "time": t.Format("15:04"), "day": day,
		})
	}
	return rows, nil
}

func deriveStats(get getter) (any, error) {
	v, _ := get("sessions")
	list, _ := v.([]any)
	var focus time.Duration
	for _, e := range list {
		m, _ := e.(map[string]any)
		if m["kind"] == kindWork {
			secs, _ := m["secs"].(float64)
			focus += time.Duration(secs) * time.Second
		}
	}
	return map[string]any{
		"count": len(list),
		"empty": len(list) == 0,
		"focus": shortDur(focus),
	}, nil
}
