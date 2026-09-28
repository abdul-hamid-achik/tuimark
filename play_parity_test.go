package tuimark_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/play"
)

// SPEC v0.3 §21 test 80: Play parity. For every size of every manifest
// entry with input (or script; none currently use script — see below),
// Play with NoHandlers on a freshly loaded app with the entry's data
// (and its theme, applied with Set("@theme", …) before the call) gives
// the events and the dump the golden holds: the JSON golden member for
// member where the entry has one, else the §13.3 text form with its
// "=== events ===" section.

// parityEntry mirrors cmd/tuimark's goldenEntry (unexported, package
// main): only the members this test reads.
type parityEntry struct {
	Name   string   `json:"name"`
	File   string   `json:"file"`
	Data   string   `json:"data,omitempty"`
	Sizes  []string `json:"sizes"`
	JSON   []string `json:"json"`
	Frozen bool     `json:"frozen"`
	Theme  *string  `json:"theme,omitempty"`
	Input  string   `json:"input,omitempty"`
	Script string   `json:"script,omitempty"`
	Styles bool     `json:"styles,omitempty"`
	Cells  bool     `json:"cells,omitempty"`
}

func parseParitySize(s string) (cols, rows int) {
	c, r, ok := strings.Cut(s, "x")
	if !ok {
		return 0, 0
	}
	cols, _ = strconv.Atoi(c)
	rows, _ = strconv.Atoi(r)
	return cols, rows
}

func TestPlayParityWithGoldenManifest(t *testing.T) {
	raw, err := os.ReadFile("testdata/golden/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []parityEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}

	tested := 0
	for _, e := range entries {
		if e.Script != "" {
			// No manifest entry currently uses "script": tuimark.Play's
			// steps are always tuimark play's --input token grammar
			// (SPEC v0.3 §18.1 rule 3), never raw NDJSON script lines, so
			// exercising this would need translating each script step
			// back to its equivalent --input token first. Flagged here
			// rather than silently skipped, so it is not forgotten if a
			// script-based entry is ever added.
			t.Logf("%s: uses \"script\", which this parity test does not translate; skipped", e.Name)
			continue
		}
		if e.Input == "" {
			continue
		}
		e := e
		t.Run(e.Name, func(t *testing.T) {
			steps := strings.Fields(e.Input)
			for _, size := range e.Sizes {
				size := size
				t.Run(size, func(t *testing.T) {
					cols, rows := parseParitySize(size)
					app, err := tuimark.Load(e.File)
					if err != nil {
						t.Fatal(err)
					}
					if e.Data != "" {
						raw, err := os.ReadFile(e.Data)
						if err != nil {
							t.Fatal(err)
						}
						var v any
						if err := json.Unmarshal(raw, &v); err != nil {
							t.Fatal(err)
						}
						if err := app.Bind("", v); err != nil {
							t.Fatal(err)
						}
					}
					if e.Theme != nil {
						if err := app.Set("@theme", *e.Theme); err != nil {
							t.Fatal(err)
						}
					}
					res, err := app.Play(tuimark.PlayOptions{
						Cols: cols, Rows: rows, NoHandlers: true,
						Styles: e.Styles, Cells: e.Cells,
					}, steps...)
					if err != nil {
						t.Fatalf("Play: %v", err)
					}

					jsonGolden := false
					for _, s := range e.JSON {
						if s == size {
							jsonGolden = true
						}
					}
					dir := filepath.Join("testdata", "golden", e.Name)
					if jsonGolden {
						want, err := os.ReadFile(filepath.Join(dir, size+".json"))
						if err != nil {
							t.Fatal(err)
						}
						got, err := json.MarshalIndent(&dump.Play{Dump: res.Dump, Events: res.Events}, "", "  ")
						if err != nil {
							t.Fatal(err)
						}
						got = append(got, '\n')
						if string(got) != string(want) {
							t.Errorf("json golden mismatch for %s %s", e.Name, size)
						}
					} else {
						want, err := os.ReadFile(filepath.Join(dir, size+".txt"))
						if err != nil {
							t.Fatal(err)
						}
						var b strings.Builder
						b.WriteString(dump.Text(res.Dump))
						play.WriteEventsSection(&b, res.Events)
						if b.String() != string(want) {
							t.Errorf("text golden mismatch for %s %s:\ngot:\n%s\nwant:\n%s", e.Name, size, b.String(), string(want))
						}
					}
				})
			}
		})
		tested++
	}
	if tested == 0 {
		t.Fatal("no manifest entries with \"input\" were found")
	}
}
