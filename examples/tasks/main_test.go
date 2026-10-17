package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// laneIDs-like helper for tests: the ids a lane shows, in order.
func lane(t *testing.T, a *app, col string) []string {
	t.Helper()
	v, ok := a.ui.Get("cols." + col)
	if !ok {
		t.Fatalf("cols.%s is missing", col)
	}
	var out []string
	for _, r := range v.([]any) {
		out = append(out, r.(map[string]any)["id"].(string))
	}
	return out
}

func colOf(t *testing.T, a *app, id string) string {
	t.Helper()
	c, ok := find(a.cards(), id)
	if !ok {
		t.Fatalf("card %s is gone", id)
	}
	return c.Col
}

func str(t *testing.T, a *app, path string) string {
	t.Helper()
	v, _ := a.ui.Get(path)
	s, _ := v.(string)
	return s
}

func play(t *testing.T, a *app, steps ...string) *tuimark.PlayResult {
	t.Helper()
	res, err := a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 24}, steps...)
	if err != nil {
		t.Fatalf("play %v: %v", steps, err)
	}
	return res
}

func actions(res *tuimark.PlayResult) []string {
	var out []string
	for _, e := range res.Events {
		out = append(out, e.Action)
	}
	return out
}

func focus(res *tuimark.PlayResult) string {
	if res.Dump.Focus == nil {
		return ""
	}
	return *res.Dump.Focus
}

func equal(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }

// ---- the view ----

func TestValidateIsClean(t *testing.T) {
	a := newApp(t)
	for _, d := range a.ui.Validate() {
		t.Errorf("diagnostic: %s", d)
	}
}

func TestDumpShowsTheBoard(t *testing.T) {
	a := newApp(t)
	for _, size := range [][2]int{{80, 24}, {120, 30}} {
		d, err := a.ui.Dump(size[0], size[1])
		if err != nil {
			t.Fatal(err)
		}
		if !d.OK {
			t.Fatalf("%dx%d dump errors: %v", size[0], size[1], d.Errors)
		}
		grid := strings.Join(d.Grid, "\n")
		for _, want := range []string{"Todo · 5", "Doing · 3", "Done · 3", "Fix login redirect", "Set up CI pipeline", "11 cards", "H/L move", "q quit"} {
			if !strings.Contains(grid, want) {
				t.Errorf("%dx%d: missing %q in\n%s", size[0], size[1], want, grid)
			}
		}
	}
	// The priority word shows only where there is room for it.
	wide, _ := a.ui.Dump(120, 30)
	narrow, _ := a.ui.Dump(80, 24)
	if !strings.Contains(strings.Join(wide.Grid, "\n"), "bug high") || strings.Contains(strings.Join(narrow.Grid, "\n"), "bug high") {
		t.Error("the priority word should show at 120 columns and hide at 80")
	}
}

func TestHandlersCoverCatalog(t *testing.T) {
	a := newApp(t)
	hs := a.handlers()
	for _, spec := range a.ui.Catalog() {
		if _, ok := hs[spec.Name]; !ok && !spec.Builtin {
			t.Errorf("no handler for %s (%v)", spec.Name, spec.Sources)
		}
		if !spec.Builtin && !spec.Registered {
			t.Errorf("%s is not registered", spec.Name)
		}
	}
	if err := hs["quit"](tuimark.Event{}); !errors.Is(err, tuimark.ErrQuit) {
		t.Errorf("quit returned %v", err)
	}
}

// Every key of mock.json is an action the document fires, and no built-in
// (those cannot be mocked), so `tuimark run --mock` accepts it.
func TestMockKeysAreDocumentActions(t *testing.T) {
	raw, err := os.ReadFile("mock.json")
	if err != nil {
		t.Fatal(err)
	}
	var mock map[string][]struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(raw, &mock); err != nil {
		t.Fatal(err)
	}
	a := newApp(t)
	known := map[string]bool{}
	for _, spec := range a.ui.Catalog() {
		if !spec.Builtin {
			known[spec.Name] = true
		}
	}
	for name := range mock {
		if !known[name] {
			t.Errorf("mock.json mocks %q, which the document does not fire (or is a built-in)", name)
		}
	}
}

// ---- play flows ----

