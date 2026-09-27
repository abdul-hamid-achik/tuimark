package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout redirects os.Stdout for the duration of f and returns
// everything written to it. run() and its helpers print with fmt.Println,
// which reads os.Stdout at call time, so this is enough to observe them
// without threading a Writer through the whole CLI.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	f()
	w.Close()
	os.Stdout = orig
	return <-done
}

func jsonLines(t *testing.T, out string) []EngineEvent {
	t.Helper()
	var evs []EngineEvent
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var ev EngineEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %q is not a JSON EngineEvent: %v", line, err)
		}
		evs = append(evs, ev)
	}
	return evs
}

func findEvent(evs []EngineEvent, typ string) (EngineEvent, bool) {
	for _, e := range evs {
		if e.Type == typ {
			return e, true
		}
	}
	return EngineEvent{}, false
}

func TestHeadlessListJSON(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ws")
	var code int
	out := captureStdout(t, func() {
		code = run([]string{
			"--dir", ".", "--headless", "--json",
			"--demo-workspace", ws, "-p", "list",
		})
	})
	if code != 0 {
		t.Fatalf("run() = %d, want 0; output:\n%s", code, out)
	}
	evs := jsonLines(t, out)
	if _, ok := findEvent(evs, "user_message"); !ok {
		t.Errorf("no user_message event in %+v", evs)
	}
	tc, ok := findEvent(evs, "tool_call")
	if !ok || tc.Tool != "list_files" {
		t.Errorf("expected a list_files tool_call, got %+v", evs)
	}
	final, ok := findEvent(evs, "assistant_message")
	if !ok || !strings.Contains(final.Text, "notes.txt") {
		t.Errorf("expected the final assistant_message to mention notes.txt, got %+v", final)
	}
}

func TestHeadlessEditDeniedByDefault(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ws")
	code := run([]string{
		"--dir", ".", "--headless", "--json",
		"--demo-workspace", ws, "-p", "edit notes.txt: before -> after",
	})
	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	got, err := os.ReadFile(filepath.Join(ws, "notes.txt"))
	if err != nil {
		t.Fatalf("reading notes.txt: %v", err)
	}
	if string(got) != "before\n" {
		t.Errorf("notes.txt = %q, want unchanged %q (edit should be denied without --approve-edits)", got, "before\n")
	}
}

func TestHeadlessEditApproved(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ws")
	code := run([]string{
		"--dir", ".", "--headless", "--json", "--approve-edits",
		"--demo-workspace", ws, "-p", "edit notes.txt: before -> after",
	})
	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	got, err := os.ReadFile(filepath.Join(ws, "notes.txt"))
	if err != nil {
		t.Fatalf("reading notes.txt: %v", err)
	}
	if string(got) != "after\n" {
		t.Errorf("notes.txt = %q, want %q", got, "after\n")
	}
}

func TestHeadlessPlanModeRefusesEdit(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ws")
	var code int
	out := captureStdout(t, func() {
		code = run([]string{
			"--dir", ".", "--headless", "--json", "--mode", "plan", "--approve-edits",
			"--demo-workspace", ws, "-p", "edit notes.txt: before -> after",
		})
	})
	if code != 0 {
		t.Fatalf("run() = %d, want 0; output:\n%s", code, out)
	}
	got, err := os.ReadFile(filepath.Join(ws, "notes.txt"))
	if err != nil {
		t.Fatalf("reading notes.txt: %v", err)
	}
	if string(got) != "before\n" {
		t.Errorf("notes.txt = %q, want unchanged (plan mode must refuse edit_file even with --approve-edits)", got)
	}
	evs := jsonLines(t, out)
	if _, ok := findEvent(evs, "approval"); ok {
		t.Error("plan mode should refuse edit_file outright, with no approval step at all")
	}
	tr, ok := findEvent(evs, "tool_result")
	if !ok || !strings.Contains(tr.Result, "plan mode") {
		t.Errorf("expected a tool_result explaining the plan-mode refusal, got %+v", evs)
	}
}

func TestHeadlessDumpFlag(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ws")
	var code int
	out := captureStdout(t, func() {
		code = run([]string{
			"--dir", ".", "--headless", "--dump", "100x24",
			"--demo-workspace", ws, "-p", "list",
		})
	})
	if code != 0 {
		t.Fatalf("run() = %d, want 0; output:\n%s", code, out)
	}
	var d struct {
		OK   bool `json:"ok"`
		Cols int  `json:"cols"`
		Rows int  `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("--dump output is not valid JSON: %v\n%s", err, out)
	}
	if !d.OK || d.Cols != 100 || d.Rows != 24 {
		t.Errorf("dump = %+v, want ok at 100x24", d)
	}
}

func TestHeadlessFixtureOllama(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ws")
	var code int
	out := captureStdout(t, func() {
		code = run([]string{
			"--dir", ".", "--headless", "--json", "--fixture",
			"--demo-workspace", ws, "-p", "read notes.txt",
		})
	})
	if code != 0 {
		t.Fatalf("run() = %d, want 0; output:\n%s", code, out)
	}
	evs := jsonLines(t, out)
	final, ok := findEvent(evs, "assistant_message")
	if !ok || !strings.Contains(final.Text, "loopback HTTP") {
		t.Errorf("expected the fixture's final answer to mention loopback HTTP, got %+v", evs)
	}
}
