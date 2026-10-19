package main

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// playAt replays steps against the app at cols x rows and fails on a dump
// with errors.
func playAt(t testing.TB, a *app, cols, rows int, steps ...string) *tuimark.PlayResult {
	t.Helper()
	res, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows, Styles: true}, steps...)
	if err != nil {
		t.Fatalf("play %v: %v", steps, err)
	}
	if !res.Dump.OK || len(res.Dump.Errors) != 0 {
		t.Fatalf("play %v: dump errors %v", steps, res.Dump.Errors)
	}
	return res
}

func play(t testing.TB, a *app, steps ...string) *tuimark.PlayResult {
	t.Helper()
	return playAt(t, a, 100, 24, steps...)
}

func actions(res *tuimark.PlayResult) []string {
	var out []string
	for _, e := range res.Events {
		out = append(out, e.Action)
	}
	return out
}

func wantActions(t testing.TB, res *tuimark.PlayResult, want ...string) {
	t.Helper()
	if got := strings.Join(actions(res), ","); got != strings.Join(want, ",") {
		t.Fatalf("actions = %q, want %q", got, strings.Join(want, ","))
	}
}

func gridText(res *tuimark.PlayResult) string { return strings.Join(res.Dump.Grid, "\n") }

// get reads a store path.
func get(t testing.TB, a *app, path string) any {
	t.Helper()
	v, ok := a.ui.Get(path)
	if !ok {
		t.Fatalf("%s is not in the store", path)
	}
	return v
}

// tickAt runs the Every function at a fake time, as the loop would.
func tickAt(t testing.TB, a *app, now time.Time) {
	t.Helper()
	if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, now) }); err != nil {
		t.Fatal(err)
	}
}

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// tickFor runs ticks of tickEvery from the app's last step for d.
func tickFor(t testing.TB, a *app, from time.Time, d time.Duration) time.Time {
	t.Helper()
	now := from
	for elapsed := time.Duration(0); elapsed < d; elapsed += tickEvery {
		now = now.Add(tickEvery)
		tickAt(t, a, now)
	}
	return now
}

// node is the dump node with this id.
func node(t testing.TB, res *tuimark.PlayResult, id string) tuimark.DumpNode {
	t.Helper()
	for _, n := range res.Dump.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("no node %q", id)
	return tuimark.DumpNode{}
}

// hintUntilWon presses H until the board is solved; every press fills one
// cell, so 81 presses are more than enough, and a dead key fails instead of
// hanging.
func hintUntilWon(t testing.TB, a *app) {
	t.Helper()
	for i := 0; i < size && !a.g.won; i++ {
		play(t, a, "H")
	}
	if !a.g.won {
		t.Fatal("pressing H did not solve the board")
	}
}

// park moves the cursor and publishes it.
func park(t testing.TB, a *app, i int) {
	t.Helper()
	a.g.cur = i
	if err := a.sync(); err != nil {
		t.Fatal(err)
	}
}

func hasClass(n tuimark.DumpNode, class string) bool {
	for _, c := range n.Classes {
		if c == class {
			return true
		}
	}
	return false
}

// click is the play step that clicks the middle of a node.
func click(t testing.TB, a *app, cols, rows int, id string) string {
	t.Helper()
	return cellStep(t, a, cols, rows, id)
}

// look is the style of one cell of the dump.
type look struct {
	FG, BG string
	A      []string
}

// span is the style run covering column x of row y.
func span(t testing.TB, res *tuimark.PlayResult, x, y int) look {
	t.Helper()
	for _, s := range res.Dump.Styles[y] {
		if x >= s.X && x < s.X+s.W {
			return look{FG: s.FG, BG: s.BG, A: s.A}
		}
	}
	t.Fatalf("no style at %d,%d", x, y)
	return look{}
}

// emptyCell is the id of the first empty cell of the puzzle at or after i.
func emptyAfter(a *app, i int) int {
	for ; i < size; i++ {
		if a.g.puz[i] == 0 {
			return i
		}
	}
	return -1
}

func TestValidateIsClean(t *testing.T) {
	a := newApp(t)
	for _, theme := range []string{"dark", "light"} {
		if err := a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		if diags := a.ui.Validate(); len(diags) != 0 {
			t.Errorf("%s: Validate gave %v", theme, diags)
		}
	}
}

// The keys the document binds, and what each fires. Every row of the
// keymap is here: a test below fails for a row missing from this table, and
// this one fails for a row missing from the document.
var keyTable = []struct{ key, action string }{
	{"?", "help_open"}, {"q", "quit"}, {"ctrl+c", "quit"},
	{"up", "up"}, {"k", "up"}, {"down", "down"}, {"j", "down"},
	{"left", "left"}, {"h", "left"}, {"right", "right"}, {"l", "right"},
	{"1", "put1"}, {"2", "put2"}, {"3", "put3"}, {"4", "put4"}, {"5", "put5"},
	{"6", "put6"}, {"7", "put7"}, {"8", "put8"}, {"9", "put9"},
	{"0", "erase"}, {"backspace", "erase"}, {"x", "erase"},
	{"n", "notes"}, {"u", "undo"}, {"U", "redo"},
	// h moves left (with hjkl), so the hint is on shift+h.
	{"H", "hint"},
	{"c", "check"}, {"p", "pause"}, {"N", "newgame_open"}, {"m", "mouse"},
}

func TestEveryKeyFiresItsAction(t *testing.T) {
	for _, k := range keyTable {
		a := newApp(t)
		res, err := a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 24}, k.key)
		if err != nil {
			t.Fatalf("%s: %v", k.key, err)
		}
		if len(res.Events) == 0 || res.Events[0].Action != k.action {
			t.Errorf("key %q fires %v, want %s", k.key, actions(&tuimark.PlayResult{Events: res.Events}), k.action)
		}
	}
}

// The table lists every key of the document's keymaps, and the keymap
// opens with help and quit, so the hints bar keeps them when it runs out of
// room.
func TestKeyTableMatchesTheDocument(t *testing.T) {
	raw, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`<bind keys="([^"]+)" action="([^"]+)"`)
	rows := re.FindAllStringSubmatch(string(raw), -1)
	inDoc := map[string]string{}
	for _, r := range rows {
		if r[2] == "help_close" {
			continue // enter, q, ? close the help: tested with the help
		}
		for _, k := range strings.Split(r[1], ",") {
			inDoc[k] = r[2]
		}
	}
	inTable := map[string]string{}
	for _, k := range keyTable {
		inTable[k.key] = k.action
	}
	for k, act := range inDoc {
		if inTable[k] != act {
			t.Errorf("document binds %q to %s, the table says %q", k, act, inTable[k])
		}
	}
	for k, act := range inTable {
		if inDoc[k] != act {
			t.Errorf("table has %q -> %s, the document binds %q", k, act, inDoc[k])
		}
	}
	// The rows the hints bar shows come in document order; the help's own
	// keymap has no labels, so skip it.
	var shown []string
	for _, r := range rows {
		if r[2] != "help_close" {
			shown = append(shown, r[2])
		}
	}
	if len(shown) < 3 || shown[0] != "help_open" || shown[1] != "quit" {
		t.Errorf("the first keymap rows are %v, want help_open then quit", shown[:2])
	}
	// help_open, quit, and ctrl+c sit in a keymap with no `when`, so they
	// work wherever the focus is, and when nothing is focused (a screen too
	// small for the board hides everything that could take the focus).
	global := regexp.MustCompile(`(?s)<keymap>\s*<bind keys="\?" action="help_open"[^\n]*\n\s*<bind keys="q" action="quit"[^\n]*\n\s*<bind keys="ctrl\+c" action="quit"/>\s*</keymap>`)
	if !global.Match(raw) {
		t.Error("? and q and ctrl+c are not together in a keymap without `when`")
	}
}