// L moves the card under the cursor right; the cursor and the focus follow
// it, and the lane titles count again.
func TestPlayMoveCardRight(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "L")
	if !equal(actions(res), []string{"col_focus", "card_right"}) {
		t.Fatalf("events: %+v", res.Events)
	}
	if e := res.Events[1]; e.Source != "todo" || e.Keys["c"] != "c1" {
		t.Errorf("card_right event: %+v", e)
	}
	if got := colOf(t, a, "c1"); got != "doing" {
		t.Fatalf("c1 is in %s, want doing", got)
	}
	if focus(res) != "doing" {
		t.Errorf("focus = %q, want doing", focus(res))
	}
	if cur, _ := a.ui.Get("cur.doing"); cur != "c1" {
		t.Errorf("cur.doing = %v, want c1", cur)
	}
	if got := str(t, a, "counts.todo") + "/" + str(t, a, "counts.doing"); got != "4/4" {
		t.Errorf("lane counts %s, want 4/4", got)
	}
	grid := strings.Join(res.Dump.Grid, "\n")
	if !strings.Contains(grid, "Todo · 4") || !strings.Contains(grid, "moved 1 card to Doing") {
		t.Errorf("grid:\n%s", grid)
	}
	// h goes back to the Todo lane; H moves the card there again.
	play(t, a, "h")
	if res = play(t, a, "l", "H"); focus(res) != "todo" || colOf(t, a, "c1") != "todo" {
		t.Errorf("H: focus=%q c1 in %s", focus(res), colOf(t, a, "c1"))
	}
	// The last lane has nowhere to go.
	play(t, a, "right", "right")
	before := a.cards()
	res = play(t, a, "L")
	if fmt.Sprint(before) != fmt.Sprint(a.cards()) || !strings.Contains(strings.Join(res.Dump.Grid, "\n"), "already in the last lane") {
		t.Errorf("L in the last lane changed something:\n%s", strings.Join(res.Dump.Grid, "\n"))
	}
}

// space marks cards with the built-in check-toggle; L then moves all of
// them, and the marks follow the cards to the lane they moved to.
func TestPlayMarkSeveralAndMove(t *testing.T) {
	a := newApp(t)
	play(t, a, "space", "j", "space")
	if got, _ := a.ui.Get("counts.marked"); got != float64(2) {
		t.Fatalf("marked = %v, want 2", got)
	}
	res := play(t, a, "L")
	for _, id := range []string{"c1", "c2"} {
		if colOf(t, a, id) != "doing" {
			t.Errorf("%s is in %s, want doing", id, colOf(t, a, id))
		}
	}
	if got := lane(t, a, "doing"); !equal(got, []string{"c6", "c7", "c8", "c1", "c2"}) {
		t.Errorf("doing = %v", got)
	}
	if v, _ := a.ui.Get("sel.doing"); fmt.Sprint(v) != "[c1 c2]" {
		t.Errorf("sel.doing = %v, want [c1 c2]", v)
	}
	if v, _ := a.ui.Get("sel.todo"); fmt.Sprint(v) != "[]" {
		t.Errorf("sel.todo = %v, want []", v)
	}
	if grid := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(grid, "moved 2 cards to Doing") || !strings.Contains(grid, "2 marked") {
		t.Errorf("grid:\n%s", grid)
	}
	// esc clears the marks (check-none, no handler).
	play(t, a, "esc")
	if got, _ := a.ui.Get("counts.marked"); got != float64(0) {
		t.Errorf("marked after esc = %v", got)
	}
}

// J and K reorder inside a lane and the cursor stays on the card.
func TestPlayReorder(t *testing.T) {
	a := newApp(t)
	play(t, a, "J")
	if got := lane(t, a, "todo"); !equal(got, []string{"c2", "c1", "c3", "c4", "c5"}) {
		t.Fatalf("after J: %v", got)
	}
	res := play(t, a, "K", "K") // back up, then already at the top: no change
	if got := lane(t, a, "todo"); !equal(got, []string{"c1", "c2", "c3", "c4", "c5"}) {
		t.Fatalf("after K K: %v", got)
	}
	if cur, _ := a.ui.Get("cur.todo"); cur != "c1" {
		t.Errorf("cur.todo = %v, want c1", cur)
	}
	if focus(res) != "todo" {
		t.Errorf("focus = %q", focus(res))
	}
}

