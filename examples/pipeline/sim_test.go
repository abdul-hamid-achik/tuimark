package main

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

// runToEnd steps a fresh simulation by one simulated second until it is
// idle, and returns it. It fails if the run does not end.
func runToEnd(t *testing.T, s *sim) {
	t.Helper()
	clock := testStart
	for i := 0; i < 400; i++ {
		if s.idle() && i > 0 {
			return
		}
		clock = clock.Add(250 * time.Millisecond)
		s.step(clock)
	}
	t.Fatalf("run still going after 400 simulated seconds: %v", statuses(s))
}

func statuses(s *sim) map[string]string {
	m := map[string]string{}
	for _, j := range s.jobs {
		m[j.ID] = j.status
	}
	return m
}

func newTestSim(flaky bool) *sim { return newSim(defaultSeed, flaky, defaultSpeed, testStart) }

// A stage starts only when every job of the stages before it has passed,
// and the jobs of one stage run side by side.
func TestStagesRunInOrder(t *testing.T) {
	s := newTestSim(false)
	runToEnd(t, s)
	end := map[string]float64{} // when each stage's last job finished
	begin := map[string]float64{}
	for _, j := range s.jobs {
		if j.status != stPassed {
			t.Fatalf("%s is %s in a green run", j.ID, j.status)
		}
		fin := j.started + j.plan.Dur
		if fin > end[j.Stage] {
			end[j.Stage] = fin
		}
		if b, ok := begin[j.Stage]; !ok || j.started < b {
			begin[j.Stage] = j.started
		}
	}
	for i := 1; i < len(stages); i++ {
		prev, cur := stages[i-1], stages[i]
		if begin[cur]+1e-6 < end[prev] {
			t.Errorf("stage %s began at %.2f, before %s ended at %.2f", cur, begin[cur], prev, end[prev])
		}
	}
	// Jobs of the test stage overlap: all three are running together at some point.
	s = newTestSim(false)
	clock := testStart
	together := false
	for i := 0; i < 200 && !s.idle(); i++ {
		clock = clock.Add(250 * time.Millisecond)
		s.step(clock)
		if s.count(stRunning) == 3 {
			together = true
		}
	}
	if !together {
		t.Error("the three test jobs never ran in parallel")
	}
}

// The flaky test fails the first time; the jobs after it never start, the
// run ends failed, and the later stages are skipped.
func TestFailureBlocksTheLaterStages(t *testing.T) {
	s := newTestSim(true)
	runToEnd(t, s)
	want := map[string]string{
		"compile": stPassed, "assets": stPassed, "unit": stPassed, "integration": stFailed, "e2e": stPassed,
		"fmt": stSkipped, "vet": stSkipped, "staticcheck": stSkipped,
		"docker": stSkipped, "binaries": stSkipped, "deploy": stSkipped,
	}
	if got := statuses(s); !reflect.DeepEqual(got, want) {
		t.Errorf("statuses = %v\nwant       %v", got, want)
	}
	if s.find("deploy").tries != 0 {
		t.Error("deploy started although a test failed")
	}
	// The failed job stopped part of the way and kept its log.
	j := s.find("integration")
	if j.pct <= 0 || j.pct >= 100 {
		t.Errorf("failed job pct = %v", j.pct)
	}
	last := j.log[len(j.log)-1]
	if last.Kind != kErr {
		t.Errorf("the last line of the failed job is %+v", last)
	}
	// The run reports it once.
	var msgs []string
	for _, e := range s.drain() {
		msgs = append(msgs, e.Text)
	}
	if n := countContaining(msgs, "Run #1 failed"); n != 1 {
		t.Errorf("failure announced %d times: %v", n, msgs)
	}
}

func countContaining(xs []string, sub string) int {
	n := 0
	for _, x := range xs {
		if strings.Contains(x, sub) {
			n++
		}
	}
	return n
}