// Below the smallest size the board is hidden and nothing takes the focus,
// yet q quits, ctrl+c quits, ? opens the help, and the hints bar says so.
func TestQuitAndHelpWorkOnATooSmallScreen(t *testing.T) {
	for _, sz := range [][2]int{{30, 10}, {39, 24}, {80, 13}, {20, 6}} {
		for _, key := range []string{"q", "ctrl+c"} {
			a := newApp(t)
			res, err := a.ui.Play(tuimark.PlayOptions{Cols: sz[0], Rows: sz[1]}, key)
			if err != nil {
				t.Fatal(err)
			}
			if !res.Quit {
				t.Errorf("%dx%d: %s did not quit", sz[0], sz[1], key)
			}
		}
		a := newApp(t)
		res, err := a.ui.Play(tuimark.PlayOptions{Cols: sz[0], Rows: sz[1]}, "?")
		if err != nil || len(res.Events) != 1 || res.Events[0].Action != "help_open" {
			t.Errorf("%dx%d: ? gave %v, %v", sz[0], sz[1], res.Events, err)
		}
		a = newApp(t)
		d, err := a.ui.Dump(sz[0], sz[1])
		if err != nil {
			t.Fatal(err)
		}
		if sz[0] >= 30 && !strings.Contains(d.Grid[sz[1]-1], "? help") {
			t.Errorf("%dx%d: the hints bar is %q", sz[0], sz[1], d.Grid[sz[1]-1])
		}
	}
}

func TestHintsKeepHelpAndQuitWhenShort(t *testing.T) {
	a := newApp(t)
	for _, sz := range [][2]int{{40, 14}, {50, 20}, {80, 24}} {
		res := playAt(t, a, sz[0], sz[1])
		bottom := res.Dump.Grid[sz[1]-1]
		if !strings.Contains(bottom, "? help") || !strings.Contains(bottom, "q quit") {
			t.Errorf("%dx%d: hints %q lack help or quit", sz[0], sz[1], strings.TrimSpace(bottom))
		}
	}
	res := playAt(t, a, 80, 24)
	if bottom := res.Dump.Grid[23]; !strings.Contains(bottom, "H hint") || !strings.Contains(bottom, "↑↓←→ move") {
		t.Errorf("80x24 hints %q", strings.TrimSpace(bottom))
	}
}

// h is left, as in vim; the hint is H.
func TestHMovesAndShiftHHints(t *testing.T) {
	a := newApp(t)
	a.g.cur = 40
	res := play(t, a, "h")
	wantActions(t, res, "left")
	if got := get(t, a, "cur"); got != float64(39) && got != 39 {
		t.Errorf("cur = %v after h, want 39", got)
	}
	res = play(t, a, "H")
	wantActions(t, res, "hint")
	if a.g.hints != 1 {
		t.Errorf("hints = %d", a.g.hints)
	}
}

func TestDigitKeysPlaceAndCountMistakes(t *testing.T) {
	a := newApp(t)
	i := a.g.cur
	right := a.g.sol[i]
	res := play(t, a, fmt.Sprint(right))
	wantActions(t, res, fmt.Sprintf("put%d", right))
	if a.g.vals[i] != right || a.g.mistakes != 0 {
		t.Fatalf("vals %d mistakes %d", a.g.vals[i], a.g.mistakes)
	}
	if get(t, a, "mistakes") != 0 && get(t, a, "mistakes") != float64(0) {
		t.Error("store mistakes != 0")
	}
	j := emptyAfter(a, i+1)
	a.g.cur = j
	play(t, a, wrongDigit(a))
	if a.g.mistakes != 1 {
		t.Errorf("mistakes = %d, want 1", a.g.mistakes)
	}
	res = play(t, a)
	if !strings.Contains(gridText(res), "mistakes               1") {
		t.Errorf("the panel does not show 1 mistake:\n%s", gridText(res))
	}
	n := node(t, res, cellID(j))
	if !strings.Contains(n.Text, fmt.Sprint(a.g.vals[j])) {
		t.Errorf("cell text %q does not show the digit", n.Text)
	}
}

func TestEraseKeys(t *testing.T) {
	for _, key := range []string{"0", "backspace", "x"} {
		a := newApp(t)
		i := a.g.cur
		play(t, a, wrongDigit(a))
		if a.g.vals[i] == 0 {
			t.Fatal("setup")
		}
		wantActions(t, play(t, a, key), "erase")
		if a.g.vals[i] != 0 {
			t.Errorf("%s left the digit", key)
		}
	}
}

func TestArrowsAndHjklMove(t *testing.T) {
	for _, tc := range []struct {
		keys []string
		dr   int
		dc   int
	}{{[]string{"up", "k"}, -1, 0}, {[]string{"down", "j"}, 1, 0}, {[]string{"left", "h"}, 0, -1}, {[]string{"right", "l"}, 0, 1}} {
		for _, key := range tc.keys {
			a := newApp(t)
			a.g.cur = 4*9 + 4
			play(t, a, key)
			if want := (4+tc.dr)*9 + 4 + tc.dc; a.g.cur != want {
				t.Errorf("%s: cursor %d, want %d", key, a.g.cur, want)
			}
		}
	}
	// And the store follows.
	a := newApp(t)
	a.g.cur = 40
	play(t, a, "right")
	if v := get(t, a, "sel.label"); v != "cell r5 c6" {
		t.Errorf("sel.label = %v", v)
	}
}

// A click on a cell fires cell with that cell's id as the source and
// moves the cursor there; the cells do not take the focus.
func TestClickMovesTheCursor(t *testing.T) {
	a := newApp(t)
	step := click(t, a, 100, 24, "c73")
	res := play(t, a, step)
	wantActions(t, res, "cell")
	if res.Events[0].Source != "c73" {
		t.Errorf("source = %q, want c73", res.Events[0].Source)
	}
	if a.g.cur != 7*9+3 {
		t.Errorf("cursor = %d, want %d", a.g.cur, 7*9+3)
	}
	if f := res.Dump.Focus; f == nil || *f != "board" {
		t.Errorf("focus = %v, want the board", f)
	}
	if !hasClass(node(t, res, "c73"), "cur") || hasClass(node(t, res, "c00"), "cur") {
		t.Error("the cursor class did not follow the click")
	}
	// Keys still act on the board after a click.
	wantActions(t, play(t, a, "up"), "up")
}