// n opens the form, typing fills it, enter in an input saves, and the new
// card lands in the focused lane with the cursor on it.
func TestPlayCreateCardThroughTheModal(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "right", "n")
	if focus(res) != "f-title" || !strings.Contains(strings.Join(res.Dump.Grid, "\n"), "New card in Doing") {
		t.Fatalf("n did not open the form: focus=%q", focus(res))
	}
	res = play(t, a, "text:Ship the beta", "tab", "text:Feature", "tab", "text:2026-10-30", "ctrl+p", "ctrl+p", "enter")
	if got := actions(res); got[len(got)-1] != "card_save" {
		t.Fatalf("events: %v", got)
	}
	cards := a.cards()
	c := cards[len(cards)-1]
	want := card{ID: "c12", Title: "Ship the beta", Tag: "feature", Prio: 1, Due: "2026-10-30", Col: "doing"}
	if c != want {
		t.Errorf("new card = %+v, want %+v", c, want)
	}
	if open, _ := a.ui.Get("form.open"); open != false {
		t.Error("the form is still open")
	}
	if cur, _ := a.ui.Get("cur.doing"); cur != "c12" {
		t.Errorf("cur.doing = %v, want c12", cur)
	}
	grid := strings.Join(res.Dump.Grid, "\n")
	if !strings.Contains(grid, "Ship the beta") || !strings.Contains(grid, "added “Ship the beta” to Doing") || !strings.Contains(grid, "12 cards") {
		t.Errorf("grid:\n%s", grid)
	}
	if focus(res) != "doing" {
		t.Errorf("focus = %q, want doing", focus(res))
	}
}

// A form with problems stays open and says why; esc closes it untouched.
func TestPlayFormValidationAndCancel(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "n", "enter")
	if focus(res) != "f-title" || !strings.Contains(strings.Join(res.Dump.Grid, "\n"), "a card needs a title") {
		t.Fatalf("empty title: focus=%q\n%s", focus(res), strings.Join(res.Dump.Grid, "\n"))
	}
	res = play(t, a, "text:x", "tab", "tab", "text:soon", "enter")
	if !strings.Contains(strings.Join(res.Dump.Grid, "\n"), "due wants YYYY-MM-DD") || len(a.cards()) != 11 {
		t.Fatalf("bad due:\n%s", strings.Join(res.Dump.Grid, "\n"))
	}
	res = play(t, a, "esc")
	if focus(res) != "todo" || len(a.cards()) != 11 {
		t.Errorf("esc: focus=%q cards=%d", focus(res), len(a.cards()))
	}
	if !equal(actions(res), []string{"card_cancel"}) {
		t.Errorf("events: %v", actions(res))
	}
}

// e fills the form from the cursor card; saving updates it in place.
func TestPlayEditCard(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "e")
	grid := strings.Join(res.Dump.Grid, "\n")
	if focus(res) != "f-title" || !strings.Contains(grid, "Edit card") || !strings.Contains(grid, "Fix login redirect") || !strings.Contains(grid, "2026-10-20") {
		t.Fatalf("form:\n%s", grid)
	}
	play(t, a, "ctrl+a", "ctrl+u", "text:Fix login loop", "ctrl+s")
	c, _ := find(a.cards(), "c1")
	if c.Title != "Fix login loop" || c.Col != "todo" || c.Tag != "bug" || c.Due != "2026-10-20" || len(a.cards()) != 11 {
		t.Errorf("edited card = %+v", c)
	}
	if got := lane(t, a, "todo"); got[0] != "c1" {
		t.Errorf("todo = %v", got)
	}
}

