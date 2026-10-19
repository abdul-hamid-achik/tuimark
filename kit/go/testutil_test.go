package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// loadComponent loads a kit component document from ../components and binds
// the JSON object data at the root ("" binds nothing).
func loadComponent(t *testing.T, name string, data string) *tuimark.App {
	t.Helper()
	app, err := tuimark.Load(filepath.Join("..", "components", name+".tui"))
	if err != nil {
		t.Fatal(err)
	}
	if data != "" {
		var v map[string]any
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			t.Fatal(err)
		}
		if err := app.Bind("", v); err != nil {
			t.Fatal(err)
		}
	}
	return app
}

// loadSample is loadComponent with the component's own NAME.sample.json.
func loadSample(t *testing.T, name string) *tuimark.App {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "components", name+".sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	return loadComponent(t, name, string(raw))
}

// bareApp is a document that reads nothing, for helpers that need no markup.
func bareApp(t *testing.T) *tuimark.App {
	t.Helper()
	app, err := tuimark.Parse(strings.NewReader(`<tui><screen id="s"><text>x</text></screen></tui>`))
	if err != nil {
		t.Fatal(err)
	}
	return app
}

// screen is the dump's grid of app at 100x30, one string.
func screen(t *testing.T, app *tuimark.App) string {
	t.Helper()
	d, err := app.Dump(100, 30)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(d.Grid, "\n")
}

func play(t *testing.T, app *tuimark.App, steps ...string) (*tuimark.PlayResult, string) {
	t.Helper()
	res, err := app.Play(tuimark.PlayOptions{Cols: 100, Rows: 30}, steps...)
	if err != nil {
		t.Fatalf("play %v: %v", steps, err)
	}
	return res, strings.Join(res.Dump.Grid, "\n")
}

func mustGet(t *testing.T, app *tuimark.App, path string) any {
	t.Helper()
	v, ok := app.Get(path)
	if !ok {
		t.Fatalf("%s is missing from the store", path)
	}
	return v
}

func getStr(t *testing.T, app *tuimark.App, path string) string {
	t.Helper()
	s, _ := mustGet(t, app, path).(string)
	return s
}

func getBool(t *testing.T, app *tuimark.App, path string) bool {
	t.Helper()
	b, _ := mustGet(t, app, path).(bool)
	return b
}

func contains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Errorf("%q is not on the screen:\n%s", want, text)
	}
}

func notContains(t *testing.T, text, bad string) {
	t.Helper()
	if strings.Contains(text, bad) {
		t.Errorf("%q should not be on the screen:\n%s", bad, text)
	}
}

// clock is a time a test moves by hand.
type clock struct{ now time.Time }

func newClock() *clock { return &clock{time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time          { return c.now }
func (c *clock) advance(d time.Duration) { c.now = c.now.Add(d) }