// Every one of the 81 cells is a one-row, three-column target that a
// click reaches.
func TestEveryCellIsClickable(t *testing.T) {
	a := newApp(t)
	d, err := a.ui.Dump(100, 24)
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string]tuimark.DumpNode{}
	for _, n := range d.Nodes {
		pos[n.ID] = n
	}
	for i := 0; i < size; i++ {
		n, ok := pos[cellID(i)]
		if !ok || n.W != 3 || n.H != 1 {
			t.Fatalf("cell %s: %+v", cellID(i), n)
		}
	}
	for i := 0; i < size; i += 7 {
		a := newApp(t)
		res := play(t, a, click(t, a, 100, 24, cellID(i)))
		if a.g.cur != i || len(res.Events) != 1 || res.Events[0].Source != cellID(i) {
			t.Errorf("click on %s: cursor %d events %v", cellID(i), a.g.cur, res.Events)
		}
	}
}

func TestNumberPadClicks(t *testing.T) {
	a := newApp(t)
	i := a.g.cur
	d := a.g.sol[i]
	res := play(t, a, click(t, a, 100, 24, fmt.Sprintf("p%d", d)))
	wantActions(t, res, fmt.Sprintf("put%d", d))
	if a.g.vals[i] != d {
		t.Errorf("the pad put %d, the cell holds %d", d, a.g.vals[i])
	}
	// All nine pad cells exist and fire their own digit.
	for k := 1; k <= 9; k++ {
		a := newApp(t)
		res := play(t, a, click(t, a, 100, 24, fmt.Sprintf("p%d", k)))
		wantActions(t, res, fmt.Sprintf("put%d", k))
	}
}

func TestToolClicks(t *testing.T) {
	for id, action := range map[string]string{"t-erase": "erase", "t-notes": "notes", "t-undo": "undo",
		"t-redo": "redo", "t-hint": "hint", "t-check": "check"} {
		a := newApp(t)
		wantActions(t, play(t, a, click(t, a, 100, 24, id)), action)
	}
	a := newApp(t)
	play(t, a, click(t, a, 100, 24, "t-notes"))
	if !a.g.pencil {
		t.Error("the notes tool did not switch notes mode on")
	}
	res := play(t, a)
	if !hasClass(node(t, res, "t-notes"), "on") {
		t.Error("the notes tool does not look on")
	}
	if !hasClass(node(t, res, "t-undo"), "off") || !hasClass(node(t, res, "t-redo"), "off") {
		t.Error("undo and redo do not look unavailable on a new game")
	}
	play(t, a, "n", "5")
	play(t, a, "n", "5")
	res = play(t, a)
	if hasClass(node(t, res, "t-undo"), "off") {
		t.Error("undo still looks unavailable after a move")
	}
}

func TestNotesModeAndTheMiniGrid(t *testing.T) {
	a := newApp(t)
	i := a.g.cur
	res := play(t, a, "n", "3", "7")
	wantActions(t, res, "notes", "put3", "put7")
	if a.g.notes[i] != set(3, 7) || a.g.vals[i] != 0 || a.g.mistakes != 0 {
		t.Fatalf("notes %b vals %d mistakes %d", a.g.notes[i], a.g.vals[i], a.g.mistakes)
	}
	// The cell carries the compact marker, the panel the 3x3 grid.
	if n := node(t, res, cellID(i)); !strings.Contains(n.Text, "·") || !hasClass(n, "note") {
		t.Errorf("cell %q classes %v: no note marker", n.Text, n.Classes)
	}
	g := gridText(res)
	for _, want := range []string{"3", "7", "notes"} {
		if !strings.Contains(g, want) {
			t.Errorf("screen lacks %q", want)
		}
	}
	if !hasClass(node(t, res, cellID(i)), "note") {
		t.Error("cell not marked as holding notes")
	}
	mini := func(res *tuimark.PlayResult) []string {
		var out []string
		for _, n := range res.Dump.Nodes {
			if n.Tag == "text" && hasClass(n, "m") {
				out = append(out, n.Text)
			}
		}
		return out
	}
	if got := strings.Join(mini(res), ""); got != "··3···7··" {
		t.Errorf("mini grid = %q, want ··3···7··", got)
	}
	on := 0
	for _, n := range res.Dump.Nodes {
		if hasClass(n, "m") && hasClass(n, "on") {
			on++
		}
	}
	if on != 2 {
		t.Errorf("%d mini cells lit, want 2", on)
	}
	// The mode shows, and n again leaves it.
	if !strings.Contains(g, "notes") {
		t.Error("the entering mode does not say notes")
	}
	play(t, a, "n")
	if a.g.pencil {
		t.Error("n did not switch back")
	}
}

func TestPlacingAClearsPeerNotesThroughTheKeys(t *testing.T) {
	a := newApp(t)
	i := a.g.cur
	peer := -1
	for _, p := range peers[i] {
		if a.g.puz[p] == 0 {
			peer = p
			break
		}
	}
	d := int(a.g.sol[i])
	a.g.cur = peer
	play(t, a, "n", fmt.Sprint(d), "n")
	if !has(a.g.notes[peer], d) {
		t.Fatal("setup: the peer has no note")
	}
	a.g.cur = i
	play(t, a, fmt.Sprint(d))
	if has(a.g.notes[peer], d) {
		t.Error("the peer kept its note")
	}
	// u brings it back with the digit's removal.
	play(t, a, "u")
	if !has(a.g.notes[peer], d) || a.g.vals[i] != 0 {
		t.Error("undo did not restore the peer's note and clear the cell")
	}
}

func TestUndoRedoKeys(t *testing.T) {
	a := newApp(t)
	i := a.g.cur
	play(t, a, fmt.Sprint(a.g.sol[i]))
	wantActions(t, play(t, a, "u"), "undo")
	if a.g.vals[i] != 0 {
		t.Error("u left the digit")
	}
	wantActions(t, play(t, a, "U"), "redo")
	if a.g.vals[i] != a.g.sol[i] {
		t.Error("U did not put the digit back")
	}
}

func TestHintShowsItsMessageAndCounts(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "H")
	wantActions(t, res, "hint")
	if a.g.hints != 1 || a.g.filled() != a.g.puz.clues()+1 {
		t.Errorf("hints %d filled %d", a.g.hints, a.g.filled())
	}
	if !strings.Contains(res.Dump.Grid[0], "hint: r") {
		t.Errorf("the top line does not say what the hint did:\n%s", res.Dump.Grid[0])
	}
	if !strings.Contains(gridText(res), "hints                  1") {
		t.Errorf("the panel does not count the hint:\n%s", gridText(res))
	}
	// The message goes at the next action.
	res = play(t, a, "right")
	if strings.Contains(res.Dump.Grid[0], "hint:") {
		t.Error("the hint message outlived the next action")
	}
}

func TestCheckMarksWrongEntries(t *testing.T) {
	a := newApp(t)
	i := a.g.cur
	play(t, a, wrongDigit(a))
	res := play(t, a, "c")
	wantActions(t, res, "check")
	if !strings.Contains(res.Dump.Grid[0], "check: 1 wrong entry marked") {
		t.Errorf("top line: %q", strings.TrimSpace(res.Dump.Grid[0]))
	}
	if !hasClass(node(t, res, cellID(i)), "wrong") {
		t.Error("the wrong entry is not marked")
	}
	// Move away and look at the mark's colour: a red background.
	park(t, a, emptyAfter(a, i+1))
	res = play(t, a)
	n := node(t, res, cellID(i))
	if s := span(t, res, n.X, n.Y); s.BG != "#5c1f24" {
		t.Errorf("wrong entry background %s, want #5c1f24", s.BG)
	}
	// Fixing it clears the mark.
	park(t, a, i)
	play(t, a, fmt.Sprint(a.g.sol[i]), "c")
	if res = play(t, a); hasClass(node(t, res, cellID(i)), "wrong") || !strings.Contains(res.Dump.Grid[0], "no wrong entries") {
		t.Errorf("after the fix: wrong=%v top=%q", hasClass(node(t, res, cellID(i)), "wrong"), strings.TrimSpace(res.Dump.Grid[0]))
	}
}

