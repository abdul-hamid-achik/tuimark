package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// sample.json and mock.json are snapshots of the real app, so they cannot
// drift from it: TestSampleInSync fails when they differ, and
//
//	UPDATE_SAMPLE=1 go test ./examples/pipeline -run TestSampleInSync
//
// rewrites them.
//
// The sample is the store at a documented point of the default run (seed 7,
// flaky, speed 4): sampleSecs simulated seconds in, the integration test
// has failed, e2e is still running, and the lint, package, and deploy stages
// wait behind them. The cursor is on the failed job, so the log pane shows
// the failure. The CLI (tuimark validate, dump, play) runs no Go host and no
// Derive, so this is what it sees: derived paths included.
const sampleSecs = 22

// storeKeys are the top-level store members, raw and derived.
var storeKeys = []string{
	"pipeline", "jobs", "clock", "spark", "cur", "follow", "freeze",
	"confirm_open", "help_open", "status", "rows", "run", "log",
}

// sampleRig is the app at the sample point.
func sampleRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t, defaultSim())
	r.advance(sampleSecs)
	if err := r.a.ui.Set("cur", "integration"); err != nil {
		t.Fatal(err)
	}
	return r
}

// snapshot reads the store members back from the app.
func (r *rig) snapshot() map[string]any {
	out := map[string]any{}
	for _, k := range storeKeys {
		v, ok := r.a.ui.Get(k)
		if !ok {
			r.t.Fatalf("store has no %q", k)
		}
		out[k] = v
	}
	return out
}

func sampleData(t *testing.T) map[string]any { return sampleRig(t).snapshot() }

// effect is {"path": P, "value": V}.
func effect(path string, v any) map[string]any { return map[string]any{"path": path, "value": v} }

// changed lists the derived members of the store after an action, as the
// literal effects a mock applies.
func (r *rig) effects(paths ...string) []any {
	out := make([]any, 0, len(paths))
	for _, p := range paths {
		out = append(out, effect(p, r.get(p)))
	}
	return out
}

// mockData applies literal values only (tuimark run has no Go host and no
// Derive), so each action carries the store members it changes, read from
// the real app after the same action.
func mockData(t *testing.T) map[string]any {
	t.Helper()
	mock := map[string]any{}

	r := sampleRig(t)
	r.play(100, 24, "r")
	mock["job_retry"] = r.effects("rows", "run", "log", "status")

	r = sampleRig(t)
	r.play(100, 24, "R")
	mock["run_rerun"] = r.effects("rows", "run", "log", "status", "pipeline.run", "follow")

	r = sampleRig(t)
	r.play(100, 24, "c")
	mock["run_cancel"] = r.effects("confirm_open")

	r = sampleRig(t)
	r.play(100, 24, "c", "y")
	mock["cancel_confirm"] = r.effects("rows", "run", "log", "status", "confirm_open")

	r = sampleRig(t)
	r.play(100, 24, "c", "n")
	mock["cancel_abort"] = r.effects("confirm_open")

	r = sampleRig(t)
	r.play(100, 24, "f")
	mock["follow_toggle"] = r.effects("follow", "freeze", "log")

	mock["job_select"] = []any{effect("status", map[string]any{"text": "Mock: the log stays on the sample job.", "err": false})}
	mock["help_open"] = []any{effect("help_open", true)}
	mock["help_close"] = []any{effect("help_open", false)}
	return mock
}

// marshal is json.Marshal without HTML escaping, so text reads as written.
func marshal(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimRight(b.String(), "\n")
}

// pretty writes v as JSON, keeping what fits in 110 columns on one line and
// breaking larger objects and arrays one member per line, so the files read
// and diff well.
func pretty(v any, indent string) string {
	one := marshal(v)
	if len(one)+len(indent) <= 110 {
		return one
	}
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString("{\n")
		for i, k := range keys {
			b.WriteString(indent + "  " + marshal(k) + ": " + pretty(x[k], indent+"  "))
			if i < len(keys)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(indent + "}")
		return b.String()
	case []any:
		var b strings.Builder
		b.WriteString("[\n")
		for i, e := range x {
			b.WriteString(indent + "  " + pretty(e, indent+"  "))
			if i < len(x)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(indent + "]")
		return b.String()
	}
	return one
}

func checkFile(t *testing.T, name string, want map[string]any) {
	t.Helper()
	// Round-trip through JSON so numbers compare as float64.
	var norm map[string]any
	if err := json.Unmarshal([]byte(marshal(want)), &norm); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("UPDATE_SAMPLE") != "" {
		if err := os.WriteFile(name, []byte(pretty(norm, "")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if !reflect.DeepEqual(got, norm) {
		t.Errorf("%s differs from the app's store; run UPDATE_SAMPLE=1 go test ./examples/pipeline -run TestSampleInSync", name)
	}
}

func TestSampleInSync(t *testing.T) {
	checkFile(t, "sample.json", sampleData(t))
	checkFile(t, "mock.json", mockData(t))
}

// The sample point is the story the README tells.
func TestSampleShowsAFailedRun(t *testing.T) {
	r := sampleRig(t)
	want := map[string]string{
		"compile": stPassed, "assets": stPassed, "unit": stPassed,
		"integration": stFailed, "e2e": stRunning,
		"fmt": stQueued, "docker": stQueued, "deploy": stQueued,
	}
	for id, st := range want {
		if got := r.status(id); got != st {
			t.Errorf("%s is %s, want %s", id, got, st)
		}
	}
	if got := r.str("run.status"); got != "running" {
		t.Errorf("run.status = %q", got)
	}
}

// mock.json names exactly the document's non-builtin actions, which is what
// tuimark run --mock accepts (an unknown or built-in action is an error).
func TestMockCoversTheActions(t *testing.T) {
	r := newRig(t, defaultSim())
	want := map[string]bool{}
	for _, spec := range r.a.ui.Catalog() {
		if !spec.Builtin {
			want[spec.Name] = true
		}
	}
	got := map[string]bool{}
	for k := range mockData(t) {
		got[k] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mock actions %v, document actions %v", got, want)
	}
}