// The filter input needs no handler for the narrowing: its text is in the
// store, and Derive recomputes the lanes. esc clears it and goes back.
func TestPlayFilter(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "/", "text:bug")
	if focus(res) != "filter" {
		t.Fatalf("focus = %q, want filter", focus(res))
	}
	for col, want := range map[string][]string{"todo": {"c1"}, "doing": {"c7"}, "done": {"c11"}} {
		if got := lane(t, a, col); !equal(got, want) {
			t.Errorf("%s = %v, want %v", col, got, want)
		}
	}
	grid := strings.Join(res.Dump.Grid, "\n")
	for _, want := range []string{"Todo · 1/5", "Doing · 1/3", "3 of 11 cards match “bug”"} {
		if !strings.Contains(grid, want) {
			t.Errorf("missing %q in\n%s", want, grid)
		}
	}
	for _, e := range res.Events {
		if e.Step > 0 { // step 0 is the first frame's on:focus
			t.Errorf("typing fired %s at step %d, want no host action", e.Action, e.Step)
		}
	}
	// q types a q into the filter: it does not quit.
	play(t, a, "text:q")
	if got := str(t, a, "filter"); got != "bugq" {
		t.Errorf("filter = %q", got)
	}
	res = play(t, a, "esc")
	if !equal(actions(res), []string{"filter_clear"}) || str(t, a, "filter") != "" || focus(res) != "todo" || len(lane(t, a, "todo")) != 5 {
		t.Errorf("esc: events=%v filter=%q focus=%q", actions(res), str(t, a, "filter"), focus(res))
	}
	// enter keeps the filter and hands the focus back to the last lane.
	res = play(t, a, "right", "/", "text:high", "enter")
	if focus(res) != "doing" || str(t, a, "filter") != "high" {
		t.Errorf("enter: focus=%q filter=%q", focus(res), str(t, a, "filter"))
	}
	// A reorder respects the filter: it swaps with the next visible card.
	play(t, a, "left", "J")
	if got := lane(t, a, "todo"); !equal(got, []string{"c4", "c1"}) {
		t.Errorf("todo under the filter = %v, want [c4 c1]", got)
	}
}

// d asks first; y deletes, n keeps. With marks, the question covers them.
func TestPlayDeleteWithConfirmation(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "d")
	if focus(res) != "confirm" || !strings.Contains(strings.Join(res.Dump.Grid, "\n"), "Delete “Fix login redirect”?") {
		t.Fatalf("d: focus=%q", focus(res))
	}
	res = play(t, a, "n")
	if len(a.cards()) != 11 || focus(res) != "todo" {
		t.Fatalf("n deleted or lost focus: %d cards, focus=%q", len(a.cards()), focus(res))
	}
	play(t, a, "space", "j", "space", "d")
	if !strings.Contains(str(t, a, "del.text"), "Delete 2 cards?") {
		t.Fatalf("del.text = %q", str(t, a, "del.text"))
	}
	res = play(t, a, "y")
	if len(a.cards()) != 9 || !equal(lane(t, a, "todo"), []string{"c3", "c4", "c5"}) {
		t.Errorf("after y: %d cards, todo=%v", len(a.cards()), lane(t, a, "todo"))
	}
	if v, _ := a.ui.Get("sel.todo"); fmt.Sprint(v) != "[]" {
		t.Errorf("sel.todo = %v, want the marks gone with their cards", v)
	}
	if !strings.Contains(strings.Join(res.Dump.Grid, "\n"), "deleted 2 cards") {
		t.Errorf("status:\n%s", strings.Join(res.Dump.Grid, "\n"))
	}
	// enter or space with the confirmation open fire nothing (its buttons
	// take clicks, not focus).
	play(t, a, "d")
	if res = play(t, a, "enter", "space"); len(res.Events) != 0 {
		t.Errorf("enter/space on the confirmation fired %v", actions(res))
	}
}

func TestPlayQuit(t *testing.T) {
	a := newApp(t)
	if res := play(t, a, "q", "n"); !res.Quit {
		t.Errorf("q did not quit: %+v", res)
	}
	b := newApp(t)
	if res := play(t, b, "ctrl+c"); !res.Quit {
		t.Errorf("ctrl+c did not quit: %+v", res)
	}
}

// ---- host logic ----

