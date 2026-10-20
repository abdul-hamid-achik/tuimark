package play

import (
	"testing"
	"time"
)

// SPEC v0.4b §15.4.1: the grammar of DUR, its sums, and its reasons.
func TestParseDur(t *testing.T) {
	const notDur = `is not a duration (want a number and a unit ms, s, m, or h, such as 250ms)`
	good := map[string]time.Duration{
		"1ms": time.Millisecond, "250ms": 250 * time.Millisecond, "1.5s": 1500 * time.Millisecond,
		"2m": 2 * time.Minute, "1m30s": 90 * time.Second, "1h": time.Hour, "60m": time.Hour,
		"0.5s": 500 * time.Millisecond, "1.001s": 1001 * time.Millisecond, "3600000ms": time.Hour,
		"30s30s": time.Minute, "0001ms": time.Millisecond, "1.0ms": time.Millisecond,
		"59m59s999ms": time.Hour - time.Millisecond,
	}
	for in, want := range good {
		if got, err := ParseDur(in); err != nil || got != want {
			t.Errorf("ParseDur(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	bad := map[string]string{
		"0ms": "tick: duration must be at least 1ms", "0s": "tick: duration must be at least 1ms",
		"0h0m0s":                "tick: duration must be at least 1ms",
		"1.5ms":                 "tick: duration must be a whole number of milliseconds",
		"0.5ms":                 "tick: duration must be a whole number of milliseconds",
		"1.0005s":               "tick: duration must be a whole number of milliseconds",
		"2h":                    "tick: duration must be at most 1h",
		"1h1ms":                 "tick: duration must be at most 1h",
		"3600001ms":             "tick: duration must be at most 1h",
		"99999999999999999999h": "tick: duration must be at most 1h",
		"-5s":                   `tick: "-5s" ` + notDur,
		"+5s":                   `tick: "+5s" ` + notDur,
		"100":                   `tick: "100" ` + notDur,
		".5s":                   `tick: ".5s" ` + notDur,
		"5.s":                   `tick: "5.s" ` + notDur,
		"1S":                    `tick: "1S" ` + notDur,
		"":                      `tick: "" ` + notDur,
		"1 s":                   `tick: "1 s" ` + notDur,
		"250us":                 `tick: "250us" ` + notDur,
		"1e3ms":                 `tick: "1e3ms" ` + notDur,
		"ms":                    `tick: "ms" ` + notDur,
		"1m 30s":                `tick: "1m 30s" ` + notDur,
		"1s2":                   `tick: "1s2" ` + notDur,
		"1sx":                   `tick: "1sx" ` + notDur,
		"0x10ms":                `tick: "0x10ms" ` + notDur,
		"1_000ms":               `tick: "1_000ms" ` + notDur,
		"１ms":                   `tick: "１ms" ` + notDur,
	}
	for in, want := range bad {
		if got, err := ParseDur(in); err == nil || err.Error() != want {
			t.Errorf("ParseDur(%q) = %v, %v; want the error %q", in, got, err, want)
		}
	}
}

// ParseStep builds the step, keeping it as written, and a Go caller's
// "tick:" + d.String() round-trips for whole milliseconds in range.
func TestParseStepTick(t *testing.T) {
	st, err := ParseStep("tick:250ms")
	if err != nil || st.Kind != KindTick || st.Dur != 250*time.Millisecond || st.Raw != "tick:250ms" {
		t.Fatalf("ParseStep = %+v, %v", st, err)
	}
	for _, d := range []time.Duration{time.Millisecond, 1500 * time.Millisecond, 90 * time.Second, time.Hour, 61*time.Minute - time.Hour} {
		if st, err := ParseStep("tick:" + d.String()); err != nil || st.Dur != d {
			t.Errorf("tick:%s = %+v, %v", d, st, err)
		}
	}
	if _, err := ParseStep("tick:"); err == nil {
		t.Error("tick: with no DUR parsed")
	}
	steps, err := ParseSteps("down  tick:1s   up ")
	if err != nil || len(steps) != 3 || steps[1].Kind != KindTick {
		t.Errorf("ParseSteps = %+v, %v", steps, err)
	}
	if _, err := ParseSteps("tick:1 s"); err == nil {
		t.Error("tick:1 s parsed: --input splits steps at spaces, and s is no key")
	}
}
