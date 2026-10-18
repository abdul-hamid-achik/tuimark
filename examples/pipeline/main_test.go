package main

import (
	"strings"
	"testing"
	"time"
)

func TestValidateIsClean(t *testing.T) {
	r := newRig(t, defaultSim())
	for _, theme := range []string{"dark", "light"} {
		if err := r.a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		if diags := r.a.ui.Validate(); len(diags) != 0 {
			t.Errorf("%s: Validate gave %v", theme, diags)
		}
	}
}

// Every size and state dumps without errors or warnings, and says what it
// should: the header, the jobs, the log, the hints.
func TestDumpAtSeveralSizes(t *testing.T) {
	r := newRig(t, defaultSim())
	for _, secs := range []int{0, 12, 30} {
		r.advance(secs)
		for _, sz := range [][2]int{{80, 24}, {120, 30}, {100, 24}, {60, 20}, {40, 14}} {
			g := r.grid(sz[0], sz[1])
			state := map[int]string{0: "QUEUED", 12: "RUNNING", 30: "FAILED"}[secs]
			for _, want := range []string{"ci", state, "jobs", "q quit"} {
				if want == "q quit" && sz[0] < 60 {
					continue
				}
				if !strings.Contains(g, want) {
					t.Errorf("t=%d %dx%d: grid has no %q:\n%s", secs, sz[0], sz[1], want, g)
				}
			}
		}
	}
}

// Every action the document names has a handler, except the built-ins.
func TestHandlersCoverCatalog(t *testing.T) {
	r := newRig(t, defaultSim())
	for _, spec := range r.a.ui.Catalog() {
		if !spec.Builtin && !spec.Registered {
			t.Errorf("no handler for %s (%v)", spec.Name, spec.Sources)
		}
	}
}

func TestRunningJobsSpin(t *testing.T) {
	r := newRig(t, defaultSim())
	r.advance(12)
	seen := map[string]bool{}
	for i := 0; i < 6; i++ {
		r.tick(250 * time.Millisecond)
		g := r.grid(100, 24)
		for _, f := range spinner {
			if strings.Contains(g, f+" ") {
				seen[f] = true
			}
		}
	}
	if len(seen) < 4 {
		t.Errorf("spinner frames seen: %v", seen)
	}
}

// j and k move the cursor, the arrows too; the log pane follows the
// selected job through a Derive with no handler of its own.
func TestPlayCursorSelectsTheLog(t *testing.T) {
	r := newRig(t, defaultSim())
	r.advance(14)
	if r.str("cur") != "compile" {
		t.Fatalf("cur = %q at the start", r.str("cur"))
	}
	res := r.play(100, 24, "j")
	if r.str("cur") != "assets" || !strings.Contains(r.str("log.title"), "build / assets") {
		t.Errorf("after j: cur=%q title=%q", r.str("cur"), r.str("log.title"))
	}
	if len(res.Events) == 0 || res.Events[len(res.Events)-1].Action != "job_select" {
		t.Errorf("events = %+v", res.Events)
	}
	r.play(100, 24, "down", "down", "k", "k")
	if r.str("cur") != "assets" {
		t.Errorf("after down down k k: cur = %q", r.str("cur"))
	}
	res = r.play(100, 24, "down", "down")
	if r.str("cur") != "integration" {
		t.Fatalf("cur = %q, want integration", r.str("cur"))
	}
	g := strings.Join(res.Dump.Grid, "\n")
	if !strings.Contains(g, "test / integration") || !strings.Contains(g, "$ go test -tags integration") {
		t.Errorf("the log pane does not show the integration job:\n%s", g)
	}
	// A job that has not started has an empty log and says why.
	r.play(100, 24, "end")
	if got := r.str("cur"); got != "deploy" {
		t.Fatalf("end: cur = %q", got)
	}
	if !r.flag("log.none") || !strings.Contains(r.str("log.empty"), "earlier stages") {
		t.Errorf("log = %v", r.get("log"))
	}
}

func TestPlayEnterFocusesTheLog(t *testing.T) {
	r := newRig(t, defaultSim())
	r.advance(10)
	r.play(100, 24)
	if f := r.get("@focus"); f != "#jobs" {
		t.Fatalf("focus = %v at the start", f)
	}
	r.play(100, 24, "enter")
	if f := r.get("@focus"); f != "#log" {
		t.Errorf("focus after enter = %v", f)
	}
	// j and k still move the job cursor while the log has focus.
	r.play(100, 24, "j")
	if r.str("cur") != "assets" {
		t.Errorf("j in the log moved cur to %q", r.str("cur"))
	}
	r.play(100, 24, "esc")
	if f := r.get("@focus"); f != "#jobs" {
		t.Errorf("focus after esc = %v", f)
	}
}