func TestDerivedLanes(t *testing.T) {
	a := newApp(t)
	for col, want := range map[string][]string{
		"todo":  {"c1", "c2", "c3", "c4", "c5"},
		"doing": {"c6", "c7", "c8"},
		"done":  {"c9", "c10", "c11"},
	} {
		if got := lane(t, a, col); !equal(got, want) {
			t.Errorf("%s = %v, want %v", col, got, want)
		}
	}
	// The rows carry the flags the markup's class guards read.
	v, _ := a.ui.Get("cols.todo")
	r1 := v.([]any)[0].(map[string]any)
	r4 := v.([]any)[3].(map[string]any)
	if r1["is_bug"] != true || r1["p_high"] != true || r1["soon"] != true || r1["due"] != "Oct 20" {
		t.Errorf("c1 row = %v", r1)
	}
	if r4["overdue"] != true || r4["is_feature"] != true {
		t.Errorf("c4 row = %v", r4)
	}
	// A done card is never overdue, and the Done lane counts like the rest.
	if v, _ := a.ui.Get("cols.done"); v.([]any)[0].(map[string]any)["overdue"] != false {
		t.Error("a done card is overdue")
	}
	if str(t, a, "summary") != "11 cards" || str(t, a, "counts.done") != "3" {
		t.Errorf("summary=%q done=%q", str(t, a, "summary"), str(t, a, "counts.done"))
	}
	// A derived path is read-only.
	if err := a.ui.Set("cols.todo", []any{}); err == nil {
		t.Error("Set on a derived path succeeded")
	}
	// Setting the filter is all it takes.
	if err := a.ui.Set("filter", "CHORE"); err != nil {
		t.Fatal(err)
	}
	if got := lane(t, a, "todo"); !equal(got, []string{"c2", "c5"}) {
		t.Errorf("todo under CHORE = %v", got)
	}
	if str(t, a, "summary") != "4 of 11 cards match “CHORE”" {
		t.Errorf("summary = %q", str(t, a, "summary"))
	}
}

func TestMoveCardsAndReorderPure(t *testing.T) {
	cards := []card{{ID: "a", Col: "todo", Title: "A"}, {ID: "b", Col: "doing", Title: "B"}, {ID: "c", Col: "todo", Title: "C"}, {ID: "d", Col: "done", Title: "D"}}
	got, n := moveCards(cards, map[string]bool{"a": true, "c": true, "d": true}, 1)
	if n != 2 {
		t.Errorf("moved %d, want 2 (d is in the last lane)", n)
	}
	var order []string
	for _, c := range got {
		order = append(order, c.ID+"@"+c.Col)
	}
	if strings.Join(order, " ") != "b@doing d@done a@doing c@doing" {
		t.Errorf("order = %v", order)
	}
	if cards[0].Col != "todo" {
		t.Error("moveCards changed its input")
	}
	if _, n := moveCards(cards, map[string]bool{"a": true}, -1); n != 0 {
		t.Error("moved a card out of the first lane")
	}
	// reorder swaps with the neighbour in the same lane, skipping other lanes.
	r, ok := reorder(cards, "a", 1, "")
	if !ok || r[0].ID != "c" || r[2].ID != "a" {
		t.Errorf("reorder down = %v %v", ok, r)
	}
	if _, ok := reorder(cards, "a", -1, ""); ok {
		t.Error("reordered the first card up")
	}
	if _, ok := reorder(cards, "zzz", 1, ""); ok {
		t.Error("reordered a missing card")
	}
	// ids continue after the highest.
	if nextID([]card{{ID: "c3"}, {ID: "c10"}, {ID: "x"}}) != 11 {
		t.Error("nextID")
	}
}

func TestFileRoundTrip(t *testing.T) {
	path := t.TempDir() + "/board.json"
	a := newApp(t)
	if err := a.useFile(path); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(str(t, a, "status"), "new board") {
		t.Errorf("status = %q", str(t, a, "status"))
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the file exists before any change: %v", err)
	}
	// Move c1, add a card, delete c5: each change saves.
	play(t, a, "L", "h", "n", "text:Persisted", "enter")
	saved, err := readBoard(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 12 || saved[0].ID != "c2" {
		t.Fatalf("saved %d cards: %+v", len(saved), saved)
	}
	if c, _ := find(saved, "c1"); c.Col != "doing" {
		t.Errorf("c1 saved in %s", c.Col)
	}
	if c, _ := find(saved, "c12"); c.Title != "Persisted" || c.Col != "todo" || c.Prio != 2 {
		t.Errorf("c12 saved as %+v", c)
	}
	play(t, a, "d", "y")
	if saved, _ = readBoard(path); len(saved) != 11 {
		t.Errorf("after delete: %d cards saved", len(saved))
	}

	// A second run picks the board up, ids included.
	b := newApp(t)
	if err := b.useFile(path); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(b.cards()) != fmt.Sprint(a.cards()) {
		t.Errorf("reloaded board differs:\n%v\n%v", b.cards(), a.cards())
	}
	if b.next != 12 || str(t, b, "file") != path || !strings.Contains(str(t, b, "status"), "loaded 11 cards") {
		t.Errorf("next=%d file=%q status=%q", b.next, str(t, b, "file"), str(t, b, "status"))
	}
	if got := lane(t, b, "doing"); !equal(got, []string{"c6", "c7", "c8", "c1"}) {
		t.Errorf("doing = %v", got)
	}
}

func TestBadBoardFiles(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"junk":      `not json`,
		"column":    `{"cards":[{"id":"c1","title":"x","col":"later"}]}`,
		"duplicate": `{"cards":[{"id":"c1","title":"x","col":"todo"},{"id":"c1","title":"y","col":"todo"}]}`,
		"title":     `{"cards":[{"id":"c1","title":" ","col":"todo"}]}`,
		"due":       `{"cards":[{"id":"c1","title":"x","col":"todo","due":"tomorrow"}]}`,
	} {
		path := dir + "/" + name + ".json"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		a := newApp(t)
		if err := a.useFile(path); err == nil || a.file != "" {
			t.Errorf("%s: err=%v file=%q, want a refusal", name, err, a.file)
		}
	}
}

