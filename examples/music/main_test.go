package main

import (
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

func play(t *testing.T, a *app, steps ...string) *tuimark.PlayResult {
	t.Helper()
	res, err := a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 24}, steps...)
	if err != nil {
		t.Fatalf("play %v: %v", steps, err)
	}
	if !res.Dump.OK {
		t.Fatalf("play %v: dump errors %v", steps, res.Dump.Errors)
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

func grid(res *tuimark.PlayResult) string { return strings.Join(res.Dump.Grid, "\n") }

func focus(res *tuimark.PlayResult) string {
	if res.Dump.Focus == nil {
		return ""
	}
	return *res.Dump.Focus
}

// get reads a store path; the tests use it for scalars.
func get(t *testing.T, a *app, path string) any {
	t.Helper()
	v, ok := a.ui.Get(path)
	if !ok {
		t.Fatalf("%s is not in the store", path)
	}
	return v
}

func current(t *testing.T, a *app) string { return get(t, a, "player.current").(string) }

func wantActions(t *testing.T, res *tuimark.PlayResult, want ...string) {
	t.Helper()
	if got := strings.Join(actions(res), ","); got != strings.Join(want, ",") {
		t.Fatalf("actions = %q, want %q", got, strings.Join(want, ","))
	}
}

// tickAt runs the Every function at a fake time, as the loop would.
func tickAt(t *testing.T, a *app, now time.Time) {
	t.Helper()
	if err := a.ui.Batch(func(b *tuimark.Batch) error { return a.tick(b, now) }); err != nil {
		t.Fatal(err)
	}
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

// Every action the document names has a handler, except the built-ins.
func TestHandlersCoverCatalog(t *testing.T) {
	a := newApp(t)
	for _, spec := range a.ui.Catalog() {
		if !spec.Builtin && !spec.Registered {
			t.Errorf("no handler for %s (%v)", spec.Name, spec.Sources)
		}
	}
}

func TestDumpShowsThePlayer(t *testing.T) {
	a := newApp(t)
	for _, sz := range [][2]int{{120, 30}, {100, 24}, {80, 24}, {60, 20}, {40, 14}} {
		d, err := a.ui.Dump(sz[0], sz[1])
		if err != nil {
			t.Fatal(err)
		}
		if !d.OK || len(d.Errors) != 0 {
			t.Errorf("%dx%d: errors %v", sz[0], sz[1], d.Errors)
		}
		if d.Wide {
			t.Errorf("%dx%d: the grid holds a wide cluster, so columns would not line up", sz[0], sz[1])
		}
		g := strings.Join(d.Grid, "\n")
		for _, want := range []string{"music", "Tessellate", "1:12 / 4:12", "library 25", "queue 2", "play/pause"} {
			if sz[0] < 50 && want == "play/pause" {
				continue // the narrowest hint row only has room for two hints
			}
			if !strings.Contains(g, want) {
				t.Errorf("%dx%d: grid has no %q:\n%s", sz[0], sz[1], want, g)
			}
		}
	}
}

// The visualizer is shown from 77 columns up and left out below.
func TestVisualizerShownWhenThereIsRoom(t *testing.T) {
	a := newApp(t)
	has := func(cols int) bool {
		d, err := a.ui.Dump(cols, 24)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range d.Nodes {
			if n.ID == "viz" {
				return true
			}
		}
		return false
	}
	if !has(100) || !has(80) {
		t.Error("no visualizer at 80 or 100 columns")
	}
	if has(60) {
		t.Error("a visualizer at 60 columns")
	}
}

func TestSpaceTogglesPlayback(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "space")
	wantActions(t, res, "toggle_play")
	if get(t, a, "player.playing") != false {
		t.Error("space did not pause")
	}
	if g := grid(res); !strings.Contains(g, "⏸") || !strings.Contains(g, "paused") {
		t.Errorf("the panel does not show the pause:\n%s", g)
	}
	if get(t, a, "now.state") != "paused" {
		t.Errorf("now.state = %v", get(t, a, "now.state"))
	}
	res = play(t, a, "space")
	if get(t, a, "player.playing") != true || !strings.Contains(grid(res), "▶") {
		t.Error("a second space did not resume")
	}
}

func TestNextTakesTheQueueThenPrevGoesBack(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "n")
	wantActions(t, res, "next")
	if current(t, a) != "t09" {
		t.Errorf("n played %s, want the queued t09", current(t, a))
	}
	if g := grid(res); !strings.Contains(g, "Window Seat Aurora") || !strings.Contains(g, "queue 1") {
		t.Errorf("the panel and the tab label did not follow:\n%s", g)
	}
	if get(t, a, "now.time") != "0:00 / 4:36" {
		t.Errorf("now.time = %v", get(t, a, "now.time"))
	}
	// At the start of a track, p goes to the track played before.
	play(t, a, "n") // t14
	play(t, a, "n") // queue empty: library order, t15
	if current(t, a) != "t15" {
		t.Fatalf("current = %s, want t15", current(t, a))
	}
	play(t, a, "p")
	if current(t, a) != "t14" {
		t.Errorf("p went to %s, want t14", current(t, a))
	}
}