// The failed job is retried with r: it goes back to the queue, the jobs
// skipped for its sake wait again, and the run continues to the end.
func TestPlayRetryContinuesTheRun(t *testing.T) {
	r := newRig(t, defaultSim())
	r.advanceUntil(60, func() bool { return r.a.sim.idle() })
	if r.status("integration") != stFailed || r.status("deploy") != stSkipped {
		t.Fatalf("statuses = %v", statuses(r.a.sim))
	}
	if r.str("run.status") != "failed" || !r.flag("run.is_failed") {
		t.Errorf("run = %v", r.get("run"))
	}
	if !strings.Contains(r.str("status.text"), "Run #1 failed") || !r.flag("status.err") {
		t.Errorf("status = %v", r.get("status"))
	}

	// r on a job that did not fail says so and changes nothing.
	r.play(100, 24, "r")
	if !strings.Contains(r.str("status.text"), "only a failed job") || !r.a.sim.idle() {
		t.Errorf("r on compile: status %v, idle %v", r.get("status"), r.a.sim.idle())
	}

	res := r.play(100, 24, "j", "j", "j", "r")
	if r.str("cur") != "integration" {
		t.Fatalf("cur = %q", r.str("cur"))
	}
	var acted bool
	for _, e := range res.Events {
		acted = acted || e.Action == "job_retry"
	}
	if !acted {
		t.Errorf("no job_retry event in %+v", res.Events)
	}
	if r.status("integration") != stQueued || r.status("deploy") != stQueued {
		t.Errorf("after r: %v", statuses(r.a.sim))
	}
	if r.str("run.status") != "running" || !strings.Contains(r.str("status.text"), "Retrying test/integration") {
		t.Errorf("after r: run=%v status=%v", r.get("run"), r.get("status"))
	}
	if g := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(g, "Retrying test/integration") {
		t.Errorf("the status line does not say so:\n%s", g)
	}
	r.advanceUntil(120, func() bool { return r.a.sim.idle() })
	if r.str("run.status") != "passed" || r.status("deploy") != stPassed {
		t.Errorf("end: %v run=%v", statuses(r.a.sim), r.get("run"))
	}
	if got := r.str("run.badge"); got != "✓ PASSED" {
		t.Errorf("badge = %q", got)
	}
	if p, _ := r.get("run.pct").(float64); p != 100 {
		t.Errorf("run.pct = %v", p)
	}
}

// r after a cancel must not run a job inside the canceled run: it says so
// and R is the way out.
func TestPlayRetryAfterCancelIsRefused(t *testing.T) {
	r := newRig(t, defaultSim())
	r.advanceUntil(60, func() bool { return r.status("integration") == stFailed })
	if r.a.sim.count(stRunning) == 0 {
		t.Fatalf("nothing left to cancel: %v", statuses(r.a.sim))
	}
	r.play(100, 24, "c", "y")
	if r.str("run.status") != "canceled" {
		t.Fatalf("run = %v", r.get("run"))
	}
	r.play(100, 24, "j", "j", "j", "r")
	if r.str("cur") != "integration" {
		t.Fatalf("cur = %q", r.str("cur"))
	}
	if st := r.str("status.text"); !strings.Contains(st, "canceled") || !strings.Contains(st, "R to rerun") || !r.flag("status.err") {
		t.Errorf("status = %v", r.get("status"))
	}
	if r.status("integration") != stFailed || !r.a.sim.idle() || r.str("run.status") != "canceled" {
		t.Errorf("r changed the canceled run: %v run=%v", statuses(r.a.sim), r.get("run"))
	}
	r.play(100, 24, "R")
	if r.str("run.status") != "queued" || r.get("pipeline.run") != float64(2) {
		t.Errorf("R after cancel: run=%v", r.get("run"))
	}
}

// The hint bar names the modal's own keys while a modal is open.
func TestModalsLabelTheirKeys(t *testing.T) {
	r := newRig(t, defaultSim())
	r.advance(10)
	for _, c := range []struct {
		keys []string
		want []string
	}{
		{[]string{"c"}, []string{"y cancel run", "n keep"}},
		{[]string{"?"}, []string{"enter/q close"}},
	} {
		res := r.play(100, 24, c.keys...)
		g := res.Dump.Grid[len(res.Dump.Grid)-1]
		for _, w := range c.want {
			if !strings.Contains(g, w) {
				t.Errorf("%v: hint bar %q lacks %q", c.keys, g, w)
			}
		}
		r.play(100, 24, "esc")
	}
}

