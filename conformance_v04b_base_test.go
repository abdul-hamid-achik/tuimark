package tuimark_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
	"github.com/abdul-hamid-achik/tuimark/internal/catalog"
	"github.com/abdul-hamid-achik/tuimark/internal/dump"
)

// SPEC v0.4b §24 conformance tests of 0.4b's base (phase F0): the current
// language and pinned documents. These are the parts the Go API observes;
// the CLI parts (exit codes, validate --catalog, the frozen copy of the
// dump schema) are cmd/tuimark's TestV04b* tests, and the 0.4.1 twins are
// the frozen entries inbox-041, monitor-041, and colrow-041 of
// testdata/golden/manifest.json, which tuimark 0.4.1 generated.

// 189 (and 129's first two bullets). A pinned document dumps exactly as in
// tuimark 0.4.1: examples/inbox (version="1"), examples/monitor/studio.tui
// (version="3"), and test 129's col/row document give, through the public
// Dump, the JSON of the twins tuimark 0.4.1 wrote, byte for byte.
func TestPinnedDocumentsDumpAsIn041(t *testing.T) {
	for _, c := range []struct {
		twin, file, data string
		sizes            [][2]int
	}{
		{"inbox-041", "examples/inbox/app.tui", "examples/inbox/sample.json", [][2]int{{80, 24}, {120, 40}}},
		{"monitor-041", "examples/monitor/studio.tui", "examples/monitor/sample.json", [][2]int{{80, 24}, {120, 40}}},
		{"colrow-041", "specs/fixtures/v04b/twins/colrow.tui", "", [][2]int{{20, 3}, {80, 24}}},
	} {
		t.Run(c.twin, func(t *testing.T) {
			app, err := tuimark.Load(c.file)
			if err != nil {
				t.Fatal(err)
			}
			if c.data != "" {
				raw, err := os.ReadFile(c.data)
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
			for _, s := range c.sizes {
				d, err := app.Dump(s[0], s[1])
				if err != nil {
					t.Fatal(err)
				}
				got, err := dump.JSON(d)
				if err != nil {
					t.Fatal(err)
				}
				want, err := os.ReadFile(filepath.Join("testdata/golden", c.twin, fmt.Sprintf("%dx%d.json", s[0], s[1])))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("%dx%d: the dump differs from tuimark 0.4.1's", s[0], s[1])
				}
			}
		})
	}
}

// keymapK is test 209's document K; ATTRS is replaced by the <tui>
// attributes of each case.
const keymapK = `<tui ATTRS>
  <screen id="main">
    <input id="q" bind="q"/>
    <list id="l"><item><text>a</text></item></list>
  </screen>
  <keymap>
    <bind keys="ctrl+i" action="a"/>
    <bind keys="ctrl+j,x" action="b"/>
    <bind keys="ctrl+m,ctrl+i" action="c"/>
  </keymap>
</tui>`

