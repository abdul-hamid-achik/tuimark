package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"
)

// sample.json is the store the host builds at start with the default seed,
// derived paths included, and mock.json gives every action literal effects
// for `tuimark run --mock`. Both are generated from the code, so they
// cannot drift: TestSampleInSync fails when they differ, and
//
//	UPDATE_SAMPLE=1 go test ./examples/snake -run TestSampleInSync
//
// rewrites them. The tuimark CLI runs no Go host and no Derive, so it reads
// the real start state from sample.json.

// startStore is the whole store of a fresh app, through JSON.
func startStore(t *testing.T) map[string]any {
	t.Helper()
	a, err := load(".", defaultSeed)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := a.ui.Get("")
	if !ok {
		t.Fatal("Get(\"\") found no store")
	}
	return normalize(t, v).(map[string]any)
}

// normalize round-trips v through JSON so numbers compare as float64.
func normalize(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func set(path string, v any) map[string]any { return map[string]any{"path": path, "value": v} }

// mockData applies literal values only: no Go host runs under
// `tuimark run`, so each action carries the derived values it changes.
func mockData(t *testing.T) map[string]any {
	t.Helper()
	g := newGame(defaultSeed)
	start := g.snake[0]
	mock := map[string]any{}
	// A steering action swings the snake round its head, as a turn would.
	for name, d := range map[string]point{"up": dirUp, "right": dirRight, "down": dirDown, "left": dirLeft} {
		body := []point{start, start.add(d.neg()), start.add(d.neg()).add(d.neg())}
		snake := make([]any, len(body))
		for i, p := range body {
			snake[i] = pointValue(p)
		}
		mock[name] = []any{
			set("snake", snake),
			set("board", buildBoard(g.w, g.h, body, g.food, false)),
		}
	}
	mock["pause"] = []any{set("paused", true), set("state", stateOf(true, false, false))}
	mock["restart"] = []any{
		set("paused", false), set("over", false), set("score", 0),
		set("state", stateOf(false, false, false)),
		set("board", buildBoard(g.w, g.h, g.snake, g.food, false)),
	}
	mock["autopilot"] = []any{set("auto", true), set("mode_label", "autopilot")}
	mock["help_open"] = []any{set("help_open", true)}
	mock["help_close"] = []any{set("help_open", false)}
	return mock
}

// compact writes a JSON object with one member per line and every array of
// objects one element per line, so the files diff and read well.
func compact(t *testing.T, v map[string]any) []byte {
	t.Helper()
	var b bytes.Buffer
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteString("{\n")
	for i, k := range keys {
		kb, _ := json.Marshal(k)
		b.WriteString("  " + string(kb) + ": ")
		b.Write(compactValue(t, v[k], "  "))
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.Bytes()
}

// compactValue writes an array whose elements are objects, or the mock's
// action lists, one element per line; anything else on one line.
func compactValue(t *testing.T, v any, indent string) []byte {
	t.Helper()
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if _, isObj := list[0].(map[string]any); !isObj {
		b, _ := json.Marshal(v)
		return b
	}
	var b bytes.Buffer
	b.WriteString("[\n")
	for j, e := range list {
		eb, _ := json.Marshal(e)
		b.WriteString(indent + "  " + string(eb))
		if j < len(list)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(indent + "]")
	return b.Bytes()
}

func checkFile(t *testing.T, name string, want map[string]any, render func() []byte) {
	t.Helper()
	if os.Getenv("UPDATE_SAMPLE") != "" {
		if err := os.WriteFile(name, render(), 0o644); err != nil {
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
	if !reflect.DeepEqual(got, normalize(t, want)) {
		t.Errorf("%s differs from the code's output; run UPDATE_SAMPLE=1 go test ./examples/snake -run TestSampleInSync", name)
	}
}

func TestSampleInSync(t *testing.T) {
	checkFile(t, "sample.json", startStore(t), func() []byte { return compact(t, startStore(t)) })
	checkFile(t, "mock.json", mockData(t), func() []byte { return compact(t, mockData(t)) })
}
