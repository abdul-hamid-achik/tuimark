package main

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The store has exactly the paths the README documents, raw and derived.
func TestStoreShape(t *testing.T) {
	raw := []string{"auto", "best", "combo", "cur", "field", "flash", "ghost", "gravity_ms", "height_hist", "help_open", "hold", "hold_used", "last", "level", "lines", "notice", "over", "over_modal", "paused", "pieces", "queue", "score"}
	derived := []string{"board", "hold_rows", "mode_label", "next_rows", "speed_label", "state"}
	want := append(append([]string(nil), raw...), derived...)
	sort.Strings(want)
	r := newRig(t)
	got := sortedKeys(store(t, r.a))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("store paths\n got %v\nwant %v", got, want)
	}
	// sample.json has the same paths.
	b, err := os.ReadFile("sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var sample map[string]any
	if err := json.Unmarshal(b, &sample); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sortedKeys(sample), want) {
		t.Errorf("sample.json paths %v, want %v", sortedKeys(sample), want)
	}
	// The shapes: a field of 22 rows of 10, a board of 20 rows of 20
	// columns, hold of 2 rows, next of 14.
	s := store(t, r.a)
	field := s["field"].([]any)
	if len(field) != boardH {
		t.Fatalf("field has %d rows", len(field))
	}
	for i, row := range field {
		if n := len(row.(string)); n != boardW {
			t.Errorf("field row %d is %d characters", i, n)
		}
	}
	for path, rows := range map[string]int{"board": visibleH, "hold_rows": 2, "next_rows": queueLen*3 - 1} {
		if got := len(s[path].([]any)); got != rows {
			t.Errorf("%s has %d rows, want %d", path, got, rows)
		}
	}
}

// With the help or the game over modal open, space is the focused button's,
// not the hard drop's.
func TestSpaceBelongsToTheModalButton(t *testing.T) {
	r := newRig(t)
	r.play("?")
	res := r.play("space")
	if len(res.Events) != 1 || res.Events[0].Action != "help_close" || r.a.g.pieces != 0 {
		t.Errorf("space in the help: events %+v pieces %d", res.Events, r.a.g.pieces)
	}
	r = newRig(t)
	r.gameOver()
	res = r.play("space")
	if len(res.Events) != 1 || res.Events[0].Action != "over_hide" || !r.a.g.over {
		t.Errorf("space in the game over modal: events %+v over %v", res.Events, r.a.g.over)
	}
}

// Hold, next, and the flash are all published as the document's runs: a
// run's text is as wide as its cells, so a row is always the panel's width.
func TestRowsHaveTheirWidth(t *testing.T) {
	r := newRig(t)
	r.step(1)
	r.play("a", "c")
	for i := 0; i < 200; i++ {
		r.step(1)
		for path, width := range map[string]int{"board": boardW * 2, "hold_rows": previewWidth, "next_rows": previewWidth} {
			for y, row := range r.get(path).([]any) {
				w := 0
				for _, run := range row.(map[string]any)["runs"].([]any) {
					w += len([]rune(run.(map[string]any)["t"].(string)))
				}
				if w != width {
					t.Fatalf("tick %d: %s row %d is %d columns, want %d", i, path, y, w, width)
				}
			}
		}
	}
	if !strings.Contains(r.grid(), "┏") {
		t.Error("no wall")
	}
}
