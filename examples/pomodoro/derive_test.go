package main

import (
	"strings"
	"testing"
	"time"
)

// mapGet is a getter over a literal store, with Get's dotted paths.
func mapGet(m map[string]any) getter {
	return func(path string) (any, bool) {
		var cur any = m
		for _, seg := range strings.Split(path, ".") {
			mm, ok := cur.(map[string]any)
			if !ok {
				return nil, false
			}
			if cur, ok = mm[seg]; !ok {
				return nil, false
			}
		}
		return cur, true
	}
}

func rawStore(pos int, left, total, flash time.Duration, running bool, now time.Time) map[string]any {
	return map[string]any{
		"pos": float64(pos), "left_ms": float64(left.Milliseconds()), "total_ms": float64(total.Milliseconds()),
		"flash_ms": float64(flash.Milliseconds()), "running": running, "now_ms": float64(now.UnixMilli()),
		"zone": map[string]any{"name": "UTC", "off": float64(0)},
		"plan": map[string]any{"work_ms": float64(25 * 60000), "short_ms": float64(5 * 60000), "long_ms": float64(15 * 60000)},
	}
}

func timerView(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	v, err := deriveTimer(mapGet(m))
	if err != nil {
		t.Fatal(err)
	}
	return v.(map[string]any)
}

func TestDeriveTimerLabelsAndDots(t *testing.T) {
	min := time.Minute
	cases := []struct {
		pos          int
		left, total  time.Duration
		flash        time.Duration
		running      bool
		label, title string
		dots, state  string
		progress     float64
		nextContains string
	}{
		{0, 25 * min, 25 * min, 0, false, "25:00", "FOCUS", "◉ ○ ○ ○", "○ READY", 0, "short break · 5m"},
		{0, 25*min - 500*time.Millisecond, 25 * min, 0, true, "25:00", "FOCUS", "◉ ○ ○ ○", "▶ RUNNING", 0, "short break"}, // 24:59.5 rounds up
		{0, 10 * min, 25 * min, 0, false, "10:00", "FOCUS", "◉ ○ ○ ○", "‖ PAUSED", 60, "short break"},
		{1, 5 * min, 5 * min, 0, true, "05:00", "SHORT BREAK", "● ○ ○ ○", "▶ RUNNING", 0, "next: work · 25m"},
		{2, 3 * min, 25 * min, 0, true, "03:00", "FOCUS", "● ◉ ○ ○", "▶ RUNNING", 88, "short break"},
		{4, 3 * min, 25 * min, 0, true, "03:00", "FOCUS", "● ● ◉ ○", "▶ RUNNING", 88, "short break"},
		{6, 3 * min, 25 * min, 0, true, "03:00", "FOCUS", "● ● ● ◉", "▶ RUNNING", 88, "long break · 15m"},
		{7, 15 * min, 15 * min, 0, true, "15:00", "LONG BREAK", "● ● ● ●", "▶ RUNNING", 0, "next: work · 25m"},
		{0, 0, 25 * min, 1500 * time.Millisecond, true, "00:00", "DONE — time for a break", "● ○ ○ ○", "✓ DONE", 100, "short break"},
		{7, 0, 15 * min, 1500 * time.Millisecond, true, "00:00", "BREAK OVER — back to work", "● ● ● ●", "✓ DONE", 100, "work"},
	}
	for i, c := range cases {
		v := timerView(t, rawStore(c.pos, c.left, c.total, c.flash, c.running, t0))
		if v["label"] != c.label || v["title"] != c.title || v["dots"] != c.dots || v["state"] != c.state {
			t.Errorf("case %d: label %q title %q dots %q state %q, want %q %q %q %q",
				i, v["label"], v["title"], v["dots"], v["state"], c.label, c.title, c.dots, c.state)
		}
		if p := v["progress"].(float64); int(p) != int(c.progress) {
			t.Errorf("case %d: progress %v, want %v", i, p, c.progress)
		}
		if !strings.Contains(v["next"].(string), c.nextContains) {
			t.Errorf("case %d: next %q lacks %q", i, v["next"], c.nextContains)
		}
		big := v["big"].(string)
		if lines := strings.Split(big, "\n"); len(lines) != fontRows || len([]rune(lines[0])) != bigWidth(c.label) {
			t.Errorf("case %d: big digits are %d rows, %d wide", i, len(lines), len([]rune(lines[0])))
		}
	}
}