// Retrying the failed job queues the skipped jobs again; once it passes the
// stages after it run in order and deploy finishes.
func TestRetryUnblocksTheRun(t *testing.T) {
	s := newTestSim(true)
	runToEnd(t, s)
	if err := s.retry("integration"); err != nil {
		t.Fatal(err)
	}
	if s.idle() {
		t.Fatal("a retried run is idle")
	}
	if got := s.find("deploy").status; got != stQueued {
		t.Errorf("deploy after retry is %s, want queued", got)
	}
	runToEnd(t, s)
	for _, j := range s.jobs {
		if j.status != stPassed {
			t.Errorf("%s is %s after the retry", j.ID, j.status)
		}
	}
	integ := s.find("integration")
	if integ.tries != 2 {
		t.Errorf("integration ran %d times, want 2", integ.tries)
	}
	if lint, dep := s.find("staticcheck"), s.find("deploy"); dep.started < lint.started+lint.plan.Dur {
		t.Error("deploy began before lint ended")
	}
	if lint, integ := s.find("fmt"), s.find("integration"); lint.started < integ.started+integ.plan.Dur {
		t.Error("lint began before the retried test ended")
	}
	// The old attempt's lines are kept, with a marker before the new ones.
	found := false
	for _, l := range integ.log {
		if strings.Contains(l.Text, "retry, attempt 2") {
			found = true
		}
	}
	if !found {
		t.Error("no retry marker in the log")
	}
}

func TestRetryNeedsAFailedJob(t *testing.T) {
	s := newTestSim(true)
	if err := s.retry("unit"); err == nil || !strings.Contains(err.Error(), "only a failed job") {
		t.Errorf("retry of a queued job: %v", err)
	}
	if err := s.retry("nope"); err == nil {
		t.Error("retry of an unknown job succeeded")
	}
}

// A canceled run is over: r refuses instead of running a job inside it.
func TestRetryRefusesOnACanceledRun(t *testing.T) {
	s := newTestSim(true)
	clock := testStart
	for i := 0; s.count(stFailed) == 0 && i < 400; i++ {
		clock = clock.Add(250 * time.Millisecond)
		s.step(clock)
	}
	failed := ""
	for _, j := range s.jobs {
		if j.status == stFailed {
			failed = j.ID
		}
	}
	if failed == "" {
		t.Fatal("no job failed")
	}
	// Another job is still running, so there is something to cancel.
	if err := s.cancel(); err != nil {
		t.Fatal(err)
	}
	before := statuses(s)
	err := s.retry(failed)
	if err == nil || !strings.Contains(err.Error(), "canceled") || !strings.Contains(err.Error(), "R to rerun") {
		t.Errorf("retry on a canceled run: %v", err)
	}
	if !reflect.DeepEqual(statuses(s), before) || !s.idle() {
		t.Errorf("the refused retry changed the run: %v -> %v", before, statuses(s))
	}
}

// A rerun starts everything over as run 2; the flake does not strike again.
func TestRerunPassesAfterTheFlake(t *testing.T) {
	s := newTestSim(true)
	runToEnd(t, s)
	s.rerun()
	if s.run != 2 || s.t != 0 {
		t.Errorf("run=%d t=%v after rerun", s.run, s.t)
	}
	for _, j := range s.jobs {
		if j.status != stQueued || len(j.log) != 0 || j.pct != 0 {
			t.Errorf("%s not reset: %s %d lines %v%%", j.ID, j.status, len(j.log), j.pct)
		}
	}
	runToEnd(t, s)
	for id, st := range statuses(s) {
		if st != stPassed {
			t.Errorf("%s is %s in run 2", id, st)
		}
	}
}

func TestCancelStopsEverything(t *testing.T) {
	s := newTestSim(true)
	clock := testStart
	for i := 0; i < 15; i++ {
		clock = clock.Add(250 * time.Millisecond)
		s.step(clock)
	}
	if err := s.cancel(); err != nil {
		t.Fatal(err)
	}
	for _, j := range s.jobs {
		if j.status == stRunning || j.status == stQueued {
			t.Errorf("%s is %s after cancel", j.ID, j.status)
		}
	}
	if s.count(stPassed) == 0 || s.count(stCanceled) == 0 {
		t.Errorf("statuses = %v", statuses(s))
	}
	if s.step(clock.Add(time.Hour)) {
		t.Error("a canceled run still steps")
	}
	if err := s.cancel(); err == nil {
		t.Error("canceling a finished run succeeded")
	}
}

// The same seed gives the same run; another seed another one.
func TestSimIsDeterministic(t *testing.T) {
	a, b := newTestSim(true), newTestSim(true)
	runToEnd(t, a)
	runToEnd(t, b)
	if !reflect.DeepEqual(a.jobsStore(), b.jobsStore()) || !reflect.DeepEqual(a.hist, b.hist) {
		t.Error("two runs with one seed differ")
	}
	c := newSim(defaultSeed+1, true, defaultSpeed, testStart)
	runToEnd(t, c)
	if reflect.DeepEqual(a.jobsStore(), c.jobsStore()) {
		t.Error("two seeds gave the same run")
	}
}

