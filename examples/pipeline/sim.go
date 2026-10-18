package main

import (
	"errors"
	"fmt"
	"time"
)

// Job statuses.
const (
	stQueued   = "queued"
	stRunning  = "running"
	stPassed   = "passed"
	stFailed   = "failed"
	stCanceled = "canceled"
	stSkipped  = "skipped"
)

const (
	subStep = 0.25 // the longest simulated step, in seconds
	histLen = 20   // samples the sparkline keeps
)

// logLine is one printed line of a job.
type logLine struct {
	N    int
	TS   string
	Kind string
	Text string
}

// job is the state of one job of the run.
type job struct {
	spec
	idx     int
	status  string
	tries   int     // attempts started so far
	readyAt float64 // when a runner picks it up; -1 until it is eligible
	started float64 // sim time its current attempt started
	elapsed float64
	pct     float64
	plan    plan
	next    int // next planned line to print
	log     []logLine
}

// sim is the whole pipeline run. It is a pure state machine: step takes
// the clock as an argument and every random choice comes from a plan seeded
// by (seed, job, attempt), so a run replays exactly from the same seed.
type sim struct {
	seed  int64
	flaky bool
	speed float64 // simulated seconds per real second

	last time.Time
	t    float64 // simulated seconds since the run started
	run  int     // run number: 1 for the first, +1 for every rerun
	jobs []*job
	hist []float64
	next float64 // the next sparkline sample time
	spin int     // spinner frame, advanced by every step
	evs  []event // messages for the status line, oldest first
}

// event is a message for the status line.
type event struct {
	Text string
	Err  bool
}

func newSim(seed int64, flaky bool, speed float64, start time.Time) *sim {
	s := &sim{seed: seed, flaky: flaky, speed: speed, last: start, run: 1}
	for i, sp := range specs {
		s.jobs = append(s.jobs, &job{spec: sp, idx: i, status: stQueued, readyAt: -1})
	}
	return s
}

// step moves the simulation to the wall-clock time now: the time since the
// last step, times the speed, is simulated. It reports whether anything the
// view shows may have changed.
func (s *sim) step(now time.Time) bool {
	dt := now.Sub(s.last).Seconds() * s.speed
	s.last = now
	if dt < 0 {
		dt = 0
	}
	if s.idle() {
		return false
	}
	s.spin++
	for dt > 1e-9 {
		d := min(dt, subStep)
		s.sub(d)
		dt -= d
	}
	return true
}

// idle reports whether the run has nothing left to do: no job is running
// or waiting for a runner, whatever the result.
func (s *sim) idle() bool {
	for _, j := range s.jobs {
		if j.status == stRunning || j.status == stQueued {
			return false
		}
	}
	return true
}

// sub advances by d simulated seconds (d <= subStep).
func (s *sim) sub(d float64) {
	if s.idle() {
		return
	}
	s.t += d
	for _, j := range s.jobs {
		if j.status == stRunning {
			s.progress(j, d)
		}
	}
	for _, j := range s.jobs {
		if j.status != stQueued || !s.eligible(j) {
			continue
		}
		if j.readyAt < 0 {
			j.readyAt = s.t + pickupDelay(s.seed, j.idx, j.tries)
		}
		if s.t >= j.readyAt {
			s.start(j)
		}
	}
	s.settle()
	for s.t >= s.next {
		s.hist = append(s.hist, float64(s.count(stRunning)))
		if len(s.hist) > histLen {
			s.hist = s.hist[len(s.hist)-histLen:]
		}
		s.next++
	}
}

// eligible reports whether every job of an earlier stage has passed.
func (s *sim) eligible(j *job) bool {
	si := stageIndex(j.Stage)
	for _, o := range s.jobs {
		if stageIndex(o.Stage) < si && o.status != stPassed {
			return false
		}
	}
	return true
}

func (s *sim) start(j *job) {
	if s.count(stRunning)+s.count(stPassed)+s.count(stFailed) == 0 {
		s.say(fmt.Sprintf("Run #%d is running.", s.run), false)
	}
	fail := s.flaky && j.Flaky && j.tries == 0 && s.run == 1
	j.plan = makePlan(s.seed, j.spec, j.idx, j.tries, fail)
	j.tries++
	j.status = stRunning
	j.started = s.t
	j.elapsed = 0
	j.pct = 0
	j.next = 0
	s.flush(j)
}

// flush prints the planned lines that are due.
func (s *sim) flush(j *job) {
	for j.next < len(j.plan.Lines) && j.plan.Lines[j.next].At <= j.elapsed+1e-9 {
		l := j.plan.Lines[j.next]
		j.next++
		j.log = append(j.log, logLine{N: len(j.log) + 1, TS: fmtClock(j.started + l.At), Kind: l.Kind, Text: l.Text})
	}
}