func TestDeriveIs(t *testing.T) {
	get := func(pos int, left, total, flash time.Duration, running bool, now time.Time) map[string]any {
		v, err := deriveIs(mapGet(rawStore(pos, left, total, flash, running, now)))
		if err != nil {
			t.Fatal(err)
		}
		return v.(map[string]any)
	}
	on := t0                              // :00.000, the pulse is on
	off := t0.Add(500 * time.Millisecond) // :00.500, off
	if is := get(0, 9*time.Second, time.Minute, 0, true, on); !is["last10"].(bool) || !is["pulse"].(bool) || is["paused"].(bool) {
		t.Errorf("9s left, running, pulse half: %v", is)
	}
	if is := get(0, 9*time.Second, time.Minute, 0, true, off); !is["last10"].(bool) || is["pulse"].(bool) {
		t.Errorf("9s left, running, off half: %v", is)
	}
	if is := get(0, 9*time.Second, time.Minute, 0, false, on); is["last10"].(bool) || is["pulse"].(bool) || !is["paused"].(bool) {
		t.Errorf("9s left, paused: %v", is)
	}
	if is := get(0, 11*time.Second, time.Minute, 0, true, on); is["last10"].(bool) || is["pulse"].(bool) {
		t.Errorf("11s left: %v", is)
	}
	if is := get(3, time.Minute, time.Minute, 0, false, on); !is["short"].(bool) || !is["ready"].(bool) || is["work"].(bool) {
		t.Errorf("a stopped, untouched break: %v", is)
	}
	// The flash alternates every half second and does not pulse.
	for i, want := range map[time.Duration]bool{1500 * time.Millisecond: true, 1000 * time.Millisecond: false, 500 * time.Millisecond: true} {
		is := get(0, 0, time.Minute, i, true, on)
		if is["flash_on"] != want || !is["flash"].(bool) || is["pulse"].(bool) || is["last10"].(bool) {
			t.Errorf("flash %s: %v, want flash_on %v", i, is, want)
		}
	}
}

func TestDeriveWorldAcrossMidnight(t *testing.T) {
	// 23:30 UTC; the local zone is UTC+2, where it is 01:30 the next day.
	now := time.Date(2026, 10, 17, 23, 30, 0, 0, time.UTC)
	m := rawStore(0, time.Minute, time.Minute, 0, false, now)
	m["zone"] = map[string]any{"name": "CEST", "off": float64(2 * 3600)}
	v, err := deriveWorld(mapGet(m))
	if err != nil {
		t.Fatal(err)
	}
	rows := v.([]any)
	if len(rows) != len(worldCities)+1 {
		t.Fatalf("%d rows", len(rows))
	}
	want := map[string][3]string{ // city -> offset, time, day
		"Local":    {"UTC+2", "01:30", ""},
		"Honolulu": {"UTC-10", "13:30", "-1d"},
		"New York": {"UTC-5", "18:30", "-1d"},
		"London":   {"UTC+0", "23:30", "-1d"},
		"Berlin":   {"UTC+1", "00:30", ""},
		"Mumbai":   {"UTC+5:30", "05:00", ""},
		"Tokyo":    {"UTC+9", "08:30", ""},
		"Sydney":   {"UTC+10", "09:30", ""},
	}
	for _, r := range rows {
		row := r.(map[string]any)
		w, ok := want[row["city"].(string)]
		if !ok {
			t.Errorf("unexpected row %v", row)
			continue
		}
		if row["offset"] != w[0] || row["time"] != w[1] || row["day"] != w[2] {
			t.Errorf("%v, want %v", row, w)
		}
	}
}

func TestDeriveClock(t *testing.T) {
	now := time.Date(2026, 3, 1, 8, 5, 9, 250_000_000, time.UTC)
	m := rawStore(0, time.Minute, time.Minute, 0, false, now)
	m["zone"] = map[string]any{"name": "IST", "off": float64(5*3600 + 1800)}
	v, err := deriveClock(mapGet(m))
	if err != nil {
		t.Fatal(err)
	}
	c := v.(map[string]any)
	if c["date"] != "Sunday, 1 March 2026" || c["zone"] != "IST · UTC+5:30" {
		t.Errorf("clock = %v", c)
	}
	want := bigText("13:35:09", true) // 250 ms into the second: the colons are on
	if c["big"] != want {
		t.Errorf("big digits:\n%v\nwant\n%v", c["big"], want)
	}
	m["now_ms"] = float64(now.Add(500 * time.Millisecond).UnixMilli())
	v, _ = deriveClock(mapGet(m))
	if v.(map[string]any)["big"] != bigText("13:35:09", false) {
		t.Error("the colons did not blink off in the second half of the second")
	}
}

func TestOffsetAndDurationText(t *testing.T) {
	for in, want := range map[int]string{0: "UTC+0", 3600: "UTC+1", -36000: "UTC-10", 19800: "UTC+5:30", -12600: "UTC-3:30"} {
		if got := offsetText(in); got != want {
			t.Errorf("offsetText(%d) = %s, want %s", in, got, want)
		}
	}
	for in, want := range map[time.Duration]string{0: "0m", 25 * time.Minute: "25m", 90 * time.Second: "1m30s", time.Hour + 15*time.Minute: "75m"} {
		if got := shortDur(in); got != want {
			t.Errorf("shortDur(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestDeriveStats(t *testing.T) {
	get := mapGet(map[string]any{"sessions": []any{
		map[string]any{"kind": "short", "secs": float64(300)},
		map[string]any{"kind": "work", "secs": float64(1500)},
		map[string]any{"kind": "work", "secs": float64(1500)},
	}})
	v, _ := deriveStats(get)
	s := v.(map[string]any)
	if s["count"] != 3 || s["empty"] != false || s["focus"] != "50m" {
		t.Errorf("stats = %v", s)
	}
	v, _ = deriveStats(mapGet(map[string]any{"sessions": []any{}}))
	if s := v.(map[string]any); s["count"] != 0 || s["empty"] != true || s["focus"] != "0m" {
		t.Errorf("empty stats = %v", s)
	}
}