func TestEnterPlaysTheSelectedTrack(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "enter")
	wantActions(t, res, "lib_play")
	if current(t, a) != "t04" {
		t.Errorf("enter played %s, want the cursor row t04", current(t, a))
	}
	res = play(t, a, "down", "enter")
	if current(t, a) != "t05" {
		t.Errorf("down enter played %s, want t05", current(t, a))
	}
	if !strings.Contains(grid(res), "Slow Cartography") {
		t.Error("the panel does not show the new track")
	}
}

func TestAddToQueue(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "a")
	wantActions(t, res, "lib_add")
	q := get(t, a, "queue").([]any)
	if len(q) != 3 || q[2].(map[string]any)["id"] != "t04" {
		t.Errorf("queue = %v, want t04 added at the end", q)
	}
	if !strings.Contains(grid(res), "queue 3") {
		t.Errorf("the tab label did not count up:\n%s", grid(res))
	}
	play(t, a, "a") // the same track twice: two entries
	if n := len(get(t, a, "queue").([]any)); n != 4 {
		t.Errorf("queue has %d entries, want 4", n)
	}
}

func TestShuffleAndRepeatCycle(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "s")
	wantActions(t, res, "toggle_shuffle")
	if get(t, a, "player.shuffle") != true || !strings.Contains(grid(res), "shuffle on") {
		t.Errorf("shuffle is not on:\n%s", grid(res))
	}
	play(t, a, "s")
	if get(t, a, "player.shuffle") != false {
		t.Error("a second s did not turn shuffle off")
	}
	for _, want := range []string{repeatAll, repeatOne, repeatOff} {
		res = play(t, a, "r")
		if got := get(t, a, "player.repeat"); got != want {
			t.Fatalf("repeat = %v, want %s", got, want)
		}
		if !strings.Contains(grid(res), "repeat "+want) {
			t.Errorf("the panel does not say repeat %s:\n%s", want, grid(res))
		}
	}
}

func TestVolumeKeys(t *testing.T) {
	a := newApp(t)
	play(t, a, "+")
	if get(t, a, "player.volume") != 75.0 {
		t.Errorf("volume after + = %v, want 75", get(t, a, "player.volume"))
	}
	play(t, a, "=")
	play(t, a, "-")
	play(t, a, "-")
	res := play(t, a, "-")
	if get(t, a, "player.volume") != 65.0 || !strings.Contains(grid(res), "65%") {
		t.Errorf("volume = %v, grid:\n%s", get(t, a, "player.volume"), grid(res))
	}
	for i := 0; i < 30; i++ {
		play(t, a, "-")
	}
	if get(t, a, "player.volume") != 0.0 {
		t.Errorf("volume did not stop at 0: %v", get(t, a, "player.volume"))
	}
}

func TestSeekKeys(t *testing.T) {
	a := newApp(t)
	before := get(t, a, "player.pos").(float64)
	res := play(t, a, "right")
	wantActions(t, res, "seek_fwd")
	if got := get(t, a, "player.pos").(float64); got != before+5 {
		t.Errorf("right: pos %v -> %v, want +5", before, got)
	}
	play(t, a, "left", "left")
	if got := get(t, a, "player.pos").(float64); got != before-5 {
		t.Errorf("left left: pos = %v, want %v", got, before-5)
	}
	// The queue tab's table does not take left and right either.
	play(t, a, "2")
	play(t, a, "right")
	if got := get(t, a, "player.pos").(float64); got != before {
		t.Errorf("right on the queue tab: pos = %v, want %v", got, before)
	}
}