func keymapApp(t *testing.T, attrs string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(strings.Replace(keymapK, "ATTRS", attrs, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", map[string]any{"q": ""}); err != nil {
		t.Fatal(err)
	}
	return app
}

// playEventsOf lists a play's events as "step action source".
func playEventsOf(evs []tuimark.PlayEvent) []string {
	var out []string
	for _, e := range evs {
		out = append(out, fmt.Sprintf("%d %s %s", e.Step, e.Action, e.Source))
	}
	return out
}

// 209. ctrl+i, ctrl+j, and ctrl+m are V003 in a keymap of the current
// language, once per token, at the <bind>, with no hint, and the other
// tokens of a list keep working; a pinned document reports nothing and
// behaves as in 0.4.1; play steps accept the three tokens in every
// document and deliver tab or enter; the catalog and AGENTS.md say so.
func TestNeverMatchKeysCurrentAndPinned(t *testing.T) {
	t.Run("current", func(t *testing.T) {
		app := keymapApp(t, "")
		var got []string
		for _, d := range app.Validate() {
			got = append(got, fmt.Sprintf("%s %s %d:%d %s", d.Severity, d.Code, d.Line, d.Col, d.Msg))
		}
		want := []string{
			`error V003 7:5 key "ctrl+i" never reaches a keymap: it arrives as tab (write tab)`,
			`error V003 8:5 key "ctrl+j" never reaches a keymap: it arrives as enter (write enter)`,
			`error V003 9:5 key "ctrl+m" never reaches a keymap: it arrives as enter (write enter)`,
			`error V003 9:5 key "ctrl+i" never reaches a keymap: it arrives as tab (write tab)`,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Validate:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
		res, err := app.Play(tuimark.PlayOptions{Cols: 40, Rows: 5, NoHandlers: true}, "tab", "x")
		if err != nil {
			t.Fatal(err)
		}
		if ev := playEventsOf(res.Events); !reflect.DeepEqual(ev, []string{"2 b l"}) || res.Dump.Focus == nil || *res.Dump.Focus != "l" {
			t.Errorf("tab x: events %v, focus %v", ev, res.Dump.Focus)
		}
		if _, err := keymapApp(t, "").Play(tuimark.PlayOptions{Cols: 40, Rows: 5, NoHandlers: true}, "ctrl+i", "ctrl+j", "ctrl+m"); err != nil {
			t.Errorf("the steps ctrl+i ctrl+j ctrl+m are refused: %v", err)
		}
	})
	for _, v := range []string{"1", "2", "3"} {
		t.Run("version="+v, func(t *testing.T) {
			app := keymapApp(t, `version="`+v+`"`)
			if d := app.Validate(); len(d) != 0 {
				t.Errorf("Validate: %v", d)
			}
			res, err := app.Play(tuimark.PlayOptions{Cols: 40, Rows: 5, NoHandlers: true, Frames: true}, "ctrl+i", "x")
			if err != nil {
				t.Fatal(err)
			}
			if f := res.Frames[1].Dump.Focus; f == nil || *f != "l" {
				t.Errorf("focus after ctrl+i: %v, want l (it arrived as tab)", f)
			}
			if ev := playEventsOf(res.Events); !reflect.DeepEqual(ev, []string{"2 b l"}) {
				t.Errorf("ctrl+i x: events %v, want only b at step 2", ev)
			}
			res, err = keymapApp(t, `version="`+v+`"`).Play(tuimark.PlayOptions{Cols: 40, Rows: 5, NoHandlers: true, Frames: true}, "ctrl+i", "ctrl+j", "ctrl+m")
			if err != nil {
				t.Fatalf("ctrl+i ctrl+j ctrl+m: %v", err)
			}
			if f := res.Frames[1].Dump.Focus; f == nil || *f != "l" || len(res.Events) != 0 {
				t.Errorf("ctrl+i ctrl+j ctrl+m: focus after step 1 %v, events %v", f, playEventsOf(res.Events))
			}
		})
	}
	t.Run("catalog and AGENTS.md", func(t *testing.T) {
		c := catalog.Build("x")
		if !reflect.DeepEqual(c.Keys.NeverMatch, []string{"ctrl+i", "ctrl+j", "ctrl+m"}) || c.Keys.RejectedSince != 4 {
			t.Errorf("catalog keys: %+v", c.Keys)
		}
		raw, err := os.ReadFile("AGENTS.md")
		if err != nil {
			t.Fatal(err)
		}
		var para string
		for _, l := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(l, "Also valid: `ctrl+<a-z>`") {
				para = l
			}
		}
		for _, s := range []string{"`ctrl+i`, `ctrl+j`, and `ctrl+m`", "V003", "a `<tui>` without `version`", "a document that writes `version` accepts them, never matching"} {
			if !strings.Contains(para, s) {
				t.Errorf("the Key tokens paragraph lacks %q: %q", s, para)
			}
		}
	})
}