// Highlights: the cursor, its twenty peers, and every cell with its digit.
func TestHighlights(t *testing.T) {
	a := newApp(t)
	// Put the cursor on a given so "same digit" has cells to light.
	i := -1
	for k := 0; k < size; k++ {
		if a.g.puz[k] != 0 {
			i = k
			break
		}
	}
	park(t, a, i)
	res := play(t, a)
	cur, peer, same, sameWant := 0, 0, 0, 0
	for k := 0; k < size; k++ {
		n := node(t, res, cellID(k))
		if hasClass(n, "cur") {
			cur++
			if k != i {
				t.Errorf("cursor class on %s", cellID(k))
			}
		}
		if hasClass(n, "peer") {
			peer++
			isPeer := false
			for _, p := range peers[i] {
				isPeer = isPeer || p == k
			}
			if !isPeer {
				t.Errorf("peer class on %s, which is no peer", cellID(k))
			}
		}
		if hasClass(n, "same") {
			same++
		}
		if a.g.vals[k] == a.g.vals[i] {
			sameWant++
		}
	}
	if cur != 1 || peer != 20 || same != sameWant || same < 2 {
		t.Errorf("cur %d peer %d same %d (want %d)", cur, peer, same, sameWant)
	}
	// The box of the cursor has the accent border.
	if !hasClass(node(t, res, fmt.Sprintf("b%d", boxOf[i])), "cur") {
		t.Error("the cursor's box is not marked")
	}
	// And colours: the cursor is accent, a peer is the peer tint.
	n := node(t, res, cellID(i))
	if s := span(t, res, n.X, n.Y); s.BG != "#58a6ff" {
		t.Errorf("cursor background %s, want the accent", s.BG)
	}
	pn := node(t, res, cellID(peers[i][0]))
	if s := span(t, res, pn.X, pn.Y); s.BG != "#2a3646" && s.BG != "#36517a" {
		t.Errorf("peer background %s, want a peer tint", s.BG)
	}
}

// Givens and entries look different: bold fg for a given, blue for the
// player's, in both themes.
func TestGivensAndEntriesLookDifferent(t *testing.T) {
	for theme, want := range map[string][2]string{"dark": {"#e6edf3", "#79c0ff"}, "light": {"#1f2328", "#0b5cad"}} {
		a := newApp(t)
		if err := a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		given := -1
		for k := 0; k < size; k++ {
			if a.g.puz[k] != 0 {
				given = k
				break
			}
		}
		entry := a.g.cur
		play(t, a, fmt.Sprint(a.g.sol[entry]))
		// Park the cursor on a cell that shares nothing with either.
		far := 80
		for a.g.puz[far] != 0 || boxOf[far] == boxOf[entry] {
			far--
		}
		park(t, a, far)
		res := play(t, a)
		gn, en := node(t, res, cellID(given)), node(t, res, cellID(entry))
		gs, es := span(t, res, gn.X+1, gn.Y), span(t, res, en.X+1, en.Y)
		if gs.FG != want[0] || es.FG != want[1] {
			t.Errorf("%s: given %s entry %s, want %v", theme, gs.FG, es.FG, want)
		}
		if len(gs.A) == 0 || gs.A[0] != "bold" {
			t.Errorf("%s: a given is not bold: %v", theme, gs.A)
		}
	}
}

// A duplicate in a row, column, or box turns both digits red.
func TestConflictsAreRed(t *testing.T) {
	for theme, red := range map[string]string{"dark": "#ff7b72", "light": "#c3261f"} {
		a := newApp(t)
		if err := a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		i := a.g.cur
		// A digit that a given in the cell's row already has.
		var peer int
		for _, p := range peers[i] {
			if a.g.puz[p] != 0 && rowOf[p] == rowOf[i] {
				peer = p
				break
			}
		}
		d := a.g.puz[peer]
		play(t, a, fmt.Sprint(d))
		far := 80
		for a.g.puz[far] != 0 || far == i {
			far--
		}
		park(t, a, far)
		res := play(t, a)
		for _, k := range []int{i, peer} {
			n := node(t, res, cellID(k))
			if !hasClass(n, "bad") {
				t.Errorf("%s: %s is not marked as a conflict", theme, cellID(k))
			}
			if s := span(t, res, n.X+1, n.Y); s.FG != red {
				t.Errorf("%s: %s is %s, want the conflict red %s", theme, cellID(k), s.FG, red)
			}
		}
		// A cell without a duplicate stays calm.
		for k := 0; k < size; k++ {
			if k != i && k != peer && hasClass(node(t, res, cellID(k)), "bad") && a.g.vals[k] != d {
				t.Errorf("%s: %s is marked as a conflict", theme, cellID(k))
			}
		}
		// Undo clears it.
		park(t, a, i)
		play(t, a, "u")
		res = play(t, a)
		if hasClass(node(t, res, cellID(peer)), "bad") {
			t.Errorf("%s: the conflict outlived the undo", theme)
		}
	}
}

func TestPauseHidesTheBoard(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "p")
	wantActions(t, res, "pause")
	if !a.paused {
		t.Fatal("not paused")
	}
	g := gridText(res)
	if !strings.Contains(g, "PAUSED") || !strings.Contains(g, "‖ PAUSED") {
		t.Errorf("the screen does not say it is paused:\n%s", g)
	}
	said := map[int]bool{39: true, 40: true, 41: true, 48: true, 49: true, 50: true} // the middle box's two rows of words
	var words []string
	for k := 0; k < size; k++ {
		n := node(t, res, cellID(k))
		text := strings.TrimSpace(n.Text)
		if said[k] {
			words = append(words, n.Text)
		} else if text != "" {
			t.Errorf("cell %s shows %q while paused", cellID(k), text)
		}
		for _, c := range []string{"cur", "peer", "same", "given"} {
			if hasClass(n, c) {
				t.Errorf("cell %s has class %s while paused", cellID(k), c)
			}
		}
	}
	if got := strings.Join(words, ""); got != " PAUSED   press p " {
		t.Errorf("the middle box says %q", got)
	}
	// Digits hide: not one digit shows left of the panel, which starts at
	// column 61 of 100.
	for r := 1; r < 23; r++ {
		if line := res.Dump.Grid[r][:60]; strings.ContainsAny(line, "123456789") {
			t.Errorf("row %d shows a digit while paused: %q", r, line)
		}
	}
	// Input does nothing while paused: keys, clicks, the pad.
	before := a.g.vals
	cur := a.g.cur
	play(t, a, "5", "right", "n")
	play(t, a, click(t, a, 100, 24, "c44"), click(t, a, 100, 24, "p5"))
	if a.g.vals != before || a.g.cur != cur || a.g.pencil {
		t.Error("the board took input while paused")
	}
	// p again brings it back.
	play(t, a, "p")
	res = play(t, a)
	if a.paused {
		t.Fatal("p did not resume")
	}
	given := -1
	for k := 0; k < size; k++ {
		if a.g.puz[k] != 0 {
			given = k
			break
		}
	}
	if got := strings.TrimSpace(node(t, res, cellID(given)).Text); got != fmt.Sprint(a.g.puz[given]) {
		t.Errorf("after resuming, cell %s shows %q, want its digit %d", cellID(given), got, a.g.puz[given])
	}
}