func TestFilterNarrowsTheLibrary(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "/")
	if focus(res) != "filter" {
		t.Fatalf("/ put the focus on %q", focus(res))
	}
	// Typing is the input's: n, s, and space do not reach the keymap.
	res = play(t, a, "text:ona bus")
	if len(res.Events) != 0 {
		t.Errorf("typing fired %v", actions(res))
	}
	if get(t, a, "filter") != "ona bus" {
		t.Errorf("filter = %v", get(t, a, "filter"))
	}
	rows := get(t, a, "lib_rows").([]any)
	if len(rows) != 5 {
		t.Errorf("filter 'ona bus' keeps %d rows, want the 5 of Night Bus Hymns", len(rows))
	}
	if !strings.Contains(grid(res), "library 5") {
		t.Errorf("the tab label does not count the filtered rows:\n%s", grid(res))
	}
	res = play(t, a, "enter")
	if focus(res) != "lib" {
		t.Errorf("enter in the filter left the focus on %q, want lib", focus(res))
	}
	// The filter hid the row the cursor rested on; the table highlights
	// the row at the same index, and enter plays that row.
	if got := selectedKey(play(t, a)); got != "t09" {
		t.Fatalf("the table highlights %q, want t09", got)
	}
	play(t, a, "enter")
	if current(t, a) != "t09" {
		t.Errorf("enter played %s, want t09, the row the table highlights", current(t, a))
	}
	// esc clears the filter and gives the table the focus.
	res = play(t, a, "/", "esc")
	if get(t, a, "filter") != "" || len(get(t, a, "lib_rows").([]any)) != 25 || focus(res) != "lib" {
		t.Errorf("esc: filter=%q focus=%q", get(t, a, "filter"), focus(res))
	}
	// No match: the table says so.
	res = play(t, a, "/", "text:zzz")
	if !strings.Contains(grid(res), "no tracks match") {
		t.Errorf("no placeholder:\n%s", grid(res))
	}
}

// selectedKey is the key of the highlighted table row. Only the active
// tab is laid out, so there is one.
func selectedKey(res *tuimark.PlayResult) string {
	for _, n := range res.Dump.Nodes {
		if n.Tag == "item" && n.Selected {
			return n.Key
		}
	}
	return ""
}

// What a row action acts on is the row the table highlights, which the
// event carries, not the key last written to cur.*: a filter that hides
// the remembered row, or a removal at the end of the queue, leaves the
// bound key naming a row that is gone.
func TestRowActionsActOnTheHighlightedRow(t *testing.T) {
	t.Run("queue remove at the end", func(t *testing.T) {
		a := newApp(t)
		play(t, a, "1", "a") // t04 is queued as q3
		play(t, a, "2", "down", "down")
		if got := selectedKey(play(t, a)); got != "q3" {
			t.Fatalf("highlighted %q, want q3", got)
		}
		play(t, a, "x") // removes q3; the cursor stays on the last row, q2
		if got := selectedKey(play(t, a)); got != "q2" {
			t.Fatalf("after x the table highlights %q, want q2", got)
		}
		res := play(t, a, "x")
		wantActions(t, res, "q_remove")
		var ids []string
		for _, q := range a.p.queue {
			ids = append(ids, q.QID)
		}
		if strings.Join(ids, ",") != "q1" {
			t.Errorf("queue = %v, want q1 left: x removes the highlighted q2", ids)
		}
	})
	t.Run("queue play at the end", func(t *testing.T) {
		a := newApp(t)
		play(t, a, "2", "down", "enter") // q2 is t14
		if current(t, a) != "t14" {
			t.Errorf("playing %s, want t14", current(t, a))
		}
	})
	t.Run("library add under a filter", func(t *testing.T) {
		a := newApp(t)
		play(t, a, "/", "text:ona bus", "enter")
		play(t, a, "a") // the highlighted row is t09
		last := a.p.queue[len(a.p.queue)-1]
		if last.ID != "t09" {
			t.Errorf("queued %s, want t09", last.ID)
		}
	})
	t.Run("albums", func(t *testing.T) {
		a := newApp(t)
		name := selectedKey(play(t, a, "3", "down"))
		if name == "" {
			t.Fatal("no album is highlighted")
		}
		play(t, a, "a")
		al, ok := albumNamed(name)
		if !ok {
			t.Fatalf("no album %q", name)
		}
		if n := len(a.p.queue); n != 2+len(al.IDs) {
			t.Errorf("queue has %d entries after adding %s, want %d", n, name, 2+len(al.IDs))
		}
		if last := a.p.queue[len(a.p.queue)-1]; catalog[trackIndex(last.ID)].Album != name {
			t.Errorf("queued %s, want a track of %s", last.ID, name)
		}
	})
	t.Run("empty table does nothing", func(t *testing.T) {
		a := newApp(t)
		play(t, a, "2", "c") // clear the queue
		before := current(t, a)
		play(t, a, "enter", "x")
		if current(t, a) != before || len(a.p.queue) != 0 {
			t.Errorf("an action on an empty queue changed the player: current=%s queue=%v", current(t, a), a.p.queue)
		}
		play(t, a, "1", "/", "text:zzz", "enter", "enter", "a")
		if current(t, a) != before || len(a.p.queue) != 0 {
			t.Errorf("an action on an empty library changed the player: current=%s queue=%v", current(t, a), a.p.queue)
		}
	})
}