func TestPlayRerun(t *testing.T) {
	r := newRig(t, defaultSim())
	r.advanceUntil(60, func() bool { return r.a.sim.idle() })
	r.play(100, 24, "R")
	if r.get("pipeline.run") != float64(2) || r.str("run.status") != "queued" {
		t.Errorf("after R: run no %v, status %q", r.get("pipeline.run"), r.str("run.status"))
	}
	for _, j := range r.a.sim.jobs {
		if j.status != stQueued {
			t.Errorf("%s is %s after R", j.ID, j.status)
		}
	}
	if g := r.grid(100, 24); !strings.Contains(g, "run #2") {
		t.Errorf("the header does not say run #2:\n%s", g)
	}
	r.advanceUntil(160, func() bool { return r.a.sim.idle() })
	if r.str("run.status") != "passed" {
		t.Errorf("run 2 is %s: %v", r.str("run.status"), statuses(r.a.sim))
	}
}

// c asks first; n or esc keeps the run, y stops it.
func TestPlayCancelAsksToConfirm(t *testing.T) {
	r := newRig(t, defaultSim())
	r.advance(10)
	res := r.play(100, 24, "c")
	if !r.flag("confirm_open") {
		t.Fatal("c did not open the confirmation")
	}
	g := strings.Join(res.Dump.Grid, "\n")
	if !strings.Contains(g, "Cancel run #1?") || !strings.Contains(g, "cancel run (y)") {
		t.Errorf("the confirmation is not shown:\n%s", g)
	}
	// q must not quit while the modal is open.
	res = r.play(100, 24, "q")
	if res.Quit || !r.flag("confirm_open") {
		t.Fatal("q reached the keymap behind the modal")
	}
	r.play(100, 24, "n")
	if r.flag("confirm_open") || r.a.sim.idle() || r.str("run.status") != "running" {
		t.Errorf("n did not keep the run: %v", r.get("run"))
	}
	r.play(100, 24, "c", "esc")
	if r.flag("confirm_open") || r.str("run.status") != "running" {
		t.Error("esc did not dismiss the confirmation")
	}

	res = r.play(100, 24, "c", "y")
	if r.flag("confirm_open") {
		t.Error("y left the modal open")
	}
	if r.str("run.status") != "canceled" || !r.flag("run.is_canceled") {
		t.Errorf("run = %v", r.get("run"))
	}
	for _, j := range r.a.sim.jobs {
		if j.status == stRunning || j.status == stQueued {
			t.Errorf("%s is %s after cancel", j.ID, j.status)
		}
	}
	if g := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(g, "CANCELED") || !strings.Contains(g, "⊘") {
		t.Errorf("the canceled state is not shown:\n%s", g)
	}
	// With nothing left to cancel, c only says so.
	r.play(100, 24, "c")
	if r.flag("confirm_open") || !strings.Contains(r.str("status.text"), "Nothing to cancel") {
		t.Errorf("c on a finished run: open=%v status=%v", r.flag("confirm_open"), r.get("status"))
	}
}

// f holds the log still while the job goes on, counts what it holds back,
// and picks up where the job is when it is pressed again.
func TestPlayFollowFreezesTheLog(t *testing.T) {
	r := newRig(t, defaultSim())
	r.advance(10)
	r.play(100, 24, "j", "j", "j") // integration, running
	if r.status("integration") != stRunning {
		t.Fatalf("integration is %s", r.status("integration"))
	}
	if !r.flag("follow") || !strings.HasSuffix(r.str("log.title"), "following") {
		t.Fatalf("follow=%v title=%q", r.flag("follow"), r.str("log.title"))
	}
	before := r.logLen()
	r.play(100, 24, "f")
	if r.flag("follow") || r.logLen() != before {
		t.Fatalf("after f: follow=%v lines %d, was %d", r.flag("follow"), r.logLen(), before)
	}
	r.advance(5)
	if got := r.logLen(); got != before {
		t.Errorf("a frozen log grew from %d to %d lines", before, got)
	}
	held, _ := r.get("log.new").(float64)
	if held == 0 || !strings.Contains(r.str("log.title"), "paused, ") {
		t.Errorf("new=%v title=%q", held, r.str("log.title"))
	}
	if g := r.grid(100, 24); !strings.Contains(g, "paused, ") {
		t.Errorf("the pane title does not say it is paused:\n%s", g)
	}
	r.play(100, 24, "f")
	if !r.flag("follow") || r.logLen() <= before {
		t.Errorf("after f again: follow=%v lines=%d", r.flag("follow"), r.logLen())
	}
	// A pause is per selection: moving the cursor follows the new job.
	r.play(100, 24, "f", "j")
	if !r.flag("follow") {
		t.Error("selecting another job left follow off")
	}
}