func TestTimer(t *testing.T) {
	a := newApp(t)
	now := epoch
	tickAt(t, a, now) // the first tick only sets the reference
	if got := get(t, a, "time"); got != "0:00" {
		t.Fatalf("time = %v", got)
	}
	now = tickFor(t, a, now, 3*time.Second)
	if got := get(t, a, "time"); got != "0:03" {
		t.Errorf("time = %v after 3 s, want 0:03", got)
	}
	// Paused: the clock stands still.
	play(t, a, "p")
	now = tickFor(t, a, now, 5*time.Second)
	if got := get(t, a, "time"); got != "0:03" {
		t.Errorf("time = %v while paused", got)
	}
	play(t, a, "p")
	now = tickFor(t, a, now, 2*time.Second)
	if got := get(t, a, "time"); got != "0:05" {
		t.Errorf("time = %v after resuming, want 0:05", got)
	}
	// A modal stops it too.
	play(t, a, "?")
	now = tickFor(t, a, now, 5*time.Second)
	if got := get(t, a, "time"); got != "0:05" {
		t.Errorf("time = %v under the help", got)
	}
	play(t, a, "esc")
	// So do the new-game and the win modals.
	play(t, a, "N")
	now = tickFor(t, a, now, 5*time.Second)
	if got := get(t, a, "time"); got != "0:05" {
		t.Errorf("time = %v under the new-game modal", got)
	}
	play(t, a, "esc")
	// A stall (a suspended terminal) is capped at one second.
	now = now.Add(time.Hour)
	tickAt(t, a, now)
	if got := get(t, a, "time"); got != "0:06" {
		t.Errorf("time = %v after an hour's stall, want 0:06", got)
	}
	// Hours show.
	a.elapsed = 3725 * time.Second
	if err := a.sync(); err != nil {
		t.Fatal(err)
	}
	if got := get(t, a, "time"); got != "1:02:05" {
		t.Errorf("time = %v at 3725 s", got)
	}
}

func TestClockOnScreen(t *testing.T) {
	a := newApp(t)
	tickAt(t, a, epoch)
	tickFor(t, a, epoch, 65*time.Second)
	res := play(t, a)
	if !strings.Contains(gridText(res), "1:05") {
		t.Errorf("the panel does not show 1:05:\n%s", gridText(res))
	}
}

func TestHelpModal(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "?")
	wantActions(t, res, "help_open")
	g := gridText(res)
	for _, want := range []string{"move", "digit", "erase", "notes", "undo", "redo", "hint", "check", "pause", "new", "mouse", "quit", "close"} {
		if !strings.Contains(g, want) {
			t.Errorf("help has no %q:\n%s", want, g)
		}
	}
	// The board keys are quiet under the help.
	res = play(t, a, "5", "n", "up", "H", "p", "N", "m", "u")
	if len(res.Events) != 0 {
		t.Errorf("keys fired %v under the help", actions(res))
	}
	for _, key := range []string{"esc", "q", "?", "enter"} {
		a := newApp(t)
		play(t, a, "?")
		res := play(t, a, key)
		wantActions(t, res, "help_close")
		if a.helpOpen {
			t.Errorf("%s did not close the help", key)
		}
	}
}

// After a modal closes the focus is back on the board, so its keys work.
func TestFocusReturnsToTheBoard(t *testing.T) {
	for name, open := range map[string][]string{"help": {"?"}, "new game": {"N"}} {
		a := newApp(t)
		res := play(t, a, append(open, "esc")...)
		if f := res.Dump.Focus; f == nil || *f != "board" {
			t.Errorf("%s: focus after esc = %v, want the board", name, f)
		}
		wantActions(t, play(t, a, "right"), "right")
	}
}

func TestMouseToggle(t *testing.T) {
	a := newApp(t)
	if get(t, a, "mouse") != true {
		t.Fatal("the mouse starts off")
	}
	step := click(t, a, 100, 24, "c80")
	wantActions(t, play(t, a, "m"), "mouse")
	if get(t, a, "mouse") != false {
		t.Fatal("m did not turn the mouse off")
	}
	res := play(t, a, step)
	if len(res.Events) != 0 {
		t.Errorf("a click acted with the mouse off: %v", actions(res))
	}
	play(t, a, "m")
	wantActions(t, play(t, a, step), "cell")
}

func TestQuit(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		a := newApp(t)
		res, err := a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 24}, key)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Quit {
			t.Errorf("%s did not quit", key)
		}
	}
	// ctrl+c quits from under a modal too; q there closes the help.
	a := newApp(t)
	play(t, a, "N")
	res, err := a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 24}, "ctrl+c")
	if err != nil || !res.Quit {
		t.Errorf("ctrl+c under a modal: quit=%v err=%v", res != nil && res.Quit, err)
	}
}

func TestNewGameModal(t *testing.T) {
	a := newApp(t)
	// Some state the new game must reset.
	play(t, a, "H", "n")
	tickFor(t, a, epoch, 4*time.Second)
	res := play(t, a, "N")
	wantActions(t, res, "newgame_open")
	g := gridText(res)
	for _, want := range []string{"easy", "medium", "hard", "expert", "cancel"} {
		if !strings.Contains(g, want) {
			t.Errorf("the new-game modal lacks %q:\n%s", want, g)
		}
	}
	// Keys are quiet under it; esc and cancel close it.
	if res = play(t, a, "5", "up", "H"); len(res.Events) != 0 {
		t.Errorf("keys fired %v under the new-game modal", actions(res))
	}
	wantActions(t, play(t, a, "esc"), "newgame_close")
	play(t, a, "N")
	wantActions(t, play(t, a, click(t, a, 100, 24, "nd-cancel")), "newgame_close")
	if a.newOpen {
		t.Fatal("cancel did not close the modal")
	}
	// Each difficulty button starts a puzzle of that grade, from the next seed.
	for level := levelEasy; level <= levelExpert; level++ {
		a := newApp(t)
		play(t, a, "H", "n")
		tickFor(t, a, epoch, 4*time.Second)
		play(t, a, "N")
		id := "nd-" + levelName(level)
		res := play(t, a, click(t, a, 100, 24, id))
		wantActions(t, res, "new_game")
		want := generateCached(defaultSeed+1, level)
		if a.g.puz != want.puz || a.g.level != level {
			t.Errorf("%s: the new game is not the seed+1 puzzle of that level", id)
		}
		if a.newOpen || a.elapsed != 0 || a.g.hints != 0 || a.g.pencil || a.g.mistakes != 0 || a.paused {
			t.Errorf("%s: state not reset: open=%v elapsed=%v hints=%d", id, a.newOpen, a.elapsed, a.g.hints)
		}
		if get(t, a, "diff") != levelName(level) || get(t, a, "time") != "0:00" {
			t.Errorf("%s: store diff=%v time=%v", id, get(t, a, "diff"), get(t, a, "time"))
		}
		if f := res.Dump.Focus; f == nil || *f != "board" {
			t.Errorf("%s: focus %v after the new game", id, f)
		}
	}
	// Enter on the focused button starts a medium game.
	a = newApp(t)
	res = play(t, a, "N", "enter")
	wantActions(t, res, "newgame_open", "new_game")
	if a.g.level != levelMedium || a.g.puz != generateCached(defaultSeed+1, levelMedium).puz {
		t.Error("enter did not start the medium game")
	}
	// A second new game uses the seed after that.
	play(t, a, "N", "enter")
	if a.g.puz != generateCached(defaultSeed+2, levelMedium).puz {
		t.Error("the third game is not the seed+2 puzzle")
	}
}