func TestTabKeys(t *testing.T) {
	a := newApp(t)
	for _, c := range []struct{ key, focus, tab string }{
		{"2", "q", "t-queue"}, {"3", "albums", "t-albums"}, {"1", "lib", "t-library"},
	} {
		res := play(t, a, c.key)
		if focus(res) != c.focus {
			t.Errorf("%s: focus = %q, want %q", c.key, focus(res), c.focus)
		}
		if get(t, a, "tab") != c.tab {
			t.Errorf("%s: tab = %v, want %s", c.key, get(t, a, "tab"), c.tab)
		}
	}
}

func TestQueueTab(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "2")
	for _, want := range []string{"Window Seat Aurora", "Soft Alloy", "queue 2"} {
		if !strings.Contains(grid(res), want) {
			t.Errorf("queue tab has no %q:\n%s", want, grid(res))
		}
	}
	res = play(t, a, "x")
	wantActions(t, res, "q_remove")
	if q := get(t, a, "queue").([]any); len(q) != 1 || q[0].(map[string]any)["id"] != "t14" {
		t.Errorf("queue after x = %v, want only t14", q)
	}
	res = play(t, a, "enter")
	wantActions(t, res, "q_play")
	if current(t, a) != "t14" || len(get(t, a, "queue").([]any)) != 0 {
		t.Errorf("current=%s queue=%v, want t14 playing and the queue empty", current(t, a), get(t, a, "queue"))
	}
	if !strings.Contains(grid(res), "the queue is empty") {
		t.Errorf("no empty-queue message:\n%s", grid(res))
	}
	play(t, a, "1", "a", "a", "2", "c")
	if n := len(get(t, a, "queue").([]any)); n != 0 {
		t.Errorf("c left %d queued", n)
	}
}

func TestAlbumsTab(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "3")
	if !strings.Contains(grid(res), "Night Bus Hymns") || !strings.Contains(grid(res), "TRACKS") {
		t.Errorf("albums tab:\n%s", grid(res))
	}
	res = play(t, a, "enter")
	wantActions(t, res, "album_play")
	// The cursor rests on Night Bus Hymns: its first track plays, the rest queue.
	if current(t, a) != "t06" {
		t.Errorf("current = %s, want t06", current(t, a))
	}
	q := get(t, a, "queue").([]any)
	if len(q) != 4 || q[0].(map[string]any)["id"] != "t07" || q[3].(map[string]any)["id"] != "t10" {
		t.Errorf("queue = %v, want t07..t10", q)
	}
	play(t, a, "down", "a")
	if n := len(get(t, a, "queue").([]any)); n != 4+4 {
		t.Errorf("a on an album queued %d, want 8 in all", n)
	}
}

func TestHelpModal(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "?")
	wantActions(t, res, "help_open")
	g := grid(res)
	for _, want := range []string{"keys", "play / pause", "shuffle", "seek 5 s", "volume"} {
		if !strings.Contains(g, want) {
			t.Errorf("help has no %q:\n%s", want, g)
		}
	}
	// The player keys are quiet while the help is open.
	res = play(t, a, "n", "s", "p")
	if len(res.Events) != 0 {
		t.Errorf("keys fired %v under the help", actions(res))
	}
	res = play(t, a, "esc")
	wantActions(t, res, "help_close")
	if get(t, a, "help_open") != false {
		t.Error("esc did not close the help")
	}
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
	// q in the filter box is a letter.
	a := newApp(t)
	res := play(t, a, "/", "text:quiet")
	if res.Quit || get(t, a, "filter") != "quiet" {
		t.Errorf("typing q in the filter: quit=%v filter=%v", res.Quit, get(t, a, "filter"))
	}
}

