package play

import (
	"errors"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

// SPEC v0.4a §21 test 114, the Play parts: a Derive error ends the step
// being applied, and a derive over an input updates on each keystroke.

var errDeriveBoom = errors.New("boom")

func deriveApp(t *testing.T, src string, data map[string]any) *host.App {
	t.Helper()
	app, err := host.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", data); err != nil {
		t.Fatal(err)
	}
	return app
}

func failOnOne(get func(string) (any, bool)) (any, error) {
	if v, _ := get("n"); v == float64(1) {
		return nil, errDeriveBoom
	}
	return "ok", nil
}

const deriveCounterDoc = `<tui version="1"><keymap><bind keys="k" action="kick"/></keymap>
<screen id="s"><text>N={n}</text></screen></tui>`

func TestPlayDeriveError(t *testing.T) {
	t.Run("ends the step being applied", func(t *testing.T) {
		app := deriveApp(t, deriveCounterDoc, map[string]any{"n": 0})
		if err := app.Derive("d", failOnOne); err != nil {
			t.Fatal(err)
		}
		res, err := Play(app, Options{Cols: 20, Rows: 2, Frames: true}, []string{"set:n=1", "k", "set:n=2"})
		if !errors.Is(err, errDeriveBoom) {
			t.Fatalf("Play = %v, want the Derive error", err)
		}
		if res == nil || len(res.Frames) != 2 {
			t.Fatalf("result so far: %+v", res)
		}
		if got := res.Frames[1].Dump.Grid[0]; !strings.HasPrefix(got, "N=1") {
			t.Fatalf("the step's frame: %q", got)
		}
		if len(res.Events) != 0 {
			t.Fatalf("events = %+v; the later steps were applied", res.Events)
		}
		if v, _ := app.Get("n"); v != float64(1) {
			t.Fatalf("n = %v; set:n=2 was applied", v)
		}
		// Reported once: the next call goes on.
		if _, err := Play(app, Options{Cols: 20, Rows: 2}, []string{"set:n=2"}); err != nil {
			t.Fatal(err)
		}
		if v, _ := app.Get("d"); v != "ok" {
			t.Fatalf("d = %v", v)
		}
	})
	t.Run("kept until step 0 of the next call", func(t *testing.T) {
		app := deriveApp(t, deriveCounterDoc, map[string]any{"n": 0})
		if err := app.Derive("d", failOnOne); err != nil {
			t.Fatal(err)
		}
		if err := app.Set("n", 1); err != nil {
			t.Fatalf("the Set that caused it returns %v", err)
		}
		res, err := Play(app, Options{Cols: 20, Rows: 2, Frames: true}, []string{"k"})
		if !errors.Is(err, errDeriveBoom) {
			t.Fatalf("Play = %v, want the Derive error", err)
		}
		if len(res.Frames) != 1 || res.Frames[0].Step != 0 {
			t.Fatalf("frames = %+v; want step 0 only", res.Frames)
		}
	})
	t.Run("ErrQuit from fn is a quit", func(t *testing.T) {
		app := deriveApp(t, deriveCounterDoc, map[string]any{"n": 0})
		if err := app.Derive("d", func(get func(string) (any, bool)) (any, error) {
			if v, _ := get("n"); v == float64(1) {
				return nil, host.ErrQuit
			}
			return nil, nil
		}); err != nil {
			t.Fatal(err)
		}
		res, err := Play(app, Options{Cols: 20, Rows: 2}, []string{"set:n=1", "set:n=2"})
		if err != nil || !res.Quit {
			t.Fatalf("Play = %v, quit %v; want a quit", err, res.Quit)
		}
	})
}

// A derive over an input's bind updates the derived list on each
// keystroke in play.
func TestPlayDeriveOverInput(t *testing.T) {
	app := deriveApp(t, `<tui version="1"><screen id="s" focus="#q">
<input id="q" bind="query"/>
<list id="hits" each="hits as h" key="h"><item><text>{h}</text></item></list>
</screen></tui>`, map[string]any{"query": "", "words": []any{"apple", "apricot", "banana"}})
	if err := app.Derive("hits", func(get func(string) (any, bool)) (any, error) {
		q, _ := get("query")
		ws, _ := get("words")
		hits := []any{}
		for _, w := range ws.([]any) {
			if strings.HasPrefix(w.(string), q.(string)) {
				hits = append(hits, w)
			}
		}
		return hits, nil
	}); err != nil {
		t.Fatal(err)
	}
	res, err := Play(app, Options{Cols: 20, Rows: 4, Frames: true}, []string{"a", "p", "r", "backspace", "text:x"})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"apple", "apricot", "banana"},
		{"apple", "apricot"},
		{"apple", "apricot"},
		{"apricot"},
		{"apple", "apricot"},
		{},
	}
	if len(res.Frames) != len(want) {
		t.Fatalf("%d frames", len(res.Frames))
	}
	for i, f := range res.Frames {
		var got []string
		for _, line := range f.Dump.Grid[1:] {
			if s := strings.TrimSpace(line); s != "" {
				got = append(got, s)
			}
		}
		if strings.Join(got, ",") != strings.Join(want[i], ",") {
			t.Errorf("step %d (%s): list %v, want %v", f.Step, f.Input, got, want[i])
		}
	}
}