func (s *sim) progress(j *job, d float64) {
	j.elapsed += d
	if f := j.plan.FailAt; f > 0 && j.elapsed >= f {
		j.elapsed = f
		s.flush(j)
		j.status = stFailed
		j.pct = j.elapsed / j.plan.Dur * 100
		s.say(fmt.Sprintf("%s/%s failed. Select it and press r to retry.", j.Stage, j.ID), true)
		return
	}
	if j.elapsed >= j.plan.Dur {
		j.elapsed = j.plan.Dur
		s.flush(j)
		j.status = stPassed
		j.pct = 100
		return
	}
	s.flush(j)
	j.pct = j.elapsed / j.plan.Dur * 100
}

// settle skips the jobs that can no longer run (an earlier stage failed
// and nothing is running or waiting) and announces the end of the run.
func (s *sim) settle() {
	if s.count(stRunning) > 0 {
		return
	}
	waiting := false
	for _, j := range s.jobs {
		if j.status == stQueued && s.eligible(j) {
			waiting = true
		}
	}
	if waiting {
		return
	}
	skipped := 0
	for _, j := range s.jobs {
		if j.status == stQueued {
			j.status = stSkipped
			skipped++
		}
	}
	switch {
	case s.count(stFailed) > 0:
		msg := fmt.Sprintf("Run #%d failed after %s.", s.run, fmtElapsed(s.t))
		if skipped > 0 {
			msg += fmt.Sprintf(" %d jobs skipped.", skipped)
		}
		s.say(msg, true)
	case s.count(stPassed) == len(s.jobs):
		s.say(fmt.Sprintf("Run #%d passed in %s.", s.run, fmtElapsed(s.t)), false)
	}
}

func (s *sim) say(text string, isErr bool) { s.evs = append(s.evs, event{Text: text, Err: isErr}) }

// drain returns the status messages since the last call.
func (s *sim) drain() []event {
	e := s.evs
	s.evs = nil
	return e
}

func (s *sim) count(status string) int {
	n := 0
	for _, j := range s.jobs {
		if j.status == status {
			n++
		}
	}
	return n
}

func (s *sim) find(id string) *job {
	for _, j := range s.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

// terminal reports whether the run can no longer be canceled.
func (s *sim) terminal() bool { return s.idle() }

// retry runs a failed job again; the jobs skipped because of it queue up
// again and the stages after it continue once it passes.
func (s *sim) retry(id string) error {
	j := s.find(id)
	if j == nil {
		return fmt.Errorf("no job %q", id)
	}
	if s.count(stCanceled) > 0 {
		return errors.New("The run was canceled: press R to rerun.")
	}
	if j.status != stFailed {
		return fmt.Errorf("only a failed job can be retried; %s is %s", j.ID, j.status)
	}
	j.status = stQueued
	j.readyAt = -1
	j.elapsed, j.pct = 0, 0
	j.next = 0
	j.plan = plan{}
	j.log = append(j.log, logLine{N: len(j.log) + 1, TS: fmtClock(s.t), Kind: kDim, Text: fmt.Sprintf("-- retry, attempt %d --", j.tries+1)})
	for _, o := range s.jobs {
		if o.status == stSkipped {
			o.status = stQueued
			o.readyAt = -1
		}
	}
	s.say(fmt.Sprintf("Retrying %s/%s (attempt %d).", j.Stage, j.ID, j.tries+1), false)
	return nil
}

// rerun starts the whole pipeline over as the next run.
func (s *sim) rerun() {
	s.run++
	s.t, s.next = 0, 0
	for _, j := range s.jobs {
		j.status = stQueued
		j.readyAt = -1
		j.elapsed, j.pct = 0, 0
		j.next = 0
		j.plan = plan{}
		j.log = nil
	}
	s.evs = nil
	s.say(fmt.Sprintf("Run #%d queued.", s.run), false)
}

// cancel stops the jobs that are running or queued.
func (s *sim) cancel() error {
	if s.idle() {
		return fmt.Errorf("nothing to cancel, the run is over")
	}
	n := 0
	for _, j := range s.jobs {
		switch j.status {
		case stRunning:
			j.log = append(j.log, logLine{N: len(j.log) + 1, TS: fmtClock(s.t), Kind: kWarn, Text: "canceled by user"})
			j.status = stCanceled
			n++
		case stQueued:
			j.status = stCanceled
			n++
		}
	}
	s.say(fmt.Sprintf("Run #%d canceled. %d jobs stopped.", s.run, n), true)
	return nil
}

// jobsStore is the jobs as the store holds them.
func (s *sim) jobsStore() []any {
	out := make([]any, len(s.jobs))
	for i, j := range s.jobs {
		lines := make([]any, len(j.log))
		for k, l := range j.log {
			lines[k] = map[string]any{"n": l.N, "ts": l.TS, "kind": l.Kind, "text": l.Text}
		}
		out[i] = map[string]any{
			"id": j.ID, "name": j.ID, "stage": j.Stage, "status": j.status,
			"pct": round1(j.pct), "elapsed": round1(j.elapsed), "log": lines,
		}
	}
	return out
}

func (s *sim) histStore() []any {
	out := make([]any, len(s.hist))
	for i, h := range s.hist {
		out[i] = h
	}
	return out
}

// clockStore is the time the derived labels read: the simulated seconds
// and the spinner frame.
func (s *sim) clockStore() map[string]any {
	return map[string]any{"t": round1(s.t), "spin": s.spin}
}