// A tick moves the position, the bar, and the clock label; the next track
// starts when this one runs out.
func TestTickAdvancesAndEndsTracks(t *testing.T) {
	a := newApp(t)
	t0 := epoch.Add((sampleTicks + 1) * tickStep)
	tickAt(t, a, t0)
	tickAt(t, a, t0.Add(time.Second))
	tickAt(t, a, t0.Add(2*time.Second))
	if got := get(t, a, "now.time"); got != "1:14 / 4:12" {
		t.Errorf("now.time = %v after 2.1 s, want 1:14 / 4:12", got)
	}
	if pct := get(t, a, "now.pct").(float64); pct < 29 || pct > 30 {
		t.Errorf("now.pct = %v", pct)
	}
	a.p.posMs = a.p.durMs() - 500
	tickAt(t, a, t0.Add(3*time.Second))
	if current(t, a) != "t09" {
		t.Errorf("current = %s after the track ended, want the queued t09", current(t, a))
	}
	if n := len(get(t, a, "queue").([]any)); n != 1 {
		t.Errorf("the queue has %d entries, want 1", n)
	}
	// The spectrum is in the store and is what the sparkline shows.
	if n := len(get(t, a, "spectrum").([]any)); n != bandCount {
		t.Errorf("spectrum has %d bands, want %d", n, bandCount)
	}
}

// The last track stops the player when repeat is off, and the panel says so.
func TestPlayerStopsAtTheEnd(t *testing.T) {
	a := newApp(t)
	a.p.queue = nil
	a.p.playID("t25")
	a.p.posMs = a.p.durMs() - 100
	a.p.speed = 1
	t0 := epoch.Add((sampleTicks + 1) * tickStep)
	tickAt(t, a, t0)
	tickAt(t, a, t0.Add(500*time.Millisecond))
	if get(t, a, "now.state") != "stopped" || get(t, a, "player.playing") != false {
		t.Errorf("state = %v, want stopped", get(t, a, "now.state"))
	}
	res := play(t, a, "space")
	if get(t, a, "player.playing") != true || !strings.Contains(grid(res), "playing") {
		t.Error("space did not start the stopped player")
	}
}

// A light-theme dump is clean too, with the palette of the light theme.
func TestLightTheme(t *testing.T) {
	a := newApp(t)
	if err := a.ui.Set("@theme", "light"); err != nil {
		t.Fatal(err)
	}
	res, err := a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 24, Styles: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Dump.OK || res.Dump.Theme != "light" {
		t.Errorf("ok=%v theme=%q errors=%v", res.Dump.OK, res.Dump.Theme, res.Dump.Errors)
	}
	// The screen paints the light background everywhere.
	for _, sp := range res.Dump.Styles[23] {
		if sp.BG == "#12141c" {
			t.Errorf("the dark background shows in the light theme: %+v", sp)
		}
	}
}

