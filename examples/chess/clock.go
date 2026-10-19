package main

import (
	"fmt"
	"strings"
)

// clockSpec is a time control: Base milliseconds each and Inc milliseconds
// added after every move. A zero Base is no clock.
type clockSpec struct {
	Base, Inc int64
}

func (c clockSpec) On() bool { return c.Base > 0 }

// String is the spec as typed: "5+0", "3+2", or "none".
func (c clockSpec) String() string {
	if !c.On() {
		return "none"
	}
	tenths := c.Base / 6000
	mins := fmt.Sprint(tenths / 10)
	if tenths%10 != 0 {
		mins += fmt.Sprintf(".%d", tenths%10)
	}
	return fmt.Sprintf("%s+%d", mins, c.Inc/1000)
}

// clockChoices are what the new game dialog cycles through.
var clockChoices = []string{"none", "5+0", "3+2", "10+5"}

// parseClock reads "none" or "MINUTES+SECONDS": the minutes are a plain
// decimal with at most one decimal place (0.5+0 is thirty seconds), the
// increment a whole number of seconds. Anything else (exponents, hex, signs,
// NaN, Inf, a fraction of a second) is an error rather than a silent rounding.
func parseClock(s string) (clockSpec, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "none" || s == "off" {
		return clockSpec{}, nil
	}
	m, i, ok := strings.Cut(s, "+")
	if !ok {
		i = "0"
	}
	tenths, okM := parseTenths(m)
	if !okM {
		return clockSpec{}, fmt.Errorf("clock %q: the minutes must be a plain number such as 5 or 0.5", s)
	}
	inc, okI := parseWhole(i)
	if !okI {
		return clockSpec{}, fmt.Errorf("clock %q: the increment must be a whole number of seconds", s)
	}
	if tenths < 1 || tenths > 1800 {
		return clockSpec{}, fmt.Errorf("clock %q: minutes must be between 0.1 and 180", s)
	}
	if inc > 60 {
		return clockSpec{}, fmt.Errorf("clock %q: the increment must be between 0 and 60 seconds", s)
	}
	return clockSpec{Base: tenths * 6000, Inc: inc * 1000}, nil
}

// parseWhole reads one to four ASCII digits.
func parseWhole(s string) (int64, bool) {
	if s == "" || len(s) > 4 {
		return 0, false
	}
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int64(r-'0')
	}
	return n, true
}

// parseTenths reads digits with an optional single decimal place, as tenths.
func parseTenths(s string) (int64, bool) {
	whole, frac, dot := strings.Cut(s, ".")
	n, ok := parseWhole(whole)
	if !ok {
		return 0, false
	}
	n *= 10
	if dot {
		if len(frac) != 1 || frac[0] < '0' || frac[0] > '9' {
			return 0, false
		}
		n += int64(frac[0] - '0')
	}
	return n, true
}

// fmtClock writes remaining milliseconds: 4:58 down to 20 seconds, then
// seconds and tenths (19.8). Integer arithmetic only.
func fmtClock(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	if ms < 20000 {
		return fmt.Sprintf("%d.%d", ms/1000, (ms%1000)/100)
	}
	s := (ms + 999) / 1000
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s%3600/60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