func TestWinFlow(t *testing.T) {
	a := newApp(t)
	tickAt(t, a, epoch)
	now := tickFor(t, a, epoch, 12*time.Second)
	hintUntilWon(t, a)
	hints := a.g.hints
	elapsedAtWin := a.elapsed
	if hints == 0 {
		t.Fatal("setup: the H key made no hint")
	}
	// The finished board shows first, every cell highlighted, with no modal.
	res := play(t, a)
	if a.winOpen || strings.Contains(gridText(res), "solved in") {
		t.Fatal("the win modal opened at once")
	}
	for k := 0; k < size; k++ {
		if !hasClass(node(t, res, cellID(k)), "done") {
			t.Fatalf("cell %s is not highlighted as done", cellID(k))
		}
	}
	if !strings.Contains(gridText(res), "SOLVED") {
		t.Error("the status line does not say solved")
	}
	// The clock stops with the win; the modal opens after winDelay.
	now = tickFor(t, a, now, 500*time.Millisecond)
	if a.winOpen {
		t.Error("the win modal opened early")
	}
	now = tickFor(t, a, now, time.Second)
	if !a.winOpen {
		t.Fatal("the win modal did not open")
	}
	if get(t, a, "time") != "0:12" {
		t.Errorf("time = %v, want 0:12: the clock ran after the win", get(t, a, "time"))
	}
	res = play(t, a)
	g := gridText(res)
	for _, want := range []string{"solved in 0:12", fmt.Sprintf("hints used %d", hints), "new game", "close"} {
		if !strings.Contains(g, want) {
			t.Errorf("win modal lacks %q:\n%s", want, g)
		}
	}
	// Keys are quiet under it; ticks do not open it a second time.
	if res = play(t, a, "5", "up", "u"); len(res.Events) != 0 {
		t.Errorf("keys fired %v under the win modal", actions(res))
	}
	wantActions(t, play(t, a, "esc"), "win_close")
	now = tickFor(t, a, now, 3*time.Second)
	if a.winOpen {
		t.Error("the win modal opened again")
	}
	// The clock never ran after the win: not before the modal, not under it,
	// not after it closed.
	if a.elapsed != elapsedAtWin || get(t, a, "time") != "0:12" {
		t.Errorf("elapsed %v, time %v: the clock ran after the win (was %v)", a.elapsed, get(t, a, "time"), elapsedAtWin)
	}
	// Closed, the finished board stays; a new game starts from N.
	if res = play(t, a); !hasClass(node(t, res, "c00"), "done") {
		t.Error("the board lost its completion highlight")
	}
	if res = play(t, a, "5", "u"); len(res.Events) != 2 || a.g.vals != a.g.sol {
		t.Error("the finished board took input")
	}
	wantActions(t, play(t, a, "N"), "newgame_open")
	_ = now
}

// The win modal's new-game button goes to the new-game modal.
func TestWinModalNewGameButton(t *testing.T) {
	a := newApp(t)
	finish(t, a)
	if !a.winOpen {
		t.Fatal("setup: no win modal")
	}
	res := play(t, a, click(t, a, 100, 24, "won-new"))
	wantActions(t, res, "newgame_open")
	if a.winOpen || !a.newOpen {
		t.Errorf("winOpen %v newOpen %v", a.winOpen, a.newOpen)
	}
	play(t, a, click(t, a, 100, 24, "nd-easy"))
	if a.g.won || a.g.level != levelEasy {
		t.Error("the new game did not start")
	}
}

