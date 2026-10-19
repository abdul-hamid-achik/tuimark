package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// sample.json and mock.json are snapshots of the real app, so they cannot
// drift from it: TestSampleInSync fails when they differ, and
//
//	UPDATE_SAMPLE=1 go test ./examples/containers -run TestSampleInSync
//
// rewrites them.
//
// The sample is the store twelve simulated seconds into the default run
// (seed 7): two rows marked (postgres and redis), the cursor on the first
// row, every tab's data present. The CLI (tuimark validate, dump, play) runs
// no Go host and no Derive, so this is what it sees: derived paths included.
const sampleSecs = 12

// storeKeys are the top-level store members, raw and derived.
var storeKeys = []string{
	"view", "detail_tab", "cur", "sel", "filter", "sort", "confirm", "palette", "help", "toast",
	"status", "theme", "all", "logs", "events", "images", "volumes", "engine", "clock",
	"rows", "counts", "detail", "found",
}

// sampleRig is the app at the sample point.
func sampleRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	r.advance(sampleSecs)
	r.set("sel", []any{r.idOf("postgres"), r.idOf("redis")})
	return r
}

func (r *rig) snapshot() map[string]any {
	out := map[string]any{}
	for _, k := range storeKeys {
		out[k] = r.get(k)
	}
	return out
}

func effect(path string, v any) map[string]any { return map[string]any{"path": path, "value": v} }

// effects lists store members after an action, as the literal writes a mock
// applies (tuimark run has no Go host and no Derive).
func (r *rig) effects(paths ...string) []any {
	out := make([]any, 0, len(paths))
	for _, p := range paths {
		out = append(out, effect(p, r.get(p)))
	}
	return out
}

var (
	confirmPaths = []string{"confirm.title", "confirm.message", "confirm.danger", "confirm.open"}
	// A mock cannot recompute the engine, so a confirmed operation only closes
	// the dialog and says so; the Go host does the work.
	donePaths = []string{"confirm.open", "toast", "status"}
)

// mockData applies literal values only, so each action carries the store
// members it changes, read from the real app after the same keys.
func mockData(t *testing.T) map[string]any {
	t.Helper()
	mock := map[string]any{}
	do := func(action string, paths []string, setup []string, steps ...string) {
		t.Helper()
		r := sampleRig(t)
		if len(setup) > 0 {
			r.play(100, 24, setup...)
		}
		r.play(100, 24, steps...)
		mock[action] = r.effects(paths...)
	}

	do("container_stop", confirmPaths, nil, "s")
	do("container_restart", confirmPaths, nil, "r")
	do("container_remove", confirmPaths, nil, "d")
	do("container_start", []string{"toast", "status"}, nil, "u")
	do("dialog_confirm", donePaths, nil, "s", "y")
	do("dialog_cancel", []string{"confirm.open", "status"}, nil, "s", "n")
	do("filter_change", []string{"filter", "rows", "counts", "cur"}, nil, "/", "text:api")
	do("filter_clear", []string{"filter", "rows", "counts", "cur"}, nil, "/", "text:api", "esc")
	do("container_sort", []string{"sort", "counts", "rows", "status"}, nil, "o")
	do("image_prune", confirmPaths, []string{"3"}, "p")
	do("volume_prune", confirmPaths, []string{"4"}, "p")
	do("help_open", []string{"help"}, nil, "?")
	do("help_close", []string{"help"}, nil, "?", "q")
	do("palette_open", []string{"palette", "found"}, nil, "ctrl+k")
	do("palette_close", []string{"palette"}, nil, "ctrl+k", "esc")
	do("palette_run", append([]string{"palette"}, confirmPaths...), nil, "ctrl+k", "enter")
	do("theme_toggle", []string{"theme", "status"}, nil, "t")
	mock["theme_toggle"] = append(mock["theme_toggle"].([]any), effect("@theme", "light"))
	do("toast_dismiss", []string{"toast"}, nil, "esc")
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
		t.Errorf("%s differs from the app's store; run UPDATE_SAMPLE=1 go test ./examples/containers -run TestSampleInSync", name)
	}
}

func TestSampleInSync(t *testing.T) {
	checkFile(t, "sample.json", sampleRig(t).snapshot())
	checkFile(t, "mock.json", mockData(t))
}

// Each mock is a batch of literal writes, as `tuimark run --mock` applies it;
// they must all apply, to the paths the sample really has.
func TestMockWritesApply(t *testing.T) {
	for action, writes := range mockData(t) {
		// A fresh document with the sample bound and no host: what tuimark run
		// --mock has, so derived paths are plain values here.
		ui, err := tuimark.Load("app.tui")
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile("sample.json")
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		if err := ui.Bind("", data); err != nil {
			t.Fatal(err)
		}
		if err := ui.Batch(func(b *tuimark.Batch) error {
			for _, w := range writes.([]any) {
				m := w.(map[string]any)
				if err := b.Set(m["path"].(string), m["value"]); err != nil {
					return fmt.Errorf("%s: %s: %w", action, m["path"], err)
				}
			}
			return nil
		}); err != nil {
			t.Error(err)
		}
	}
}

// mock.json names exactly the document's non-builtin actions, which is what
// tuimark run --mock accepts (an unknown or built-in action is an error).
func TestMockCoversTheActions(t *testing.T) {
	r := newRig(t)
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
