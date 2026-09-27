package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// loadSample loads agent.tui with sample.json bound, exactly as
// `tuimark dump examples/agent/agent.tui --data examples/agent/sample.json`
// does, so these tests catch anything that would make that command fail.
func loadSample(t *testing.T) *tuimark.App {
	t.Helper()
	ui, err := tuimark.Load("agent.tui")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	raw, err := os.ReadFile("sample.json")
	if err != nil {
		t.Fatalf("reading sample.json: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshal sample.json: %v", err)
	}
	if err := ui.Bind("", data); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	return ui
}

func hasNodeID(d *tuimark.Dump, id string) bool {
	for _, n := range d.Nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

func TestDumpNoErrorsAt80And120(t *testing.T) {
	for _, cols := range []int{80, 120} {
		ui := loadSample(t)
		d, err := ui.Dump(cols, 30)
		if err != nil {
			t.Fatalf("Dump(%d,30): %v", cols, err)
		}
		if !d.OK {
			t.Fatalf("Dump(%d,30).OK = false, errors: %+v", cols, d.Errors)
		}
		for _, e := range d.Errors {
			if e.Severity == "error" {
				t.Errorf("Dump(%d,30) reported an error diagnostic: %s", cols, e.String())
			}
		}
	}
}

func TestSidePanelHiddenBelow100Cols(t *testing.T) {
	ui := loadSample(t)
	d80, err := ui.Dump(80, 30)
	if err != nil {
		t.Fatalf("Dump(80,30): %v", err)
	}
	if hasNodeID(d80, "side") {
		t.Error("at 80 cols, #side should be hidden by @media (max-cols: 99)")
	}

	ui = loadSample(t)
	d120, err := ui.Dump(120, 30)
	if err != nil {
		t.Fatalf("Dump(120,30): %v", err)
	}
	if !hasNodeID(d120, "side") {
		t.Error("at 120 cols, #side should be visible")
	}
}

func TestDumpRendersTranscriptAndComposer(t *testing.T) {
	ui := loadSample(t)
	d, err := ui.Dump(120, 30)
	if err != nil {
		t.Fatalf("Dump: %v", err)
	}
	for _, id := range []string{"transcript", "composer", "ctxbar", "header", "status"} {
		if !hasNodeID(d, id) {
			t.Errorf("Dump is missing expected node %q", id)
		}
	}
}