func TestOnErrorShowsTheNoticeAndClears(t *testing.T) {
	a := newApp(t)
	if err := a.onError(nil, errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	res := play(t, a)
	if !strings.Contains(res.Dump.Grid[0], "boom") {
		t.Errorf("the notice is not shown:\n%s", res.Dump.Grid[0])
	}
	// The next successful action clears it.
	res = play(t, a, "right")
	if get(t, a, "notice") != "" || strings.Contains(res.Dump.Grid[0], "boom") {
		t.Error("the notice outlived an action")
	}
	// So does the next tick.
	_ = a.onError(nil, errors.New("again"))
	tickAt(t, a, epoch)
	if get(t, a, "notice") != "" {
		t.Error("the notice outlived a tick")
	}
	// A quit is not an error to show.
	if err := a.onError(nil, tuimark.ErrQuit); !errors.Is(err, tuimark.ErrQuit) {
		t.Errorf("onError(ErrQuit) = %v, want ErrQuit", err)
	}
	if get(t, a, "notice") != "" {
		t.Error("quit left a notice")
	}
}

// A handler that fails shows its message instead of ending the game.
func TestFailingHandlerReachesOnError(t *testing.T) {
	a := newApp(t)
	// A click whose source is no cell of the board.
	if err := a.handlers()["cell"](tuimark.Event{Action: "cell", Source: "c99"}); err == nil {
		t.Error("a click on c99 was accepted")
	}
	if err := a.handlers()["new_game"](tuimark.Event{Action: "new_game", Source: "nd-nightmare"}); err == nil {
		t.Error("an unknown difficulty was accepted")
	}
}

func TestLayoutBySize(t *testing.T) {
	a := newApp(t)
	for _, tc := range []struct {
		cols, rows int
		panel      bool
		borders    bool
	}{{80, 24, true, true}, {100, 30, true, true}, {64, 20, true, true}, {63, 24, false, true}, {60, 20, false, true},
		{80, 19, false, false}, {40, 14, false, false}} {
		res := playAt(t, a, tc.cols, tc.rows)
		d := res.Dump
		var side, board bool
		for _, n := range d.Nodes {
			side = side || (n.ID == "side" && n.W > 0)
			board = board || (n.ID == "board" && n.W > 0 && n.H > 0)
		}
		if side != tc.panel || !board {
			t.Errorf("%dx%d: side panel %v board %v, want panel %v and a board", tc.cols, tc.rows, side, board, tc.panel)
		}
		wantW, wantH := 29, 11
		if tc.borders {
			wantW, wantH = 33, 15
		}
		for _, n := range d.Nodes {
			if n.ID == "board" && (n.W != wantW || n.H != wantH) {
				t.Errorf("%dx%d: the board is %dx%d, want %dx%d", tc.cols, tc.rows, n.W, n.H, wantW, wantH)
			}
		}
		// Every cell is on the screen.
		for k := 0; k < size; k++ {
			n := node(t, res, cellID(k))
			if n.X < 0 || n.Y < 0 || n.X+n.W > tc.cols || n.Y+n.H > tc.rows || n.W != 3 || n.H != 1 {
				t.Errorf("%dx%d: cell %s at %+v", tc.cols, tc.rows, cellID(k), n)
				break
			}
		}
		// The top line has the clock, in the panel or in the bar.
		if tc.panel {
			if !strings.Contains(gridText(res), "mistakes") {
				t.Errorf("%dx%d: no panel text", tc.cols, tc.rows)
			}
		} else if !strings.Contains(d.Grid[0], "0:00") {
			t.Errorf("%dx%d: the top line has no clock: %q", tc.cols, tc.rows, d.Grid[0])
		}
	}
}

func TestTooSmallSaysSo(t *testing.T) {
	a := newApp(t)
	for _, sz := range [][2]int{{39, 24}, {80, 13}, {20, 8}} {
		res := playAt(t, a, sz[0], sz[1])
		g := gridText(res)
		if !strings.Contains(g, "too small") {
			t.Errorf("%dx%d: no notice:\n%s", sz[0], sz[1], g)
		}
		for _, n := range res.Dump.Nodes {
			if n.ID == "c00" && n.W > 0 {
				t.Errorf("%dx%d: the board shows on a screen too small for it", sz[0], sz[1])
			}
		}
	}
	// And the board is back at the smallest supported size.
	res := playAt(t, a, 40, 14)
	if strings.Contains(gridText(res), "too small") {
		t.Error("40x14 says too small")
	}
}

func TestLightTheme(t *testing.T) {
	a := newApp(t)
	if err := a.ui.Set("@theme", "light"); err != nil {
		t.Fatal(err)
	}
	res := play(t, a)
	if res.Dump.Theme != "light" {
		t.Errorf("theme = %q", res.Dump.Theme)
	}
	for _, row := range res.Dump.Styles {
		for _, sp := range row {
			if sp.BG == "#12151a" || sp.BG == "#181d24" {
				t.Fatalf("a dark background shows in the light theme: %+v", sp)
			}
		}
	}
}

// Every state at every size and in both themes lays out without a
// diagnostic, and with each modal open in turn its buttons are on screen.
func TestNoDiagnosticsAcrossStatesSizesAndModals(t *testing.T) {
	type state struct {
		name    string
		setup   func(t *testing.T, a *app)
		reached func(a *app) bool // the state really is what its name says
	}
	anyBad := func(a *app) bool {
		v, _ := a.ui.Get("cells")
		for _, c := range v.(map[string]any) {
			if c.(map[string]any)["bad"] == true {
				return true
			}
		}
		return false
	}
	states := []state{
		{"new", func(*testing.T, *app) {}, func(a *app) bool { return a.g.filled() == a.g.puz.clues() && a.elapsed == 0 }},
		{"midgame", func(t *testing.T, a *app) {
			i := a.g.cur
			play(t, a, "H") // the first empty cell
			a.g.cur = emptyAfter(a, i+1)
			play(t, a, "n", "1", "2", "n")
			a.g.cur = emptyAfter(a, a.g.cur+1)
			play(t, a, wrongDigit(a), "c")
			a.g.cur = 40
			play(t, a, "right", "down")
		}, func(a *app) bool {
			marks, notes := 0, 0
			for i := range a.g.marks {
				if a.g.marks[i] {
					marks++
				}
				if a.g.notes[i] != 0 {
					notes++
				}
			}
			return a.g.hints == 1 && a.g.mistakes == 1 && marks == 1 && notes >= 1 && a.info == ""
		}},
		{"conflict", func(t *testing.T, a *app) {
			i := a.g.cur
			for _, p := range peers[i] {
				if a.g.puz[p] != 0 {
					play(t, a, fmt.Sprint(a.g.puz[p]))
					return
				}
			}
		}, anyBad},
		{"paused", func(t *testing.T, a *app) { play(t, a, "p") }, func(a *app) bool { return a.paused }},
		{"won", func(t *testing.T, a *app) { hintUntilWon(t, a) }, func(a *app) bool { return a.g.won && !a.winOpen }},
		{"notice", func(t *testing.T, a *app) {
			_ = a.onError(nil, errors.New("a very long error message that has to fit on one line without being cut anywhere at all"))
		}, func(a *app) bool { return a.notice != "" }},
		{"hours", func(t *testing.T, a *app) {
			a.elapsed = 3725 * time.Second
			_ = a.sync()
		}, func(a *app) bool { v, _ := a.ui.Get("time"); return v == "1:02:05" }},
	}
	modals := []struct {
		name  string
		flag  string
		close string
		btns  []string
	}{
		{"closed", "", "", nil},
		{"help", "help_open", "", []string{"help-ok"}},
		{"won", "win_open", "", []string{"won-new", "won-close"}},
		{"newgame", "new_open", "", []string{"nd-easy", "nd-medium", "nd-hard", "nd-expert", "nd-cancel"}},
	}
	sizes := [][2]int{{40, 14}, {60, 20}, {80, 24}, {100, 30}, {120, 30}, {64, 20}, {41, 15}, {63, 19}}
	for _, st := range states {
		for _, theme := range []string{"dark", "light"} {
			a := newApp(t)
			if err := a.ui.Set("@theme", theme); err != nil {
				t.Fatal(err)
			}
			st.setup(t, a)
			if !st.reached(a) {
				t.Fatalf("state %s was not reached", st.name)
			}
			for _, m := range modals {
				if m.flag != "" {
					if err := a.ui.Set(m.flag, true); err != nil {
						t.Fatal(err)
					}
				}
				for _, sz := range sizes {
					d, err := a.ui.Dump(sz[0], sz[1])
					if err != nil {
						t.Fatal(err)
					}
					if !d.OK || len(d.Errors) != 0 {
						t.Errorf("%s/%s/%s at %dx%d: %v", st.name, theme, m.name, sz[0], sz[1], d.Errors)
						continue
					}
					for _, id := range m.btns {
						found := false
						for _, n := range d.Nodes {
							if n.ID == id {
								found = true
								if n.W < 1 || n.H < 1 || n.X < 0 || n.Y < 0 || n.X+n.W > sz[0] || n.Y+n.H > sz[1] {
									t.Errorf("%s/%s/%s at %dx%d: button %s at %+v is not on the screen", st.name, theme, m.name, sz[0], sz[1], id, n)
								}
							}
						}
						if !found {
							t.Errorf("%s/%s/%s at %dx%d: no button %s", st.name, theme, m.name, sz[0], sz[1], id)
						}
					}
				}
				if m.flag != "" {
					if err := a.ui.Set(m.flag, false); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}

// The claim in the README: every size from 40x14 up is clean, closed and
// with each modal open. The sweep takes every row count from 14 to 30 at the
// column counts on and beside each breakpoint of the stylesheet (the panel at
// 64 columns, the brand at 56, the board's borders at 20 rows). One theme is
// enough: the light theme only redefines the colour tokens, and the states
// test above runs both.
func TestCleanAtEverySizeFrom40x14(t *testing.T) {
	a := newApp(t)
	sizes := 0
	for _, flag := range []string{"", "help_open", "win_open", "new_open"} {
		if flag != "" {
			if err := a.ui.Set(flag, true); err != nil {
				t.Fatal(err)
			}
		}
		for _, cols := range []int{40, 55, 56, 63, 64, 80, 120} {
			for rows := 14; rows <= 30; rows++ {
				d, err := a.ui.Dump(cols, rows)
				if err != nil {
					t.Fatal(err)
				}
				if !d.OK || len(d.Errors) != 0 {
					t.Fatalf("%q at %dx%d: %v", flag, cols, rows, d.Errors)
				}
				sizes++
			}
		}
		if flag != "" {
			if err := a.ui.Set(flag, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	if sizes != 4*7*17 {
		t.Errorf("%d dumps checked", sizes)
	}
}

// store is the whole store through JSON.
func store(t testing.TB, a *app) any {
	t.Helper()
	v, _ := a.ui.Get("")
	return normalize(t, v)
}

// While paused the board takes no input: every key and click that acts on
// the board fires its action and changes nothing in the store. The keys that
// are not about the board work: ? opens the help, N the new-game modal, m
// toggles the mouse, q quits.
func TestPausedBoardTakesNoInput(t *testing.T) {
	boardKeys := []string{"1", "5", "9", "0", "x", "backspace", "n", "u", "U", "H", "c", "up", "down", "left", "right", "h", "j", "k", "l"}
	for _, key := range boardKeys {
		a := newApp(t)
		// Leave the cursor on a wrong entry, which undo, erase, hint, and
		// check all act on; redo needs that entry undone.
		setup := []string{wrongDigit(a), "p"}
		if key == "U" {
			setup = []string{wrongDigit(a), "u", "p"}
		}
		play(t, a, setup...)
		if !a.paused {
			t.Fatal("setup: not paused")
		}
		before := store(t, a)
		play(t, a, key)
		if !reflect.DeepEqual(before, store(t, a)) {
			t.Errorf("key %q changed the store while paused", key)
		}
	}
	// Clicks: a cell, the pad, and every tool.
	for _, id := range []string{"c44", "p5", "t-erase", "t-notes", "t-undo", "t-redo", "t-hint", "t-check"} {
		a := newApp(t)
		play(t, a, wrongDigit(a), "p")
		step := click(t, a, 100, 24, id)
		before := store(t, a)
		play(t, a, step)
		if !reflect.DeepEqual(before, store(t, a)) {
			t.Errorf("a click on %s changed the store while paused", id)
		}
	}
	// What is not about the board still works while paused.
	a := newApp(t)
	play(t, a, "p")
	play(t, a, "?")
	if !a.helpOpen || !a.paused {
		t.Errorf("? while paused: help %v paused %v", a.helpOpen, a.paused)
	}
	play(t, a, "esc")
	play(t, a, "m")
	if a.mouse {
		t.Error("m did not toggle the mouse while paused")
	}
	play(t, a, "m", "N")
	if !a.newOpen {
		t.Error("N did not open the new-game modal while paused")
	}
	play(t, a, "enter")
	if a.paused || a.newOpen {
		t.Errorf("a new game left paused=%v newOpen=%v", a.paused, a.newOpen)
	}
	a = newApp(t)
	play(t, a, "p")
	if res, err := a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 24}, "q"); err != nil || !res.Quit {
		t.Error("q did not quit while paused")
	}
}

// Once the board is solved nothing but the new-game key does anything: not
// the digits, undo, redo, hint, check, erase, notes, and not pause (a
// paused finished board would hide the win).
func TestFinishedBoardTakesNoInput(t *testing.T) {
	a := newApp(t)
	finish(t, a)
	play(t, a, "esc") // the win modal
	if a.winOpen || !a.g.won {
		t.Fatal("setup")
	}
	before := store(t, a)
	for _, key := range []string{"p", "1", "5", "0", "x", "u", "U", "H", "c", "n"} {
		play(t, a, key)
		if !reflect.DeepEqual(before, store(t, a)) {
			t.Errorf("key %q changed the finished board's store", key)
		}
	}
	if a.paused {
		t.Error("p paused a finished game")
	}
	wantActions(t, play(t, a, "N"), "newgame_open")
}

// A check mark shows on the cursor cell too: the cursor colour alone would
// hide it until the cursor moved.
func TestCheckMarkShowsOnTheCursorCell(t *testing.T) {
	for theme, red := range map[string]string{"dark": "#f85149", "light": "#c3261f"} {
		a := newApp(t)
		if err := a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		i := a.g.cur
		plain := play(t, a)
		pn := node(t, plain, cellID(i))
		cursorLook := span(t, plain, pn.X+1, pn.Y)
		// A digit that clashes with nothing, so only the check mark can
		// change how the cursor cell looks.
		d := -1
		for k := 1; k <= 9; k++ {
			clash := false
			for _, p := range peers[i] {
				clash = clash || int(a.g.vals[p]) == k
			}
			if !clash && byte(k) != a.g.sol[i] {
				d = k
				break
			}
		}
		if d < 0 {
			t.Fatal("no quiet wrong digit for the cursor cell")
		}
		res := play(t, a, fmt.Sprint(d))
		n := node(t, res, cellID(i))
		before := span(t, res, n.X+1, n.Y)
		res = play(t, a, "c")
		if !hasClass(node(t, res, cellID(i)), "wrong") || !hasClass(node(t, res, cellID(i)), "cur") {
			t.Fatalf("%s: classes %v", theme, node(t, res, cellID(i)).Classes)
		}
		after := span(t, res, n.X+1, n.Y)
		if after.BG != red || len(after.A) == 0 || !contains(after.A, "underline") {
			t.Errorf("%s: a checked cursor cell is %+v, want the red background %s with underline", theme, after, red)
		}
		if after.BG == before.BG && after.FG == before.FG {
			t.Errorf("%s: the mark is invisible on the cursor cell: %+v both ways", theme, after)
		}
		_ = cursorLook
	}
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

// The too-small view is clean from 10x3 up: below 40x14 it says so, in a
// sentence that fits or a shorter one, and the keys q and ? still work.
func TestTooSmallViewIsCleanFrom10x3(t *testing.T) {
	n := 0
	for _, theme := range []string{"dark", "light"} {
		a := newApp(t)
		if err := a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		for _, cols := range []int{10, 11, 12, 15, 19, 20, 21, 24, 29, 30, 31, 35, 39, 40, 41, 45} {
			for rows := 3; rows <= 16; rows++ {
				d, err := a.ui.Dump(cols, rows)
				if err != nil {
					t.Fatal(err)
				}
				if !d.OK || len(d.Errors) != 0 {
					t.Fatalf("%s at %dx%d: %v\n%s", theme, cols, rows, d.Errors, strings.Join(d.Grid, "\n"))
				}
				small := cols < minCols || rows < minRows
				says := strings.Contains(strings.Join(d.Grid, "\n"), "too small")
				if small && !says {
					t.Errorf("%s at %dx%d: no notice\n%s", theme, cols, rows, strings.Join(d.Grid, "\n"))
				}
				if !small && says {
					t.Errorf("%s at %dx%d: says too small", theme, cols, rows)
				}
				n++
			}
		}
	}
	if n != 2*16*14 {
		t.Errorf("%d sizes", n)
	}
}
