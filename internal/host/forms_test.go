package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// SPEC v0.4b test 188, "Under TUIMARK_LOG": toggling a checkbox in a Run
// session logs its on:change with the new boolean, empty keys, and no
// redacted.
func TestRunLogCheckboxAction(t *testing.T) {
	const src = `<tui>
  <screen id="main" focus="#dark">
    <checkbox id="dark" bind="settings.dark" label="Dark mode" on:change="dark_changed"/>
    <checkbox id="beta" label="Beta"/>
  </screen>
</tui>`
	path := filepath.Join(t.TempDir(), "run.log")
	lg, err := openRunLog(path, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan Event, 4)
	r := startSessionWith(t, src, &termSession{log: lg}, func(a *App) {
		_ = a.Bind("", map[string]any{"settings": map[string]any{"dark": true}})
		a.On("dark_changed", func(ev Event) error { got <- ev; return nil })
	})
	r.send(t, " ")
	select {
	case ev := <-got:
		if ev.Source != "dark" || ev.Value != false {
			t.Fatalf("event %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("space fired no dark_changed")
	}
	r.send(t, "\x03")
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	lg.end("quit")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var line string
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(l, `{"ev":"action",`) && strings.Contains(l, `"action":"dark_changed"`) {
			line = l
		}
	}
	if !strings.HasSuffix(line, `,"action":"dark_changed","source":"dark","keys":{},"value":false}`) || strings.Contains(line, "redacted") {
		t.Errorf("action record %q in:\n%s", line, raw)
	}
}