func TestOnErrorShowsTheNotice(t *testing.T) {
	a := newApp(t)
	if err := a.onError(nil, errTest("boom")); err != nil {
		t.Fatal(err)
	}
	res := play(t, a)
	if !strings.Contains(grid(res), "boom") {
		t.Errorf("the notice is not shown:\n%s", grid(res))
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

// The opening state ran on the fake clock. After run resets the reference
// time, the first real tick moves nothing, whatever the date is.
func TestFirstRealTickDoesNotJump(t *testing.T) {
	a := newApp(t)
	a.p.last = time.Time{} // what run does before Every
	before := get(t, a, "player.pos")
	tickAt(t, a, time.Date(2031, 1, 1, 12, 0, 0, 0, time.UTC))
	if got := get(t, a, "player.pos"); got != before {
		t.Errorf("pos moved from %v to %v on the first tick", before, got)
	}
	tickAt(t, a, time.Date(2031, 1, 1, 12, 0, 1, 0, time.UTC))
	if got := get(t, a, "player.pos").(float64); got != before.(float64)+1 {
		t.Errorf("pos = %v one second later, want %v", got, before.(float64)+1)
	}
}

// The help modal lays out cleanly at every size, not only the ones the
// closed document is validated at.
func TestHelpModalIsCleanAtEverySize(t *testing.T) {
	a := newApp(t)
	if err := a.ui.Set("help_open", true); err != nil {
		t.Fatal(err)
	}
	for _, sz := range [][2]int{{40, 14}, {60, 20}, {80, 24}, {120, 30}} {
		d, err := a.ui.Dump(sz[0], sz[1])
		if err != nil {
			t.Fatal(err)
		}
		if !d.OK || len(d.Errors) != 0 {
			t.Errorf("%dx%d: %v", sz[0], sz[1], d.Errors)
		}
	}
}

// With repeat one a track end plays the track again, and the panel says so.
func TestUpNextHonorsRepeatOne(t *testing.T) {
	a := newApp(t)
	if got := get(t, a, "now.next"); got != "Window Seat Aurora" {
		t.Fatalf("up next = %v, want the first queued track", got)
	}
	play(t, a, "r", "r")
	if get(t, a, "player.repeat") != repeatOne {
		t.Fatalf("repeat = %v, want one", get(t, a, "player.repeat"))
	}
	if got := get(t, a, "now.next"); got != "Tessellate" {
		t.Errorf("up next = %v with repeat one, want Tessellate", got)
	}
	// Past one: back to the queue.
	play(t, a, "r")
	if got := get(t, a, "now.next"); got != "Window Seat Aurora" {
		t.Errorf("up next = %v with repeat off, want Window Seat Aurora", got)
	}
}

// A failure's notice goes away with the next successful action, or on
// its own after noticeTicks ticks, and "up next" comes back.
func TestNoticeClears(t *testing.T) {
	a := newApp(t)
	_ = a.onError(nil, errTest("boom"))
	if !strings.Contains(grid(play(t, a)), "boom") {
		t.Fatal("the notice is not shown")
	}
	res := play(t, a, "n")
	if get(t, a, "notice") != "" || strings.Contains(grid(res), "boom") || !strings.Contains(grid(res), "up next") {
		t.Errorf("notice = %q after a successful action:\n%s", get(t, a, "notice"), grid(res))
	}
	// A keypress that fires no handler does not clear it...
	_ = a.onError(nil, errTest("again"))
	play(t, a, "up")
	if get(t, a, "notice") != "again" {
		t.Errorf("notice = %q after a key that fires nothing, want again", get(t, a, "notice"))
	}
	// ...and the ticks do, after noticeTicks of them.
	t0 := epoch.Add((sampleTicks + 1) * tickStep)
	for i := 0; i < noticeTicks-1; i++ {
		tickAt(t, a, t0.Add(time.Duration(i)*tickStep))
	}
	if get(t, a, "notice") != "again" {
		t.Fatalf("notice = %q before %d ticks, want it kept", get(t, a, "notice"), noticeTicks)
	}
	tickAt(t, a, t0.Add(noticeTicks*tickStep))
	if get(t, a, "notice") != "" {
		t.Errorf("notice = %q after %d ticks, want it cleared", get(t, a, "notice"), noticeTicks)
	}
}

// The hints drop from the end, so the keys that matter on every screen
// (help, quit) come right after the first player keys.
func TestHintsKeepHelpAndQuit(t *testing.T) {
	a := newApp(t)
	for _, cols := range []int{60, 80, 100, 120} {
		d, err := a.ui.Dump(cols, 24)
		if err != nil {
			t.Fatal(err)
		}
		g := d.Grid[len(d.Grid)-1]
		for _, want := range []string{"? help", "q quit"} {
			if !strings.Contains(g, want) {
				t.Errorf("%d columns: the hint bar has no %q: %q", cols, want, g)
			}
		}
	}
}

// The help modal keeps its close button at every height and closes with
// esc, ?, enter, q, or the button.
func TestHelpModalStaysClosable(t *testing.T) {
	for _, sz := range [][2]int{{30, 10}, {40, 10}, {40, 11}, {40, 12}, {50, 13}, {60, 20}} {
		a := newApp(t)
		res, err := a.ui.Play(tuimark.PlayOptions{Cols: sz[0], Rows: sz[1]}, "?")
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Dump.Errors) != 0 {
			t.Errorf("%dx%d: %v", sz[0], sz[1], res.Dump.Errors)
		}
		if !strings.Contains(grid(res), "close") {
			t.Errorf("%dx%d: no close button:\n%s", sz[0], sz[1], grid(res))
		}
	}
	for _, key := range []string{"esc", "?", "enter", "q", "space"} {
		a := newApp(t)
		play(t, a, "?")
		play(t, a, key)
		if get(t, a, "help_open") != false {
			t.Errorf("%s did not close the help", key)
		}
	}
}
