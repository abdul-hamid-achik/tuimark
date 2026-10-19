// format: numbers, sizes and times as strings ready for {path}.
//
// Copy this file into your project and change the package name. It has no
// dependencies (not even tuimark).
//
// Markup has no expressions, so a size or a duration is formatted in the host
// and written to the store as text: Set("disk.text", kit.Bytes(n)).
package kit

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Bytes formats a byte count with binary units: 0 B, 999 B, 1.0 KiB,
// 1.5 MiB, 3.2 GiB. One decimal under 100 of a unit, none above. Halves round
// up (1280 is "1.3 KiB"), as the TypeScript bytes does.
func Bytes(n int64) string {
	if n < 0 {
		return "-" + bytesOf(-float64(n)) // float64, so math.MinInt64 is safe
	}
	return bytesOf(float64(n))
}

func bytesOf(n float64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", int64(n))
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	v := n
	u := -1
	for v >= 1024 && u < len(units)-1 {
		v /= 1024
		u++
	}
	if v >= 1023.5 && u < len(units)-1 { // rounds up to the next unit
		v /= 1024
		u++
	}
	if v >= 99.95 { // one decimal would print 100.0
		return toFixed(v, 0) + " " + units[u]
	}
	return toFixed(v, 1) + " " + units[u]
}

// Duration formats a duration with its two largest units: 450ms, 3s, 2m 5s,
// 1h 5m, 3d 4h. Under a second it is milliseconds; a negative duration has a
// leading "-".
func Duration(d time.Duration) string {
	if d < 0 {
		if d == math.MinInt64 {
			d = math.MaxInt64 // -MinInt64 overflows
		} else {
			d = -d
		}
		return "-" + Duration(d)
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	secs := int64(d / time.Second)
	days, secs := secs/86400, secs%86400
	hours, secs := secs/3600, secs%3600
	mins, secs := secs/60, secs%60
	switch {
	case days > 0:
		return unitPair(days, "d", hours, "h")
	case hours > 0:
		return unitPair(hours, "h", mins, "m")
	case mins > 0:
		return unitPair(mins, "m", secs, "s")
	}
	return fmt.Sprintf("%ds", secs)
}

func unitPair(a int64, au string, b int64, bu string) string {
	if b == 0 {
		return fmt.Sprintf("%d%s", a, au)
	}
	return fmt.Sprintf("%d%s %d%s", a, au, b, bu)
}

// toFixed is x with d decimals, a half rounding away from zero, which is what
// JavaScript's toFixed does (Go's own formatting rounds a half to even). A tie
// is a value whose exact decimal expansion ends in a 5 right after the digits
// kept; 1.005 is not one, because the double nearest to it is a little below.
func toFixed(x float64, d int) string {
	neg := x < 0
	if neg {
		x = -x
	}
	const extra = 25
	digits := strconv.FormatFloat(x, 'f', d+extra, 64)
	if tail := digits[len(digits)-extra:]; tail[0] == '5' && strings.Trim(tail[1:], "0") == "" {
		x = math.Nextafter(x, math.Inf(1))
	}
	out := strconv.FormatFloat(x, 'f', d, 64)
	if neg && strings.Trim(out, "0.") != "" {
		return "-" + out
	}
	return out
}

// Relative formats t relative to now: "just now" (under 10 s), "5 minutes
// ago", "1 hour ago", "in 2 days". now is a parameter so a test, or a frame
// that must not change between redraws, can fix it.
func Relative(t, now time.Time) string {
	d := now.Sub(t)
	future := d < 0
	if future {
		d = -d
	}
	if d < 10*time.Second {
		return "just now"
	}
	var n int64
	var unit string
	switch {
	case d < time.Minute:
		n, unit = int64(d/time.Second), "second"
	case d < time.Hour:
		n, unit = int64(d/time.Minute), "minute"
	case d < 24*time.Hour:
		n, unit = int64(d/time.Hour), "hour"
	case d < 30*24*time.Hour:
		n, unit = int64(d/(24*time.Hour)), "day"
	case d < 365*24*time.Hour:
		n, unit = int64(d/(30*24*time.Hour)), "month"
	default:
		n, unit = int64(d/(365*24*time.Hour)), "year"
	}
	if n != 1 {
		unit += "s"
	}
	if future {
		return fmt.Sprintf("in %d %s", n, unit)
	}
	return fmt.Sprintf("%d %s ago", n, unit)
}

// Percent formats a fraction as a percentage with the given number of
// decimals: Percent(0.256, 0) is "26%", Percent(0.256, 1) is "25.6%".
func Percent(fraction float64, decimals int) string {
	if math.IsNaN(fraction) || math.IsInf(fraction, 0) {
		return "-"
	}
	if decimals < 0 {
		decimals = 0
	}
	return toFixed(fraction*100, decimals) + "%"
}

// Thousands formats an integer with comma separators: 1234567 is "1,234,567".
func Thousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