func TestPlayHelpAndQuit(t *testing.T) {
	r := newRig(t, defaultSim())
	res := r.play(100, 24, "?")
	if !r.flag("help_open") {
		t.Fatal("? did not open help")
	}
	g := strings.Join(res.Dump.Grid, "\n")
	for _, want := range []string{"keys", "retry", "rerun", "follow"} {
		if !strings.Contains(g, want) {
			t.Errorf("help has no %q:\n%s", want, g)
		}
	}
	res = r.play(100, 24, "q")
	if r.flag("help_open") || res.Quit {
		t.Errorf("q in help: open=%v quit=%v", r.flag("help_open"), res.Quit)
	}
	if res = r.play(100, 24, "q"); !res.Quit {
		t.Error("q did not quit")
	}
	r = newRig(t, defaultSim())
	if res = r.play(100, 24, "ctrl+c"); !res.Quit {
		t.Error("ctrl+c did not quit")
	}
}

// The overall bar is the mean of the jobs' progress and its color follows
// the status; the sparkline reads the running-jobs history.
func TestOverallProgressAndSparkline(t *testing.T) {
	r := newRig(t, defaultSim())
	r.advance(8)
	p1, _ := r.get("run.pct").(float64)
	r.advance(8)
	p2, _ := r.get("run.pct").(float64)
	if p1 <= 0 || p2 <= p1 || p2 >= 100 {
		t.Errorf("run.pct went %v then %v", p1, p2)
	}
	if h, _ := r.get("spark").([]any); len(h) == 0 {
		t.Error("no sparkline history")
	}
	if !strings.Contains(r.str("run.summary"), "jobs") || !strings.Contains(r.str("run.live"), "running") {
		t.Errorf("run = %v", r.get("run"))
	}
}

// -flaky=false gives a green run.
func TestNotFlakyIsGreen(t *testing.T) {
	so := defaultSim()
	so.flaky = false
	r := newRig(t, so)
	r.advanceUntil(160, func() bool { return r.a.sim.idle() })
	if r.str("run.status") != "passed" || r.a.sim.count(stFailed) != 0 {
		t.Errorf("run = %v, statuses %v", r.get("run"), statuses(r.a.sim))
	}
}

// A failing handler lands in the status line and the session goes on.
func TestOnErrorShowsInTheStatusLine(t *testing.T) {
	r := newRig(t, defaultSim())
	if err := r.a.onError(nil, errTest); err != nil {
		t.Fatal(err)
	}
	if r.str("status.text") != errTest.Error() || !r.flag("status.err") {
		t.Errorf("status = %v", r.get("status"))
	}
}

type testError string

func (e testError) Error() string { return string(e) }

const errTest = testError("boom")

// Every state the keys can reach dumps without errors or warnings at every
// size: the modals, a canceled run, a paused log, a failed run.
func TestInteractiveStatesAreClean(t *testing.T) {
	sizes := [][2]int{{40, 14}, {60, 20}, {80, 24}, {100, 24}, {120, 30}}
	states := []struct {
		name string
		keys []string
		want []string // text the grid must show in full
	}{
		{"help", []string{"?"}, []string{"[ close ]"}},
		{"confirm", []string{"c"}, []string{"[ cancel run (y) ]", "[ keep (n) ]"}},
		{"log focused", []string{"enter"}, nil},
		{"paused", []string{"f"}, nil},
		{"canceled", []string{"c", "y"}, nil},
		{"retrying", []string{"j", "j", "j", "r"}, nil},
		{"rerun", []string{"R"}, nil},
	}
	for _, sz := range sizes {
		for _, st := range states {
			for _, theme := range []string{"dark", "light"} {
				r := newRig(t, defaultSim())
				if err := r.a.ui.Set("@theme", theme); err != nil {
					t.Fatal(err)
				}
				// The cancel flows need jobs still running: c on a finished
				// run only sets a status message and opens no modal.
				live := st.name == "canceled" || st.name == "confirm"
				r.advanceUntil(60, func() bool { return r.a.sim.idle() || live && r.a.sim.count(stRunning) > 0 })
				if live && r.a.sim.idle() {
					t.Fatalf("%s: the run ended before the keys were pressed", st.name)
				}
				if st.name == "retrying" || st.name == "rerun" || st.name == "log focused" {
					r.advanceUntil(60, r.a.sim.idle)
				}
				res := r.play(sz[0], sz[1], st.keys...)
				if d := res.Dump; len(d.Errors) != 0 {
					t.Errorf("%s %dx%d %s: %v", st.name, sz[0], sz[1], theme, d.Errors)
				}
				grid := strings.Join(res.Dump.Grid, "\n")
				for _, w := range st.want {
					if !strings.Contains(grid, w) {
						t.Errorf("%s %dx%d %s: %q is not shown in full:\n%s", st.name, sz[0], sz[1], theme, w, grid)
					}
				}
			}
		}
	}
}