// A job's plan depends only on (seed, job, attempt), so it is the same
// however the run got there.
func TestPlansDependOnlyOnTheirSeed(t *testing.T) {
	sp := specs[3] // integration
	p1 := makePlan(7, sp, 3, 0, true)
	p2 := makePlan(7, sp, 3, 0, true)
	if !reflect.DeepEqual(p1, p2) {
		t.Error("one attempt, two plans")
	}
	if p1.FailAt <= 0 || p1.FailAt >= p1.Dur {
		t.Errorf("failAt = %v of %v", p1.FailAt, p1.Dur)
	}
	if p3 := makePlan(7, sp, 3, 1, false); p3.FailAt != 0 || reflect.DeepEqual(p1.Lines, p3.Lines) {
		t.Error("the second attempt should pass, with its own lines")
	}
	for i, l := range p1.Lines {
		if l.At < 0 || l.At > p1.Dur+1e-9 || (i > 0 && l.At < p1.Lines[i-1].At) {
			t.Fatalf("line %d out of order: %+v", i, l)
		}
	}
}

// How the clock is sliced does not change which lines a job prints.
func TestStepSizeDoesNotChangeTheOutput(t *testing.T) {
	texts := func(dt time.Duration) []string {
		s := newTestSim(true)
		clock := testStart
		for i := 0; i < 2000 && (i == 0 || !s.idle()); i++ {
			clock = clock.Add(dt)
			s.step(clock)
		}
		var out []string
		for _, j := range s.jobs {
			for _, l := range j.log {
				out = append(out, j.ID+": "+l.Text)
			}
		}
		return out
	}
	a, b, c := texts(250*time.Millisecond), texts(100*time.Millisecond), texts(900*time.Millisecond)
	if len(a) < 50 {
		t.Fatalf("only %d lines", len(a))
	}
	if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(a, c) {
		t.Errorf("logs differ with the tick size: %d, %d, %d lines", len(a), len(b), len(c))
	}
}

func TestSparklineHistoryIsBounded(t *testing.T) {
	s := newTestSim(false)
	runToEnd(t, s)
	if len(s.hist) == 0 || len(s.hist) > histLen {
		t.Fatalf("history has %d samples", len(s.hist))
	}
	for _, h := range s.hist {
		if h < 0 || h > 3 {
			t.Errorf("sample %v out of range", h)
		}
	}
}

func TestTimeFormats(t *testing.T) {
	for _, c := range []struct {
		f    func(float64) string
		in   float64
		want string
	}{
		{fmtClock, 7.34, "00:07.3"}, {fmtClock, 67.2, "01:07.2"}, {fmtClock, -1, "00:00.0"},
		{fmtClock, 59.94, "00:59.9"}, {fmtClock, 59.96, "01:00.0"}, {fmtClock, 119.97, "02:00.0"},
		{fmtClock, 60, "01:00.0"}, {fmtClock, 125.04, "02:05.0"}, {fmtClock, 3599.99, "60:00.0"},
		{fmtElapsed, 8.9, "0:08"}, {fmtElapsed, 75, "1:15"},
	} {
		if got := c.f(c.in); got != c.want {
			t.Errorf("format(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// checkSpeed rejects what would hang or freeze step.
func TestCheckSpeed(t *testing.T) {
	for _, v := range []float64{1, 0.5, 4, 8, 1000} {
		if err := checkSpeed(v); err != nil {
			t.Errorf("checkSpeed(%v) = %v", v, err)
		}
	}
	for _, v := range []float64{0, -1, 1000.5, 1e12, math.Inf(1), math.Inf(-1), math.NaN()} {
		if err := checkSpeed(v); err == nil || !strings.Contains(err.Error(), "-speed") {
			t.Errorf("checkSpeed(%v) = %v, want a usage error", v, err)
		}
	}
	// run reports it before touching the terminal or the files.
	for _, v := range []float64{math.Inf(1), math.NaN(), 1e12} {
		o := options{dir: ".", dump: "80x24", sim: simOptions{seed: 1, flaky: true, speed: v}}
		if err := run(o); err == nil || !strings.Contains(err.Error(), "-speed") {
			t.Errorf("run with -speed %v: %v", v, err)
		}
	}
}