// A save that fails is an error from the handler; RunWith's OnError (here
// called directly) shows it in the status line and the board on screen
// has changed anyway.
func TestSaveFailureReachesTheStatusLine(t *testing.T) {
	a := newApp(t)
	a.file = t.TempDir() + "/missing-dir/board.json"
	_, err := a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 24}, "L")
	if err == nil || !strings.Contains(err.Error(), "save") {
		t.Fatalf("Play error = %v, want a save error", err)
	}
	if colOf(t, a, "c1") != "doing" {
		t.Error("the move was not applied")
	}
	if err := a.onError(nil, err); err != nil {
		t.Fatal(err)
	}
	d, _ := a.ui.Dump(100, 24)
	if !strings.Contains(strings.Join(d.Grid, "\n"), "error: save") {
		t.Errorf("status line:\n%s", strings.Join(d.Grid, "\n"))
	}
	if bad, _ := a.ui.Get("status_err"); bad != true {
		t.Error("status_err is not set")
	}
}

// tick is what Every calls: it follows the clock for -today now, and clears
// the status after statusTTL ticks. Dump and Play never tick, so the test
// calls it.
func TestTick(t *testing.T) {
	a := newApp(t)
	tick := func(now time.Time) {
		t.Helper()
		if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, now) }); err != nil {
			t.Fatal(err)
		}
	}
	day := func(s string) time.Time { d, _ := time.Parse(dateLayout, s); return d }
	row := func(id string) map[string]any {
		v, _ := a.ui.Get("cols.todo")
		for _, r := range v.([]any) {
			if m := r.(map[string]any); m["id"] == id {
				return m
			}
		}
		return nil
	}

	// Not following the clock: the sample's date stays.
	tick(day("2026-12-01"))
	if str(t, a, "today") != "2026-10-19" {
		t.Fatalf("today moved to %s", str(t, a, "today"))
	}
	a.live = true // what setToday("now") sets, without the machine's date
	tick(day("2026-10-19"))
	if err := a.setToday("now"); err != nil {
		t.Fatal(err)
	}
	tick(day("2026-10-19"))
	if row("c1")["soon"] != true {
		t.Fatal("c1 should be due soon on the 19th")
	}
	tick(day("2026-10-25"))
	if str(t, a, "today") != "2026-10-25" || row("c1")["overdue"] != true || row("c1")["soon"] != false {
		t.Errorf("after the date moved: today=%s c1=%v", str(t, a, "today"), row("c1"))
	}

	// The status clears after its TTL.
	if err := a.say(a.ui, "hello", false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < statusTTL-1; i++ {
		tick(day("2026-10-25"))
		if str(t, a, "status") != "hello" {
			t.Fatalf("status cleared after %d ticks", i+1)
		}
	}
	tick(day("2026-10-25"))
	if str(t, a, "status") != idle {
		t.Errorf("status = %q after %d ticks, want %q", str(t, a, "status"), statusTTL, idle)
	}
	if err := a.setToday("tomorrow"); err == nil {
		t.Error("setToday accepted tomorrow")
	}
}

// enter edits too, from any lane.
func TestPlayEnterEditsTheCursorCard(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "right", "down", "enter")
	if focus(res) != "f-title" || str(t, a, "form.title") != "Empty import crash" {
		t.Errorf("focus=%q title=%q", focus(res), str(t, a, "form.title"))
	}
}
